package cli

import "github.com/spf13/cobra"

func newConfigCmd() *cobra.Command {
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
		newConfigShowCmd(),
		newConfigSetCmd(),
		newConfigKeyCmd(),
	)

	return cmd
}

func newConfigShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print the current resolved config as YAML",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return notImplemented("config show")
		},
	}
	configureVersion(cmd)

	return cmd
}

func newConfigSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, _ []string) error {
			return notImplemented("config set")
		},
	}
	configureVersion(cmd)

	return cmd
}

func newConfigKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key",
		Short: "Print the current API key",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return notImplemented("config key")
		},
	}
	configureVersion(cmd)

	cmd.AddCommand(newConfigKeyRotateCmd())

	return cmd
}

func newConfigKeyRotateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rotate",
		Short: "Generate and save a new API key",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return notImplemented("config key rotate")
		},
	}
	configureVersion(cmd)

	return cmd
}
