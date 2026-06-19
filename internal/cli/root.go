package cli

import (
	"fmt"

	"github.com/Ameb8/chime/internal/config"
	"github.com/Ameb8/chime/internal/paths"
	"github.com/spf13/cobra"
)

const version = "v0.1.0"

type rootOptions struct {
	configPath string
}

func NewRootCmd() *cobra.Command {
	var cfg *config.Config
	paths.SetConfigFile("")
	opts := &rootOptions{
		configPath: paths.ConfigFile(),
	}

	rootCmd := &cobra.Command{
		Use:           "chime",
		Short:         "Cross-platform notification daemon for coding agents",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if cmd == cmd.Root() {
				return nil
			}

			paths.SetConfigFile(opts.configPath)
			loaded, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			cfg = loaded
			return nil
		},
	}

	configureVersion(rootCmd)
	rootCmd.PersistentFlags().StringVar(&opts.configPath, "config", opts.configPath, "Path to config file")
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	registerSubcommands(rootCmd, &cfg)

	return rootCmd
}

func registerSubcommands(rootCmd *cobra.Command, cfg **config.Config) {
	rootCmd.AddCommand(
		newStartCmd(cfg),
		newStopCmd(cfg),
		newStatusCmd(cfg),
		newNotifyCmd(cfg),
		newRunCmd(cfg),
		newInstallCmd(cfg),
		newConfigCmd(cfg),
	)
}

func configureVersion(cmd *cobra.Command) {
	cmd.Version = version
	cmd.SetVersionTemplate("chime {{.Version}}\n")
}

func notImplemented(command string) error {
	return fmt.Errorf("%s command is not implemented yet", command)
}
