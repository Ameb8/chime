package cli

import "github.com/spf13/cobra"

type startOptions struct {
	bind       string
	foreground bool
	logPath    string
}

func newStartCmd() *cobra.Command {
	opts := &startOptions{
		bind:    "0.0.0.0:7777",
		logPath: "~/.local/share/chime/chime.log",
	}

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the notification server",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return notImplemented("start")
		},
	}
	configureVersion(cmd)

	cmd.Flags().StringVar(&opts.bind, "bind", opts.bind, "Address and port to listen on")
	cmd.Flags().BoolVar(&opts.foreground, "foreground", opts.foreground, "Run in the foreground; do not daemonize")
	cmd.Flags().StringVar(&opts.logPath, "log", opts.logPath, "Log file path")

	return cmd
}
