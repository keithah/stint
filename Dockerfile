# syntax=docker/dockerfile:1.7
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY sqlc.yaml ./
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -o /out/stint ./cmd/server && \
    CGO_ENABLED=0 go build -o /out/stint-collect ./cmd/collect

FROM alpine:3.21 AS api
RUN adduser -D -H stint && mkdir -p /data/dumps && chown -R stint:stint /data
USER stint
COPY --from=build /out/stint /stint
VOLUME ["/data/dumps"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
ENTRYPOINT ["/stint"]

FROM api AS collector
HEALTHCHECK NONE
COPY --from=build /out/stint-collect /stint-collect
ENTRYPOINT ["/stint-collect"]
