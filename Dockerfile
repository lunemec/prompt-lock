# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /out/promptlockd ./cmd/promptlockd && \
    go build -o /out/promptlock ./cmd/promptlock && \
    go build -o /out/promptlock-mcp ./cmd/promptlock-mcp

FROM alpine:3.20
RUN adduser -D -u 10001 promptlock
WORKDIR /app
COPY --from=build /out/promptlockd /usr/local/bin/promptlockd
COPY --from=build /out/promptlock /usr/local/bin/promptlock
COPY --from=build /out/promptlock-mcp /usr/local/bin/promptlock-mcp
USER promptlock
ENTRYPOINT ["/usr/local/bin/promptlockd"]
