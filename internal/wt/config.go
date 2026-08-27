package wt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type config struct {
	root           string
	namespace      string
	branchTemplate string
}

func (a *App) loadConfig(overrides configOverrides) (config, error) {
	home := environmentValue(a.env, "HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return config{}, fmt.Errorf("determine home directory: %w", err)
		}
	}

	cfg := config{
		root:           filepath.Join(home, "code"),
		branchTemplate: "%s",
	}

	gitValues := []struct {
		key    string
		target *string
	}{
		{"wt.root", &cfg.root},
		{"wt.namespace", &cfg.namespace},
		{"wt.branchTemplate", &cfg.branchTemplate},
	}
	for _, item := range gitValues {
		value, found, err := a.git.globalConfig(item.key)
		if err != nil {
			return config{}, err
		}
		if found {
			*item.target = value
		}
	}

	if value := environmentValue(a.env, "WT_ROOT"); value != "" {
		cfg.root = value
	}
	if value := environmentValue(a.env, "WT_NAMESPACE"); value != "" {
		cfg.namespace = value
	}
	if value := environmentValue(a.env, "WT_BRANCH_TEMPLATE"); value != "" {
		cfg.branchTemplate = value
	}

	if overrides.root != "" {
		cfg.root = overrides.root
	}
	if overrides.namespace != "" {
		cfg.namespace = overrides.namespace
	}
	if overrides.branchTemplate != "" {
		cfg.branchTemplate = overrides.branchTemplate
	}

	cfg.root = expandHome(cfg.root, home)
	root, err := absolutePath(a.cwd, cfg.root)
	if err != nil {
		return config{}, fmt.Errorf("resolve workspace root: %w", err)
	}
	cfg.root = root

	if cfg.namespace != "" {
		if err := validateComponent("namespace", cfg.namespace); err != nil {
			return config{}, err
		}
	}
	if !strings.Contains(cfg.branchTemplate, "%s") {
		return config{}, fmt.Errorf("branch template %q must contain %%s", cfg.branchTemplate)
	}
	return cfg, nil
}

func environmentValue(env []string, key string) string {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix)
		}
	}
	return ""
}

func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}
