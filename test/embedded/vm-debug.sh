#!/bin/bash
echo "=== ctr binary ==="
which ctr 2>&1 || echo "ctr not installed"
ctr --version 2>&1 || true

echo ""
echo "=== socket permissions ==="
ls -la /run/edgelet/containerd.sock

echo ""
echo "=== ctr version via socket ==="
ctr --address /run/edgelet/containerd.sock version 2>&1

echo ""
echo "=== ctr namespaces ==="
ctr --address /run/edgelet/containerd.sock namespaces list 2>&1

echo ""
echo "=== network connectivity ==="
curl -s --connect-timeout 5 https://registry-1.docker.io/v2/ 2>&1 | head -3 || echo "registry not reachable"

echo ""
echo "=== ctr pull alpine ==="
ctr --address /run/edgelet/containerd.sock --namespace iofog \
    images pull docker.io/library/alpine:3.19 2>&1
echo "pull exit: $?"
