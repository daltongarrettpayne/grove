# grove Makefile
# All targets are PHONY (no files named build/test/etc. in the repo).
.PHONY: build test test-integration lint clean fixtures docker-build docker-run

BINARY  := grove
CMD     := ./cmd/grove
BIN_DIR := bin

## build: compile the grove binary to bin/grove
build:
	go build -o $(BIN_DIR)/$(BINARY) $(CMD)

## test: run all unit tests with the race detector
test:
	go test -race -cover ./...

## test-integration: generate fixtures then run integration tests
test-integration: fixtures
	go test -race -tags integration ./test/...

## lint: run golangci-lint (install: brew install golangci-lint)
lint:
	golangci-lint run

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR)/

## fixtures: generate the deterministic test world at /tmp/grove-fixtures
fixtures:
	go run ./test/fixtures/gen

## docker-build: build the dev/test clean-room image
docker-build:
	docker build -t grove-dev .

## docker-run: start an interactive shell inside the clean-room
##   Inside: `grove session list`, `grove window`, etc. all work against fixtures.
##   Start tmux with: tmux new-session -s dev
docker-run:
	docker run -it --rm grove-dev

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
