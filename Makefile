VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test lint install uninstall clean

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
