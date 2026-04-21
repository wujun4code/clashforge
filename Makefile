BINARY=clashforge

.PHONY: build test fmt

build:
	go build -trimpath -ldflags='-s -w' -o bin/$(BINARY) ./cmd/clashforge

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal
