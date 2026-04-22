FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /bin/clashforge ./cmd/clashforge
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /bin/genconfig ./cmd/genconfig

# ---- runtime ----
FROM alpine:3.19

RUN apk add --no-cache curl bind-tools bash ca-certificates python3

# Download mihomo compatible binary
ARG MIHOMO_VERSION=v1.19.24
RUN wget -q -O /tmp/mihomo.gz \
    "https://github.com/MetaCubeX/mihomo/releases/download/${MIHOMO_VERSION}/mihomo-linux-amd64-compatible-${MIHOMO_VERSION}.gz" \
    && gunzip /tmp/mihomo.gz \
    && mv /tmp/mihomo /usr/local/bin/mihomo \
    && chmod +x /usr/local/bin/mihomo \
    && mihomo -v

COPY --from=builder /bin/clashforge /usr/local/bin/clashforge
COPY --from=builder /bin/genconfig  /usr/local/bin/genconfig

COPY tests/docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

RUN mkdir -p /etc/metaclash /var/run/metaclash

ENTRYPOINT ["/entrypoint.sh"]
