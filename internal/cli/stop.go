package cli

import (
	appconfig "github.com/Ameb8/chime/internal/config"
	"github.com/spf13/cobra"
)

func newStopCmd(_ **appconfig.Config) *cobra.Command {
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
