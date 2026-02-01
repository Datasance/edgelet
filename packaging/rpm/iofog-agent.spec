Name:           iofog-agent
Version:        1.0.0
Release:        1%{?dist}
Summary:        ioFog Agent - Edge computing agent for ioFog platform
License:        EPL-2.0
URL:            https://github.com/eclipse-iofog/agent
Source0:        %{name}-%{version}.tar.gz

Requires:       docker >= 20.10
Requires:       ca-certificates

%description
The ioFog Agent manages microservices on edge devices, providing
container orchestration, message routing, and communication with
the ioFog Controller.

This is the Go implementation of the ioFog Agent, providing improved
performance and reduced resource usage compared to the Java version.

%prep
%setup -q

%build
# Binaries are pre-built

%install
mkdir -p %{buildroot}/usr/bin
mkdir -p %{buildroot}/usr/share/iofog-agent
mkdir -p %{buildroot}/etc/systemd/system
mkdir -p %{buildroot}/etc/iofog-agent
mkdir -p %{buildroot}/etc/bash_completion.d

install -m 755 build/iofog-agent-linux-%{_arch} %{buildroot}/usr/bin/iofog-agent
install -m 755 build/iofog-agentd-linux-%{_arch} %{buildroot}/usr/bin/iofog-agentd

# Copy systemd service
if [ -f packaging/systemd/iofog-agent.service ]; then
    install -m 644 packaging/systemd/iofog-agent.service %{buildroot}/etc/systemd/system/iofog-agent.service
else
    cat > %{buildroot}/etc/systemd/system/iofog-agent.service <<EOF
[Unit]
Description=ioFog Agent
After=docker.service
Requires=docker.service

[Service]
Type=simple
ExecStart=/usr/bin/iofog-agentd
Restart=always
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
EOF
fi

# Copy config files
if [ -f packaging/iofog-agent/etc/iofog-agent/config_new.yaml ]; then
    install -m 644 packaging/iofog-agent/etc/iofog-agent/config_new.yaml %{buildroot}/etc/iofog-agent/
fi

if [ -f packaging/iofog-agent/etc/iofog-agent/cert_new.crt ]; then
    install -m 644 packaging/iofog-agent/etc/iofog-agent/cert_new.crt %{buildroot}/etc/iofog-agent/
fi

# Copy bash completion
if [ -f packaging/iofog-agent/etc/bash_completion.d/iofog-agent ]; then
    install -m 644 packaging/iofog-agent/etc/bash_completion.d/iofog-agent %{buildroot}/etc/bash_completion.d/
fi

# Copy upgrade/rollback scripts
if [ -f packaging/iofog-agent/usr/share/iofog-agent/upgrade.sh ]; then
    install -m 755 packaging/iofog-agent/usr/share/iofog-agent/upgrade.sh %{buildroot}/usr/share/iofog-agent/
fi

if [ -f packaging/iofog-agent/usr/share/iofog-agent/rollback.sh ]; then
    install -m 755 packaging/iofog-agent/usr/share/iofog-agent/rollback.sh %{buildroot}/usr/share/iofog-agent/
fi

%pre
# Create directories
mkdir -p /etc/iofog-agent
mkdir -p /var/lib/iofog-agent
mkdir -p /var/log/iofog-agent
mkdir -p /var/run/iofog-agent
mkdir -p /var/backups/iofog-agent

# Create user and group
if ! getent group iofog-agent > /dev/null 2>&1; then
    groupadd -r iofog-agent
fi
if ! getent passwd iofog-agent > /dev/null 2>&1; then
    useradd -r -g iofog-agent iofog-agent
fi

%post
# Handle config_new.yaml -> config.yaml
if [ -f /etc/iofog-agent/config.yaml ]; then
   rm -f /etc/iofog-agent/config_new.yaml
else
   if [ -f /etc/iofog-agent/config_new.yaml ]; then
       mv /etc/iofog-agent/config_new.yaml /etc/iofog-agent/config.yaml
   fi
fi

# Handle cert_new.crt -> cert.crt
if [ -f /etc/iofog-agent/cert.crt ]; then
   rm -f /etc/iofog-agent/cert_new.crt
else
   if [ -f /etc/iofog-agent/cert_new.crt ]; then
       mv /etc/iofog-agent/cert_new.crt /etc/iofog-agent/cert.crt
   fi
fi

# Generate local API token if it doesn't exist
if [ ! -f /etc/iofog-agent/local-api ]; then
    </dev/urandom tr -dc A-Za-z0-9 | head -c32 > /etc/iofog-agent/local-api
    chmod 600 /etc/iofog-agent/local-api
fi

# Set ownership and permissions
chown -R :iofog-agent /etc/iofog-agent
chown -R :iofog-agent /var/log/iofog-agent
chown -R :iofog-agent /var/lib/iofog-agent
chown -R :iofog-agent /var/run/iofog-agent
chown -R :iofog-agent /var/backups/iofog-agent
chown -R :iofog-agent /usr/share/iofog-agent 2>/dev/null || true

chmod 774 -R /etc/iofog-agent
chmod 774 -R /var/log/iofog-agent
chmod 774 -R /var/lib/iofog-agent
chmod 774 -R /var/run/iofog-agent
chmod 774 -R /var/backups/iofog-agent
chmod 754 -R /usr/share/iofog-agent 2>/dev/null || true
chmod 774 /etc/systemd/system/iofog-agent.service 2>/dev/null || true
chmod 754 /usr/bin/iofog-agent 2>/dev/null || true
chown :iofog-agent /usr/bin/iofog-agent 2>/dev/null || true

# Enable service
systemctl daemon-reload
systemctl enable iofog-agent.service || true

%preun
if [ $1 -eq 0 ]; then
    systemctl stop iofog-agent.service || true
fi

%postun
if [ $1 -eq 0 ]; then
    systemctl daemon-reload || true
fi

%files
/usr/bin/iofog-agent
/usr/bin/iofog-agentd
/usr/share/iofog-agent/upgrade.sh
/usr/share/iofog-agent/rollback.sh
/etc/systemd/system/iofog-agent.service
/etc/iofog-agent/config_new.yaml
/etc/iofog-agent/cert_new.crt
/etc/bash_completion.d/iofog-agent

%changelog
* Mon Jan 01 2024 Eclipse ioFog <iofog@eclipse.org> - 1.0.0-1
- Initial release of Go implementation
