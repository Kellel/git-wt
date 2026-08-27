package wt

import (
	_ "embed"
	"fmt"
	"os"
)

//go:embed project_agents.md
var projectAgents string

func createProjectAgents(p project) (string, error) {
	path := p.agents
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("create project guidance %s: %w", path, err)
	}
	if _, err := file.WriteString(projectAgents); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write project guidance %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close project guidance %s: %w", path, err)
	}
	return path, nil
}
