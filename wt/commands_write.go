package wt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Clone creates a managed project containing a clone of remote.
func (a *Manager) Clone(remote, destination string) error {
	if destination == "" {
		name, err := projectNameFromRemote(remote)
		if err != nil {
			return err
		}
		destination = name
	}
	root, err := absolutePath(a.cwd, destination)
	if err != nil {
		return fmt.Errorf("resolve destination: %w", err)
	}
	p := projectAtRoot(root)
	if err := validateNewProjectPath(p.root); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(a.out, "Remote: %s\nProject root: %s\nTrunk: %s\n", remote, p.root, p.trunk); err != nil {
		return err
	}
	created, err := createProjectSkeleton(p)
	if err != nil {
		return err
	}
	if _, err := a.git.run("", "clone", "--", remote, p.trunk); err != nil {
		return fmt.Errorf("clone failed; partial project remains at %s (created paths: %s): %w", p.root, strings.Join(created, ", "), err)
	}
	_, err = fmt.Fprintf(a.out, "Created project %s\n", p.root)
	return err
}

// Adopt converts a standalone checkout into a managed project in place.
func (a *Manager) Adopt(source string, dryRun bool) error {
	var err error
	if source == "" {
		source, err = a.git.output(a.cwd, "rev-parse", "--path-format=absolute", "--show-toplevel")
		if err != nil {
			return errors.New("adopt must be run inside a Git checkout or given an explicit source")
		}
	} else {
		source, err = absolutePath(a.cwd, source)
		if err != nil {
			return fmt.Errorf("resolve source: %w", err)
		}
	}
	unresolvedSource := source
	source, err = filepath.EvalSymlinks(unresolvedSource)
	if err != nil {
		return fmt.Errorf("resolve source %s: %w", unresolvedSource, err)
	}
	p := projectAtRoot(source)
	if err := a.validateAdoption(source, p); err != nil {
		return err
	}
	callerInsideSource := false
	if cwd, err := filepath.EvalSymlinks(a.cwd); err == nil {
		callerInsideSource = pathContains(source, cwd)
	}

	if _, err := fmt.Fprintf(a.out, "Source: %s\nProject root: %s\nTrunk: %s\n", source, p.root, p.trunk); err != nil {
		return err
	}
	if dryRun {
		_, err := fmt.Fprintln(a.out, "Dry run: adoption checks passed; no files changed")
		return err
	}

	temporary, err := reserveAdoptionPath(source)
	if err != nil {
		return fmt.Errorf("prepare adoption beside %s: %w", source, err)
	}
	if err := a.rename(source, temporary); err != nil {
		return fmt.Errorf("move checkout aside for adoption (source left unchanged): %w", err)
	}
	created, err := createInPlaceProjectSkeleton(p)
	if err != nil {
		return a.rollbackAdoption(source, temporary, created, err)
	}
	if err := a.rename(temporary, p.trunk); err != nil {
		return a.rollbackAdoption(source, temporary, created, fmt.Errorf("move checkout into trunk: %w", err))
	}

	if _, err := fmt.Fprintf(a.out, "Adopted project at %s\n", p.trunk); err != nil {
		return err
	}
	if callerInsideSource {
		if _, err := fmt.Fprintf(a.err, "warning: your shell may still display the old working directory; run: cd %q\n", p.trunk); err != nil {
			return err
		}
	}
	return nil
}

func reserveAdoptionPath(source string) (string, error) {
	temporary, err := os.MkdirTemp(filepath.Dir(source), "."+filepath.Base(source)+".git-wt-adopt-")
	if err != nil {
		return "", err
	}
	if err := os.Remove(temporary); err != nil {
		return "", err
	}
	return temporary, nil
}

func createInPlaceProjectSkeleton(p project) ([]string, error) {
	var created []string
	for _, path := range []string{p.root, p.worktrees, p.notes} {
		if err := os.Mkdir(path, 0o755); err != nil {
			return created, fmt.Errorf("create project directory %s: %w", path, err)
		}
		created = append(created, path)
	}
	agents, err := createProjectAgents(p)
	if err != nil {
		return created, err
	}
	created = append(created, agents)
	return created, nil
}

func (a *Manager) rollbackAdoption(source, temporary string, created []string, cause error) error {
	rollbackEmptyDirectories(created)
	if err := a.rename(temporary, source); err != nil {
		return fmt.Errorf("adoption failed: %v; rollback also failed: checkout remains at %s: %w", cause, temporary, err)
	}
	return fmt.Errorf("adoption failed; source restored at %s: %w", source, cause)
}

func (a *Manager) validateAdoption(source string, p project) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("inspect source %s: %w", source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("adoption source %s is not a directory", source)
	}
	gitEntry, err := os.Lstat(filepath.Join(source, ".git"))
	if err != nil {
		return fmt.Errorf("adoption source must have its own .git directory: %w", err)
	}
	if !gitEntry.IsDir() {
		return errors.New("adoption source .git is not a directory; linked worktrees and separate git directories are unsupported")
	}
	bare, err := a.git.output(source, "rev-parse", "--is-bare-repository")
	if err != nil || bare != "false" {
		return errors.New("adoption source must be an ordinary non-bare Git checkout")
	}
	top, err := a.git.output(source, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("inspect adoption source: %w", err)
	}
	if !sameResolvedPath(top, source) {
		return fmt.Errorf("adoption source must be the repository top level %s", top)
	}
	output, err := a.git.run(source, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return err
	}
	worktrees, err := parseWorktrees(output)
	if err != nil {
		return err
	}
	if len(worktrees) != 1 || !sameResolvedPath(worktrees[0].path, source) {
		return errors.New("adoption source already has linked worktrees; only standalone checkouts are supported")
	}
	if operation, err := a.activeGitOperation(source); err != nil {
		return fmt.Errorf("check active Git operations: %w", err)
	} else if operation != "" {
		return fmt.Errorf("cannot adopt while Git operation marker %s exists", operation)
	}
	for _, reserved := range []string{p.trunk, p.worktrees, p.notes} {
		if _, err := os.Lstat(reserved); err == nil {
			return fmt.Errorf("cannot adopt in place: reserved path already exists: %s", reserved)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect reserved path %s: %w", reserved, err)
		}
	}
	return nil
}

// NewWorktree creates a task worktree and its branch.
func (a *Manager) NewWorktree(task string, options NewOptions) error {
	cfg, err := a.loadConfig(options.BranchTemplate)
	if err != nil {
		return err
	}
	if err := validateTask(task); err != nil {
		return err
	}
	p, err := a.resolveProject()
	if err != nil {
		return err
	}
	destination := filepath.Join(p.worktrees, task)
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("task destination already exists: %s", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect task destination: %w", err)
	}
	worktrees, err := a.projectWorktrees(p, false)
	if err != nil {
		return err
	}
	for _, item := range worktrees {
		if sameResolvedPath(item.path, destination) || worktreeTask(p, item.path) == task {
			return fmt.Errorf("task %q already has a registered worktree at %s", task, item.path)
		}
	}

	branch := options.Branch
	if branch == "" {
		branch = strings.ReplaceAll(cfg.branchTemplate, "%s", task)
	}
	if _, err := a.git.run(p.trunk, "check-ref-format", "--branch", branch); err != nil {
		return fmt.Errorf("invalid branch name %q: %w", branch, err)
	}
	base := options.Base
	if base == "" {
		base, err = a.defaultBase(p)
		if err != nil {
			return err
		}
	}
	if _, err := a.git.run(p.trunk, "rev-parse", "--verify", "--quiet", base+"^{commit}"); err != nil {
		return fmt.Errorf("base %q is not a commit: %w", base, err)
	}
	if _, err := a.git.run(p.trunk, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		return fmt.Errorf("branch %q already exists", branch)
	} else if gitExitCode(err) != 1 {
		return fmt.Errorf("check branch %q: %w", branch, err)
	}

	if _, err := fmt.Fprintf(a.out, "Project: %s\nTask: %s\nDestination: %s\nBase: %s\nBranch: %s\n", p.root, task, destination, base, branch); err != nil {
		return err
	}
	if _, err := a.git.run(p.trunk, "worktree", "add", "-b", branch, destination, base); err != nil {
		return fmt.Errorf("create task worktree: %w", err)
	}
	_, err = fmt.Fprintf(a.out, "Created task worktree %s\n", destination)
	return err
}

// Pull fast-forwards the current managed project's trunk.
func (a *Manager) Pull() error {
	p, err := a.resolveProject()
	if err != nil {
		return err
	}
	status, err := a.git.run(p.trunk, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return fmt.Errorf("inspect trunk worktree: %w", err)
	}
	if len(status) != 0 {
		return fmt.Errorf("refusing to pull into dirty trunk %s", p.trunk)
	}
	if operation, err := a.activeGitOperation(p.trunk); err != nil {
		return fmt.Errorf("check trunk Git operations: %w", err)
	} else if operation != "" {
		return fmt.Errorf("refusing to pull with active trunk Git operation marker %s", operation)
	}

	branch, err := a.git.output(p.trunk, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return errors.New("refusing to pull: trunk is in detached HEAD state")
	}
	base, err := a.defaultBase(p)
	if err != nil {
		return fmt.Errorf("determine trunk default branch: %w", err)
	}
	defaultBranch := strings.TrimPrefix(base, "origin/")
	if branch != defaultBranch {
		return fmt.Errorf("refusing to pull: trunk is on branch %q, expected default branch %q", branch, defaultBranch)
	}
	before, err := a.git.output(p.trunk, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("inspect trunk HEAD: %w", err)
	}
	if _, err := fmt.Fprintf(a.out, "Project: %s\nTrunk: %s\nBranch: %s\nRemote: origin/%s\n", p.root, p.trunk, branch, defaultBranch); err != nil {
		return err
	}
	if _, err := a.git.run(p.trunk, "pull", "--ff-only", "origin", defaultBranch); err != nil {
		return fmt.Errorf("pull trunk: %w", err)
	}
	after, err := a.git.output(p.trunk, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("inspect updated trunk HEAD: %w", err)
	}
	if before == after {
		_, err = fmt.Fprintf(a.out, "Trunk already up to date at %s\n", after)
		return err
	}
	_, err = fmt.Fprintf(a.out, "Updated trunk %s: %s -> %s\n", branch, before, after)
	return err
}

// Remove removes a task worktree and optionally its merged branch.
func (a *Manager) Remove(task string, options RemoveOptions) error {
	if err := validateTask(task); err != nil {
		return err
	}
	p, err := a.resolveProject()
	if err != nil {
		return err
	}
	destination := filepath.Join(p.worktrees, task)
	worktrees, err := a.projectWorktrees(p, false)
	if err != nil {
		return err
	}
	var selected *worktree
	for i := range worktrees {
		if sameResolvedPath(worktrees[i].path, destination) {
			selected = &worktrees[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("task %q is not a registered worktree", task)
	}
	if !options.Force {
		status, err := a.git.run(destination, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignored=matching")
		if err != nil {
			return fmt.Errorf("inspect task worktree: %w", err)
		}
		if len(status) != 0 {
			return fmt.Errorf("refusing to remove dirty task worktree %s; use --force to discard local files", destination)
		}
	}
	if operation, err := a.activeGitOperation(destination); err != nil {
		return err
	} else if operation != "" {
		return fmt.Errorf("refusing to remove task with active Git operation marker %s", operation)
	}

	branch := strings.TrimPrefix(selected.branchRef, "refs/heads/")
	if options.DeleteBranch {
		if selected.detached || branch == "" {
			return errors.New("cannot delete a branch for a detached worktree")
		}
		base, err := a.defaultBase(p)
		if err != nil {
			return fmt.Errorf("verify branch is merged: %w", err)
		}
		if _, err := a.git.run(p.trunk, "merge-base", "--is-ancestor", "refs/heads/"+branch, base); err != nil {
			if gitExitCode(err) == 1 {
				return fmt.Errorf("refusing to delete unmerged branch %q (not merged into %s)", branch, base)
			}
			return fmt.Errorf("verify branch is merged: %w", err)
		}
	}

	if _, err := fmt.Fprintf(a.out, "Task: %s\nWorktree: %s\nBranch: %s\n", task, destination, branch); err != nil {
		return err
	}
	removeArgs := []string{"worktree", "remove"}
	if options.Force {
		removeArgs = append(removeArgs, "--force")
	}
	removeArgs = append(removeArgs, destination)
	if _, err := a.git.run(p.trunk, removeArgs...); err != nil {
		return fmt.Errorf("remove task worktree: %w", err)
	}
	if options.DeleteBranch {
		if _, err := a.git.run(p.trunk, "update-ref", "-d", "refs/heads/"+branch, selected.head); err != nil {
			return fmt.Errorf("worktree removed but branch %q could not be deleted: %w", branch, err)
		}
		_, err = fmt.Fprintf(a.out, "Removed task worktree and branch %s\n", branch)
		return err
	}
	_, err = fmt.Fprintln(a.out, "Removed task worktree; branch retained")
	return err
}

func projectNameFromRemote(remote string) (string, error) {
	trimmed := strings.TrimSuffix(strings.TrimRight(remote, "/"), ".git")
	if index := strings.LastIndex(trimmed, "/"); index >= 0 {
		trimmed = trimmed[index+1:]
	} else if index := strings.LastIndex(trimmed, ":"); index >= 0 {
		trimmed = trimmed[index+1:]
	}
	if err := validateComponent("project derived from remote", trimmed); err != nil {
		return "", err
	}
	return trimmed, nil
}

func validateNewProjectPath(projectRoot string) error {
	if _, err := os.Lstat(projectRoot); err == nil {
		return fmt.Errorf("destination project already exists: %s", projectRoot)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect destination project: %w", err)
	}
	return nil
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative))
}

func createProjectSkeleton(p project) ([]string, error) {
	var created []string
	parentCreated, err := mkdirAllTracked(filepath.Dir(p.root), 0o755)
	if err != nil {
		return nil, err
	}
	created = append(created, parentCreated...)
	for _, path := range []string{p.root, p.worktrees, p.notes} {
		if err := os.Mkdir(path, 0o755); err != nil {
			rollbackEmptyDirectories(created)
			return nil, fmt.Errorf("create project directory %s: %w", path, err)
		}
		created = append(created, path)
	}
	agents, err := createProjectAgents(p)
	if err != nil {
		rollbackEmptyDirectories(created)
		return nil, err
	}
	created = append(created, agents)
	return created, nil
}

func mkdirAllTracked(path string, mode os.FileMode) ([]string, error) {
	var missing []string
	for candidate := filepath.Clean(path); ; candidate = filepath.Dir(candidate) {
		if _, err := os.Stat(candidate); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		missing = append(missing, candidate)
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
	}
	var created []string
	for i := len(missing) - 1; i >= 0; i-- {
		if err := os.Mkdir(missing[i], mode); err != nil {
			rollbackEmptyDirectories(created)
			return nil, err
		}
		created = append(created, missing[i])
	}
	return created, nil
}

func rollbackEmptyDirectories(created []string) {
	for i := len(created) - 1; i >= 0; i-- {
		_ = os.Remove(created[i])
	}
}
