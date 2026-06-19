package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Ameb8/chime/internal/cli"
	"github.com/Ameb8/chime/internal/exitcode"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		var silentErr *exitcode.SilentError
		if errors.As(err, &silentErr) {
			os.Exit(silentErr.Code)
		}

		var codeErr *exitcode.Error
		if errors.As(err, &codeErr) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(codeErr.Code)
		}

		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitcode.General)
	}
}
