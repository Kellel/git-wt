# Worktree-First Git Workspace

Status: Implemented v1

## 1. Purpose

This document specifies a local workspace and toolset for a worktree-first Git
workflow. The system gives every project a predictable home, gives every task
an isolated checkout, and makes both projects and tasks quick to find from a
shell.

The canonical command is `git wt`. The installed executable is named `git-wt`,
which Git discovers on `PATH` and invokes as an external subcommand.

The words MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY describe requirements in
this document.

## 2. Goals

The system MUST:

- provide one deterministic directory layout for all managed projects;
- keep all checkouts for a project together;
- make a task worktree cheap enough to be the default way to begin work;
- migrate an existing standalone checkout without losing local repository state;
- preserve ordinary Git behavior inside every checkout;
- support direct navigation with normal shell paths as well as helper commands;
- prevent accidental loss of dirty or untracked work;
- work without a daemon, database, or shell-specific state; and
- allow useful operations to be exposed as Git aliases without depending on
  aliases for correctness.

The system SHOULD:

- derive defaults from the repository rather than requiring per-project
  configuration;
- keep task directory names stable when branches are renamed;
- produce script-friendly output; and
- remain usable when optional navigation helpers are not installed.

## 3. Non-goals

The first version will not:

- manage remote merge requests or pull requests;
- decide when a branch is ready to merge;
- automatically delete merged worktrees;
- replace `git worktree` for advanced operations;
- manage editor, terminal multiplexer, or development-environment sessions;
- synchronize worktrees between machines; or
- migrate repositories that already have linked worktrees in the first version.

## 4. Terminology

- **Workspace root**: the directory containing all managed namespaces.
- **Namespace**: a short, user-chosen grouping such as `cf` or `personal`.
- **Project**: one Git repository and all of its worktrees.
- **Project root**: the directory containing a project's primary checkout and
  task worktrees, plus local project notes.
- **Primary checkout**: the stable, long-lived checkout that owns the project's
  shared Git metadata. Its directory is named `trunk`; this name describes its
  role and does not require the checked-out branch to be named `trunk`.
- **Task**: a unit of work with a stable local identifier.
- **Task worktree**: an isolated checkout created for one task.

## 5. Directory contract

### 5.1 Layout

The proposed default workspace root is `$HOME/code`. It MUST be configurable.

Every managed project MUST have this layout:

```text
$WORKSPACE_ROOT/
  <namespace>/
    <project>/
      AGENTS.md
      trunk/
      worktrees/
        <task>/
      notes/
```

`$WORKSPACE_ROOT` is notation for the resolved `WT_ROOT` setting; it is not an
additional environment variable.

Examples:

```text
~/code/cf/addr-api/trunk
~/code/cf/addr-api/worktrees/ADDR-6584-bookworm-to-trixie
~/code/cf/addr-api/notes
~/code/cf/proto-schema/worktrees/ADDR-6472-protobuf-editions
~/code/personal/ansible-config/trunk
```

The `<namespace>/<project>` pair MUST uniquely identify a managed project.
Namespace names are local organizational labels; they do not have to reproduce
the remote forge or owner path.

The project root is a container, not a Git checkout. Running Git commands from
the project root is not guaranteed to work. `trunk` and every child of
`worktrees` MUST be normal Git working trees.

### 5.2 Local notes

Every project root MUST contain a `notes` directory for local, unmanaged
project material. It sits outside every Git checkout, so its contents cannot be
included in the project repository by an ordinary `git add`.

The tools create `notes` but MUST NOT interpret, index, synchronize, prune, or
delete anything inside it. Removing a task worktree MUST never affect notes.
Removing or migrating an entire project MUST refuse to proceed if doing so
would remove the notes directory or its contents unless a future command has an
explicit notes-preservation contract.

“Unmanaged” means the system does not provide versioning or backup. Users are
responsible for protecting notes that need to survive disk loss.

### 5.3 Agent guidance

Every newly cloned or adopted project MUST contain a generated `AGENTS.md` at
the project root. It is local project metadata outside every checkout and MUST
NOT be added to the repository.

The guidance MUST explain the `trunk`, `worktrees/<task>`, and `notes` layout;
direct agents to look inside a checkout for repository code and more specific
instructions; and tell agents to keep `trunk` stable. For a new body of
implementation work, agents SHOULD inspect existing worktrees, reuse a suitable
one when present, and otherwise default to `git wt new <task>`. Read-only
inspection does not require creating a worktree. The guidance MUST also tell
agents that temporary working files, scratchpads, plans, and handoff material
may be placed in the local, unmanaged `notes` directory and must not be
committed.

### 5.4 Primary checkout

`trunk` MUST be created as an ordinary clone. Git's shared repository metadata
therefore lives under `trunk/.git`, and task worktrees refer back to it through
Git's standard worktree mechanism.

`trunk` MUST remain at a stable path while task worktrees exist. Moving it is a
migration operation that requires `git worktree repair` and is outside the
normal workflow.

The branch checked out in `trunk` SHOULD track the remote's default branch. The
tool MUST NOT assume that branch is named `main`, `master`, or `trunk`.

### 5.5 Task identifiers

A task identifier is a local directory name, not a branch name. It MUST:

- contain only ASCII letters, digits, `.`, `_`, and `-`;
- begin with an ASCII letter or digit;
- not equal `trunk` or `worktrees`; and
- be unique within its project.

When a ticket exists, the identifier SHOULD start with the ticket key followed
by a short description, for example `ADDR-6584-bookworm-to-trixie`. Otherwise,
a short kebab-case description is sufficient.

The tool MAY normalize human input into a valid identifier, but MUST show the
resulting identifier and path. It MUST NOT silently reuse an existing task.

### 5.6 Branch names

Task and branch names are intentionally separate. Creating a task MUST accept
an explicit branch name. A configurable branch template MAY provide a default,
for example `kellen/%s`, where `%s` is the normalized task identifier. Without
configuration, the branch name defaults to the task identifier.

Renaming a branch MUST NOT rename its task directory. This keeps editor,
terminal, and navigation paths stable for the lifetime of the task.

## 6. Configuration

Configuration precedence, from highest to lowest, is:

1. command-line flags;
2. environment variables;
3. user configuration; and
4. built-in defaults.

The initial configuration surface is:

| Setting | Environment variable | Proposed default |
| --- | --- | --- |
| Workspace root | `WT_ROOT` | `$HOME/code` |
| Default namespace | `WT_NAMESPACE` | none |
| Branch template | `WT_BRANCH_TEMPLATE` | `%s` |

User configuration SHOULD use Git's global configuration rather than introduce
a separate configuration file:

```ini
[wt]
    root = ~/code
    namespace = cf
    branchTemplate = kellen/%s
```

These configuration keys are part of the v1 command contract.

## 7. Command interface

Commands MUST support `--help`. Human-readable output goes to standard output,
diagnostics go to standard error, and failures return a non-zero status.

### 7.1 `git wt clone`

```text
git wt clone <remote> [<namespace>/<project>]
```

Clone a repository into `<project-root>/trunk`, create the empty
`<project-root>/worktrees` and `<project-root>/notes` directories, and generate
the project-root `AGENTS.md`.

If the project key is omitted, the tool MAY derive the project name from the
remote, but MUST require either a configured namespace or an explicit
namespace. It MUST fail if the destination project root is not empty.

### 7.2 `git wt adopt`

```text
git wt adopt [--dry-run]
git wt adopt <source> [--dry-run]
```

Convert an existing standalone checkout into a managed project in place. The
checkout's current directory becomes the project root, the complete checkout
moves to `<project-root>/trunk`, and the command creates empty `worktrees` and
`notes` directories plus the generated `AGENTS.md` beside it. Adoption does not
use `WT_ROOT` to choose a new location and does not require a project key.

When `<source>` is omitted, the command MUST discover the top level of the Git
checkout containing the current directory and adopt that checkout. This form
MUST work from any directory inside the checkout. An explicit `<source>` adopts
that checkout in place instead.

Adoption MUST move the complete checkout rather than clone it. This preserves
local branches, tags, reflogs, repository configuration, hooks, ignored files,
untracked files, staged changes, and working-tree modifications.

Before making changes, the command MUST verify that:

- `<source>` is the top level of an ordinary non-bare Git checkout;
- its `.git` entry is a directory owned by that checkout, not a file pointing
  to another repository;
- `git worktree list --porcelain` reports only the source checkout;
- no Git operation such as a merge, rebase, cherry-pick, or bisect is active;
- the reserved `trunk`, `worktrees`, and `notes` paths do not already exist.

The command MUST display the resolved source, project root, and resulting trunk
path before moving anything. `--dry-run` performs every possible preflight
check and prints the proposed operation without changing the filesystem.

The command MUST use filesystem renames rather than copying the checkout. It
MUST first move the source to a uniquely named temporary sibling, create the
project root and its empty directories, and then move the temporary checkout
to `trunk`. If adoption fails after the first rename, it MUST remove only empty
directories that it created and restore the checkout at its original path. If
rollback itself fails, it MUST report the exact temporary checkout path. It
MUST never overwrite or merge with an existing reserved path.

Adoption MAY be invoked while the caller's shell is inside the source checkout,
but the command MUST warn that the shell's logical working directory may still
show the old path and print the new `trunk` path for navigation. It MUST NOT
leave a compatibility symlink at the old location.

The source repository does not need an `origin` remote. If its remote default
branch cannot be determined later, `git wt new` requires an explicit `--base`.

### 7.3 `git wt new`

```text
git wt new <task> [--branch <name>] [--base <ref>]
```

Create a task worktree in the current project.

The command MUST work when invoked from `trunk`, a task worktree, or the project
root. It MUST resolve and display all of the following before invoking Git:

- project root;
- task identifier;
- destination path;
- base revision; and
- new branch name.

The default base SHOULD be the remote-tracking default branch. Base selection
MUST NOT depend on whichever branch happens to be checked out in `trunk`.

The command always creates a new branch. Checking out an existing branch or a
detached commit is an advanced operation left to `git worktree add` in the
first version.

### 7.4 `git wt pull`

```text
git wt pull
```

Resolve the current project from its root, `trunk`, or any task worktree and
update the project's primary checkout. The command MUST operate on `trunk`
regardless of the caller's current task worktree.

Before contacting the remote, it MUST refuse a trunk with tracked, staged,
untracked, or ignored changes; an active Git operation; a detached HEAD; or a
checked-out branch other than `origin`'s default branch. It MUST pull the
explicit default branch from `origin` using fast-forward-only semantics and
MUST NOT merge or rebase a divergent trunk.

### 7.5 `git wt list`

```text
git wt list [<namespace>/<project>] [--porcelain]
```

List the project's primary checkout and task worktrees. The human format SHOULD
include task, branch or detached state, abbreviated commit, path, and dirty
state. `--porcelain` MUST provide a stable machine-readable format.

The implementation SHOULD use `git worktree list --porcelain -z` rather than
parse column-oriented human output.

### 7.6 `git wt path`

```text
git wt path [<task>] [--project <namespace>/<project>] [--notes]
```

Print exactly one absolute path and no other standard output. With no task, it
prints the path to `trunk`. Without `--project`, the current project is used.
`--notes` prints the project's notes path and is mutually exclusive with a task
argument. This command is the integration point for shell navigation and other
tools.

Interactive selection is not required in the first version.

### 7.7 `git wt remove`

```text
git wt remove <task> [--delete-branch]
```

Remove a task worktree through `git worktree remove`.

The command MUST refuse to remove a worktree with tracked modifications,
staged changes, untracked files, or an in-progress Git operation. It MUST NOT
offer a force flag in the first version.

Branch deletion is a separate, opt-in action. `--delete-branch` MUST refuse to
delete an unmerged branch and MUST never delete a remote branch.

### 7.7 Later commands

The following are useful but outside the first implementation slice:

- `git wt doctor`: validate layout and Git metadata;
- `git wt prune`: expose safe cleanup of stale worktree metadata;
- `git wt move`: move or rename a task and repair Git metadata; and
- `git wt each`: run a command across selected worktrees.

## 8. Project discovery

When run inside a Git working tree, the tool MUST discover the repository using
Git plumbing rather than string operations on the current path. It SHOULD use
`git rev-parse --path-format=absolute --git-common-dir` and validate the result
against the expected project layout.

When run from a project root, the tool MUST validate `trunk` as the primary
checkout before operating.

When a project key is supplied, the tool resolves it beneath the configured
workspace root. Resolved paths MUST remain inside that root after cleaning and
symbolic-link resolution.

The first version MUST NOT maintain a separate project index. A bounded scan of
the workspace root MAY support listing or completion later.

## 9. Navigation

An executable cannot change the working directory of its parent shell.
Therefore `git wt` MUST provide paths and shell initialization, while a shell
function performs `cd` in the caller. Bash integration is loaded with:

```bash
eval "$(git wt shell-init bash)"
```

The generated `cwt` function MUST support both navigation and create-and-enter:

```bash
cwt new ADDR-6584-bookworm-to-trixie
cwt ADDR-6584-bookworm-to-trixie
cwt --notes
```

Direct navigation remains straightforward and is part of the contract:

```bash
cd ~/code/cf/addr-api/trunk
cd ~/code/cf/addr-api/worktrees/ADDR-6584-bookworm-to-trixie
```

Shell completion is desirable after the command and naming contracts stabilize.

## 10. Git integration and aliases

The executable MUST be installed as `git-wt` in a directory on `PATH`. Git then
provides the canonical interface without any alias configuration:

```text
git wt new ADDR-6584-bookworm-to-trixie
git wt list
```

Git aliases MAY provide shorter entry points. They delegate to the `wt`
subcommand rather than contain workflow logic:

```ini
[alias]
    wn = wt new
    wp = wt pull
    wl = wt list
    wr = wt remove
```

Navigation SHOULD remain a shell function because neither a normal Git alias
nor a shell-command Git alias can change the caller's directory.

`git wt shell-init bash` MUST print the Bash integration without reading project
configuration. The integration MUST make `cwt new <task> [options]` run
`git wt new`, resolve the new task with `git wt path`, and change the caller's
directory only after both commands succeed.

Aliases MUST NOT contain independent workflow logic. This keeps behavior and
safety checks identical whether a user runs `git wt new` or `git wn`.

The final shorthand alias names will be evaluated after the canonical commands
are in use. The tool MUST remain fully usable without installing aliases.

## 11. Safety invariants

All mutating commands MUST:

- validate the project and destination before invoking Git;
- reject destinations outside the configured workspace root;
- pass user-controlled values as arguments, never as interpolated shell code;
- fail rather than overwrite an existing path;
- preserve dirty, staged, and untracked content;
- avoid implicit fetches unless the command documents and displays them;
- leave the primary checkout's current branch and files unchanged; and
- report partial state clearly if Git succeeds but later bookkeeping fails.

Removal commands MUST prefer Git's worktree operations over direct filesystem
deletion. No command may run recursive deletion against a user-derived path.

Read-only commands SHOULD still work when the repository is offline.

## 12. Initial implementation slice

The first usable increment consists of:

1. project discovery and layout validation;
2. `git wt adopt --dry-run` and `git wt adopt`;
3. `git wt new` with explicit task, branch, and base inputs;
4. `git wt list`;
5. `git wt path` plus the `cwt` shell function; and
6. focused tests using temporary repositories and local remotes.

`git wt clone` and `git wt remove` follow once creation and discovery have
proven the directory contract. Shorthand Git aliases should be evaluated only
after the canonical workflow is comfortable in daily use.

## 13. Acceptance scenarios

The first implementation is acceptable when it can demonstrate all of these:

1. From an `addr-api` task worktree, create a second task based on the remote's
   default branch without changing the first task or `trunk`.
2. From the project root, list `trunk` and every task with correct paths and
   branches.
3. Resolve `git wt path --project cf/addr-api ADDR-6584-bookworm-to-trixie` to
   exactly one absolute path suitable for `cd "$(git wt path ...)"`.
4. Refuse to create a task whose normalized identifier collides with an
   existing directory or registered worktree.
5. Refuse to remove a task containing only untracked files as well as one with
   tracked modifications.
6. Continue to list and navigate existing worktrees without network access.
7. Adopt a dirty standalone checkout and verify that its branches, repository
   configuration, staged changes, untracked files, and ignored files are
   unchanged at the new `trunk` path.
8. Run argument-free adoption from a nested directory of a standalone checkout
   and verify that the checkout directory becomes the project root with the
   repository under `trunk`.
9. Refuse to adopt a linked worktree or a checkout containing an existing
   reserved layout path without changing the source.
10. Create an unmanaged `notes` directory during clone or adoption, resolve it
    through `git wt path --notes`, and leave its contents untouched by every
    task-lifecycle command.
11. Generate a project-root `AGENTS.md` during clone and adoption that explains
    the layout and defaults new implementation work to a task worktree.
12. From a task worktree, fast-forward a clean default-branch `trunk` without
    changing the task checkout, and refuse dirty or non-default-branch trunks.

## 14. V1 implementation decisions

The v1 implementation uses:

1. `$HOME/code` as the configurable default workspace root;
2. `trunk` as the stable primary-checkout role name;
3. user-chosen namespaces such as `cf` and `personal`;
4. `%s` as the default branch template, with `--branch` available per task; and
5. a single Go binary named `git-wt` that delegates repository operations to
   the installed Git CLI; and
6. no third-party Go packages, with Makefile builds routed through Cloudflare's
   Athens module proxy as required for binary-producing Go modules.
