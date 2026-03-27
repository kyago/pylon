VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test lint install uninstall clean release-dry-run tag

build:
	go build $(LDFLAGS) -o bin/pylon ./cmd/pylon

test:
	go test ./... -race -count=1

lint:
	golangci-lint run ./...

install:
	go install $(LDFLAGS) ./cmd/pylon

uninstall:
	@rm -f $(shell go env GOPATH)/bin/pylon
	@echo "pylon binary removed from $$(go env GOPATH)/bin/"

clean:
	rm -rf bin/

release-dry-run:
	goreleaser release --snapshot --clean

tag:
	@if [ -z "$(TAG)" ]; then echo "ERROR: TAG is required. Usage: make tag TAG=2026.3.1"; exit 1; fi
	@echo "$(TAG)" | grep -qE '^[0-9]{4}\.[0-9]{1,2}\.[0-9]+$$' || (echo "ERROR: TAG must match YYYY.M.SEQ format (e.g. 2026.3.1)"; exit 1)
	@git diff --quiet && git diff --cached --quiet || (echo "ERROR: working tree is dirty. Commit or stash changes first."; exit 1)
	git tag -a "$(TAG)" -m "Release $(TAG)"
	git push origin "$(TAG)"
