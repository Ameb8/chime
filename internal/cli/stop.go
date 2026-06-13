package cli

import "github.com/spf13/cobra"

func newStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the running background server",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return notImplemented("stop")
		},
	}
	configureVersion(cmd)

	return cmd
}
