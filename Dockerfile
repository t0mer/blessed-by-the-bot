# Stage 1: the frontend, built once on the native build platform.
#
# Its output is static JS/CSS and therefore platform-independent, so building it
# per target under QEMU would be pure waste — and routinely breaks, because
# emulated npm needs every target's native optional dependencies.
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend

WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
# The Vite build writes into ../internal/webui/dist, so that path must exist.
RUN mkdir -p /internal/webui/dist && npm run build

# Stage 2: build the static binary on the native build platform and
# cross-compile to the target arch (no QEMU for the compile itself).
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

WORKDIR /src
ENV CGO_ENABLED=0
ENV GOTOOLCHAIN=local

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Overwrite the tracked placeholder with the real build from stage 1.
COPY --from=frontend /internal/webui/dist ./internal/webui/dist

ARG VERSION=docker
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/blessedbot ./cmd/blessedbot

# A writable data dir owned by the nonroot uid, copied into scratch below.
RUN mkdir -p /out/data && chown 65532:65532 /out/data

# Stage 3: runtime.
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /out/blessedbot /blessedbot
COPY --from=builder --chown=65532:65532 /out/data /data
COPY rootfs/etc/passwd /etc/passwd

LABEL org.opencontainers.image.title="blessed-by-the-bot" \
      org.opencontainers.image.description="Self-hosted WhatsApp blessing bot" \
      org.opencontainers.image.source="https://github.com/t0mer/blessed-by-the-bot" \
      org.opencontainers.image.licenses="Apache-2.0"

USER 65532:65532
WORKDIR /
ENV BBTB_DATA_DIR=/data
EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/blessedbot"]
