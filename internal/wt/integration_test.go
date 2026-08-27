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

func TestCloneNewListPathAndRemove(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	remote := createRemote(t, testRoot, env)
	workspace := filepath.Join(testRoot, "workspace")

	stdout, _ := runTestApp(t, testRoot, env,
		"--root", workspace,
		"clone", remote, "cf/demo",
	)
	projectRoot := filepath.Join(workspace, "cf", "demo")
	trunk := filepath.Join(projectRoot, "trunk")
	worktrees := filepath.Join(projectRoot, "worktrees")
	notes := filepath.Join(projectRoot, "notes")
	assertContains(t, stdout, "Created project "+projectRoot)
	assertDirectory(t, trunk)
	assertDirectory(t, worktrees)
	assertDirectory(t, notes)

	stdout, _ = runTestApp(t, testRoot, env,
		"--root", workspace,
		"path", "--project", "cf/demo",
	)
	if got := strings.TrimSpace(stdout); got != trunk {
		t.Fatalf("trunk path = %q, want %q", got, trunk)
	}
	stdout, _ = runTestApp(t, testRoot, env,
		"--root", workspace,
		"path", "--notes", "--project", "cf/demo",
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
	stdout, _ = runTestApp(t, nested, envWith(env, "WT_ROOT", workspace),
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

	stdout, _ = runTestApp(t, taskPath, envWith(env, "WT_ROOT", workspace), "path", "TASK-123-example")
	if got := strings.TrimSpace(stdout); got != taskPath {
		t.Fatalf("task path = %q, want %q", got, taskPath)
	}

	// Listing does not contact the remote.
	offlineRemote := remote + ".offline"
	if err := os.Rename(remote, offlineRemote); err != nil {
		t.Fatal(err)
	}
	stdout, _ = runTestApp(t, projectRoot, envWith(env, "WT_ROOT", workspace), "list")
	assertContains(t, stdout, "TASK-123-example")
	assertContains(t, stdout, "trunk")
	assertContains(t, stdout, "no")

	writeFile(t, filepath.Join(taskPath, "untracked.txt"), "do not lose\n")
	stdout, _ = runTestApp(t, projectRoot, envWith(env, "WT_ROOT", workspace), "list")
	assertContains(t, stdout, "TASK-123-example")
	assertContains(t, stdout, "yes")

	stdout, _ = runTestApp(t, projectRoot, envWith(env, "WT_ROOT", workspace), "list", "--porcelain")
	if !bytes.Contains([]byte(stdout), []byte("task TASK-123-example\x00")) ||
		!bytes.Contains([]byte(stdout), []byte("dirty true\x00")) {
		t.Fatalf("porcelain output missing task or dirty state: %q", stdout)
	}

	_, _, err := runTestAppError(projectRoot, envWith(env, "WT_ROOT", workspace), "remove", "TASK-123-example")
	assertErrorContains(t, err, "refusing to remove dirty task worktree")
	assertDirectory(t, taskPath)
	if got := readFile(t, filepath.Join(taskPath, "untracked.txt")); got != "do not lose\n" {
		t.Fatalf("untracked file changed: %q", got)
	}

	if err := os.Remove(filepath.Join(taskPath, "untracked.txt")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(taskPath, "tracked.txt"), "modified\n")
	_, _, err = runTestAppError(projectRoot, envWith(env, "WT_ROOT", workspace), "remove", "TASK-123-example")
	assertErrorContains(t, err, "refusing to remove dirty task worktree")
	assertDirectory(t, taskPath)
	gitRun(t, env, taskPath, "restore", "tracked.txt")
	writeFile(t, filepath.Join(taskPath, "ignored.txt"), "ignored build output\n")
	_, _, err = runTestAppError(projectRoot, envWith(env, "WT_ROOT", workspace), "remove", "TASK-123-example")
	assertErrorContains(t, err, "refusing to remove dirty task worktree")
	if got := readFile(t, filepath.Join(taskPath, "ignored.txt")); got != "ignored build output\n" {
		t.Fatalf("ignored file changed: %q", got)
	}
	if err := os.Remove(filepath.Join(taskPath, "ignored.txt")); err != nil {
		t.Fatal(err)
	}
	stdout, _ = runTestApp(t, projectRoot, envWith(env, "WT_ROOT", workspace),
		"remove", "TASK-123-example", "--delete-branch",
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

	statusBefore := gitOutputRaw(t, env, source, "status", "--porcelain=v1", "-z", "--ignored")
	refsBefore := gitOutputRaw(t, env, source, "for-each-ref", "--format=%(refname)%00%(objectname)")
	reflogBefore := gitOutputRaw(t, env, source, "reflog", "show", "--all", "--format=%H%x00%gs")
	hookBefore := readFile(t, filepath.Join(source, ".git", "hooks", "pre-commit"))

	stdout, _ := runTestApp(t, nested, env,
		"adopt", "--dry-run",
	)
	assertContains(t, stdout, "Source: "+source)
	assertContains(t, stdout, "Dry run: adoption checks passed")
	assertDirectory(t, source)
	assertNotExist(t, filepath.Join(source, "trunk"))
	assertNotExist(t, filepath.Join(source, "worktrees"))
	assertNotExist(t, filepath.Join(source, "notes"))

	stdout, stderr := runTestApp(t, nested, env,
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

	if got := gitOutputRaw(t, env, trunk, "status", "--porcelain=v1", "-z", "--ignored"); got != statusBefore {
		t.Fatalf("status changed during adoption\nbefore: %q\nafter:  %q", statusBefore, got)
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
	_, _, err := runTestAppError(trunk, env, "new", "local-task")
	assertErrorContains(t, err, "pass --base explicitly")
	runTestApp(t, trunk, env, "new", "local-task", "--base", "HEAD")
	assertDirectory(t, filepath.Join(source, "worktrees", "local-task"))
}

func TestAdoptRefusesUnsupportedOrUnsafeSources(t *testing.T) {
	t.Parallel()

	t.Run("implicit source outside checkout", func(t *testing.T) {
		testRoot := t.TempDir()
		env := testEnvironment(t, testRoot)

		_, _, err := runTestAppError(testRoot, env,
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

		_, _, err := runTestAppError(testRoot, env,
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

		_, _, err := runTestAppError(testRoot, env,
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

		_, _, err := runTestAppError(testRoot, env,
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

		_, _, err := runTestAppError(testRoot, env,
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
		app, _, _ := newTestApp(testRoot, env)
		app.rename = func(string, string) error { return errors.New("injected rename failure") }
		err := app.Run([]string{"adopt", source})
		assertErrorContains(t, err, "source left unchanged")
		assertDirectory(t, source)
	})

	t.Run("second rename failure restores source", func(t *testing.T) {
		testRoot := t.TempDir()
		env := testEnvironment(t, testRoot)
		source := filepath.Join(testRoot, "source")
		initRepository(t, source, env)
		app, _, _ := newTestApp(testRoot, env)
		calls := 0
		app.rename = func(oldPath, newPath string) error {
			calls++
			if calls == 2 {
				return errors.New("injected rename failure")
			}
			return os.Rename(oldPath, newPath)
		}
		err := app.Run([]string{"adopt", source})
		assertErrorContains(t, err, "source restored")
		assertDirectory(t, source)
		assertDirectory(t, filepath.Join(source, ".git"))
		assertNotExist(t, filepath.Join(source, "trunk"))
		assertNotExist(t, filepath.Join(source, "AGENTS.md"))
		matches, globErr := filepath.Glob(filepath.Join(testRoot, ".source.git-wt-adopt-*"))
		if globErr != nil {
			t.Fatal(globErr)
		}
		if len(matches) != 0 {
			t.Fatalf("temporary adoption paths remain: %v", matches)
		}
	})
}

func TestCloneDerivesProjectAndReportsPartialFailure(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	remote := createRemote(t, testRoot, env)
	workspace := filepath.Join(testRoot, "workspace")
	configured := envWith(env, "WT_ROOT", workspace, "WT_NAMESPACE", "personal")
	runTestApp(t, testRoot, configured, "clone", remote)
	projectRoot := filepath.Join(workspace, "personal", "remote")
	assertDirectory(t, filepath.Join(projectRoot, "trunk"))
	assertContains(t, readFile(t, filepath.Join(projectRoot, "AGENTS.md")), "This directory is managed by `git wt`")

	missingRemote := filepath.Join(testRoot, "missing.git")
	_, _, err := runTestAppError(testRoot, configured, "clone", missingRemote, "personal/broken")
	assertErrorContains(t, err, "partial project remains")
	assertDirectory(t, filepath.Join(workspace, "personal", "broken", "worktrees"))
	assertDirectory(t, filepath.Join(workspace, "personal", "broken", "notes"))
}

func TestCloneRejectsNamespaceSymlinkEscapingWorkspace(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	remote := createRemote(t, testRoot, env)
	workspace := filepath.Join(testRoot, "workspace")
	outside := filepath.Join(testRoot, "outside")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "cf")); err != nil {
		t.Fatal(err)
	}

	_, _, err := runTestAppError(testRoot, envWith(env, "WT_ROOT", workspace),
		"clone", remote, "cf/demo",
	)
	assertErrorContains(t, err, "resolves outside workspace root")
	assertNotExist(t, filepath.Join(outside, "demo"))
}

func TestRemoveWithoutBranchDeletionRetainsBranch(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	remote := createRemote(t, testRoot, env)
	workspace := filepath.Join(testRoot, "workspace")
	configured := envWith(env, "WT_ROOT", workspace)
	runTestApp(t, testRoot, configured, "clone", remote, "cf/demo")
	trunk := filepath.Join(workspace, "cf", "demo", "trunk")
	runTestApp(t, trunk, configured, "new", "retained")
	runTestApp(t, trunk, configured, "remove", "retained")
	assertNotExist(t, filepath.Join(workspace, "cf", "demo", "worktrees", "retained"))
	if code := gitExit(t, env, trunk, "show-ref", "--verify", "--quiet", "refs/heads/retained"); code != 0 {
		t.Fatalf("retained branch show-ref exit = %d, want 0", code)
	}
}

func TestEveryCommandProvidesHelp(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	for _, command := range []string{"clone", "adopt", "new", "list", "path", "pull", "remove", "shell-init"} {
		t.Run(command, func(t *testing.T) {
			stdout, _ := runTestApp(t, testRoot, env, command, "--help")
			assertContains(t, stdout, "usage: git wt "+command)
		})
	}
}

func TestPullUpdatesTrunkFromTaskWorktree(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	remote := createRemote(t, testRoot, env)
	workspace := filepath.Join(testRoot, "workspace")
	configured := envWith(env, "WT_ROOT", workspace)
	runTestApp(t, testRoot, configured, "clone", remote, "cf/demo")

	trunk := filepath.Join(workspace, "cf", "demo", "trunk")
	runTestApp(t, trunk, configured, "new", "TASK-123")
	task := filepath.Join(workspace, "cf", "demo", "worktrees", "TASK-123")

	seed := filepath.Join(testRoot, "seed")
	writeFile(t, filepath.Join(seed, "tracked.txt"), "upstream update\n")
	gitRun(t, env, seed, "add", "tracked.txt")
	gitRun(t, env, seed, "commit", "-m", "upstream update")
	gitRun(t, env, seed, "push", "origin", "main")

	stdout, _ := runTestApp(t, task, configured, "pull")
	assertContains(t, stdout, "Trunk: "+trunk)
	assertContains(t, stdout, "Branch: main")
	assertContains(t, stdout, "Updated trunk main:")
	if got := readFile(t, filepath.Join(trunk, "tracked.txt")); got != "upstream update\n" {
		t.Fatalf("trunk was not updated: %q", got)
	}
	if got := readFile(t, filepath.Join(task, "tracked.txt")); got != "initial\n" {
		t.Fatalf("task worktree changed during trunk pull: %q", got)
	}

	writeFile(t, filepath.Join(trunk, "tracked.txt"), "dirty trunk\n")
	_, _, err := runTestAppError(task, configured, "pull")
	assertErrorContains(t, err, "refusing to pull into dirty trunk")
	gitRun(t, env, trunk, "restore", "tracked.txt")

	gitRun(t, env, trunk, "switch", "-c", "feature-in-trunk")
	_, _, err = runTestAppError(task, configured, "pull")
	assertErrorContains(t, err, "expected default branch \"main\"")
}

func TestBashShellInitCreatesAndEntersWorktree(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	destination := filepath.Join(testRoot, "worktree")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	stdout, _ := runTestApp(t, testRoot, testEnvironment(t, testRoot), "shell-init", "bash")
	assertContains(t, stdout, "cwt()")

	script := stdout + `
git() {
    if [ "$1" = "wt" ] && [ "$2" = "new" ] && [ "$3" = "TASK-123" ]; then
        return 0
    fi
    if [ "$1" = "wt" ] && [ "$2" = "path" ] && [ "$3" = "TASK-123" ]; then
        printf '%s\n' "$CWT_TEST_DESTINATION"
        return 0
    fi
    return 1
}
cwt new TASK-123 --base HEAD
pwd -P
`
	command := exec.Command("bash", "--noprofile", "--norc")
	command.Dir = testRoot
	command.Env = envWith(os.Environ(), "CWT_TEST_DESTINATION", destination)
	command.Stdin = strings.NewReader(script)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run generated bash integration: %v\n%s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != destination {
		t.Fatalf("shell helper ended in %q, want %q", got, destination)
	}
}

func TestNewUsesConfiguredBranchTemplateAndRefusesCollisions(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	remote := createRemote(t, testRoot, env)
	workspace := filepath.Join(testRoot, "workspace")
	runTestApp(t, testRoot, env,
		"--root", workspace, "clone", remote, "cf/demo",
	)
	trunk := filepath.Join(workspace, "cf", "demo", "trunk")
	configured := envWith(env, "WT_ROOT", workspace, "WT_BRANCH_TEMPLATE", "kellen/%s")
	runTestApp(t, trunk, configured, "new", "TASK-9-name")
	taskPath := filepath.Join(workspace, "cf", "demo", "worktrees", "TASK-9-name")
	if got := gitOutput(t, env, taskPath, "branch", "--show-current"); got != "kellen/TASK-9-name" {
		t.Fatalf("branch = %q", got)
	}

	_, _, err := runTestAppError(trunk, configured, "new", "TASK-9-name")
	assertErrorContains(t, err, "task destination already exists")
	_, _, err = runTestAppError(trunk, configured, "new", "bad/task")
	assertErrorContains(t, err, "invalid task")
	_, _, err = runTestAppError(trunk, configured, "new", "another", "--branch", "kellen/TASK-9-name")
	assertErrorContains(t, err, "already exists")
}

func TestRemoveRefusesUnmergedBranchAndActiveOperation(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	env := testEnvironment(t, testRoot)
	remote := createRemote(t, testRoot, env)
	workspace := filepath.Join(testRoot, "workspace")
	runTestApp(t, testRoot, env, "--root", workspace, "clone", remote, "cf/demo")
	trunk := filepath.Join(workspace, "cf", "demo", "trunk")
	gitRun(t, env, trunk, "config", "user.name", "Git WT Tests")
	gitRun(t, env, trunk, "config", "user.email", "git-wt@example.invalid")
	configured := envWith(env, "WT_ROOT", workspace)
	runTestApp(t, trunk, configured, "new", "unmerged")
	taskPath := filepath.Join(workspace, "cf", "demo", "worktrees", "unmerged")
	writeFile(t, filepath.Join(taskPath, "feature.txt"), "feature\n")
	gitRun(t, env, taskPath, "add", "feature.txt")
	gitRun(t, env, taskPath, "commit", "-m", "feature")

	_, _, err := runTestAppError(trunk, configured, "remove", "unmerged", "--delete-branch")
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
	_, _, err = runTestAppError(trunk, configured, "remove", "unmerged")
	assertErrorContains(t, err, "active Git operation marker rebase-merge")
	assertDirectory(t, taskPath)
}

func TestConfigurationPrecedence(t *testing.T) {
	t.Parallel()

	testRoot := t.TempDir()
	globalConfig := filepath.Join(testRoot, "gitconfig")
	env := envWith(testEnvironment(t, testRoot), "GIT_CONFIG_GLOBAL", globalConfig)
	gitRunNoDir(t, env, "config", "--global", "wt.root", filepath.Join(testRoot, "from-git"))
	gitRunNoDir(t, env, "config", "--global", "wt.namespace", "from-git")
	gitRunNoDir(t, env, "config", "--global", "wt.branchTemplate", "git/%s")

	app, _, _ := newTestApp(testRoot, env)
	cfg, err := app.loadConfig(configOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.namespace != "from-git" || cfg.branchTemplate != "git/%s" {
		t.Fatalf("git config not loaded: %+v", cfg)
	}

	env = envWith(env,
		"WT_ROOT", filepath.Join(testRoot, "from-env"),
		"WT_NAMESPACE", "from-env",
		"WT_BRANCH_TEMPLATE", "env/%s",
	)
	app, _, _ = newTestApp(testRoot, env)
	cfg, err = app.loadConfig(configOverrides{
		root:           filepath.Join(testRoot, "from-flag"),
		namespace:      "from-flag",
		branchTemplate: "flag/%s",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.root != filepath.Join(testRoot, "from-flag") || cfg.namespace != "from-flag" || cfg.branchTemplate != "flag/%s" {
		t.Fatalf("flag precedence failed: %+v", cfg)
	}
}

func runTestApp(t *testing.T, cwd string, env []string, args ...string) (string, string) {
	t.Helper()
	stdout, stderr, err := runTestAppError(cwd, env, args...)
	if err != nil {
		t.Fatalf("git wt %s failed: %v\nstdout: %s\nstderr: %s", strings.Join(args, " "), err, stdout, stderr)
	}
	return stdout, stderr
}

func runTestAppError(cwd string, env []string, args ...string) (string, string, error) {
	app, stdout, stderr := newTestApp(cwd, env)
	err := app.Run(args)
	return stdout.String(), stderr.String(), err
}

func newTestApp(cwd string, env []string) (*App, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return New(cwd, env, &stdout, &stderr), &stdout, &stderr
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

func assertContains(t *testing.T, value, substring string) {
	t.Helper()
	if !strings.Contains(value, substring) {
		t.Fatalf("%q does not contain %q", value, substring)
	}
}

func assertErrorContains(t *testing.T, err error, substring string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), substring) {
		t.Fatalf("error %v does not contain %q", err, substring)
	}
}

func ExampleApp_Run() {
	fmt.Println("git wt path --project cf/addr-api")
	// Output: git wt path --project cf/addr-api
}
