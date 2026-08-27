# Managed Git Worktree Project

This directory is managed by `git wt`. It is a project container, not a Git checkout.

## Layout

- `trunk/` is the stable primary checkout and owns the shared Git metadata.
- `worktrees/<task>/` contains isolated task checkouts.
- `notes/` contains local, unmanaged project notes. Do not commit its contents.

Repository code and repository-specific instructions live in `trunk/` and the task worktrees, not in this directory.

## Agent workflow

- Run `git wt list` before starting a new body of implementation work.
- Use `git wt pull` from anywhere in the project to fast-forward the clean `trunk/` default branch.
- If an appropriate task worktree already exists, continue there.
- Otherwise, default to creating one with `git wt new <task>`, then work from the path returned by `git wt path <task>`.
- Keep `trunk/` stable; do not make task-specific changes there unless the user explicitly requests it.
- Read-only inspection can use `trunk/` without creating a worktree.
- Temporary agent working files, scratchpads, plans, and handoff notes may be placed in `notes/`. They are local and unmanaged; do not commit them.
- Follow any more specific `AGENTS.md` files inside the selected checkout.
