---
name: code-review
description: Interactive code review for Git diffs, commits, branch comparisons, or GitLab merge request links. Use when the user asks to walk through changes one meaningful block at a time. Do not use for ordinary non-interactive review requests.
---

# Code Review

Conduct an interactive, read-only review. Assess the complete diff first, then
present one meaningful review block at a time and wait for the user before
continuing. Do not modify files unless the user explicitly asks.

## GitLab merge request setup

For a GitLab merge request URL, use `git wt` instead of a separate review
launcher:

1. Parse and validate the URL. Extract the scheme, host, namespace, project,
   and numeric MR IID from
   `https://<host>/<namespace>/<project>/-/merge_requests/<iid>`.
2. Reuse the current managed project only when its `origin` identifies the
   same host and repository path. Otherwise use a dedicated project below
   `${CODE_REVIEW_ROOT:-$HOME/code-review/projects}`. Name it with a
   collision-resistant slug containing the host and full project path.
3. If that managed project does not exist, create it with:

   ```bash
   git wt clone git@<host>:<namespace>/<project>.git <project-path>
   ```

   Use HTTPS only when SSH is unavailable. Never replace or adopt an unrelated
   existing directory.
4. Fetch branches and the MR head into the managed project's trunk:

   ```bash
   git -C <project-path>/trunk fetch origin \
     '+refs/heads/*:refs/remotes/origin/*' \
     '+refs/merge-requests/<iid>/head:refs/review/mr/<iid>'
   ```

5. Use the MR target branch when the user supplied it. Otherwise use
   `refs/remotes/origin/HEAD` and tell the user that this assumes the MR targets
   the default branch.
6. Use task name `mr-<iid>`. If it has no registered worktree, create a
   detached one:

   ```bash
   git -C <project-path> wt new mr-<iid> \
     --detach --base refs/review/mr/<iid>
   ```

   If it already exists, inspect it before updating. A clean detached worktree
   may be refreshed with `git checkout --detach refs/review/mr/<iid>`. Do not
   reset, remove, or overwrite a dirty worktree; report its path and ask the
   user how to proceed.
7. Review from the MR worktree with:

   ```bash
   git diff origin/<target-branch>...HEAD
   ```

Report the project, worktree, target branch, and exact diff command before the
review starts. Network fetches and clone operations may require user approval
in restricted environments.

## Review workflow

1. Run `git status` and obtain the requested diff. If no target was given, ask
   whether to review unstaged changes, staged changes, a commit, or a branch
   comparison.
2. Read the whole diff before presenting any part. Flag unrelated subsystems or
   multiple major logical changes as a `[scope]` concern.
3. Group related hunks in the same file when splitting them would repeat
   context, such as setup plus usage or a behavior change plus its local test.
   Do not group unrelated edits merely because they share a file.
4. Skip import-only, formatting-only, comment-only, and generated hunks unless
   they affect behavior. Briefly state what was skipped or grouped.
5. Present one review block and wait for `next`, `skip`, `discuss`, `back`,
   `summary`, `quit`, or `apply`. Applying a suggestion still requires an
   explicit request to edit.

## Review block

Use this compact structure:

````markdown
### File: `path/to/file.ext` (block X of Y)

**Removed:**

```language
removed code
```

**Added:**

```language
added code
```

**What changed:** Brief explanation.

**Feedback:**
- [bug] Concrete issue or observation.

---

**[n]ext** | **[s]kip** | **[d]iscuss** | **[b]ack** | **[q]uit** | **[a]pply**
````

Omit an empty Removed or Added section. Show only the code needed to understand
the block. When there is no issue, say `Looks good.`

Prioritize correctness, operability, maintainability, and reviewability. Label
feedback as `[bug]`, `[perf]`, `[quality]`, `[test]`, `[scope]`, `[dry-run]`, or
`[question]`.

## Completion

When the user quits or all blocks are reviewed, summarize files and blocks
reviewed, critical issues, suggestions, residual test gaps, and whether the
change appears ready to commit or merge.
