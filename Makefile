# grove Makefile
# All targets are PHONY (no files named build/test/etc. in the repo).
.PHONY: build install test test-integration lint clean fixtures docker-build docker-run dev dev-fast

BINARY   := grove
CMD      := ./cmd/grove
BIN_DIR  := bin
INSTALL  := $(HOME)/.local/bin

## build: compile the grove binary to bin/grove
build:
	go build -o $(BIN_DIR)/$(BINARY) $(CMD)

## install: build and install grove to ~/.local/bin/grove
install: build
	install -m 755 $(BIN_DIR)/$(BINARY) $(INSTALL)/$(BINARY)
	xattr -c $(INSTALL)/$(BINARY)

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

## fixtures: build grove then generate the deterministic test world at /tmp/grove-fixtures
fixtures: build
	go run ./test/fixtures/gen -grove $(BIN_DIR)/$(BINARY)

## docker-build: build the dev/test clean-room image
docker-build:
	docker build -t grove-dev .

## docker-run: start an interactive shell inside the clean-room (bash, no tmux)
docker-run:
	docker run -it --rm --entrypoint bash grove-dev

## dev: build image and drop into a live grove tmux session with test fixtures
##   Lands in coding-project-big with fixture repos as windows.
##   grove, tmux, git, fzf all on PATH. Exit tmux to leave the container.
dev: docker-build
	docker run -it --rm grove-dev

## dev-fast: start the dev container without rebuilding the image
dev-fast:
	docker run -it --rm grove-dev

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //'
