# syntax=docker/dockerfile:1

# Stage 1: Build the static Go binary
FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Cache dependency layer
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build stripped static binary
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /app/ops-lens-mcp \
    ./cmd/ops-lens-mcp

# Stage 2: Minimal runtime image
FROM alpine:3.20

# Install CA certificates and SSH client (required for remote Docker Compose hosts over SSH)
RUN apk --no-cache add ca-certificates openssh-client tzdata && \
    mkdir -p /etc/ops-lens /root/.ssh /root/.kube

WORKDIR /

COPY --from=builder /app/ops-lens-mcp /usr/local/bin/ops-lens-mcp

ENTRYPOINT ["ops-lens-mcp"]
CMD ["--config", "/etc/ops-lens/config.yaml"]
