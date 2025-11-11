//go:build linux

package crep

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/privileged"
)

// AddMITMRootCertIntoSystem 将 MITM 根证书添加到 Linux 系统信任库
// 支持多个发行版的证书安装路径
func AddMITMRootCertIntoSystem() error {
	ctx := context.Background()
	executor := privileged.NewExecutor("Install MITM Root Certificate")

	// 初始化 MITM 证书
	InitMITMCert()

	// 获取根证书
	ca, _, err := GetDefaultCaAndKey()
	if err != nil {
		return utils.Errorf("failed to get MITM root certificate: %v", err)
	}

	// 创建临时文件保存证书
	tmpFile := filepath.Join(os.TempDir(), "yaklang-mitm-ca.crt")
	err = os.WriteFile(tmpFile, ca, 0644)
	if err != nil {
		return utils.Errorf("failed to write certificate to temp file: %v", err)
	}
	defer os.Remove(tmpFile)

	// 构建安装脚本
	script := buildLinuxInstallCertScript(tmpFile)

	log.Info("installing MITM root certificate into Linux system trust store")
	output, err := executor.Execute(ctx, script,
		privileged.WithDescription("Install MITM root certificate for secure traffic inspection"),
		privileged.WithTitle("Install MITM Certificate"),
	)

	if err != nil {
		return utils.Errorf("failed to install certificate: %s, output: %s", err, string(output))
	}

	log.Infof("certificate installation output: %s", string(output))
	return nil
}

// WithdrawMITMRootCertFromSystem 从 Linux 系统信任库中移除 MITM 根证书
func WithdrawMITMRootCertFromSystem() error {
	ctx := context.Background()
	executor := privileged.NewExecutor("Remove MITM Root Certificate")

	// 构建移除脚本
	script := buildLinuxRemoveCertScript()

	log.Info("removing MITM root certificate from Linux system trust store")
	output, err := executor.Execute(ctx, script,
		privileged.WithDescription("Remove MITM root certificate from system"),
		privileged.WithTitle("Remove MITM Certificate"),
	)

	if err != nil {
		return utils.Errorf("failed to remove certificate: %s, output: %s", err, string(output))
	}

	log.Infof("certificate removal output: %s", string(output))
	return nil
}

// buildLinuxInstallCertScript 构建 Linux 证书安装脚本
// 支持多个发行版：Debian/Ubuntu, RHEL/CentOS/Fedora, Arch等
func buildLinuxInstallCertScript(certPath string) string {
	const certFileName = "yaklang-mitm-ca.crt"
	
	script := fmt.Sprintf(`#!/bin/bash
# MITM Root Certificate Installation Script for Linux
# Supports multiple distributions

set -e

echo "Starting MITM certificate installation..."

CERT_PATH="%s"
CERT_NAME="%s"

# Check if certificate file exists
if [ ! -f "$CERT_PATH" ]; then
    echo "Error: Certificate file not found: $CERT_PATH"
    exit 1
fi

echo "Certificate file: $CERT_PATH"

# Detect distribution and install accordingly
install_cert() {
    if [ -f /etc/debian_version ]; then
        # Debian/Ubuntu
        echo "Detected Debian/Ubuntu system"
        CERT_DIR="/usr/local/share/ca-certificates"
        mkdir -p "$CERT_DIR"
        cp "$CERT_PATH" "$CERT_DIR/$CERT_NAME"
        update-ca-certificates
        echo "✓ Certificate installed using update-ca-certificates"
        
    elif [ -f /etc/redhat-release ]; then
        # RHEL/CentOS/Fedora
        echo "Detected RHEL/CentOS/Fedora system"
        CERT_DIR="/etc/pki/ca-trust/source/anchors"
        mkdir -p "$CERT_DIR"
        cp "$CERT_PATH" "$CERT_DIR/$CERT_NAME"
        update-ca-trust
        echo "✓ Certificate installed using update-ca-trust"
        
    elif [ -f /etc/arch-release ]; then
        # Arch Linux
        echo "Detected Arch Linux system"
        CERT_DIR="/etc/ca-certificates/trust-source/anchors"
        mkdir -p "$CERT_DIR"
        cp "$CERT_PATH" "$CERT_DIR/$CERT_NAME"
        trust extract-compat
        echo "✓ Certificate installed using trust extract-compat"
        
    elif [ -f /etc/alpine-release ]; then
        # Alpine Linux
        echo "Detected Alpine Linux system"
        CERT_DIR="/usr/local/share/ca-certificates"
        mkdir -p "$CERT_DIR"
        cp "$CERT_PATH" "$CERT_DIR/$CERT_NAME"
        update-ca-certificates
        echo "✓ Certificate installed using update-ca-certificates"
        
    else
        # Generic Linux - try common methods
        echo "Unknown distribution, trying generic installation..."
        
        if command -v update-ca-certificates >/dev/null 2>&1; then
            CERT_DIR="/usr/local/share/ca-certificates"
            mkdir -p "$CERT_DIR"
            cp "$CERT_PATH" "$CERT_DIR/$CERT_NAME"
            update-ca-certificates
            echo "✓ Certificate installed using update-ca-certificates"
            
        elif command -v update-ca-trust >/dev/null 2>&1; then
            CERT_DIR="/etc/pki/ca-trust/source/anchors"
            mkdir -p "$CERT_DIR"
            cp "$CERT_PATH" "$CERT_DIR/$CERT_NAME"
            update-ca-trust
            echo "✓ Certificate installed using update-ca-trust"
            
        else
            echo "✗ Could not find certificate update tool"
            echo "Please install ca-certificates package for your distribution"
            exit 1
        fi
    fi
}

# Remove old certificate if exists
remove_old_cert() {
    local cert_dirs=(
        "/usr/local/share/ca-certificates"
        "/etc/pki/ca-trust/source/anchors"
        "/etc/ca-certificates/trust-source/anchors"
    )
    
    for dir in "${cert_dirs[@]}"; do
        if [ -f "$dir/$CERT_NAME" ]; then
            echo "Removing old certificate from $dir..."
            rm -f "$dir/$CERT_NAME"
        fi
    done
}

# Remove old certificate first
remove_old_cert

# Install new certificate
install_cert

echo "MITM certificate installation completed successfully!"
`, certPath, certFileName)

	return script
}

// buildLinuxRemoveCertScript 构建 Linux 证书移除脚本
func buildLinuxRemoveCertScript() string {
	const certFileName = "yaklang-mitm-ca.crt"
	
	script := fmt.Sprintf(`#!/bin/bash
# MITM Root Certificate Removal Script for Linux

set -e

echo "Starting MITM certificate removal..."

CERT_NAME="%s"

# Certificate directories for different distributions
CERT_DIRS=(
    "/usr/local/share/ca-certificates"
    "/etc/pki/ca-trust/source/anchors"
    "/etc/ca-certificates/trust-source/anchors"
)

FOUND=0

# Remove certificate from all possible locations
for DIR in "${CERT_DIRS[@]}"; do
    if [ -f "$DIR/$CERT_NAME" ]; then
        echo "Found certificate in $DIR"
        rm -f "$DIR/$CERT_NAME"
        FOUND=1
        echo "✓ Certificate removed from $DIR"
    fi
done

if [ $FOUND -eq 0 ]; then
    echo "Certificate not found in system trust store"
    exit 0
fi

# Update certificate trust store
update_trust() {
    if [ -f /etc/debian_version ]; then
        update-ca-certificates --fresh
        echo "✓ Updated Debian/Ubuntu trust store"
        
    elif [ -f /etc/redhat-release ]; then
        update-ca-trust
        echo "✓ Updated RHEL/CentOS/Fedora trust store"
        
    elif [ -f /etc/arch-release ]; then
        trust extract-compat
        echo "✓ Updated Arch Linux trust store"
        
    elif [ -f /etc/alpine-release ]; then
        update-ca-certificates
        echo "✓ Updated Alpine Linux trust store"
        
    else
        if command -v update-ca-certificates >/dev/null 2>&1; then
            update-ca-certificates --fresh
            echo "✓ Updated trust store using update-ca-certificates"
        elif command -v update-ca-trust >/dev/null 2>&1; then
            update-ca-trust
            echo "✓ Updated trust store using update-ca-trust"
        fi
    fi
}

update_trust

echo "MITM certificate removal completed successfully!"
`, certFileName)

	return script
}

