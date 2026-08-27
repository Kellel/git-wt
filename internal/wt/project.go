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
	key       string
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

func projectForKey(cfg config, key string) (project, error) {
	parts := strings.Split(key, "/")
	if len(parts) != 2 {
		return project{}, fmt.Errorf("project %q must have the form <namespace>/<project>", key)
	}
	if err := validateComponent("namespace", parts[0]); err != nil {
		return project{}, err
	}
	if err := validateComponent("project", parts[1]); err != nil {
		return project{}, err
	}
	root := filepath.Join(cfg.root, parts[0], parts[1])
	if err := ensureLexicallyWithin(cfg.root, root); err != nil {
		return project{}, err
	}
	p := projectAtRoot(root)
	p.key = key
	return p, nil
}

func projectAtRoot(root string) project {
	return project{
		root:      root,
		trunk:     filepath.Join(root, "trunk"),
		worktrees: filepath.Join(root, "worktrees"),
		notes:     filepath.Join(root, "notes"),
		agents:    filepath.Join(root, "AGENTS.md"),
		key:       filepath.Base(root),
	}
}

func (a *App) resolveProject(cfg config, key string) (project, error) {
	if key != "" {
		p, err := projectForKey(cfg, key)
		if err != nil {
			return project{}, err
		}
		if err := ensureExistingWithin(cfg.root, p.root); err != nil {
			return project{}, err
		}
		if err := a.validateProject(cfg, p); err != nil {
			return project{}, err
		}
		return p, nil
	}

	if top, err := a.git.output(a.cwd, "rev-parse", "--path-format=absolute", "--show-toplevel"); err == nil {
		p, err := a.projectFromWorktree(cfg, top)
		if err != nil {
			return project{}, err
		}
		if err := a.validateProject(cfg, p); err != nil {
			return project{}, err
		}
		return p, nil
	}

	for candidate := filepath.Clean(a.cwd); ; candidate = filepath.Dir(candidate) {
		p, err := a.projectFromRoot(cfg, candidate)
		if err == nil {
			if err := a.validateProject(cfg, p); err == nil {
				return p, nil
			}
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
	}
	return project{}, errors.New("current directory is not inside a managed project; use --project where supported")
}

func (a *App) projectFromWorktree(cfg config, top string) (project, error) {
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
	return a.projectFromRoot(cfg, root)
}

func (a *App) projectFromRoot(cfg config, root string) (project, error) {
	root, err := absolutePath(a.cwd, root)
	if err != nil {
		return project{}, err
	}
	p := projectAtRoot(root)
	relative, err := filepath.Rel(cfg.root, root)
	if err == nil {
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) == 2 && validateComponent("namespace", parts[0]) == nil && validateComponent("project", parts[1]) == nil {
			p.key = parts[0] + "/" + parts[1]
		}
	}
	return p, nil
}

func (a *App) validateProject(_ config, p project) error {
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

func ensureLexicallyWithin(root, path string) error {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("compare %s with workspace root %s: %w", path, root, err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("path %s is outside workspace root %s", path, root)
	}
	return nil
}

func ensureExistingWithin(root, path string) error {
	if err := ensureLexicallyWithin(root, path); err != nil {
		return err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve workspace root %s: %w", root, err)
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve path %s: %w", path, err)
	}
	if err := ensureLexicallyWithin(resolvedRoot, resolvedPath); err != nil {
		return fmt.Errorf("resolved path escapes workspace root: %w", err)
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

func (a *App) projectWorktrees(p project, includeDirty bool) ([]worktree, error) {
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
			status, err := a.git.run(worktrees[i].path, "status", "--porcelain=v1", "-z", "--untracked-files=all")
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

func (a *App) activeGitOperation(path string) (string, error) {
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

func (a *App) defaultBase(p project) (string, error) {
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
