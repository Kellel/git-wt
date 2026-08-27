package wt

import (
	"fmt"
	"strings"
)

type config struct {
	branchTemplate string
}

func (a *Manager) loadConfig(branchTemplate string) (config, error) {
	cfg := config{
		branchTemplate: "%s",
	}

	if value, found, err := a.git.globalConfig("wt.branchTemplate"); err != nil {
		return config{}, err
	} else if found {
		cfg.branchTemplate = value
	}
	if value := environmentValue(a.env, "WT_BRANCH_TEMPLATE"); value != "" {
		cfg.branchTemplate = value
	}

	if branchTemplate != "" {
		cfg.branchTemplate = branchTemplate
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
