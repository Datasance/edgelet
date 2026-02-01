# Deployment Guide

This guide covers deploying the ioFog Agent on various platforms.

## Prerequisites

- Linux (Ubuntu 20.04+, RHEL/CentOS 8+, Debian 11+)
- Docker 20.10+ or Podman 3.0+
- Root or sudo access
- Network connectivity to ioFog Controller

## Installation Methods

### Package Installation (Recommended)

#### Ubuntu/Debian

```bash
# Download DEB package
wget https://github.com/eclipse-iofog/agent/releases/download/vX.X.X/iofog-agent_X.X.X_amd64.deb

# Install
sudo dpkg -i iofog-agent_X.X.X_amd64.deb

# Install dependencies if needed
sudo apt-get install -f
```

#### RHEL/CentOS

```bash
# Download RPM package
wget https://github.com/eclipse-iofog/agent/releases/download/vX.X.X/iofog-agent-X.X.X-1.x86_64.rpm

# Install
sudo rpm -i iofog-agent-X.X.X-1.x86_64.rpm
```

### Binary Installation

```bash
# Download binary for your architecture
wget https://github.com/eclipse-iofog/agent/releases/download/vX.X.X/iofog-agent-linux-amd64

# Make executable
chmod +x iofog-agent-linux-amd64

# Install to system
sudo cp iofog-agent-linux-amd64 /usr/local/bin/iofog-agent
sudo cp iofog-agent-linux-amd64 /usr/local/bin/iofog-agentd
```

### Docker Installation

```bash
# Pull image
docker pull iofog/agent:latest

# Run container
docker run -d \
  --name iofog-agent \
  --privileged \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /etc/iofog-agent:/etc/iofog-agent \
  -v /var/lib/iofog-agent:/var/lib/iofog-agent \
  iofog/agent:latest
```

## Configuration

### Initial Configuration

Create configuration file at `/etc/iofog-agent/config.yaml`:

```yaml
controller: https://controller.example.com
deviceName: edge-device-01
logLevel: INFO
logDiskDirectory: /var/log/iofog-agent
```

### Provisioning

Provision the agent with the controller:

```bash
sudo iofog-agent provision <controller-url> <fog-token>
```

Or manually edit the configuration file with provisioning details.

## Service Management

### systemd Service

The package installation automatically creates a systemd service.

**Start service:**
```bash
sudo systemctl start iofog-agent
```

**Stop service:**
```bash
sudo systemctl stop iofog-agent
```

**Enable on boot:**
```bash
sudo systemctl enable iofog-agent
```

**Check status:**
```bash
sudo systemctl status iofog-agent
```

**View logs:**
```bash
sudo journalctl -u iofog-agent -f
```

### Manual Service File

If installing from binary, create a systemd service file:

```ini
[Unit]
Description=ioFog Agent
After=docker.service
Requires=docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/iofog-agentd
Restart=always
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
```

Save to `/etc/systemd/system/iofog-agent.service` and:

```bash
sudo systemctl daemon-reload
sudo systemctl enable iofog-agent
sudo systemctl start iofog-agent
```

## Docker Deployment

### Docker Compose

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  iofog-agent:
    image: iofog/agent:latest
    container_name: iofog-agent
    privileged: true
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /etc/iofog-agent:/etc/iofog-agent
      - /var/lib/iofog-agent:/var/lib/iofog-agent
    restart: unless-stopped
    environment:
      - LOG_LEVEL=INFO
```

Run:
```bash
docker-compose up -d
```

### Kubernetes Deployment

Create `k8s-deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: iofog-agent
spec:
  selector:
    matchLabels:
      app: iofog-agent
  template:
    metadata:
      labels:
        app: iofog-agent
    spec:
      containers:
      - name: iofog-agent
        image: iofog/agent:latest
        securityContext:
          privileged: true
        volumeMounts:
        - name: docker-sock
          mountPath: /var/run/docker.sock
        - name: config
          mountPath: /etc/iofog-agent
        - name: data
          mountPath: /var/lib/iofog-agent
      volumes:
      - name: docker-sock
        hostPath:
          path: /var/run/docker.sock
      - name: config
        hostPath:
          path: /etc/iofog-agent
      - name: data
        hostPath:
          path: /var/lib/iofog-agent
```

Deploy:
```bash
kubectl apply -f k8s-deployment.yaml
```

## Verification

### Check Agent Status

```bash
# CLI status
sudo iofog-agent status

# API status
curl http://localhost:54321/api/v3/status
```

### Check Logs

```bash
# systemd logs
sudo journalctl -u iofog-agent -f

# File logs
tail -f /var/log/iofog-agent/agent.log
```

### Test Connectivity

```bash
# Test Local API
curl http://localhost:54321/api/v3/status

# Test Docker
docker ps

# Test Controller connection
curl https://controller.example.com/api/v3/status
```

## Upgrades

### Package Upgrade

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install iofog-agent

# RHEL/CentOS
sudo yum update iofog-agent
```

### Binary Upgrade

```bash
# Stop service
sudo systemctl stop iofog-agent

# Backup
sudo cp /usr/local/bin/iofog-agent /usr/local/bin/iofog-agent.backup

# Install new binary
sudo cp iofog-agent-linux-amd64 /usr/local/bin/iofog-agent
sudo cp iofog-agent-linux-amd64 /usr/local/bin/iofog-agentd

# Start service
sudo systemctl start iofog-agent
```

### Docker Upgrade

```bash
# Pull new image
docker pull iofog/agent:latest

# Restart container
docker restart iofog-agent
```

## Uninstallation

### Package Uninstallation

```bash
# Ubuntu/Debian
sudo apt-get remove iofog-agent

# RHEL/CentOS
sudo rpm -e iofog-agent
```

### Binary Uninstallation

```bash
# Stop service
sudo systemctl stop iofog-agent
sudo systemctl disable iofog-agent

# Remove binaries
sudo rm /usr/local/bin/iofog-agent
sudo rm /usr/local/bin/iofog-agentd

# Remove service file
sudo rm /etc/systemd/system/iofog-agent.service
sudo systemctl daemon-reload
```

### Docker Uninstallation

```bash
# Stop and remove container
docker stop iofog-agent
docker rm iofog-agent

# Remove image (optional)
docker rmi iofog/agent:latest
```

## Troubleshooting

See [Troubleshooting Guide](troubleshooting.md) for common issues and solutions.

## Security Considerations

- Run agent with appropriate permissions
- Use TLS for controller communication
- Secure configuration files
- Regular security updates
- Network isolation when possible

## Performance Tuning

- Adjust log levels for production
- Configure resource limits
- Optimize Docker settings
- Monitor resource usage
