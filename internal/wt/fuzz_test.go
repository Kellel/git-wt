//go:build fuzz

package wt

import (
	"bytes"
	"path/filepath"
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

func FuzzProjectKeyStaysWithinWorkspace(f *testing.F) {
	for _, seed := range []string{
		"team/example-api",
		"personal/git-tools",
		"../outside",
		"team/../../outside",
		"team/project\x00suffix",
		"team/\u202eproject",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, key string) {
		const root = "/workspace"
		project, err := projectForKey(config{root: root}, key)
		if err != nil {
			return
		}
		if err := ensureLexicallyWithin(root, project.root); err != nil {
			t.Fatalf("accepted project escaped workspace: key=%q root=%q: %v", key, project.root, err)
		}
		relative, err := filepath.Rel(root, project.root)
		if err != nil {
			t.Fatal(err)
		}
		if len(strings.Split(filepath.ToSlash(relative), "/")) != 2 {
			t.Fatalf("accepted project does not have two components: %q", relative)
		}
	})
}

func FuzzParseOptions(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("task"),
		[]byte("--base\x00origin/main\x00task"),
		[]byte("task\x00--branch=kellen/task"),
		[]byte("--\x00--root\x00../../outside"),
		[]byte("--unknown"),
		bytes.Repeat([]byte{'a'}, 4096),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 64*1024 {
			t.Skip()
		}
		parts := bytes.Split(input, []byte{0})
		if len(parts) > 64 {
			t.Skip()
		}
		args := make([]string, len(parts))
		for i := range parts {
			args[i] = string(parts[i])
		}
		var base, branch string
		_, _, _ = parseOptions(args,
			map[string]*string{"--base": &base, "--branch": &branch},
			map[string]*bool{},
		)
	})
}
