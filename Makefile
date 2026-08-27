GOLANGCI_LINT_VERSION ?= v2.13.1

.PHONY: check install lint test fuzz

check: test lint
	go vet ./...

install:
	go install ./cmd/git-wt

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --build-tags=fuzz ./...

test:
	go test -race ./...

fuzz:
	go test -tags=fuzz -run='^$$' -fuzz=FuzzParseWorktrees -fuzztime=30s -parallel=1 ./wt
	go test -tags=fuzz -run='^$$' -fuzz=FuzzProjectNameFromRemote -fuzztime=30s -parallel=1 ./wt
