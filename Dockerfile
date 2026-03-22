# syntax=docker/dockerfile:1

ARG GO_BUILD_IMAGE=golang:1.26.1-alpine3.23
ARG RUNTIME_IMAGE=alpine:3.23
ARG CODEX_CLI_VERSION=0.116.0
ARG CLAUDE_CODE_CLI_VERSION=2.1.79
ARG GEMINI_CLI_VERSION=0.34.0

FROM ${GO_BUILD_IMAGE} AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /out/promptlockd ./cmd/promptlockd && \
    go build -o /out/promptlock ./cmd/promptlock && \
    go build -o /out/promptlock-mcp ./cmd/promptlock-mcp && \
    go build -o /out/promptlock-mcp-launch ./cmd/promptlock-mcp-launch

FROM ${RUNTIME_IMAGE} AS runtime-base
RUN adduser -D -u 10001 promptlock && \
    mkdir -p /app /workspace && \
    chown -R promptlock:promptlock /app /workspace
WORKDIR /app
COPY --from=build /out/promptlockd /usr/local/bin/promptlockd
COPY --from=build /out/promptlock /usr/local/bin/promptlock
COPY --from=build /out/promptlock-mcp /usr/local/bin/promptlock-mcp
COPY --from=build /out/promptlock-mcp-launch /usr/local/bin/promptlock-mcp-launch
COPY --chmod=0755 scripts/secretctl.sh /usr/local/bin/secretctl.sh
COPY skills/secret-request/SKILL.md /opt/promptlock/skills/secret-request/SKILL.md
USER promptlock

FROM runtime-base AS runtime
ENTRYPOINT ["/usr/local/bin/promptlockd"]

FROM runtime-base AS agent-lab
ARG CODEX_CLI_VERSION
ARG CLAUDE_CODE_CLI_VERSION
ARG GEMINI_CLI_VERSION
USER root
RUN apk add --no-cache \
      bash \
      build-base \
      ca-certificates \
      cmake \
      coreutils \
      curl \
      fd \
      git \
      github-cli \
      jq \
      go \
      libffi-dev \
      make \
      ninja-build \
      nodejs \
      npm \
      openssl-dev \
      openssh-client \
      pkgconf \
      py3-pip \
      python3 \
      ripgrep \
      rust \
      cargo \
      shellcheck \
      shfmt \
      yq-go && \
    ln -sf /usr/lib/ninja-build/bin/ninja /usr/local/bin/ninja && \
    npm install -g \
      pnpm@latest \
      "@openai/codex@${CODEX_CLI_VERSION}" \
      "@anthropic-ai/claude-code@${CLAUDE_CODE_CLI_VERSION}" \
      "@google/gemini-cli@${GEMINI_CLI_VERSION}" && \
    mkdir -p /home/promptlock/.codex /home/promptlock/.claude /home/promptlock/.config /home/promptlock/.gemini && \
    printf '%s\n' '{"projects":{}}' > /home/promptlock/.gemini/projects.json && \
    chown -R promptlock:promptlock /home/promptlock /workspace
ENV HOME=/home/promptlock \
    SHELL=/bin/bash
WORKDIR /workspace
USER promptlock
CMD ["/bin/sh"]
