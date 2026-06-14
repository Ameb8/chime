package cli

import (
	"errors"

	"github.com/Ameb8/chime/internal/client"
	appconfig "github.com/Ameb8/chime/internal/config"
	"github.com/Ameb8/chime/internal/exitcode"
	"github.com/Ameb8/chime/internal/notify"
	"github.com/spf13/cobra"
)

type notifyOptions struct {
	event   string
	agent   string
	message string
	server  string
	key     string
}

func newNotifyCmd(cfg **appconfig.Config) *cobra.Command {
	opts := &notifyOptions{}

	cmd := &cobra.Command{
		Use:   "notify --event <event>",
		Short: "Send a notification event to the chime server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runNotify(cmd, cfg, opts)
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

func runNotify(cmd *cobra.Command, cfgPtr **appconfig.Config, opts *notifyOptions) error {
	cfg, err := requireConfig(cfgPtr)
	if err != nil {
		return err
	}

	event, err := parseNotifyEvent(opts.event)
	if err != nil {
		return err
	}

	serverURL := appconfig.Resolve(opts.server, "CHIME_SERVER", cfg.Client.Server)
	apiKey := appconfig.Resolve(opts.key, "CHIME_KEY", cfg.Auth.Key)

	c, err := client.New(client.Options{
		BaseURL: serverURL,
		APIKey:  apiKey,
	})
	if err != nil {
		return classifyNotifyError(err)
	}

	err = c.Notify(cmd.Context(), client.Notification{
		Event:   string(event),
		Agent:   opts.agent,
		Message: opts.message,
	})
	return classifyNotifyError(err)
}

func parseNotifyEvent(raw string) (notify.Event, error) {
	switch notify.Event(raw) {
	case notify.EventComplete:
		return notify.EventComplete, nil
	case notify.EventWaiting:
		return notify.EventWaiting, nil
	default:
		if raw == "" {
			return "", errors.New("missing event")
		}
		return "", errors.New("invalid event: must be complete or waiting")
	}
}

func classifyNotifyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, client.ErrAuth) {
		return &exitcode.Error{Code: exitcode.AuthFailure, Message: err.Error()}
	}
	if errors.Is(err, client.ErrUnavailable) {
		return &exitcode.Error{Code: exitcode.ServerUnreachable, Message: err.Error()}
	}
	return err
}
