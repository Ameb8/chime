package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const version = "v0.1.0"

type rootOptions struct {
	configPath string
}

func NewRootCmd() *cobra.Command {
	opts := &rootOptions{
		configPath: defaultConfigPath(),
	}

	rootCmd := &cobra.Command{
		Use:           "chime",
		Short:         "Cross-platform notification daemon for coding agents",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	configureVersion(rootCmd)
	rootCmd.PersistentFlags().StringVar(&opts.configPath, "config", opts.configPath, "Path to config file")
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	registerSubcommands(rootCmd)

	return rootCmd
}

func registerSubcommands(rootCmd *cobra.Command) {
	rootCmd.AddCommand(
		newStartCmd(),
		newStopCmd(),
		newStatusCmd(),
		newNotifyCmd(),
		newInstallCmd(),
		newConfigCmd(),
	)
}

func configureVersion(cmd *cobra.Command) {
	cmd.Version = version
	cmd.SetVersionTemplate("chime {{.Version}}\n")
}

func notImplemented(command string) error {
	return fmt.Errorf("%s command is not implemented yet", command)
}

func defaultConfigPath() string {
	if configPath := os.Getenv("CHIME_CONFIG"); configPath != "" {
		return configPath
	}

	return "~/.config/chime/config.yaml"
}
