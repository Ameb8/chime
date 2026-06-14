//go:build darwin

package notify

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCommandToastSenderRunsOsascriptWithNotificationScript(t *testing.T) {
	var gotName string
	var gotArgs []string
	sender := commandToastSender{
		timeout: time.Second,
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			gotName = name
			gotArgs = append([]string(nil), args...)
			return nil, nil
		},
	}

	if err := sender.Show("chime \u2014 codex", `Task "done"`); err != nil {
		t.Fatalf("Show returned error: %v", err)
	}
	if gotName != "osascript" {
		t.Fatalf("command name = %q, want osascript", gotName)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "-e" {
		t.Fatalf("command args = %v, want [-e <script>]", gotArgs)
	}

	script := gotArgs[1]
	for _, want := range []string{
		`display notification "Task \"done\""`,
		"with title \"chime \u2014 codex\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script %q does not contain %q", script, want)
		}
	}
}

func TestCommandToastSenderReturnsCommandError(t *testing.T) {
	sender := commandToastSender{
		timeout: time.Second,
		run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("bad notification"), errors.New("exit status 1")
		},
	}

	err := sender.Show("chime", "Task complete")
	if err == nil {
		t.Fatal("Show returned nil, want error")
	}
	if !strings.Contains(err.Error(), "bad notification") {
		t.Fatalf("Show error = %q, want command output", err)
	}
}
