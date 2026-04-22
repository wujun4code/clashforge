# =============================================================
# Stage 1: Build React frontend
# =============================================================
FROM node:22-alpine AS frontend-builder

WORKDIR /ui
COPY ui/package*.json ./
RUN npm ci --prefer-offline

COPY ui/ ./
RUN npm run build
# output: /ui/../internal/api/ui_dist  (vite outDir is relative)
# We re-target manually:
RUN cp -r dist /ui_dist

# =============================================================
# Stage 2: Build Go backend (with embedded UI)
# =============================================================
FROM golang:1.25-alpine AS backend-builder

WORKDIR /src

# Dependencies first (layer cache)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Inject UI build into embed path
COPY --from=frontend-builder /ui_dist ./internal/api/ui_dist

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags='-s -w' \
    -o /bin/clashforge \
    ./cmd/clashforge

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags='-s -w' \
    -o /bin/genconfig \
    ./cmd/genconfig

# =============================================================
# Stage 3: Minimal runtime image
# =============================================================
FROM alpine:3.19

LABEL org.opencontainers.image.title="clashforge"
LABEL org.opencontainers.image.description="Mihomo proxy manager for OpenWrt"
LABEL org.opencontainers.image.source="https://github.com/wujun4code/clashforge"

RUN apk add --no-cache ca-certificates tzdata

COPY --from=backend-builder /bin/clashforge /usr/local/bin/clashforge
COPY --from=backend-builder /bin/genconfig  /usr/local/bin/genconfig

# Default dirs
RUN mkdir -p /etc/metaclash /var/run/metaclash

COPY openwrt/files/etc/metaclash/config.toml.example   /etc/metaclash/config.toml
COPY openwrt/files/etc/metaclash/overrides.yaml.example /etc/metaclash/overrides.yaml

EXPOSE 7777

ENTRYPOINT ["/usr/local/bin/clashforge"]
CMD ["-config", "/etc/metaclash/config.toml"]
