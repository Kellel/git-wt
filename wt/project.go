package wt

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var componentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type project struct {
	root      string
	trunk     string
	worktrees string
	notes     string
	agents    string
}

type worktree struct {
	path      string
	head      string
	branchRef string
	detached  bool
	locked    string
	prunable  string
	dirty     bool
}

func validateComponent(kind, value string) error {
	if !componentPattern.MatchString(value) || value == "." || value == ".." {
		return fmt.Errorf("invalid %s %q: use ASCII letters, digits, '.', '_', and '-', starting with a letter or digit", kind, value)
	}
	return nil
}

func validateTask(task string) error {
	if err := validateComponent("task", task); err != nil {
		return err
	}
	if task == "trunk" || task == "worktrees" {
		return fmt.Errorf("task name %q is reserved", task)
	}
	return nil
}

func projectAtRoot(root string) project {
	return project{
		root:      root,
		trunk:     filepath.Join(root, "trunk"),
		worktrees: filepath.Join(root, "worktrees"),
		notes:     filepath.Join(root, "notes"),
		agents:    filepath.Join(root, "AGENTS.md"),
	}
}

func (a *Manager) resolveProject() (project, error) {
	if top, err := a.git.output(a.cwd, "rev-parse", "--path-format=absolute", "--show-toplevel"); err == nil {
		p, err := a.projectFromWorktree(top)
		if err != nil {
			return project{}, err
		}
		if err := a.validateProject(p); err != nil {
			return project{}, err
		}
		return p, nil
	}

	for candidate := filepath.Clean(a.cwd); ; candidate = filepath.Dir(candidate) {
		p, err := a.projectFromRoot(candidate)
		if err == nil {
			if err := a.validateProject(p); err == nil {
				return p, nil
			}
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
	}
	return project{}, errors.New("current directory is not inside a managed project")
}

func (a *Manager) projectFromWorktree(top string) (project, error) {
	top, err := absolutePath(a.cwd, top)
	if err != nil {
		return project{}, err
	}

	var root string
	if filepath.Base(top) == "trunk" {
		root = filepath.Dir(top)
	} else if filepath.Base(filepath.Dir(top)) == "worktrees" {
		root = filepath.Dir(filepath.Dir(top))
	} else {
		return project{}, fmt.Errorf("git checkout %s does not follow the managed trunk/worktrees layout", top)
	}
	return a.projectFromRoot(root)
}

func (a *Manager) projectFromRoot(root string) (project, error) {
	root, err := absolutePath(a.cwd, root)
	if err != nil {
		return project{}, err
	}
	return projectAtRoot(root), nil
}

func (a *Manager) validateProject(p project) error {
	for name, path := range map[string]string{
		"project root": p.root,
		"trunk":        p.trunk,
		"worktrees":    p.worktrees,
		"notes":        p.notes,
	} {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("invalid managed project %s: %s is unavailable: %w", p.root, name, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("invalid managed project %s: %s is not a directory", p.root, path)
		}
	}

	gitEntry, err := os.Lstat(filepath.Join(p.trunk, ".git"))
	if err != nil {
		return fmt.Errorf("invalid managed project %s: trunk has no .git directory: %w", p.root, err)
	}
	if !gitEntry.IsDir() {
		return fmt.Errorf("invalid managed project %s: trunk .git must be a directory", p.root)
	}

	common, err := a.git.output(p.trunk, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("validate trunk repository: %w", err)
	}
	common, err = absolutePath(p.trunk, common)
	if err != nil {
		return fmt.Errorf("resolve trunk git directory: %w", err)
	}
	expected := filepath.Join(p.trunk, ".git")
	if !sameResolvedPath(common, expected) {
		return fmt.Errorf("invalid managed project %s: trunk uses git directory %s, expected %s", p.root, common, expected)
	}
	return nil
}

func sameResolvedPath(left, right string) bool {
	resolvedLeft, leftErr := filepath.EvalSymlinks(left)
	resolvedRight, rightErr := filepath.EvalSymlinks(right)
	if leftErr == nil && rightErr == nil {
		return filepath.Clean(resolvedLeft) == filepath.Clean(resolvedRight)
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func parseWorktrees(output []byte) ([]worktree, error) {
	var result []worktree
	var current *worktree
	for _, rawField := range bytes.Split(output, []byte{0}) {
		field := string(rawField)
		if field == "" {
			if current != nil {
				result = append(result, *current)
				current = nil
			}
			continue
		}
		name, value, _ := strings.Cut(field, " ")
		if name == "worktree" {
			if value == "" {
				return nil, errors.New("git worktree record has an empty path")
			}
			if current != nil {
				result = append(result, *current)
			}
			current = &worktree{path: value}
			continue
		}
		if current == nil {
			return nil, fmt.Errorf("unexpected git worktree field %q before worktree path", field)
		}
		switch name {
		case "HEAD":
			current.head = value
		case "branch":
			current.branchRef = value
		case "detached":
			current.detached = true
		case "locked":
			current.locked = value
		case "prunable":
			current.prunable = value
		case "bare":
			// A managed project never has a bare worktree, but retain parser compatibility.
		}
	}
	if current != nil {
		result = append(result, *current)
	}
	if len(result) == 0 {
		return nil, errors.New("git returned no worktrees")
	}
	return result, nil
}

func (a *Manager) projectWorktrees(p project, includeDirty bool) ([]worktree, error) {
	output, err := a.git.run(p.trunk, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	worktrees, err := parseWorktrees(output)
	if err != nil {
		return nil, err
	}
	if includeDirty {
		for i := range worktrees {
			status, err := a.git.run(worktrees[i].path, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignored=matching")
			if err != nil {
				return nil, fmt.Errorf("inspect worktree %s: %w", worktrees[i].path, err)
			}
			worktrees[i].dirty = len(status) > 0
		}
	}
	return worktrees, nil
}

func worktreeTask(p project, path string) string {
	if sameResolvedPath(path, p.trunk) {
		return "trunk"
	}
	if filepath.Clean(filepath.Dir(path)) == filepath.Clean(p.worktrees) {
		return filepath.Base(path)
	}
	return "external:" + filepath.Base(path)
}

func (a *Manager) activeGitOperation(path string) (string, error) {
	markers := []string{
		"index.lock",
		"MERGE_HEAD",
		"CHERRY_PICK_HEAD",
		"REVERT_HEAD",
		"BISECT_LOG",
		"rebase-merge",
		"rebase-apply",
		"sequencer",
	}
	for _, marker := range markers {
		resolved, err := a.git.output(path, "rev-parse", "--path-format=absolute", "--git-path", marker)
		if err != nil {
			return "", err
		}
		if _, err := os.Lstat(resolved); err == nil {
			return marker, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return "", nil
}

func (a *Manager) defaultBase(p project) (string, error) {
	ref, err := a.git.output(p.trunk, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", errors.New("cannot determine origin's default branch; pass --base explicitly")
	}
	const prefix = "refs/remotes/"
	if !strings.HasPrefix(ref, prefix) {
		return "", fmt.Errorf("unexpected origin default branch reference %q", ref)
	}
	return strings.TrimPrefix(ref, prefix), nil
}
