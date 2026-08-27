package wt

import (
	"io"
	"os"
	"path/filepath"
)

// Manager performs worktree operations using the process context supplied at construction.
type Manager struct {
	cwd    string
	env    []string
	out    io.Writer
	err    io.Writer
	git    gitRunner
	rename func(string, string) error
}

// NewManager creates a Manager rooted at cwd with the supplied environment and output streams.
func NewManager(cwd string, env []string, stdout, stderr io.Writer) *Manager {
	return newManagerWithRename(cwd, env, stdout, stderr, os.Rename)
}

func newManagerWithRename(cwd string, env []string, stdout, stderr io.Writer, rename func(string, string) error) *Manager {
	return &Manager{
		cwd:    cwd,
		env:    append([]string(nil), env...),
		out:    stdout,
		err:    stderr,
		git:    gitRunner{env: cleanGitEnvironment(env)},
		rename: rename,
	}
}

func absolutePath(base, path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(path), nil
}
