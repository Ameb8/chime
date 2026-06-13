package cli

import "github.com/spf13/cobra"

type notifyOptions struct {
	event   string
	agent   string
	message string
	server  string
	key     string
}

func newNotifyCmd() *cobra.Command {
	opts := &notifyOptions{}

	cmd := &cobra.Command{
		Use:   "notify --event <event>",
		Short: "Send a notification event to the chime server",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return notImplemented("notify")
		},
	}
	configureVersion(cmd)

	cmd.Flags().StringVar(&opts.event, "event", opts.event, "Event type: complete or waiting")
	cmd.Flags().StringVar(&opts.agent, "agent", opts.agent, "Agent name, e.g. claude-code, codex, aider")
	cmd.Flags().StringVar(&opts.message, "message", opts.message, "Optional human-readable detail from the agent")
	cmd.Flags().StringVar(&opts.server, "server", opts.server, "Server base URL")
	cmd.Flags().StringVar(&opts.key, "key", opts.key, "API key")
	_ = cmd.MarkFlagRequired("event")

	return cmd
}
