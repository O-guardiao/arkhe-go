# ============================================================
# arkhe-go — Minimal runtime image
# Build context: arkhe-go root (../..)
# Includes picoclaw + rlm-go fusion
# ============================================================

# Stage 1: Build from the arkhe-go monorepo
FROM golang:1.26.0-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /src/picoclaw-main

# Cache Go deps — include the local rlm-go replace target.
COPY picoclaw-main/go.mod picoclaw-main/go.sum ./
COPY rlm-go/go.mod rlm-go/go.sum /src/rlm-go/
RUN go mod download

# Copy full source (picoclaw + rlm-go) and build.
COPY picoclaw-main /src/picoclaw-main
COPY rlm-go /src/rlm-go
RUN make build

# Stage 2: Minimal Alpine runtime
FROM alpine:3.23

LABEL org.opencontainers.image.title="arkhe-go" \
      org.opencontainers.image.description="PicoClaw + rlm-go fusion" \
      org.opencontainers.image.source="https://github.com/O-guardiao/arkhe-go"

RUN apk add --no-cache ca-certificates tzdata curl

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q --spider http://localhost:18790/health || exit 1

COPY --from=builder /src/picoclaw-main/build/picoclaw /usr/local/bin/picoclaw
COPY picoclaw-main/docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 18790

ENTRYPOINT ["/entrypoint.sh"]
