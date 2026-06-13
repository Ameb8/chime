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
		fmt.Fprintln(os.Stderr, err)

		var codeErr *exitcode.Error
		if errors.As(err, &codeErr) {
			os.Exit(codeErr.Code)
		}

		os.Exit(exitcode.General)
	}
}
