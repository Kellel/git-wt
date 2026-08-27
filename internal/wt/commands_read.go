package wt

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
)

func (a *App) runPath(cfg config, args []string) error {
	var key string
	var notes bool
	positional, help, err := parseOptions(args,
		map[string]*string{"--project": &key},
		map[string]*bool{"--notes": &notes},
	)
	if err != nil {
		return err
	}
	if help {
		_, err := fmt.Fprintln(a.out, "usage: git wt path [<task>] [--project <namespace>/<project>] [--notes]")
		return err
	}
	if len(positional) > 1 {
		return errorsForUsage("path accepts at most one task")
	}
	if notes && len(positional) != 0 {
		return errorsForUsage("--notes cannot be combined with a task")
	}

	p, err := a.resolveProject(cfg, key)
	if err != nil {
		return err
	}
	path := p.trunk
	if notes {
		path = p.notes
	} else if len(positional) == 1 {
		task := positional[0]
		if err := validateTask(task); err != nil {
			return err
		}
		path = filepath.Join(p.worktrees, task)
		if err := a.requireRegisteredWorktree(p, path); err != nil {
			return err
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path %s is unavailable: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %s is not a directory", path)
	}
	_, err = fmt.Fprintln(a.out, path)
	return err
}

func (a *App) runList(cfg config, args []string) error {
	var porcelain bool
	positional, help, err := parseOptions(args, nil, map[string]*bool{"--porcelain": &porcelain})
	if err != nil {
		return err
	}
	if help {
		_, err := fmt.Fprintln(a.out, "usage: git wt list [<namespace>/<project>] [--porcelain]")
		return err
	}
	if len(positional) > 1 {
		return errorsForUsage("list accepts at most one project")
	}
	key := ""
	if len(positional) == 1 {
		key = positional[0]
	}
	p, err := a.resolveProject(cfg, key)
	if err != nil {
		return err
	}
	worktrees, err := a.projectWorktrees(p, true)
	if err != nil {
		return err
	}
	if porcelain {
		return writePorcelainWorktrees(a.out, p, worktrees)
	}

	w := tabwriter.NewWriter(a.out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "TASK\tBRANCH\tCOMMIT\tDIRTY\tPATH"); err != nil {
		return err
	}
	for _, item := range worktrees {
		branch := strings.TrimPrefix(item.branchRef, "refs/heads/")
		if item.detached || branch == "" {
			branch = "(detached)"
		}
		head := item.head
		if len(head) > 12 {
			head = head[:12]
		}
		dirty := "no"
		if item.dirty {
			dirty = "yes"
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", worktreeTask(p, item.path), branch, head, dirty, item.path); err != nil {
			return err
		}
	}
	return w.Flush()
}

func writePorcelainWorktrees(w io.Writer, p project, worktrees []worktree) error {
	for _, item := range worktrees {
		branch := strings.TrimPrefix(item.branchRef, "refs/heads/")
		fields := []string{
			"task " + worktreeTask(p, item.path),
			"path " + item.path,
			"head " + item.head,
			"branch " + branch,
			fmt.Sprintf("detached %t", item.detached),
			fmt.Sprintf("dirty %t", item.dirty),
		}
		for _, field := range fields {
			if _, err := fmt.Fprintf(w, "%s%c", field, byte(0)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "%c", byte(0)); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) requireRegisteredWorktree(p project, path string) error {
	worktrees, err := a.projectWorktrees(p, false)
	if err != nil {
		return err
	}
	for _, item := range worktrees {
		if sameResolvedPath(item.path, path) {
			return nil
		}
	}
	return fmt.Errorf("%s is not a registered worktree", path)
}

func errorsForUsage(message string) error {
	return fmt.Errorf("%s; run the command with --help for usage", message)
}
