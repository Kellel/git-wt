package wt

import (
	_ "embed"
	"fmt"
)

//go:embed shell_init.bash
var bashShellInit string

func (a *App) runShellInit(args []string) error {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		_, err := fmt.Fprintln(a.out, "usage: git wt shell-init [bash]")
		return err
	}
	if len(args) > 1 || (len(args) == 1 && args[0] != "bash") {
		return errorsForUsage("shell-init supports only bash")
	}
	_, err := fmt.Fprint(a.out, bashShellInit)
	return err
}
