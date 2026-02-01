#!/bin/bash
set -e

VERSION=${1:-"1.0.0"}
ARCH=${2:-"amd64"}
BUILD_DIR="../../build"
PACKAGE_DIR="iofog-agent_${VERSION}_${ARCH}"
PACKAGING_ROOT="../../packaging/iofog-agent"

echo "Building DEB package for version ${VERSION} (${ARCH})..."

# Create package directory structure
mkdir -p "${PACKAGE_DIR}/DEBIAN"
mkdir -p "${PACKAGE_DIR}/usr/bin"
mkdir -p "${PACKAGE_DIR}/usr/share/iofog-agent"
mkdir -p "${PACKAGE_DIR}/etc/systemd/system"
mkdir -p "${PACKAGE_DIR}/etc/iofog-agent"
mkdir -p "${PACKAGE_DIR}/etc/bash_completion.d"

# Copy binaries
if [ -f "${BUILD_DIR}/iofog-agent-linux-${ARCH}" ]; then
    cp "${BUILD_DIR}/iofog-agent-linux-${ARCH}" "${PACKAGE_DIR}/usr/bin/iofog-agent"
elif [ -f "${BUILD_DIR}/iofog-agent" ]; then
    cp "${BUILD_DIR}/iofog-agent" "${PACKAGE_DIR}/usr/bin/iofog-agent"
else
    echo "Error: iofog-agent binary not found"
    exit 1
fi

if [ -f "${BUILD_DIR}/iofog-agentd-linux-${ARCH}" ]; then
    cp "${BUILD_DIR}/iofog-agentd-linux-${ARCH}" "${PACKAGE_DIR}/usr/bin/iofog-agentd"
elif [ -f "${BUILD_DIR}/iofog-agentd" ]; then
    cp "${BUILD_DIR}/iofog-agentd" "${PACKAGE_DIR}/usr/bin/iofog-agentd"
else
    echo "Error: iofog-agentd binary not found"
    exit 1
fi

chmod +x "${PACKAGE_DIR}/usr/bin/iofog-agent"
chmod +x "${PACKAGE_DIR}/usr/bin/iofog-agentd"

# Copy systemd service file
if [ -f "${PACKAGING_ROOT}/../systemd/iofog-agent.service" ]; then
    cp "${PACKAGING_ROOT}/../systemd/iofog-agent.service" "${PACKAGE_DIR}/etc/systemd/system/iofog-agent.service"
else
    cat > "${PACKAGE_DIR}/etc/systemd/system/iofog-agent.service" <<EOF
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
if [ -f "${PACKAGING_ROOT}/etc/iofog-agent/config_new.yaml" ]; then
    cp "${PACKAGING_ROOT}/etc/iofog-agent/config_new.yaml" "${PACKAGE_DIR}/etc/iofog-agent/"
fi

if [ -f "${PACKAGING_ROOT}/etc/iofog-agent/cert_new.crt" ]; then
    cp "${PACKAGING_ROOT}/etc/iofog-agent/cert_new.crt" "${PACKAGE_DIR}/etc/iofog-agent/"
fi

# Copy bash completion
if [ -f "${PACKAGING_ROOT}/etc/bash_completion.d/iofog-agent" ]; then
    cp "${PACKAGING_ROOT}/etc/bash_completion.d/iofog-agent" "${PACKAGE_DIR}/etc/bash_completion.d/"
fi

# Copy upgrade/rollback scripts
if [ -f "${PACKAGING_ROOT}/usr/share/iofog-agent/upgrade.sh" ]; then
    cp "${PACKAGING_ROOT}/usr/share/iofog-agent/upgrade.sh" "${PACKAGE_DIR}/usr/share/iofog-agent/"
    chmod +x "${PACKAGE_DIR}/usr/share/iofog-agent/upgrade.sh"
fi

if [ -f "${PACKAGING_ROOT}/usr/share/iofog-agent/rollback.sh" ]; then
    cp "${PACKAGING_ROOT}/usr/share/iofog-agent/rollback.sh" "${PACKAGE_DIR}/usr/share/iofog-agent/"
    chmod +x "${PACKAGE_DIR}/usr/share/iofog-agent/rollback.sh"
fi

# Copy control file
sed "s/Version: .*/Version: ${VERSION}/" control > "${PACKAGE_DIR}/DEBIAN/control"
sed -i "s/Architecture: .*/Architecture: ${ARCH}/" "${PACKAGE_DIR}/DEBIAN/control"

# Copy scripts
cp postinst "${PACKAGE_DIR}/DEBIAN/"
cp prerm "${PACKAGE_DIR}/DEBIAN/"
cp postrm "${PACKAGE_DIR}/DEBIAN/"
chmod +x "${PACKAGE_DIR}/DEBIAN/"*

# Build package
dpkg-deb --build "${PACKAGE_DIR}"

# Move to build directory
mv "${PACKAGE_DIR}.deb" "${BUILD_DIR}/"

# Cleanup
rm -rf "${PACKAGE_DIR}"

echo "Package built: ${BUILD_DIR}/${PACKAGE_DIR}.deb"
