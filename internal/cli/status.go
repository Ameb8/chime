package cli

import "github.com/spf13/cobra"

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print the current server status",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return notImplemented("status")
		},
	}
	configureVersion(cmd)

	return cmd
}
