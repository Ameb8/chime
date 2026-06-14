package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	appconfig "github.com/Ameb8/chime/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newConfigCmd(cfg **appconfig.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config <subcommand>",
		Short: "View and manage configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	configureVersion(cmd)

	cmd.AddCommand(
		newConfigShowCmd(cfg),
		newConfigSetCmd(cfg),
		newConfigKeyCmd(cfg),
	)

	return cmd
}

func newConfigShowCmd(cfg **appconfig.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print the current resolved config as YAML",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			current, err := requireConfig(cfg)
			if err != nil {
				return err
			}

			masked := *current
			masked.Auth.Key = maskKey(masked.Auth.Key)

			b, err := yaml.Marshal(masked)
			if err != nil {
				return fmt.Errorf("marshal config: %w", err)
			}
			_, err = fmt.Fprint(os.Stdout, string(b))
			return err
		},
	}
	configureVersion(cmd)

	return cmd
}

func newConfigSetCmd(cfg **appconfig.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			current, err := requireConfig(cfg)
			if err != nil {
				return err
			}
			if err := setConfigValue(current, args[0], args[1]); err != nil {
				return err
			}
			return appconfig.Save(current)
		},
	}
	configureVersion(cmd)

	return cmd
}

func newConfigKeyCmd(cfg **appconfig.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key",
		Short: "Print the current API key",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			current, err := requireConfig(cfg)
			if err != nil {
				return err
			}
			if current.Auth.Key == "" {
				return fmt.Errorf("no API key configured; run `chime start` to generate one")
			}
			_, err = fmt.Fprintln(os.Stdout, current.Auth.Key)
			return err
		},
	}
	configureVersion(cmd)

	cmd.AddCommand(newConfigKeyRotateCmd(cfg))

	return cmd
}

func newConfigKeyRotateCmd(cfg **appconfig.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rotate",
		Short: "Generate and save a new API key",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			current, err := requireConfig(cfg)
			if err != nil {
				return err
			}
			key, err := appconfig.GenerateKey()
			if err != nil {
				return err
			}
			current.Auth.Key = key
			if err := appconfig.Save(current); err != nil {
				return err
			}
			_, err = fmt.Fprintln(os.Stdout, key)
			return err
		},
	}
	configureVersion(cmd)

	return cmd
}

func requireConfig(cfg **appconfig.Config) (*appconfig.Config, error) {
	if cfg == nil || *cfg == nil {
		return nil, fmt.Errorf("config is not loaded")
	}
	return *cfg, nil
}

func setConfigValue(cfg *appconfig.Config, key, value string) error {
	switch key {
	case "server.bind":
		cfg.Server.Bind = value
	case "auth.key":
		return fmt.Errorf("auth.key cannot be set directly; use `chime config key rotate`")
	case "client.server":
		cfg.Client.Server = value
	case "notifications.toast.enabled":
		v, err := parseBool(key, value)
		if err != nil {
			return err
		}
		cfg.Notifications.Toast.Enabled = v
	case "notifications.toast.events":
		cfg.Notifications.Toast.Events = parseList(value)
	case "notifications.sound.enabled":
		v, err := parseBool(key, value)
		if err != nil {
			return err
		}
		cfg.Notifications.Sound.Enabled = v
	case "notifications.sound.events":
		cfg.Notifications.Sound.Events = parseList(value)
	case "notifications.sound.complete_sound":
		cfg.Notifications.Sound.CompleteSound = value
	case "notifications.sound.waiting_sound":
		cfg.Notifications.Sound.WaitingSound = value
	case "log.level":
		cfg.Log.Level = value
	case "log.file":
		cfg.Log.File = value
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

func parseBool(key, value string) (bool, error) {
	v, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return v, nil
}

func parseList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return parts
}

func maskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 13 {
		return "..."
	}
	return key[:10] + "..." + key[len(key)-4:]
}
