# git-wt

`git wt` is a worktree-first project manager. It keeps a stable primary
checkout, task worktrees, and local notes together under one project directory:

```text
~/code/<namespace>/<project>/
  AGENTS.md
  trunk/
  worktrees/<task>/
  notes/
```

New projects receive a local `AGENTS.md` explaining the layout and directing
coding agents to use a task worktree for new implementation work. Because it
lives at the project root, it is outside the repository and is not committed.
Agents may keep temporary working files, scratchpads, plans, and handoff notes
in the local, unmanaged `notes/` directory.

## Install

Build and install the external Git subcommand:

```bash
make install
```

Ensure the Go binary directory is on `PATH`. Git automatically discovers the
`git-wt` executable and exposes it as `git wt`:

```bash
git wt -h
```

V1 targets Linux workstations. Git must support
`git worktree list --porcelain -z`.

## Commands

Create a managed project from a remote:

```bash
git wt clone git@example.com:team/project.git work/project
```

Move an existing standalone checkout into the layout without losing local
branches, configuration, hooks, staged changes, or untracked files:

```bash
cd ~/old-checkouts/project
git wt adopt --dry-run
git wt adopt
```

`adopt` discovers the checkout containing the current directory and turns that
directory into the project root. The checkout moves into `trunk`; it is not
relocated elsewhere under `WT_ROOT`. An explicit source path is also supported:

```bash
git wt adopt ~/old-checkouts/project
```

Create, inspect, navigate to, and remove task worktrees:

```bash
git wt new TASK-123-description
git wt pull
git wt list
git wt path TASK-123-description
git wt path --notes
git wt remove TASK-123-description
git wt remove TASK-123-description --delete-branch
```

`git wt pull` can be run from the project root, `trunk`, or any task worktree.
It refuses a dirty or non-default-branch trunk and runs a fast-forward-only pull
of `origin`'s default branch in `trunk`.

`remove` refuses tracked, staged, untracked, or ignored files and active Git
operations. Branch deletion is opt-in and refuses branches that are not merged
into the remote default branch.

## Configuration

Configuration precedence is command-line flags, environment, global Git
configuration, then defaults.

```ini
[wt]
    root = ~/code
    namespace = work
    branchTemplate = kellen/%s
```

The corresponding environment variables are `WT_ROOT`, `WT_NAMESPACE`, and
`WT_BRANCH_TEMPLATE`. Global flags are `--root`, `--namespace`, and
`--branch-template`.

## Shell navigation

A child process cannot change its parent shell directory. Load the Bash helper
from `.bashrc` so creation and navigation can perform `cd` in the current shell:

```bash
eval "$(git wt shell-init bash)"
```

Then create and enter a task worktree in one command:

```bash
cwt new TASK-123-description
cwt TASK-123-description
cwt --notes
```

Optional shorthand Git aliases contain no workflow logic:

```ini
[alias]
    wn = wt new
    wp = wt pull
    wl = wt list
    wr = wt remove
```

## Development

The tests use real temporary Git repositories and local remotes; they do not
require network access.

```bash
go test ./...
go test -race ./...
go vet ./...
make lint
```

The CLI and Git-porcelain parsing boundaries have native Go fuzz targets gated
behind the `fuzz` build tag. Run bounded fuzz campaigns with:

```bash
make fuzz
```

## License

MIT. See [LICENSE](LICENSE).
