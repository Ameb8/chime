//go:build darwin

package notify

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCommandSoundPlayerRunsAfplayWithPath(t *testing.T) {
	var gotName string
	var gotArgs []string
	player := commandSoundPlayer{
		timeout: time.Second,
		run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			gotName = name
			gotArgs = append([]string(nil), args...)
			return nil, nil
		},
	}

	if err := player.Play("/tmp/chime.aiff"); err != nil {
		t.Fatalf("Play returned error: %v", err)
	}
	if gotName != "afplay" {
		t.Fatalf("command name = %q, want afplay", gotName)
	}
	if len(gotArgs) != 1 || gotArgs[0] != "/tmp/chime.aiff" {
		t.Fatalf("command args = %v, want [/tmp/chime.aiff]", gotArgs)
	}
}

func TestCommandSoundPlayerReturnsCommandError(t *testing.T) {
	player := commandSoundPlayer{
		timeout: time.Second,
		run: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("bad file"), errors.New("exit status 1")
		},
	}

	err := player.Play("/tmp/chime.aiff")
	if err == nil {
		t.Fatal("Play returned nil, want error")
	}
	if !strings.Contains(err.Error(), "bad file") {
		t.Fatalf("Play error = %q, want command output", err)
	}
}
