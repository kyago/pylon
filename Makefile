VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test lint install clean

build:
	go build $(LDFLAGS) -o bin/pylon ./cmd/pylon

test:
	go test ./... -race -count=1

lint:
	golangci-lint run ./...

install:
	go install $(LDFLAGS) ./cmd/pylon

clean:
	rm -rf bin/
