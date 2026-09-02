package main

import (
	"errors"
	"io"

	"github.com/Kellel/git-wt/wt"
	"github.com/spf13/cobra"
)

func newRootCommand(manager wt.Worktree, stdout, stderr io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:           "git-wt",
		Short:         "Worktree-first project management",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := command.Help(); err != nil {
				return err
			}
			return errors.New("a command is required")
		},
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		Annotations: map[string]string{
			cobra.CommandDisplayNameAnnotation: "git wt",
		},
	}
	command.SetOut(stdout)
	command.SetErr(stderr)
	command.AddCommand(
		newCloneCommand(manager),
		newAdoptCommand(manager),
		newWorktreeCommand(manager),
		newListCommand(manager),
		newPathCommand(manager),
		newPullCommand(manager),
		newRemoveCommand(manager),
	)
	return command
}

func newCloneCommand(manager wt.Worktree) *cobra.Command {
	return &cobra.Command{
		Use:   "clone <remote> [directory]",
		Short: "Clone a repository into a managed project",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(_ *cobra.Command, args []string) error {
			destination := ""
			if len(args) == 2 {
				destination = args[1]
			}
			return manager.Clone(args[0], destination)
		},
	}
}

func newAdoptCommand(manager wt.Worktree) *cobra.Command {
	var dryRun bool
	command := &cobra.Command{
		Use:   "adopt [source]",
		Short: "Convert a standalone checkout into a managed project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			source := ""
			if len(args) == 1 {
				source = args[0]
			}
			return manager.Adopt(source, dryRun)
		},
	}
	command.Flags().BoolVar(&dryRun, "dry-run", false, "validate without changing files")
	return command
}

func newWorktreeCommand(manager wt.Worktree) *cobra.Command {
	var options wt.NewOptions
	command := &cobra.Command{
		Use:   "new <task>",
		Short: "Create a task worktree",
		Long: `Create a task worktree and a new branch at the selected base revision.

The base defaults to origin's default branch. Use --base to start from a
different branch, tag, or commit. For an adopted repository without an origin,
use --base HEAD to start from the current trunk commit.

Use --detach for a temporary inspection or review worktree that does not need a
local branch.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return manager.NewWorktree(args[0], options)
		},
	}
	command.Flags().StringVar(&options.Branch, "branch", "", "branch name")
	command.Flags().StringVar(&options.Base, "base", "", "branch, tag, or commit from which to create the worktree")
	command.Flags().StringVar(&options.BranchTemplate, "branch-template", "", "branch template containing %s")
	command.Flags().BoolVar(&options.Detach, "detach", false, "create a detached worktree without a branch")
	return command
}

func newListCommand(manager wt.Worktree) *cobra.Command {
	var porcelain bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List a project's worktrees",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return manager.List(porcelain)
		},
	}
	command.Flags().BoolVar(&porcelain, "porcelain", false, "print stable machine-readable output")
	return command
}

func newPathCommand(manager wt.Worktree) *cobra.Command {
	var notes bool
	command := &cobra.Command{
		Use:   "path [task]",
		Short: "Print a trunk, task, or notes path",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			task := ""
			if len(args) == 1 {
				task = args[0]
			}
			return manager.Path(task, notes)
		},
	}
	command.Flags().BoolVar(&notes, "notes", false, "print the local notes directory")
	return command
}

func newPullCommand(manager wt.Worktree) *cobra.Command {
	return &cobra.Command{
		Use:   "pull",
		Short: "Fast-forward the project's trunk",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return manager.Pull()
		},
	}
}

func newRemoveCommand(manager wt.Worktree) *cobra.Command {
	var options wt.RemoveOptions
	command := &cobra.Command{
		Use:   "remove <task>",
		Short: "Safely remove a task worktree",
		Long: `Remove a task worktree while retaining its branch by default.

The command refuses local changes and untracked or ignored files. Use --force
to permanently discard those files. Active Git operations and unmerged branch
deletion remain protected even when --force is set.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return manager.Remove(args[0], options)
		},
	}
	command.Flags().BoolVar(&options.DeleteBranch, "delete-branch", false, "also delete the merged task branch")
	command.Flags().BoolVarP(&options.Force, "force", "f", false, "discard local changes and untracked or ignored files")
	return command
}
