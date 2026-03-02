VERSION := $(shell git describe --tags --always 2>/dev/null || echo "dev")
LDFLAGS := -X main.version=$(VERSION) -X main.buildDate=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/monoes ./cmd/monoes

.PHONY: build-all
build-all:
	GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/monoes-darwin-amd64 ./cmd/monoes
	GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/monoes-darwin-arm64 ./cmd/monoes
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/monoes-linux-amd64 ./cmd/monoes
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/monoes-windows-amd64.exe ./cmd/monoes

.PHONY: test
test:
	go test -race -v ./...

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: clean
clean:
	rm -rf bin/

.PHONY: tidy
tidy:
	go mod tidy
