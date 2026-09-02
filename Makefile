.PHONY: build test

build:
	mkdir -p build
	npm --prefix frontend ci
	npm --prefix frontend run build
	CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o build/bpm ./cmd/bpm

test:
	go test ./...
	npm --prefix frontend test -- --run
