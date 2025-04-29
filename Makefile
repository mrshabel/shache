.PHONY: build run test serve

build:
	@go build -o bin/shache

test:
	@go test -v -race .../.

run: build
	@echo "running shache..."

serve:
	@go run ./cmd/api .