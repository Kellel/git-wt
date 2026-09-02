package wt

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var gitWTTestBinary string

func TestMain(m *testing.M) {
	temporary, err := os.MkdirTemp("", "git-wt-integration-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	gitWTTestBinary = filepath.Join(temporary, "git-wt")
	build := exec.Command("go", "build", "-o", gitWTTestBinary, "../cmd/git-wt")
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build git-wt test binary: %v\n%s", err, output)
		_ = os.RemoveAll(temporary)
		os.Exit(1)
	}

	code := m.Run()
	if err := os.RemoveAll(temporary); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func TestCloneNewListPathAndRemove(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	remote := createRemote(t, testRoot, env)
	workspace := filepath.Join(testRoot, "workspace")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, _ := runTestCLI(t, workspace, env,
		"clone", remote,
	)
	projectRoot := filepath.Join(workspace, "remote")
	trunk := filepath.Join(projectRoot, "trunk")
	worktrees := filepath.Join(projectRoot, "worktrees")
	notes := filepath.Join(projectRoot, "notes")
	assertContains(t, stdout, "Created project "+projectRoot)
	assertDirectory(t, trunk)
	assertDirectory(t, worktrees)
	assertDirectory(t, notes)
	assertCodexTrunkGuard(t, env, trunk)

	stdout, _ = runTestCLI(t, projectRoot, env,
		"path",
	)
	if got := strings.TrimSpace(stdout); got != trunk {
		t.Fatalf("trunk path = %q, want %q", got, trunk)
	}
	stdout, _ = runTestCLI(t, projectRoot, env,
		"path", "--notes",
	)
	if got := strings.TrimSpace(stdout); got != notes {
		t.Fatalf("notes path = %q, want %q", got, notes)
	}

	notePath := filepath.Join(notes, "investigation.md")
	writeFile(t, notePath, "local notes\n")
	trunkBranch := gitOutput(t, env, trunk, "branch", "--show-current")
	nested := filepath.Join(trunk, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, _ = runTestCLI(t, nested, env,
		"new", "TASK-123-example",
	)
	taskPath := filepath.Join(worktrees, "TASK-123-example")
	assertContains(t, stdout, "Base: origin/main")
	assertDirectory(t, taskPath)
	if got := gitOutput(t, env, trunk, "branch", "--show-current"); got != trunkBranch {
		t.Fatalf("trunk branch changed from %q to %q", trunkBranch, got)
	}
	if got := gitOutput(t, env, taskPath, "branch", "--show-current"); got != "TASK-123-example" {
		t.Fatalf("task branch = %q", got)
	}
	assertNotExist(t, filepath.Join(taskPath, filepath.FromSlash(codexConfigRelativePath)))

	stdout, _ = runTestCLI(t, taskPath, env, "path", "TASK-123-example")
	if got := strings.TrimSpace(stdout); got != taskPath {
		t.Fatalf("task path = %q, want %q", got, taskPath)
	}

	// Listing does not contact the remote.
	offlineRemote := remote + ".offline"
	if err := os.Rename(remote, offlineRemote); err != nil {
		t.Fatal(err)
	}
	stdout, _ = runTestCLI(t, projectRoot, env, "list")
	assertContains(t, stdout, "TASK-123-example")
	assertContains(t, stdout, "trunk")
	assertContains(t, stdout, "no")

	writeFile(t, filepath.Join(taskPath, "untracked.txt"), "do not lose\n")
	stdout, _ = runTestCLI(t, projectRoot, env, "list")
	assertContains(t, stdout, "TASK-123-example")
	assertContains(t, stdout, "yes")

	stdout, _ = runTestCLI(t, projectRoot, env, "list", "--porcelain")
	if !bytes.Contains([]byte(stdout), []byte("task TASK-123-example\x00")) ||
		!bytes.Contains([]byte(stdout), []byte("dirty true\x00")) {
		t.Fatalf("porcelain output missing task or dirty state: %q", stdout)
	}

	_, _, err := runTestCLIError(projectRoot, env, "remove", "TASK-123-example")
	assertErrorContains(t, err, "refusing to remove dirty task worktree")
	assertDirectory(t, taskPath)
	if got := readFile(t, filepath.Join(taskPath, "untracked.txt")); got != "do not lose\n" {
		t.Fatalf("untracked file changed: %q", got)
	}

	if err := os.Remove(filepath.Join(taskPath, "untracked.txt")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(taskPath, "tracked.txt"), "modified\n")
	_, _, err = runTestCLIError(projectRoot, env, "remove", "TASK-123-example")
	assertErrorContains(t, err, "refusing to remove dirty task worktree")
	assertDirectory(t, taskPath)
	gitRun(t, env, taskPath, "restore", "tracked.txt")
	writeFile(t, filepath.Join(taskPath, "ignored.txt"), "ignored build output\n")
	stdout, _ = runTestCLI(t, projectRoot, env, "list", "--porcelain")
	assertPorcelainDirty(t, stdout, "TASK-123-example", false)
	_, _, err = runTestCLIError(projectRoot, env, "remove", "TASK-123-example")
	assertErrorContains(t, err, "refusing to remove dirty task worktree")
	if got := readFile(t, filepath.Join(taskPath, "ignored.txt")); got != "ignored build output\n" {
		t.Fatalf("ignored file changed: %q", got)
	}
	writeFile(t, filepath.Join(taskPath, "tracked.txt"), "discarded tracked change\n")
	writeFile(t, filepath.Join(taskPath, "untracked.txt"), "discarded untracked file\n")
	stdout, _ = runTestCLI(t, projectRoot, env,
		"remove", "TASK-123-example", "--delete-branch", "--force",
	)
	assertContains(t, stdout, "Removed task worktree and branch TASK-123-example")
	if _, err := os.Stat(taskPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("task worktree still exists: %v", err)
	}
	if code := gitExit(t, env, trunk, "show-ref", "--verify", "--quiet", "refs/heads/TASK-123-example"); code != 1 {
		t.Fatalf("deleted branch show-ref exit = %d, want 1", code)
	}
	if got := readFile(t, notePath); got != "local notes\n" {
		t.Fatalf("notes changed: %q", got)
	}
}

func TestAdoptPreservesCompleteCheckout(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	source := filepath.Join(testRoot, "demo")
	initRepository(t, source, env)
	gitRun(t, env, source, "branch", "local-only")
	gitRun(t, env, source, "tag", "local-tag")
	gitRun(t, env, source, "config", "custom.preserved", "yes")
	writeFile(t, filepath.Join(source, ".git", "hooks", "pre-commit"), "#!/bin/sh\nexit 0\n")
	writeFile(t, filepath.Join(source, "tracked.txt"), "staged\n")
	gitRun(t, env, source, "add", "tracked.txt")
	writeFile(t, filepath.Join(source, "tracked.txt"), "worktree\n")
	writeFile(t, filepath.Join(source, "untracked.txt"), "untracked\n")
	writeFile(t, filepath.Join(source, "ignored.txt"), "ignored\n")
	nested := filepath.Join(source, "nested", "directory")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	statusBefore := gitOutputRaw(t, env, source, "status", "--porcelain=v1", "-z")
	refsBefore := gitOutputRaw(t, env, source, "for-each-ref", "--format=%(refname)%00%(objectname)")
	reflogBefore := gitOutputRaw(t, env, source, "reflog", "show", "--all", "--format=%H%x00%gs")
	hookBefore := readFile(t, filepath.Join(source, ".git", "hooks", "pre-commit"))

	stdout, _ := runTestCLI(t, nested, env,
		"adopt", "--dry-run",
	)
	assertContains(t, stdout, "Source: "+source)
	assertContains(t, stdout, "Dry run: adoption checks passed")
	assertDirectory(t, source)
	assertNotExist(t, filepath.Join(source, "trunk"))
	assertNotExist(t, filepath.Join(source, "worktrees"))
	assertNotExist(t, filepath.Join(source, "notes"))
	assertNotExist(t, filepath.Join(source, filepath.FromSlash(codexConfigRelativePath)))

	stdout, stderr := runTestCLI(t, nested, env,
		"adopt",
	)
	trunk := filepath.Join(source, "trunk")
	assertContains(t, stdout, "Adopted project at "+trunk)
	assertContains(t, stderr, "your shell may still display the old working directory")
	assertDirectory(t, source)
	assertDirectory(t, trunk)
	assertDirectory(t, filepath.Join(source, "worktrees"))
	assertDirectory(t, filepath.Join(source, "notes"))
	assertNotExist(t, filepath.Join(source, ".git"))
	guidance := readFile(t, filepath.Join(source, "AGENTS.md"))
	assertContains(t, guidance, "default to creating one with `git wt new <task>`")
	assertContains(t, guidance, "Repository code and repository-specific instructions live in `trunk/`")
	assertContains(t, guidance, "Temporary agent working files, scratchpads, plans, and handoff notes may be placed in `notes/`")
	assertCodexTrunkGuard(t, env, trunk)

	if got := gitOutputRaw(t, env, trunk, "status", "--porcelain=v1", "-z"); got != statusBefore {
		t.Fatalf("status changed during adoption\nbefore: %q\nafter:  %q", statusBefore, got)
	}
	if got := readFile(t, filepath.Join(trunk, "ignored.txt")); got != "ignored\n" {
		t.Fatalf("ignored file changed: %q", got)
	}
	if got := gitOutputRaw(t, env, trunk, "for-each-ref", "--format=%(refname)%00%(objectname)"); got != refsBefore {
		t.Fatalf("refs changed during adoption\nbefore: %q\nafter:  %q", refsBefore, got)
	}
	if got := gitOutputRaw(t, env, trunk, "reflog", "show", "--all", "--format=%H%x00%gs"); got != reflogBefore {
		t.Fatalf("reflogs changed during adoption\nbefore: %q\nafter:  %q", reflogBefore, got)
	}
	if got := gitOutput(t, env, trunk, "config", "custom.preserved"); got != "yes" {
		t.Fatalf("repository config not preserved: %q", got)
	}
	if got := readFile(t, filepath.Join(trunk, ".git", "hooks", "pre-commit")); got != hookBefore {
		t.Fatalf("hook changed: %q", got)
	}

	// An adopted repository without origin requires an explicit base.
	_, _, err := runTestCLIError(trunk, env, "new", "local-task")
	assertErrorContains(t, err, "pass --base explicitly")
	runTestCLI(t, trunk, env, "new", "local-task", "--base", "HEAD")
	assertDirectory(t, filepath.Join(source, "worktrees", "local-task"))
}

func TestAdoptRefusesUnsupportedOrUnsafeSources(t *testing.T) {
	t.Parallel()

	t.Run("implicit source outside checkout", func(t *testing.T) {
		testRoot := t.TempDir()
		env := testEnvironment(t, testRoot)

		_, _, err := runTestCLIError(testRoot, env,
			"adopt",
		)
		assertErrorContains(t, err, "must be run inside a Git checkout")
	})

	t.Run("linked worktree", func(t *testing.T) {
		testRoot := t.TempDir()
		env := testEnvironment(t, testRoot)
		source := filepath.Join(testRoot, "source")
		initRepository(t, source, env)
		linked := filepath.Join(testRoot, "linked")
		gitRun(t, env, source, "worktree", "add", "-b", "linked", linked, "HEAD")

		_, _, err := runTestCLIError(testRoot, env,
			"adopt", source,
		)
		assertErrorContains(t, err, "already has linked worktrees")
		assertDirectory(t, source)
	})

	t.Run("linked checkout as source", func(t *testing.T) {
		testRoot := t.TempDir()
		env := testEnvironment(t, testRoot)
		source := filepath.Join(testRoot, "source")
		initRepository(t, source, env)
		linked := filepath.Join(testRoot, "linked")
		gitRun(t, env, source, "worktree", "add", "-b", "linked", linked, "HEAD")

		_, _, err := runTestCLIError(testRoot, env,
			"adopt", linked,
		)
		assertErrorContains(t, err, ".git is not a directory")
		assertDirectory(t, linked)
	})

	t.Run("reserved layout path", func(t *testing.T) {
		testRoot := t.TempDir()
		env := testEnvironment(t, testRoot)
		source := filepath.Join(testRoot, "source")
		initRepository(t, source, env)
		if err := os.Mkdir(filepath.Join(source, "worktrees"), 0o755); err != nil {
			t.Fatal(err)
		}

		_, _, err := runTestCLIError(testRoot, env,
			"adopt", source,
		)
		assertErrorContains(t, err, "reserved path already exists")
		assertDirectory(t, source)
	})

	t.Run("active operation", func(t *testing.T) {
		testRoot := t.TempDir()
		env := testEnvironment(t, testRoot)
		source := filepath.Join(testRoot, "source")
		initRepository(t, source, env)
		writeFile(t, filepath.Join(source, ".git", "MERGE_HEAD"), strings.Repeat("0", 40)+"\n")

		_, _, err := runTestCLIError(testRoot, env,
			"adopt", source,
		)
		assertErrorContains(t, err, "Git operation marker MERGE_HEAD")
		assertDirectory(t, source)
	})

	t.Run("first rename failure leaves source unchanged", func(t *testing.T) {
		testRoot := t.TempDir()
		env := testEnvironment(t, testRoot)
		source := filepath.Join(testRoot, "source")
		initRepository(t, source, env)
		manager, _, _ := newTestManagerWithRename(testRoot, env, func(string, string) error {
			return errors.New("injected rename failure")
		})
		err := manager.Adopt(source, false)
		assertErrorContains(t, err, "source left unchanged")
		assertDirectory(t, source)
	})

	t.Run("second rename failure restores source", func(t *testing.T) {
		testRoot := t.TempDir()
		env := testEnvironment(t, testRoot)
		source := filepath.Join(testRoot, "source")
		initRepository(t, source, env)
		excludePath := filepath.Join(source, ".git", "info", "exclude")
		excludeBefore := readFile(t, excludePath)
		calls := 0
		manager, _, _ := newTestManagerWithRename(testRoot, env, func(oldPath, newPath string) error {
			calls++
			if calls == 2 {
				return errors.New("injected rename failure")
			}
			return os.Rename(oldPath, newPath)
		})
		err := manager.Adopt(source, false)
		assertErrorContains(t, err, "source restored")
		assertDirectory(t, source)
		assertDirectory(t, filepath.Join(source, ".git"))
		assertNotExist(t, filepath.Join(source, "trunk"))
		assertNotExist(t, filepath.Join(source, "AGENTS.md"))
		assertNotExist(t, filepath.Join(source, filepath.FromSlash(codexConfigRelativePath)))
		if got := readFile(t, excludePath); got != excludeBefore {
			t.Fatalf("Git exclude file changed during rollback\nbefore: %q\nafter:  %q", excludeBefore, got)
		}
		matches, globErr := filepath.Glob(filepath.Join(testRoot, ".source.git-wt-adopt-*"))
		if globErr != nil {
			t.Fatal(globErr)
		}
		if len(matches) != 0 {
			t.Fatalf("temporary adoption paths remain: %v", matches)
		}
	})
}

func TestAdoptPreservesExistingCodexConfig(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	source := filepath.Join(testRoot, "demo")
	initRepository(t, source, env)
	configPath := filepath.Join(source, filepath.FromSlash(codexConfigRelativePath))
	const existingConfig = "sandbox_mode = \"workspace-write\"\n"
	writeFile(t, configPath, existingConfig)
	excludePath := filepath.Join(source, ".git", "info", "exclude")
	excludeBefore := readFile(t, excludePath)

	_, stderr := runTestCLI(t, source, env, "adopt")
	trunk := filepath.Join(source, "trunk")
	assertContains(t, stderr, "Codex trunk protection was not installed")
	if got := readFile(t, filepath.Join(trunk, filepath.FromSlash(codexConfigRelativePath))); got != existingConfig {
		t.Fatalf("existing Codex config changed: %q", got)
	}
	if got := readFile(t, filepath.Join(trunk, ".git", "info", "exclude")); got != excludeBefore {
		t.Fatalf("Git exclude file changed for existing Codex config\nbefore: %q\nafter:  %q", excludeBefore, got)
	}
}

func TestCloneDerivesProjectAndReportsPartialFailure(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	remote := createRemote(t, testRoot, env)
	workspace := filepath.Join(testRoot, "workspace")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	runTestCLI(t, workspace, env, "clone", remote)
	projectRoot := filepath.Join(workspace, "remote")
	assertDirectory(t, filepath.Join(projectRoot, "trunk"))
	assertContains(t, readFile(t, filepath.Join(projectRoot, "AGENTS.md")), "This directory is managed by `git wt`")

	missingRemote := filepath.Join(testRoot, "missing.git")
	_, _, err := runTestCLIError(workspace, env, "clone", missingRemote, "broken")
	assertErrorContains(t, err, "partial project remains")
	assertDirectory(t, filepath.Join(workspace, "broken", "worktrees"))
	assertDirectory(t, filepath.Join(workspace, "broken", "notes"))
}

func TestCloneRefusesExistingDestination(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	remote := createRemote(t, testRoot, env)
	workspace := filepath.Join(testRoot, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(workspace, "demo")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := runTestCLIError(workspace, env,
		"clone", remote, "demo",
	)
	assertErrorContains(t, err, "destination project already exists")
}

func TestRemoveWithoutBranchDeletionRetainsBranch(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	remote := createRemote(t, testRoot, env)
	workspace := filepath.Join(testRoot, "workspace")
	projectRoot := filepath.Join(workspace, "demo")
	runTestCLI(t, testRoot, env, "clone", remote, projectRoot)
	trunk := filepath.Join(projectRoot, "trunk")
	runTestCLI(t, trunk, env, "new", "retained")
	runTestCLI(t, trunk, env, "remove", "retained")
	assertNotExist(t, filepath.Join(projectRoot, "worktrees", "retained"))
	if code := gitExit(t, env, trunk, "show-ref", "--verify", "--quiet", "refs/heads/retained"); code != 0 {
		t.Fatalf("retained branch show-ref exit = %d, want 0", code)
	}
}

func TestNewDetachedWorktree(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	remote := createRemote(t, testRoot, env)
	projectRoot := filepath.Join(testRoot, "workspace", "demo")
	runTestCLI(t, testRoot, env, "clone", remote, projectRoot)
	trunk := filepath.Join(projectRoot, "trunk")

	stdout, _ := runTestCLI(t, trunk, env, "new", "mr-123", "--detach", "--base", "HEAD")
	review := filepath.Join(projectRoot, "worktrees", "mr-123")
	assertContains(t, stdout, "Branch: (detached)")
	if branch := gitOutput(t, env, review, "branch", "--show-current"); branch != "" {
		t.Fatalf("detached worktree branch = %q, want empty", branch)
	}

	stdout, _ = runTestCLI(t, review, env, "list", "--porcelain")
	if !bytes.Contains([]byte(stdout), []byte("task mr-123\x00")) ||
		!bytes.Contains([]byte(stdout), []byte("detached true\x00")) {
		t.Fatalf("porcelain output missing detached review worktree: %q", stdout)
	}

	_, _, err := runTestCLIError(trunk, env, "new", "invalid", "--detach", "--branch", "review")
	assertErrorContains(t, err, "--detach cannot be combined with --branch")
	_, _, err = runTestCLIError(trunk, env, "new", "invalid", "--detach", "--branch-template", "review/%s")
	assertErrorContains(t, err, "--detach cannot be combined with --branch-template")

	stdout, _ = runTestCLI(t, review, env, "remove", "mr-123")
	assertContains(t, stdout, "Removed detached task worktree")
	assertNotExist(t, review)
}

func TestEveryCommandProvidesHelp(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	for _, command := range []string{"clone", "adopt", "new", "list", "path", "pull", "remove"} {
		t.Run(command, func(t *testing.T) {
			stdout, _ := runTestCLI(t, testRoot, env, command, "--help")
			assertContains(t, stdout, "Usage:\n  git wt "+command)
		})
	}
}

func TestCobraRejectsInvalidSyntax(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)

	_, _, err := runTestCLIError(testRoot, env, "list", "--unknown")
	assertErrorContains(t, err, "unknown flag: --unknown")

	_, _, err = runTestCLIError(testRoot, env, "pull", "extra")
	assertErrorContains(t, err, "unknown command \"extra\" for \"git wt pull\"")

	_, _, err = runTestCLIError(testRoot, env, "path", "task", "--notes")
	assertErrorContains(t, err, "--notes cannot be combined with a task")

	_, _, err = runTestCLIError(testRoot, env, "path", "--", "--notes")
	assertErrorContains(t, err, "current directory is not inside a managed project")
}

func TestPullUpdatesTrunkFromTaskWorktree(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	remote := createRemote(t, testRoot, env)
	workspace := filepath.Join(testRoot, "workspace")
	projectRoot := filepath.Join(workspace, "demo")
	runTestCLI(t, testRoot, env, "clone", remote, projectRoot)

	trunk := filepath.Join(projectRoot, "trunk")
	runTestCLI(t, trunk, env, "new", "TASK-123")
	task := filepath.Join(projectRoot, "worktrees", "TASK-123")

	seed := filepath.Join(testRoot, "seed")
	writeFile(t, filepath.Join(seed, "tracked.txt"), "upstream update\n")
	gitRun(t, env, seed, "add", "tracked.txt")
	gitRun(t, env, seed, "commit", "-m", "upstream update")
	gitRun(t, env, seed, "push", "origin", "main")

	writeFile(t, filepath.Join(trunk, "ignored.txt"), "ignored build output\n")
	stdout, _ := runTestCLI(t, task, env, "pull")
	assertContains(t, stdout, "Trunk: "+trunk)
	assertContains(t, stdout, "Branch: main")
	assertContains(t, stdout, "Updated trunk main:")
	if got := readFile(t, filepath.Join(trunk, "tracked.txt")); got != "upstream update\n" {
		t.Fatalf("trunk was not updated: %q", got)
	}
	if got := readFile(t, filepath.Join(task, "tracked.txt")); got != "initial\n" {
		t.Fatalf("task worktree changed during trunk pull: %q", got)
	}
	if got := readFile(t, filepath.Join(trunk, "ignored.txt")); got != "ignored build output\n" {
		t.Fatalf("ignored trunk file changed during pull: %q", got)
	}

	writeFile(t, filepath.Join(trunk, "tracked.txt"), "dirty trunk\n")
	_, _, err := runTestCLIError(task, env, "pull")
	assertErrorContains(t, err, "refusing to pull into dirty trunk")
	gitRun(t, env, trunk, "restore", "tracked.txt")

	gitRun(t, env, trunk, "switch", "-c", "feature-in-trunk")
	_, _, err = runTestCLIError(task, env, "pull")
	assertErrorContains(t, err, "expected default branch \"main\"")
}

func TestNewUsesConfiguredBranchTemplateAndRefusesCollisions(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	remote := createRemote(t, testRoot, env)
	workspace := filepath.Join(testRoot, "workspace")
	projectRoot := filepath.Join(workspace, "demo")
	runTestCLI(t, testRoot, env, "clone", remote, projectRoot)
	trunk := filepath.Join(projectRoot, "trunk")
	configured := envWith(env, "WT_BRANCH_TEMPLATE", "kellen/%s")
	runTestCLI(t, trunk, configured, "new", "TASK-9-name")
	taskPath := filepath.Join(projectRoot, "worktrees", "TASK-9-name")
	if got := gitOutput(t, env, taskPath, "branch", "--show-current"); got != "kellen/TASK-9-name" {
		t.Fatalf("branch = %q", got)
	}
	runTestCLI(t, trunk, env, "new", "--branch-template", "before/%s", "flag-before")
	if got := gitOutput(t, env, filepath.Join(projectRoot, "worktrees", "flag-before"), "branch", "--show-current"); got != "before/flag-before" {
		t.Fatalf("branch from flag before positional argument = %q", got)
	}
	runTestCLI(t, trunk, env, "new", "flag-after", "--branch-template", "after/%s")
	if got := gitOutput(t, env, filepath.Join(projectRoot, "worktrees", "flag-after"), "branch", "--show-current"); got != "after/flag-after" {
		t.Fatalf("branch from flag after positional argument = %q", got)
	}

	_, _, err := runTestCLIError(trunk, configured, "new", "TASK-9-name")
	assertErrorContains(t, err, "task destination already exists")
	_, _, err = runTestCLIError(trunk, configured, "new", "bad/task")
	assertErrorContains(t, err, "invalid task")
	_, _, err = runTestCLIError(trunk, configured, "new", "another", "--branch", "kellen/TASK-9-name")
	assertErrorContains(t, err, "already exists")
}

func TestRemoveRefusesUnmergedBranchAndActiveOperation(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	remote := createRemote(t, testRoot, env)
	workspace := filepath.Join(testRoot, "workspace")
	projectRoot := filepath.Join(workspace, "demo")
	runTestCLI(t, testRoot, env, "clone", remote, projectRoot)
	trunk := filepath.Join(projectRoot, "trunk")
	gitRun(t, env, trunk, "config", "user.name", "Git WT Tests")
	gitRun(t, env, trunk, "config", "user.email", "git-wt@example.invalid")
	runTestCLI(t, trunk, env, "new", "unmerged")
	taskPath := filepath.Join(projectRoot, "worktrees", "unmerged")
	writeFile(t, filepath.Join(taskPath, "feature.txt"), "feature\n")
	gitRun(t, env, taskPath, "add", "feature.txt")
	gitRun(t, env, taskPath, "commit", "-m", "feature")

	_, _, err := runTestCLIError(trunk, env, "remove", "unmerged", "--delete-branch", "--force")
	assertErrorContains(t, err, "refusing to delete unmerged branch")
	assertDirectory(t, taskPath)

	writeFile(t, filepath.Join(taskPath, ".git-operation-placeholder"), "placeholder")
	gitPath := gitOutput(t, env, taskPath, "rev-parse", "--path-format=absolute", "--git-path", "rebase-merge")
	if err := os.MkdirAll(gitPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(taskPath, ".git-operation-placeholder")); err != nil {
		t.Fatal(err)
	}
	_, _, err = runTestCLIError(trunk, env, "remove", "unmerged", "--force")
	assertErrorContains(t, err, "active Git operation marker rebase-merge")
	assertDirectory(t, taskPath)
}

func TestConfigurationPrecedence(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	globalConfig := filepath.Join(testRoot, "gitconfig")
	env := envWith(testEnvironment(t, testRoot), "GIT_CONFIG_GLOBAL", globalConfig)
	gitRunNoDir(t, env, "config", "--global", "wt.branchTemplate", "git/%s")

	manager, _, _ := newTestManager(testRoot, env)
	cfg, err := manager.loadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.branchTemplate != "git/%s" {
		t.Fatalf("git config not loaded: %+v", cfg)
	}

	env = envWith(env, "WT_BRANCH_TEMPLATE", "env/%s")
	manager, _, _ = newTestManager(testRoot, env)
	cfg, err = manager.loadConfig("flag/%s")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.branchTemplate != "flag/%s" {
		t.Fatalf("flag precedence failed: %+v", cfg)
	}
}

func runTestCLI(t *testing.T, cwd string, env []string, args ...string) (string, string) {
	t.Helper()
	stdout, stderr, err := runTestCLIError(cwd, env, args...)
	if err != nil {
		t.Fatalf("git wt %s failed: %v\nstdout: %s\nstderr: %s", strings.Join(args, " "), err, stdout, stderr)
	}
	return stdout, stderr
}

func runTestCLIError(cwd string, env []string, args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	command := exec.Command(gitWTTestBinary, args...)
	command.Dir = cwd
	command.Env = env
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err != nil && stderr.Len() != 0 {
		err = errors.New(strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), stderr.String(), err
}

func newTestManager(cwd string, env []string) (*Manager, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return NewManager(cwd, env, &stdout, &stderr), &stdout, &stderr
}

func newTestManagerWithRename(cwd string, env []string, rename func(string, string) error) (*Manager, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return newManagerWithRename(cwd, env, &stdout, &stderr, rename), &stdout, &stderr
}

func testEnvironment(t *testing.T, root string) []string {
	t.Helper()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	return envWith(os.Environ(),
		"HOME", home,
		"GIT_CONFIG_GLOBAL", os.DevNull,
		"GIT_CONFIG_NOSYSTEM", "1",
		"LC_ALL", "C",
	)
}

func envWith(env []string, values ...string) []string {
	result := append([]string(nil), env...)
	for i := 0; i < len(values); i += 2 {
		prefix := values[i] + "="
		filtered := result[:0]
		for _, entry := range result {
			if !strings.HasPrefix(entry, prefix) {
				filtered = append(filtered, entry)
			}
		}
		result = append(filtered, prefix+values[i+1])
	}
	return result
}

func createRemote(t *testing.T, root string, env []string) string {
	t.Helper()
	seed := filepath.Join(root, "seed")
	initRepository(t, seed, env)
	remote := filepath.Join(root, "remote.git")
	gitRunNoDir(t, env, "init", "--bare", "--initial-branch=main", remote)
	gitRun(t, env, seed, "remote", "add", "origin", remote)
	gitRun(t, env, seed, "push", "-u", "origin", "main")
	gitRunNoDir(t, env, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/main")
	return remote
}

func initRepository(t *testing.T, path string, env []string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRunNoDir(t, env, "init", "--initial-branch=main", path)
	gitRun(t, env, path, "config", "user.name", "Git WT Tests")
	gitRun(t, env, path, "config", "user.email", "git-wt@example.invalid")
	writeFile(t, filepath.Join(path, ".gitignore"), "ignored.txt\n")
	writeFile(t, filepath.Join(path, "tracked.txt"), "initial\n")
	gitRun(t, env, path, "add", ".gitignore", "tracked.txt")
	gitRun(t, env, path, "commit", "-m", "initial")
}

func gitRun(t *testing.T, env []string, dir string, args ...string) {
	t.Helper()
	gitRunNoDir(t, env, append([]string{"-C", dir}, args...)...)
}

func gitRunNoDir(t *testing.T, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = env
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func gitOutput(t *testing.T, env []string, dir string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(gitOutputRaw(t, env, dir, args...))
}

func gitOutputRaw(t *testing.T, env []string, dir string, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", cmdArgs...)
	cmd.Env = env
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(cmdArgs, " "), err)
	}
	return string(output)
}

func gitExit(t *testing.T, env []string, dir string, args ...string) int {
	t.Helper()
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", cmdArgs...)
	cmd.Env = env
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	t.Fatalf("git %s: %v", strings.Join(cmdArgs, " "), err)
	return -1
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func assertDirectory(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected directory %s: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", path)
	}
}

func assertNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s not to exist, got %v", path, err)
	}
}

func assertCodexTrunkGuard(t *testing.T, env []string, trunk string) {
	t.Helper()
	configPath := filepath.Join(trunk, filepath.FromSlash(codexConfigRelativePath))
	config := readFile(t, configPath)
	assertContains(t, config, "approval_policy = \"on-request\"")
	assertContains(t, config, "sandbox_mode = \"read-only\"")
	assertContains(t, config, "ask the user to choose")
	if code := gitExit(t, env, trunk, "check-ignore", "--quiet", "--", codexConfigRelativePath); code != 0 {
		t.Fatalf("Codex trunk config is not ignored; git check-ignore exit = %d", code)
	}
}

func assertContains(t *testing.T, value, substring string) {
	t.Helper()
	if !strings.Contains(value, substring) {
		t.Fatalf("%q does not contain %q", value, substring)
	}
}

func assertPorcelainDirty(t *testing.T, output, task string, want bool) {
	t.Helper()
	for _, record := range bytes.Split([]byte(output), []byte{0, 0}) {
		if bytes.Contains(record, []byte("task "+task+"\x00")) {
			if !bytes.Contains(record, []byte(fmt.Sprintf("dirty %t", want))) {
				t.Fatalf("task %q dirty state is not %t: %q", task, want, record)
			}
			return
		}
	}
	t.Fatalf("task %q not found in porcelain output: %q", task, output)
}

func assertErrorContains(t *testing.T, err error, substring string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), substring) {
		t.Fatalf("error %v does not contain %q", err, substring)
	}
}
