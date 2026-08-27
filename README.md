# git-wt

`git wt` is a worktree-first project manager. It keeps a stable primary
checkout, task worktrees, and local notes together in one project directory:

```text
<parent>/<project>/
  AGENTS.md
  trunk/
  worktrees/<task>/
  notes/
```

`trunk/` is a normal Git checkout. Each task gets its own linked checkout under
`worktrees/`. The project-level `notes/` directory is outside Git; its contents
are never inspected or removed by `git wt`.

New projects also receive a local `AGENTS.md` that explains the layout and asks
coding agents to create a task worktree for new bodies of work. It lives outside
the repository and is not committed.

## Requirements

- Git with `git worktree list --porcelain -z` support
- Go 1.26 or later for installation

`git wt` is tested on Linux and expected to work on macOS and other Unix-like
systems supported by Go and Git.

## Install

Install the latest public version:

```bash
go install github.com/Kellel/git-wt/cmd/git-wt@latest
```

Alternatively, install from a local checkout:

```bash
make install
```

Ensure your Go binary directory is on `PATH`. Git discovers the `git-wt`
executable automatically and exposes it as `git wt`:

```bash
git wt --help
```

## Quick start

Create a managed project from a remote. Like `git clone`, the default project
name is derived from the remote:

```bash
cd ~/projects
git wt clone git@github.com:example/project.git
cd project
```

Create a worktree for a new body of work:

```bash
git wt new TASK-123-description
cd worktrees/TASK-123-description
```

`git wt` prints paths but cannot change the working directory of the calling
shell.

## Commands

### `git wt clone <remote> [directory]`

Clone a repository into a new managed project beneath the current directory.
An optional directory overrides the name derived from the remote.

### `git wt adopt [source] [--dry-run]`

Convert an existing standalone checkout into the managed layout in place:

```bash
cd ~/old-checkouts/project
git wt adopt --dry-run
git wt adopt
```

The checkout moves into `trunk/` while retaining its current branch, local
branches, configuration, hooks, staged changes, untracked files, and ignored
files. Adoption refuses repositories that already have linked worktrees or an
active Git operation. If the checkout is not on the remote's default branch,
switch `trunk/` to that branch before using `git wt pull`.

### `git wt new <task>`

Create a new branch and linked task worktree. By default, the branch name is the
task name and it starts from `origin`'s default branch.

```bash
git wt new TASK-123-description
git wt new TASK-123-description --base origin/develop
git wt new TASK-123-description --branch kellen/TASK-123-description
```

Use `--base HEAD` for an adopted repository without an `origin`. Run
`git wt new --help` for all options.

### `git wt list [--porcelain]`

List the project's registered worktrees, branches, commits, and dirty state.
Tracked and untracked changes count as dirty; ignored files do not.
`--porcelain` emits NUL-delimited machine-readable records.

### `git wt path [task] [--notes]`

Print a path for scripts or shell composition:

```bash
git wt path                 # trunk
git wt path TASK-123        # task worktree
git wt path --notes         # unmanaged notes
```

### `git wt pull`

Fast-forward the project's `trunk/` from `origin`'s default branch. The command
can be run from the project root, `trunk/`, or any registered task worktree. It
refuses a dirty trunk, an active Git operation, detached HEAD, a non-default
branch, or a non-fast-forward update. Ignored files do not block a pull.

### `git wt remove <task> [--delete-branch] [--force]`

Remove a clean task worktree. The branch is retained by default. The command
refuses tracked changes and untracked or ignored files because removing the
worktree would delete them. Use `-f` or `--force` to permanently discard those
files. With `--delete-branch`, branch deletion is allowed only when the branch
is merged into the remote default branch, even when force is enabled.

## Configuration

The task branch template defaults to `%s`, where `%s` is replaced by the task
name. Configure a prefix globally with Git:

```ini
[wt]
    branchTemplate = kellen/%s
```

The equivalent environment variable is `WT_BRANCH_TEMPLATE`. Override it for
one task with:

```bash
git wt new TASK-123 --branch-template 'review/%s'
```

Precedence is the `new` command flag, environment, global Git configuration,
then the default.

## Development

The tests use real temporary Git repositories and local remotes; they do not
require network access after dependencies are available.

```bash
make check
```

Git-porcelain and remote-name parsing have native Go fuzz targets behind the
`fuzz` build tag. Run bounded fuzz campaigns with:

```bash
make fuzz
```

## License

MIT. See [LICENSE](LICENSE).
