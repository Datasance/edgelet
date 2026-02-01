# Multi-stage build for production
# Stage 1: Build
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binaries
ARG VERSION=dev
ARG BUILD_TIME
ARG GIT_COMMIT
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT} -s -w" \
    -o /build/bin/iofog-agent ./cmd/iofog-agent

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT} -s -w" \
    -o /build/bin/iofog-agentd ./cmd/iofog-agentd

# Stage 2: Prepare dependencies from UBI Minimal (UBI Micro doesn't have microdnf)
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest AS ubi-dep

# Install necessary dependencies
RUN microdnf install -y shadow-utils tzdata && \
    microdnf clean all && \
    rm -rf /var/cache/*

# Create user
RUN true && \
    useradd -r -U -s /usr/bin/nologin iofog-agent && \
    usermod -aG root,wheel iofog-agent && \
    true

# Stage 3: Stage dependencies for UBI Micro
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest AS ubi-dep-staging

# Copy timezone data
COPY --from=ubi-dep /usr/share/zoneinfo /staging/usr/share/zoneinfo

# Copy user/group files
COPY --from=ubi-dep /etc/passwd /staging/etc/passwd
COPY --from=ubi-dep /etc/group /staging/etc/group
COPY --from=ubi-dep /etc/shadow /staging/etc/shadow

# CA certificates are already in UBI Micro, no need to copy

# Stage 4: Stage packaging files
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest AS builder-staging

# Copy binaries
COPY --from=builder /build/bin/iofog-agent /staging/usr/bin/iofog-agent
COPY --from=builder /build/bin/iofog-agentd /staging/usr/bin/iofog-agentd

# Copy packaging files
COPY --from=builder /build/packaging/iofog-agent/usr /staging/usr
COPY --from=builder /build/packaging/iofog-agent/etc /staging/etc

# Stage 5: Final stage using UBI Micro
FROM registry.access.redhat.com/ubi9/ubi-micro:latest

# Copy dependencies from staging
COPY --from=ubi-dep-staging /staging/ /

# Copy all files from builder staging stage in a single layer
COPY --from=builder-staging /staging/ /

# Set timezone and environment
ENV LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    TZ=UTC \
    IOFOG_DAEMON=container

# Setup directories and permissions
RUN true && \
    mv /etc/iofog-agent/config_new.yaml /etc/iofog-agent/config.yaml 2>/dev/null || true && \
    mv /etc/iofog-agent/cert_new.crt /etc/iofog-agent/cert.crt 2>/dev/null || true && \
    mkdir -p /var/backups/iofog-agent && \
    mkdir -p /var/log/iofog-agent && \
    mkdir -p /var/lib/iofog-agent && \
    mkdir -p /var/run/iofog-agent && \
    mkdir -p /var/log/iofog-microservices && \
    chown -R :iofog-agent /etc/iofog-agent && \
    chown -R :iofog-agent /var/log/iofog-agent && \
    chown -R :iofog-agent /var/lib/iofog-agent && \
    chown -R :iofog-agent /var/run/iofog-agent && \
    chown -R :iofog-agent /var/backups/iofog-agent && \
    chown -R :iofog-agent /usr/share/iofog-agent && \
    chmod 774 -R /etc/iofog-agent && \
    chmod 774 -R /var/log/iofog-agent && \
    chmod 774 -R /var/lib/iofog-agent && \
    chmod 774 -R /var/run/iofog-agent && \
    chmod 774 -R /var/backups/iofog-agent && \
    chmod 754 -R /usr/share/iofog-agent && \
    chmod 754 /usr/bin/iofog-agent && \
    chmod 754 /usr/bin/iofog-agentd && \
    chown :iofog-agent /usr/bin/iofog-agent && \
    chown :iofog-agent /usr/bin/iofog-agentd && \
    true

# Copy entrypoint script
COPY entrypoint.sh /etc/iofog-agent/entrypoint.sh
RUN chmod +x /etc/iofog-agent/entrypoint.sh

COPY LICENSE /licenses/LICENSE
# Set labels
LABEL org.opencontainers.image.description="ioFog Agent - Edge computing agent"
LABEL org.opencontainers.image.source="https://github.com/eclipse-iofog/agent"
LABEL org.opencontainers.image.licenses="EPL-2.0"

# Set entrypoint
ENTRYPOINT ["/etc/iofog-agent/entrypoint.sh"]
