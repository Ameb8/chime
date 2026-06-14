//go:build darwin

package notify

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

const soundPlaybackTimeout = 5 * time.Second

type commandSoundPlayer struct {
	timeout time.Duration
	run     func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func newPlatformSoundPlayer() soundPlayer {
	return commandSoundPlayer{
		timeout: soundPlaybackTimeout,
		run:     runSoundCommand,
	}
}

func (p commandSoundPlayer) Play(path string) error {
	if path == "" {
		return fmt.Errorf("sound path is empty")
	}
	if p.timeout <= 0 {
		p.timeout = soundPlaybackTimeout
	}
	if p.run == nil {
		p.run = runSoundCommand
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	output, err := p.run(ctx, "afplay", path)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("afplay timed out: %w", ctx.Err())
	}
	output = bytes.TrimSpace(output)
	if len(output) == 0 {
		return fmt.Errorf("afplay: %w", err)
	}
	return fmt.Errorf("afplay: %w: %s", err, output)
}

func runSoundCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
