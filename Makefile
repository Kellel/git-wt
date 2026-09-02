GOLANGCI_LINT_VERSION ?= v2.13.1
CODEX_SKILLS_DIR ?= $(HOME)/.codex/skills

.PHONY: check install install-cli install-skill lint test fuzz

check: test lint
	go vet ./...

install: install-cli install-skill

install-cli:
	go install ./cmd/git-wt

install-skill:
	mkdir -p "$(CODEX_SKILLS_DIR)/code-review/agents"
	cp skills/code-review/SKILL.md "$(CODEX_SKILLS_DIR)/code-review/SKILL.md"
	cp skills/code-review/agents/openai.yaml "$(CODEX_SKILLS_DIR)/code-review/agents/openai.yaml"

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --build-tags=fuzz ./...

test:
	go test -race ./...

fuzz:
	go test -tags=fuzz -run='^$$' -fuzz=FuzzParseWorktrees -fuzztime=30s -parallel=1 ./wt
	go test -tags=fuzz -run='^$$' -fuzz=FuzzProjectNameFromRemote -fuzztime=30s -parallel=1 ./wt
