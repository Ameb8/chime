package cli

import (
	appconfig "github.com/Ameb8/chime/internal/config"
	"github.com/spf13/cobra"
)

func newStatusCmd(_ **appconfig.Config) *cobra.Command {
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
