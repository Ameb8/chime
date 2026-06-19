//go:build unix

package cli

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func waitForWrappedCommand(child *exec.Cmd) error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	waited := make(chan error, 1)
	go func() {
		waited <- child.Wait()
	}()

	for {
		select {
		case sig := <-signals:
			if child.Process != nil {
				_ = child.Process.Signal(sig)
			}
		case err := <-waited:
			return err
		}
	}
}

func commandExitStatus(err error) (int, string, bool) {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 0, "", false
	}

	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		code := exitErr.ExitCode()
		return code, "", code >= 0
	}
	if status.Signaled() {
		sig := status.Signal()
		return 128 + int(sig), signalName(sig), true
	}
	if status.Exited() {
		return status.ExitStatus(), "", true
	}
	return 0, "", false
}

func signalName(sig syscall.Signal) string {
	switch sig {
	case syscall.SIGINT:
		return "SIGINT"
	case syscall.SIGTERM:
		return "SIGTERM"
	case syscall.SIGKILL:
		return "SIGKILL"
	default:
		return "signal " + sig.String()
	}
}
