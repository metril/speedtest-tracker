# syntax=docker/dockerfile:1

# ---- stage 1: build the SPA into internal/web/dist -------------------------
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- stage 2: build the static Go binary ----------------------------------
FROM --platform=$BUILDPLATFORM golang:1.25 AS build
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/speedtest-tracker ./cmd/speedtest-tracker

# ---- stage 3: runtime -----------------------------------------------------
# trixie-slim carries iperf3 >= 3.17 (--json-stream support), unlike
# bookworm-slim's 3.12.
# Pinned to the debian:trixie-slim tag as of 2026-09-14; bump by re-pulling
# the tag and updating both the digest and this date.
FROM debian:trixie-slim@sha256:d7e12182ce18b85b93007c1dedf31f2d29e01ccf3182cc4017c709b6259bc132
ARG TARGETARCH
ARG OOKLA_VERSION=1.2.0
# Pinned checksums for the Ookla CLI tarballs; recompute when bumping
# OOKLA_VERSION (curl -fsSL <url> | sha256sum).
ARG OOKLA_SHA256_AMD64=5690596c54ff9bed63fa3732f818a05dbc2db19ad36ed68f21ca5f64d5cfeeb7
ARG OOKLA_SHA256_ARM64=3953d231da3783e2bf8904b6dd72767c5c6e533e163d3742fd0437affa431bd3

RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends iperf3 ca-certificates curl tzdata; \
    case "${TARGETARCH}" in \
      amd64) OOKLA_ARCH=x86_64; OOKLA_SHA256="${OOKLA_SHA256_AMD64}" ;; \
      arm64) OOKLA_ARCH=aarch64; OOKLA_SHA256="${OOKLA_SHA256_ARM64}" ;; \
      *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/ookla.tgz \
      "https://install.speedtest.net/app/cli/ookla-speedtest-${OOKLA_VERSION}-linux-${OOKLA_ARCH}.tgz"; \
    echo "${OOKLA_SHA256}  /tmp/ookla.tgz" | sha256sum -c -; \
    tar -xzf /tmp/ookla.tgz -C /usr/local/bin speedtest; \
    rm -f /tmp/ookla.tgz; \
    apt-get purge -y curl; \
    apt-get autoremove -y; \
    rm -rf /var/lib/apt/lists/*; \
    speedtest --version; \
    iperf3 --version

# uid/gid 1000 match the first regular user on most Linux hosts, so a
# bind-mounted /data owned by that user is writable without a chown.
RUN groupadd --gid 1000 app \
 && useradd --uid 1000 --gid app --home-dir /data --shell /usr/sbin/nologin app \
 && mkdir -p /data && chown app:app /data

COPY --from=build /out/speedtest-tracker /usr/local/bin/speedtest-tracker

USER app
WORKDIR /data
VOLUME /data
EXPOSE 8080
ENV ST_DB_PATH=/data/speedtest.db ST_LISTEN=:8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
    CMD ["/usr/local/bin/speedtest-tracker", "-healthcheck"]
ENTRYPOINT ["/usr/local/bin/speedtest-tracker"]
