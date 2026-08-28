package wt

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const codexConfigRelativePath = ".codex/config.toml"
const codexExcludePattern = "/.codex/config.toml"

//go:embed trunk_codex_config.toml
var trunkCodexConfig string

// installCodexTrunkGuard creates an ignored, trunk-local Codex policy. The
// returned function removes everything created by this call so adoption can
// still roll back without changing the source checkout.
func (a *Manager) installCodexTrunkGuard(checkout, displayCheckout string) (func() error, error) {
	configPath := filepath.Join(checkout, filepath.FromSlash(codexConfigRelativePath))
	if _, err := os.Lstat(configPath); err == nil {
		_, writeErr := fmt.Fprintf(a.err, "warning: Codex trunk protection was not installed because %s already exists; leaving it unchanged\n", filepath.Join(displayCheckout, filepath.FromSlash(codexConfigRelativePath)))
		return noCleanup, writeErr
	} else if !errors.Is(err, os.ErrNotExist) {
		return noCleanup, fmt.Errorf("inspect existing Codex config %s: %w", configPath, err)
	}

	excludePath, err := a.git.output(checkout, "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
	if err != nil {
		return noCleanup, fmt.Errorf("locate Git exclude file: %w", err)
	}
	originalExclude, excludeMode, excludeExisted, err := readOptionalFile(excludePath)
	if err != nil {
		return noCleanup, fmt.Errorf("read Git exclude file %s: %w", excludePath, err)
	}

	configDir := filepath.Dir(configPath)
	dirCreated := false
	if info, err := os.Lstat(configDir); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(configDir, 0o755); err != nil {
			return noCleanup, fmt.Errorf("create Codex config directory %s: %w", configDir, err)
		}
		dirCreated = true
	} else if err != nil {
		return noCleanup, fmt.Errorf("inspect Codex config directory %s: %w", configDir, err)
	} else if !info.IsDir() {
		return noCleanup, fmt.Errorf("codex config path %s exists and is not a directory", configDir)
	}

	cleanup := func() error {
		var cleanupErr error
		if err := os.Remove(configPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove Codex config: %w", err))
		}
		if dirCreated {
			if err := os.Remove(configDir); err != nil && !errors.Is(err, os.ErrNotExist) {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove Codex config directory: %w", err))
			}
		}
		if excludeExisted {
			if err := os.WriteFile(excludePath, originalExclude, excludeMode); err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("restore Git exclude file: %w", err))
			}
		} else if err := os.Remove(excludePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove Git exclude file: %w", err))
		}
		return cleanupErr
	}

	if err := writeExclusiveFile(configPath, trunkCodexConfig); err != nil {
		if dirCreated {
			_ = os.Remove(configDir)
		}
		return noCleanup, fmt.Errorf("create Codex trunk config %s: %w", configPath, err)
	}
	if !hasLine(originalExclude, codexExcludePattern) {
		updatedExclude := appendLine(originalExclude, codexExcludePattern)
		if err := os.WriteFile(excludePath, updatedExclude, excludeMode); err != nil {
			cleanupErr := cleanup()
			return noCleanup, errors.Join(fmt.Errorf("update Git exclude file %s: %w", excludePath, err), cleanupErr)
		}
	}
	return cleanup, nil
}

func noCleanup() error { return nil }

func readOptionalFile(path string) (contents []byte, mode os.FileMode, existed bool, err error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0o644, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	contents, err = os.ReadFile(path)
	return contents, info.Mode().Perm(), true, err
}

func writeExclusiveFile(path, contents string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(contents); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func hasLine(contents []byte, want string) bool {
	for line := range strings.SplitSeq(string(contents), "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

func appendLine(contents []byte, line string) []byte {
	result := append([]byte(nil), contents...)
	if len(result) != 0 && result[len(result)-1] != '\n' {
		result = append(result, '\n')
	}
	return append(result, line+"\n"...)
}
