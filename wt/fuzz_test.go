//go:build fuzz

package wt

import (
	"strings"
	"testing"
)

func FuzzParseWorktrees(f *testing.F) {
	f.Add("worktree /repo/trunk\x00HEAD 0123456789012345678901234567890123456789\x00branch refs/heads/main\x00\x00")
	f.Add("worktree /repo/worktrees/task\x00HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd\x00detached\x00locked reason\x00\x00")
	f.Add("worktree ../../outside\x00HEAD deadbeef\x00prunable missing\x00\x00")
	f.Add("worktree \x00HEAD deadbeef\x00\x00")
	f.Add("")
	f.Add("\x00\xff\x00")

	f.Fuzz(func(t *testing.T, input string) {
		worktrees, err := parseWorktrees([]byte(input))
		if err != nil {
			return
		}
		if len(worktrees) == 0 {
			t.Fatal("successful parse returned no worktrees")
		}
		for _, item := range worktrees {
			if item.path == "" {
				t.Fatal("successful parse returned an empty worktree path")
			}
		}
	})
}

func FuzzProjectNameFromRemote(f *testing.F) {
	for _, seed := range []string{
		"git@example.com:team/project.git",
		"ssh://git@example.com/team/project.git",
		"https://example.com/team/project",
		"/tmp/project.git",
		"../../etc/passwd",
		"project\x00suffix",
		"\u202eproject.git",
		strings.Repeat("a", 4096) + ".git",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, remote string) {
		name, err := projectNameFromRemote(remote)
		if err != nil {
			return
		}
		if err := validateComponent("project", name); err != nil {
			t.Fatalf("accepted invalid project %q: %v", name, err)
		}
		if strings.ContainsAny(name, `/\\`) {
			t.Fatalf("project name contains a path separator: %q", name)
		}
	})
}
