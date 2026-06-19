//go:build !unix

package cli

import (
	"errors"
	"os/exec"
)

func waitForWrappedCommand(child *exec.Cmd) error {
	return child.Wait()
}

func commandExitStatus(err error) (int, string, bool) {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 0, "", false
	}
	code := exitErr.ExitCode()
	return code, "", code >= 0
}
