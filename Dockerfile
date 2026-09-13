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
FROM debian:bookworm-slim
ARG TARGETARCH
# Ookla CLI is pinned; checksum verification arrives with the engine work.
ARG OOKLA_VERSION=1.2.0

RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends iperf3 ca-certificates curl tzdata; \
    case "${TARGETARCH}" in \
      amd64) OOKLA_ARCH=x86_64 ;; \
      arm64) OOKLA_ARCH=aarch64 ;; \
      *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/ookla.tgz \
      "https://install.speedtest.net/app/cli/ookla-speedtest-${OOKLA_VERSION}-linux-${OOKLA_ARCH}.tgz"; \
    tar -xzf /tmp/ookla.tgz -C /usr/local/bin speedtest; \
    rm -f /tmp/ookla.tgz; \
    apt-get purge -y curl; \
    apt-get autoremove -y; \
    rm -rf /var/lib/apt/lists/*; \
    speedtest --version

RUN useradd --system --uid 10001 --home-dir /data --shell /usr/sbin/nologin app \
 && mkdir -p /data && chown app:app /data

COPY --from=build /out/speedtest-tracker /usr/local/bin/speedtest-tracker

USER app
WORKDIR /data
VOLUME /data
EXPOSE 8080
ENV ST_DB_PATH=/data/speedtest.db ST_LISTEN=:8080
ENTRYPOINT ["/usr/local/bin/speedtest-tracker"]
