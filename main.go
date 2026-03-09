package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/alphaleonis/cctote/cmd"
	"github.com/alphaleonis/cctote/internal/buildinfo"
)

func main() {
	app := cmd.NewApp(buildinfo.Version())
	if err := app.Execute(); err != nil {
		var exitErr *cmd.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}
