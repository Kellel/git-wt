# git-wt

This repository contains `git wt`, a worktree-first project manager, and a
Codex skill for interactive code review. `git wt` keeps a stable primary
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

`clone` and `adopt` also install an ignored `trunk/.codex/config.toml`. It makes
the stable trunk read-only to Codex and requires confirmation before direct
trunk changes; task worktrees remain writable. An existing Codex config is left
unchanged and produces a warning.

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

The local Make target installs both the `git-wt` binary and the bundled
`code-review` skill. Set `CODEX_SKILLS_DIR` to override the default
`~/.codex/skills` destination. To install only one component, use
`make install-cli` or `make install-skill`.

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
git wt new mr-123 --detach --base refs/review/mr/123
```

Use `--base HEAD` for an adopted repository without an `origin`. Run
`git wt new --help` for all options. `--detach` creates a worktree without a
local branch, which is useful for temporary review checkouts.

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

## Code-review skill

The bundled [`code-review`](skills/code-review/SKILL.md) Codex skill performs
interactive reviews and uses `git wt` to prepare detached worktrees for GitLab
merge requests. It replaces the former standalone Python launcher; the skill
contains the review workflow while Git and `git wt` perform the checkout work.

## License

MIT. See [LICENSE](LICENSE).
