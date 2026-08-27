package main

import (
	"fmt"
	"os"

	"github.com/Kellel/git-wt/wt"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "git wt: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("determine working directory: %w", err)
	}

	manager := wt.NewManager(cwd, os.Environ(), os.Stdout, os.Stderr)
	command := newRootCommand(manager, os.Stdout, os.Stderr)
	command.SetArgs(os.Args[1:])
	return command.Execute()
}
