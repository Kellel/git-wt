package wt

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type gitRunner struct {
	env []string
}

type gitError struct {
	dir    string
	args   []string
	output string
	err    error
}

func (e *gitError) Error() string {
	command := "git " + strings.Join(e.args, " ")
	if e.dir != "" {
		command = "git -C " + e.dir + " " + strings.Join(e.args, " ")
	}
	if e.output == "" {
		return fmt.Sprintf("%s: %v", command, e.err)
	}
	return fmt.Sprintf("%s: %s", command, e.output)
}

func (e *gitError) Unwrap() error { return e.err }

func (g gitRunner) run(dir string, args ...string) ([]byte, error) {
	commandArgs := append([]string(nil), args...)
	if dir != "" {
		commandArgs = append([]string{"-C", dir}, commandArgs...)
	}
	cmd := exec.Command("git", commandArgs...)
	cmd.Env = g.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		output := strings.TrimSpace(stderr.String())
		if output == "" {
			output = strings.TrimSpace(stdout.String())
		}
		return nil, &gitError{dir: dir, args: args, output: output, err: err}
	}
	return stdout.Bytes(), nil
}

func (g gitRunner) output(dir string, args ...string) (string, error) {
	output, err := g.run(dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (g gitRunner) globalConfig(key string) (string, bool, error) {
	value, err := g.output("", "config", "--global", "--get", key)
	if err == nil {
		return value, true, nil
	}
	if gitExitCode(err) == 1 {
		return "", false, nil
	}
	return "", false, fmt.Errorf("read git config %s: %w", key, err)
}

func gitExitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func cleanGitEnvironment(env []string) []string {
	blocked := map[string]bool{
		"GIT_DIR":                          true,
		"GIT_WORK_TREE":                    true,
		"GIT_COMMON_DIR":                   true,
		"GIT_INDEX_FILE":                   true,
		"GIT_PREFIX":                       true,
		"GIT_OBJECT_DIRECTORY":             true,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
		"GIT_CEILING_DIRECTORIES":          true,
		"GIT_DISCOVERY_ACROSS_FILESYSTEM":  true,
	}
	cleaned := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if !blocked[name] {
			cleaned = append(cleaned, entry)
		}
	}
	return cleaned
}
