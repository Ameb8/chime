package cli

import (
	"fmt"

	appconfig "github.com/Ameb8/chime/internal/config"
	"github.com/spf13/cobra"
)

var validAgents = []string{"claude-code", "codex", "aider"}

type installOptions struct {
	server string
	key    string
}

func newInstallCmd(_ **appconfig.Config) *cobra.Command {
	opts := &installOptions{}

	cmd := &cobra.Command{
		Use:       "install <agent>",
		Short:     "Print hook configuration snippets for an agent tool",
		Args:      validateAgentArg,
		ValidArgs: validAgents,
		RunE: func(_ *cobra.Command, _ []string) error {
			return notImplemented("install")
		},
	}
	configureVersion(cmd)

	cmd.Flags().StringVar(&opts.server, "server", opts.server, "Server URL to embed in snippets")
	cmd.Flags().StringVar(&opts.key, "key", opts.key, "API key to embed in snippets")

	return cmd
}

func validateAgentArg(_ *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("expected one agent argument; valid agents: %s", validAgentList())
	}

	for _, agent := range validAgents {
		if args[0] == agent {
			return nil
		}
	}

	return fmt.Errorf("unknown agent %q; valid agents: %s", args[0], validAgentList())
}

func validAgentList() string {
	return "claude-code, codex, aider"
}
