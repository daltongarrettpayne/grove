# grove dev / test clean-room image.
#
# What this proves: grove builds and its picker works with ONLY these declared
# dependencies — no personal config, no host environment, no PATH leakage.
# That is the portability thesis made executable.
#
# Usage:
#   make dev              # build image and drop into a live grove session
#   make dev-fast         # skip rebuild, reuse existing image

FROM golang:1.25-bookworm

# Runtime deps grove shells out to.
# fzf = default picker backend.
RUN apt-get update && apt-get install -y \
    tmux \
    git \
    fzf \
    && rm -rf /var/lib/apt/lists/*

# Set a git identity so the fixture generator can commit without a user config.
RUN git config --global user.name "Grove Fixture" \
 && git config --global user.email "fixture@grove.test"

WORKDIR /grove

# Copy dependency manifests first so Docker caches the module download layer
# separately from source. The download step only re-runs when go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build.
COPY . .
RUN go build -o /usr/local/bin/grove ./cmd/grove
COPY scripts/dev-entrypoint.sh /usr/local/bin/dev-entrypoint
RUN chmod +x /usr/local/bin/dev-entrypoint

# Generate the fixture world under $HOME so the layout mirrors the real system:
# ~/code/ and ~/vault/ rather than a separate /fixtures/ directory.
RUN go run ./test/fixtures/gen --out /root

# Tell grove where to find code and vault without any user config.
ENV GROVE_CODE_ROOT=/root/code
ENV GROVE_HOME_ROOT=/root/vault

# Use a private tmux socket so the container's tmux is isolated from any
# socket that might exist on the host if volumes are mounted.
ENV GROVE_TMUX_SOCKET=/tmp/grove-dev.sock

# Open the coding-project-big fixture context and attach.
# Drops you into a live grove tmux session with all fixture repos as windows.
# Exit tmux to leave the container.
CMD ["dev-entrypoint"]
