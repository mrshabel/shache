.PHONY: build run test

build:
	@go build -o bin/shache

test:
	@go test -v -race .../.

run: build
	@echo "running shache..."