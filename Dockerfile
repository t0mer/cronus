# syntax=docker/dockerfile:1

# Stage 1: build the frontend once on the native build platform. Vite/esbuild
# native deps must not run under QEMU, so this stage is pinned to BUILDPLATFORM.
FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: cross-compile the Go binary on the native build platform.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
WORKDIR /app
ENV GOTOOLCHAIN=local
ENV CGO_ENABLED=0
RUN apk add --no-cache ca-certificates tzdata
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Embed the freshly built frontend (overwrites the tracked dist placeholder).
COPY --from=frontend /app/web/dist ./web/dist
ARG VERSION=docker
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} \
    go build -trimpath \
    -ldflags "-s -w -X github.com/t0mer/cronus/internal/version.Version=${VERSION}" \
    -o /out/cronus ./cmd/cronus
# Pre-create the data dir owned by the non-root runtime user; scratch has no
# shell to do this at runtime, and the SQLite DB/key files live here.
RUN mkdir -p /data && chown 65534:65534 /data

# Stage 3: minimal scratch runtime.
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder --chown=65534:65534 /data /data
COPY --from=builder /out/cronus /cronus

ENV CRONUS_DB_PATH=/data/cronus.db
EXPOSE 8080
VOLUME ["/data"]
USER 65534:65534

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/cronus", "healthcheck"]

ENTRYPOINT ["/cronus"]
