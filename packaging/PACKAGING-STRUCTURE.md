# Packaging Structure Documentation

## Overview
This document describes the packaging structure for the ioFog Agent Go implementation, which matches the Java agent packaging structure.

## Directory Structure

```
packaging/iofog-agent/
├── etc/
│   ├── bash_completion.d/
│   │   └── iofog-agent          # Bash completion script
│   ├── iofog-agent/
│   │   ├── config_new.yaml      # Default configuration (renamed to config.yaml on install)
│   │   └── cert_new.crt         # Placeholder for CA certificate (renamed to cert.crt on install)
│   └── systemd/
│       └── system/
│           └── iofog-agent.service  # Systemd service file
└── usr/
    ├── bin/
    │   ├── iofog-agent          # CLI binary (installed here)
    │   └── iofog-agentd         # Daemon binary (installed here)
    └── share/
        └── iofog-agent/
            ├── upgrade.sh       # Upgrade script
            └── rollback.sh      # Rollback script
```

## Installation Paths

### Binaries
- `/usr/bin/iofog-agent` - CLI binary
- `/usr/bin/iofog-agentd` - Daemon binary
- `/usr/local/bin/iofog-agent` - Symlink to `/usr/bin/iofog-agent`

### Configuration
- `/etc/iofog-agent/config.yaml` - Main configuration file (created from config_new.yaml)
- `/etc/iofog-agent/cert.crt` - Controller CA certificate (created from cert_new.crt)
- `/etc/iofog-agent/local-api` - Local API token (generated on install)

### Scripts
- `/usr/share/iofog-agent/upgrade.sh` - Upgrade script
- `/usr/share/iofog-agent/rollback.sh` - Rollback script

### System Files
- `/etc/systemd/system/iofog-agent.service` - Systemd service file
- `/etc/bash_completion.d/iofog-agent` - Bash completion

### Runtime Directories
- `/var/lib/iofog-agent/` - Data directory
- `/var/log/iofog-agent/` - Log directory
- `/var/run/iofog-agent/` - Runtime directory
- `/var/backups/iofog-agent/` - Backup directory
- `/var/log/iofog-microservices/` - Microservice logs

## Post-Installation Process

### DEB Package (postinst)
1. Create user and group `iofog-agent`
2. Create runtime directories
3. Rename `config_new.yaml` → `config.yaml` (if config.yaml doesn't exist)
4. Rename `cert_new.crt` → `cert.crt` (if cert.crt doesn't exist)
5. Generate local API token if it doesn't exist
6. Set ownership and permissions
7. Enable systemd service

### RPM Package (%post)
1. Create user and group `iofog-agent`
2. Rename config and cert files
3. Generate local API token
4. Set ownership and permissions
5. Enable systemd service

## File Permissions

- `/etc/iofog-agent/` - 774 (rwxrwxr--)
- `/var/log/iofog-agent/` - 774
- `/var/lib/iofog-agent/` - 774
- `/var/run/iofog-agent/` - 774
- `/var/backups/iofog-agent/` - 774
- `/usr/share/iofog-agent/` - 754 (rwxr-xr--)
- `/usr/bin/iofog-agent` - 754
- `/usr/bin/iofog-agentd` - 754

All directories and files are owned by group `iofog-agent`.

## Docker Image Structure

The Dockerfile follows the same structure:
- Copies binaries to `/usr/bin/`
- Copies packaging files from `packaging/iofog-agent/`
- Sets up directories and permissions
- Renames config_new.yaml → config.yaml and cert_new.crt → cert.crt

## Comparison with Java Agent

The Go agent packaging structure matches the Java agent structure:
- Same directory layout
- Same file locations
- Same post-installation process
- Same permissions model
- Compatible upgrade/rollback scripts

## Building Packages

### DEB Package
```bash
cd packaging/deb
./build.sh <version> <arch>
```

### RPM Package
```bash
rpmbuild -ba packaging/rpm/iofog-agent.spec
```

## Notes

- `config_new.yaml` and `cert_new.crt` are renamed during installation to preserve existing configurations
- Local API token is generated randomly on first install
- All scripts are executable and owned by root:iofog-agent group
- Systemd service runs as root (required for Docker access)
