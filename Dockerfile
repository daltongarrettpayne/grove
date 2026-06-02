FROM golang:1.24
RUN apt-get update && apt-get install -y tmux gif fzf && rm -rf /var/lib/apt/lists/*
WORKDIR /app
CMD ["bash"]
