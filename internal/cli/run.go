package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Ameb8/chime/internal/client"
	appconfig "github.com/Ameb8/chime/internal/config"
	"github.com/Ameb8/chime/internal/exitcode"
	"github.com/Ameb8/chime/internal/notify"
	"github.com/spf13/cobra"
)

const runUsage = "usage: chime run [flags] -- <command> [args...]"

type runOptions struct {
	agent   string
	message string
	server  string
	key     string
}

type commandResult struct {
	DisplayCommand string
	Executable     string
	ExitCode       int
	Started        bool
	Duration       time.Duration
	SignalName     string
}

type runDeps struct {
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	execute func(context.Context, []string, io.Reader, io.Writer, io.Writer) (commandResult, error)
	notify  func(context.Context, *appconfig.Config, *runOptions, commandResult) error
}

func newRunCmd(cfg **appconfig.Config) *cobra.Command {
	return newRunCmdWithDeps(cfg, defaultRunDeps())
}

func newRunCmdWithDeps(cfg **appconfig.Config, deps runDeps) *cobra.Command {
	opts := &runOptions{}
	deps = normalizeRunDeps(deps)

	cmd := &cobra.Command{
		Use:   "run [flags] -- <command> [args...]",
		Short: "Run a command and notify when it completes",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, cfg, opts, args, deps)
		},
	}
	configureVersion(cmd)

	cmd.Flags().StringVar(&opts.agent, "agent", opts.agent, "Agent/source name sent with the notification")
	cmd.Flags().StringVar(&opts.message, "message", opts.message, "Optional label included in the notification message")
	cmd.Flags().StringVar(&opts.server, "server", opts.server, "Server base URL")
	cmd.Flags().StringVar(&opts.key, "key", opts.key, "API key")
	cmd.Flags().SetInterspersed(false)

	return cmd
}

func defaultRunDeps() runDeps {
	return runDeps{
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
		execute: executeWrappedCommand,
		notify:  notifyCommandComplete,
	}
}

func normalizeRunDeps(deps runDeps) runDeps {
	defaults := defaultRunDeps()
	if deps.stdin == nil {
		deps.stdin = defaults.stdin
	}
	if deps.stdout == nil {
		deps.stdout = defaults.stdout
	}
	if deps.stderr == nil {
		deps.stderr = defaults.stderr
	}
	if deps.execute == nil {
		deps.execute = defaults.execute
	}
	if deps.notify == nil {
		deps.notify = defaults.notify
	}
	return deps
}

func runCommand(cmd *cobra.Command, cfgPtr **appconfig.Config, opts *runOptions, args []string, deps runDeps) error {
	cfg, err := requireConfig(cfgPtr)
	if err != nil {
		return err
	}

	argv, err := parseRunArgs(cmd, args)
	if err != nil {
		return err
	}

	result, err := deps.execute(cmd.Context(), argv, deps.stdin, deps.stdout, deps.stderr)
	if err != nil {
		return err
	}
	if !result.Started {
		return fmt.Errorf("command did not start")
	}
	if result.DisplayCommand == "" {
		result.DisplayCommand = displayCommand(argv)
	}
	if result.Executable == "" {
		result.Executable = argv[0]
	}

	if err := deps.notify(cmd.Context(), cfg, opts, result); err != nil {
		if _, writeErr := fmt.Fprintf(deps.stderr, "chime: notification failed: %v\n", err); writeErr != nil {
			return writeErr
		}
	}

	if result.ExitCode != exitcode.Success {
		return &exitcode.SilentError{Code: result.ExitCode}
	}
	return nil
}

func parseRunArgs(cmd *cobra.Command, args []string) ([]string, error) {
	dash := cmd.Flags().ArgsLenAtDash()
	switch {
	case dash < 0:
		if len(args) == 0 {
			return nil, fmt.Errorf("missing command; %s", runUsage)
		}
		return nil, fmt.Errorf("missing -- delimiter; use %s", runUsage)
	case dash > 0:
		return nil, fmt.Errorf("arguments before -- are not allowed; use %s", runUsage)
	case len(args) == 0:
		return nil, fmt.Errorf("missing command after --; %s", runUsage)
	default:
		return args, nil
	}
}

func executeWrappedCommand(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer) (commandResult, error) {
	result := commandResult{
		DisplayCommand: displayCommand(argv),
	}
	if len(argv) == 0 {
		return result, fmt.Errorf("missing command; %s", runUsage)
	}
	result.Executable = argv[0]

	child := exec.CommandContext(ctx, argv[0], argv[1:]...)
	child.Stdin = stdin
	child.Stdout = stdout
	child.Stderr = stderr

	start := time.Now()
	if err := child.Start(); err != nil {
		result.Duration = time.Since(start)
		return result, fmt.Errorf("start command %q: %w", argv[0], err)
	}
	result.Started = true

	err := waitForWrappedCommand(child)
	result.Duration = time.Since(start)
	if err != nil {
		exitCode, signalName, ok := commandExitStatus(err)
		if ok {
			result.ExitCode = exitCode
			result.SignalName = signalName
			return result, nil
		}
		return result, fmt.Errorf("wait for command %q: %w", argv[0], err)
	}

	result.ExitCode = exitcode.Success
	return result, nil
}

func notifyCommandComplete(ctx context.Context, cfg *appconfig.Config, opts *runOptions, result commandResult) error {
	serverURL := appconfig.Resolve(opts.server, "CHIME_SERVER", cfg.Client.Server)
	apiKey := appconfig.Resolve(opts.key, "CHIME_KEY", cfg.Auth.Key)

	c, err := client.New(client.Options{
		BaseURL: serverURL,
		APIKey:  apiKey,
	})
	if err != nil {
		return err
	}

	agent := opts.agent
	if agent == "" {
		agent = defaultRunAgent(result.Executable)
	}

	return c.Notify(ctx, client.Notification{
		Event:   string(notify.EventComplete),
		Agent:   agent,
		Message: formatRunMessage(opts.message, result),
	})
}

func defaultRunAgent(executable string) string {
	base := filepath.Base(executable)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "command"
	}
	return base
}

func formatRunMessage(label string, result commandResult) string {
	command := result.DisplayCommand
	if command == "" {
		command = "command"
	}

	var message string
	duration := formatRunDuration(result.Duration)
	switch {
	case result.SignalName != "":
		message = fmt.Sprintf("%s interrupted by %s with exit code %d after %s", command, result.SignalName, result.ExitCode, duration)
	case result.ExitCode == exitcode.Success:
		message = fmt.Sprintf("%s completed successfully in %s", command, duration)
	default:
		message = fmt.Sprintf("%s failed with exit code %d after %s", command, result.ExitCode, duration)
	}

	if label == "" {
		return message
	}
	return label + ": " + message
}

func formatRunDuration(d time.Duration) string {
	if d < time.Second {
		return "<1s"
	}

	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	if d < time.Hour {
		minutes := int(d / time.Minute)
		seconds := int((d % time.Minute) / time.Second)
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}

	hours := int(d / time.Hour)
	minutes := int((d % time.Hour) / time.Minute)
	return fmt.Sprintf("%dh%dm", hours, minutes)
}

func displayCommand(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, arg := range argv {
		parts = append(parts, displayArg(arg))
	}
	return strings.Join(parts, " ")
}

func displayArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if strings.ContainsAny(arg, " \t\r\n\"'\\") {
		return strconv.Quote(arg)
	}
	return arg
}
