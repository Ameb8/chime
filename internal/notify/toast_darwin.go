//go:build darwin

package notify

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const toastDisplayTimeout = 5 * time.Second

type commandToastSender struct {
	timeout time.Duration
	run     func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func newPlatformToastSender() toastSender {
	return commandToastSender{
		timeout: toastDisplayTimeout,
		run:     runToastCommand,
	}
}

func (s commandToastSender) Show(title, body string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("toast title is empty")
	}
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("toast body is empty")
	}
	if s.timeout <= 0 {
		s.timeout = toastDisplayTimeout
	}
	if s.run == nil {
		s.run = runToastCommand
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	script := fmt.Sprintf(
		"display notification %s with title %s",
		appleScriptQuote(body),
		appleScriptQuote(title),
	)
	output, err := s.run(ctx, "osascript", "-e", script)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("osascript timed out: %w", ctx.Err())
	}
	output = bytes.TrimSpace(output)
	if len(output) == 0 {
		return fmt.Errorf("osascript: %w", err)
	}
	return fmt.Errorf("osascript: %w: %s", err, output)
}

func appleScriptQuote(s string) string {
	escaped := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\r\n", " ",
		"\n", " ",
		"\r", " ",
	).Replace(s)
	return `"` + escaped + `"`
}

func runToastCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
