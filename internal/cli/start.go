package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	appconfig "github.com/Ameb8/chime/internal/config"
	"github.com/Ameb8/chime/internal/notify"
	"github.com/Ameb8/chime/internal/paths"
	"github.com/Ameb8/chime/internal/server"
	"github.com/spf13/cobra"
)

type startOptions struct {
	bind       string
	foreground bool
	logPath    string
}

func newStartCmd(cfg **appconfig.Config) *cobra.Command {
	opts := &startOptions{}

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the notification server",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStart(cfg, opts)
		},
	}
	configureVersion(cmd)

	cmd.Flags().StringVar(&opts.bind, "bind", opts.bind, "Address and port to listen on")
	cmd.Flags().BoolVar(&opts.foreground, "foreground", opts.foreground, "Run in the foreground; do not daemonize")
	cmd.Flags().StringVar(&opts.logPath, "log", opts.logPath, "Log file path")

	return cmd
}

func runStart(cfgPtr **appconfig.Config, opts *startOptions) error {
	cfg, err := requireConfig(cfgPtr)
	if err != nil {
		return err
	}

	bind := appconfig.Resolve(opts.bind, "CHIME_BIND", cfg.Server.Bind)
	if bind == "" {
		bind = "0.0.0.0:7777"
	}

	logPath := resolveLogPath(opts.logPath, cfg.Log.File)

	generatedKey, err := ensureAPIKey(cfg)
	if err != nil {
		return err
	}
	if generatedKey != "" {
		if _, err := fmt.Fprintf(os.Stdout, "API key: %s\n", generatedKey); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(os.Stdout, "Add to remote machines: export CHIME_KEY=%s\n", generatedKey); err != nil {
			return err
		}
	}

	cleanup, err := configureServerLogger(cfg.Log.Level, logPath)
	if err != nil {
		return err
	}
	defer cleanup()

	backends := buildBackends(cfg)
	dispatcher := notify.NewDispatcher(backends)
	ready := make(chan struct{})
	srv := server.New(cfg, dispatcher, server.Options{
		Bind:    bind,
		LogPath: logPath,
		Version: version,
		Ready:   ready,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ready:
	}

	if _, err := fmt.Fprintf(os.Stdout, "Server listening on %s\n", bind); err != nil {
		stop()
		_ = srv.Shutdown(context.Background())
		return err
	}

	if err := <-errCh; err != nil {
		return err
	}

	_, err = fmt.Fprintln(os.Stdout, "Server stopped.")
	return err
}

func ensureAPIKey(cfg *appconfig.Config) (string, error) {
	if cfg.Auth.Key != "" {
		return "", nil
	}

	key, err := appconfig.GenerateKey()
	if err != nil {
		return "", err
	}
	cfg.Auth.Key = key
	if err := appconfig.Save(cfg); err != nil {
		return "", err
	}
	return key, nil
}

func resolveLogPath(flagPath, configPath string) string {
	switch {
	case flagPath != "":
		return expandPath(flagPath)
	case configPath != "":
		return expandPath(configPath)
	default:
		return paths.LogFile()
	}
}

func configureServerLogger(level, logPath string) (func(), error) {
	slogLevel, err := parseLogLevel(level)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("set log permissions: %w", err)
	}

	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(file, &slog.HandlerOptions{
		Level: slogLevel,
	})))

	return func() {
		slog.SetDefault(previous)
		_ = file.Close()
	}, nil
}

func parseLogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("invalid log level %q", level)
	}
}

func buildBackends(_ *appconfig.Config) []notify.Backend {
	return []notify.Backend{}
}

func expandPath(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}
