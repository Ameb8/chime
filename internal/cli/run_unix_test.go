//go:build unix

package cli

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestCommandExitStatusSignals(t *testing.T) {
	tests := []struct {
		name       string
		signal     syscall.Signal
		wantCode   int
		wantSignal string
	}{
		{
			name:       "sigint",
			signal:     syscall.SIGINT,
			wantCode:   130,
			wantSignal: "SIGINT",
		},
		{
			name:       "sigterm",
			signal:     syscall.SIGTERM,
			wantCode:   143,
			wantSignal: "SIGTERM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			child := exec.Command("sleep", "10")
			if err := child.Start(); err != nil {
				t.Fatalf("start child: %v", err)
			}
			if err := child.Process.Signal(tt.signal); err != nil {
				t.Fatalf("signal child: %v", err)
			}
			err := child.Wait()
			code, signalName, ok := commandExitStatus(err)
			if !ok {
				t.Fatalf("commandExitStatus ok = false for error %v", err)
			}
			if code != tt.wantCode {
				t.Fatalf("code = %d, want %d", code, tt.wantCode)
			}
			if signalName != tt.wantSignal {
				t.Fatalf("signal = %q, want %q", signalName, tt.wantSignal)
			}
		})
	}
}
