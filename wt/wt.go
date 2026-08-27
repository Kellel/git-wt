// Package wt manages worktree-first Git project layouts.
package wt

// Worktree provides the operations supported by a worktree manager.
type Worktree interface {
	Clone(remote, destination string) error
	Adopt(source string, dryRun bool) error
	NewWorktree(task string, options NewOptions) error
	List(porcelain bool) error
	Path(task string, notes bool) error
	Pull() error
	Remove(task string, deleteBranch bool) error
}

// NewOptions controls the branch created for a new task worktree.
type NewOptions struct {
	// Branch is the new branch name. The configured branch template is used when empty.
	Branch string
	// Base is the branch, tag, or commit from which the new branch starts.
	// The origin default branch is used when empty.
	Base string
	// BranchTemplate overrides the configured branch template and must contain %s.
	BranchTemplate string
}

var _ Worktree = (*Manager)(nil)
