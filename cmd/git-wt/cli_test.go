package main

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/Kellel/git-wt/wt"
)

type managerCall struct {
	method string
	args   []any
}

type fakeWorktreeManager struct {
	call managerCall
	err  error
}

func (f *fakeWorktreeManager) Clone(remote, destination string) error {
	f.call = managerCall{"Clone", []any{remote, destination}}
	return f.err
}

func (f *fakeWorktreeManager) Adopt(source string, dryRun bool) error {
	f.call = managerCall{"Adopt", []any{source, dryRun}}
	return f.err
}

func (f *fakeWorktreeManager) NewWorktree(task string, options wt.NewOptions) error {
	f.call = managerCall{"NewWorktree", []any{task, options}}
	return f.err
}

func (f *fakeWorktreeManager) List(porcelain bool) error {
	f.call = managerCall{"List", []any{porcelain}}
	return f.err
}

func (f *fakeWorktreeManager) Path(task string, notes bool) error {
	f.call = managerCall{"Path", []any{task, notes}}
	return f.err
}

func (f *fakeWorktreeManager) Pull() error {
	f.call = managerCall{"Pull", nil}
	return f.err
}

func (f *fakeWorktreeManager) Remove(task string, options wt.RemoveOptions) error {
	f.call = managerCall{"Remove", []any{task, options}}
	return f.err
}

func TestCommandDispatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want managerCall
	}{
		{"clone", []string{"clone", "git@example.com:team/project.git"}, managerCall{"Clone", []any{"git@example.com:team/project.git", ""}}},
		{"clone destination", []string{"clone", "remote", "project"}, managerCall{"Clone", []any{"remote", "project"}}},
		{"adopt", []string{"adopt", "source", "--dry-run"}, managerCall{"Adopt", []any{"source", true}}},
		{"new", []string{"new", "task", "--branch", "feature", "--base", "HEAD", "--branch-template", "user/%s"}, managerCall{"NewWorktree", []any{"task", wt.NewOptions{Branch: "feature", Base: "HEAD", BranchTemplate: "user/%s"}}}},
		{"list", []string{"list", "--porcelain"}, managerCall{"List", []any{true}}},
		{"task path", []string{"path", "task"}, managerCall{"Path", []any{"task", false}}},
		{"notes path", []string{"path", "--notes"}, managerCall{"Path", []any{"", true}}},
		{"pull", []string{"pull"}, managerCall{"Pull", nil}},
		{"remove", []string{"remove", "task", "--delete-branch", "-f"}, managerCall{"Remove", []any{"task", wt.RemoveOptions{DeleteBranch: true, Force: true}}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			manager := &fakeWorktreeManager{}
			command := newRootCommand(manager, &bytes.Buffer{}, &bytes.Buffer{})
			command.SetArgs(test.args)
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(manager.call, test.want) {
				t.Fatalf("call = %#v, want %#v", manager.call, test.want)
			}
		})
	}
}

func TestCommandValidationDoesNotDispatch(t *testing.T) {
	t.Parallel()

	manager := &fakeWorktreeManager{}
	command := newRootCommand(manager, &bytes.Buffer{}, &bytes.Buffer{})
	command.SetArgs([]string{"list", "--unknown"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected an unknown flag error")
	}
	if manager.call.method != "" {
		t.Fatalf("unexpected manager call: %#v", manager.call)
	}
}

func TestCommandReturnsManagerError(t *testing.T) {
	t.Parallel()

	want := errors.New("manager failed")
	manager := &fakeWorktreeManager{err: want}
	command := newRootCommand(manager, &bytes.Buffer{}, &bytes.Buffer{})
	command.SetArgs([]string{"pull"})
	if err := command.Execute(); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
