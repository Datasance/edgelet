#!/bin/bash
# Security audit script for ioFog Agent

set -e

echo "=== Security Audit for ioFog Agent ==="
echo ""

# Check for security tools
echo "1. Checking for security tools..."

if ! command -v nancy &> /dev/null; then
    echo "   Installing nancy..."
    go install github.com/sonatypecommunity/nancy@latest
fi

if ! command -v gosec &> /dev/null; then
    echo "   Installing gosec..."
    go install github.com/securego/gosec/v2/cmd/gosec@latest
fi

echo "   ✓ Security tools ready"
echo ""

# Dependency audit
echo "2. Running dependency audit..."
if go list -json -deps ./... 2>/dev/null | nancy sleuth; then
    echo "   ✓ No known vulnerabilities in dependencies"
else
    echo "   ⚠ Some vulnerabilities found. Review output above."
fi
echo ""

# Code security scan
echo "3. Running code security scan (gosec)..."
if gosec ./... 2>/dev/null; then
    echo "   ✓ Code security scan completed"
else
    echo "   ⚠ Some security issues found. Review output above."
fi
echo ""

# Check for hardcoded secrets
echo "4. Checking for hardcoded secrets..."
if grep -r "password.*=.*['\"].*[a-zA-Z0-9]" --include="*.go" . 2>/dev/null | grep -v "test" | grep -v "example"; then
    echo "   ⚠ Potential hardcoded passwords found"
else
    echo "   ✓ No obvious hardcoded passwords found"
fi
echo ""

# Check for insecure TLS
echo "5. Checking for insecure TLS configurations..."
if grep -r "InsecureSkipVerify.*true" --include="*.go" . 2>/dev/null | grep -v "test"; then
    echo "   ⚠ Insecure TLS configurations found"
else
    echo "   ✓ No insecure TLS configurations found"
fi
echo ""

# Check certificate handling
echo "6. Reviewing certificate handling..."
if grep -r "certificate" --include="*.go" internal/auth/ | grep -v "test"; then
    echo "   ✓ Certificate handling code found"
else
    echo "   ⚠ Certificate handling not found"
fi
echo ""

# Check input validation
echo "7. Checking for input validation..."
if grep -r "Validate\|Sanitize" --include="*.go" . | grep -v "test" | head -5; then
    echo "   ✓ Input validation found"
else
    echo "   ⚠ Limited input validation found"
fi
echo ""

echo "=== Security Audit Complete ==="
echo ""
echo "Review the output above and address any issues found."
