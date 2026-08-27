package wt

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
)

// Path prints the path to the trunk, a task worktree, or the notes directory.
func (a *Manager) Path(task string, notes bool) error {
	if notes && task != "" {
		return fmt.Errorf("--notes cannot be combined with a task")
	}

	p, err := a.resolveProject()
	if err != nil {
		return err
	}
	path := p.trunk
	if notes {
		path = p.notes
	} else if task != "" {
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

// List prints the worktrees registered with the current managed project.
func (a *Manager) List(porcelain bool) error {
	p, err := a.resolveProject()
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

func (a *Manager) requireRegisteredWorktree(p project, path string) error {
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
