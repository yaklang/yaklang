//go:build darwin

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

// AddMITMRootCertIntoSystem 将 MITM 根证书添加到 macOS 系统钥匙串并设置为信任
// 这个函数会：
// 1. 获取或生成 MITM 根证书
// 2. 将证书导入到系统钥匙串
// 3. 设置证书为受信任的根证书
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
	script := buildMacOSInstallCertScript(tmpFile)

	log.Info("installing MITM root certificate into macOS system keychain.")
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

// WithdrawMITMRootCertFromSystem 从 macOS 系统钥匙串中移除 MITM 根证书
func WithdrawMITMRootCertFromSystem() error {
	ctx := context.Background()
	executor := privileged.NewExecutor("Remove MITM Root Certificate")

	// 构建移除脚本
	script := buildMacOSRemoveCertScript()

	log.Info("removing MITM root certificate from macOS system keychain")
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

// buildMacOSInstallCertScript 构建 macOS 证书安装脚本
// 使用 security 命令行工具来操作钥匙串
func buildMacOSInstallCertScript(certPath string) string {
	const certName = "Yakit MITM Root CA"

	script := fmt.Sprintf(`#!/bin/zsh
# MITM Root Certificate Installation Script for macOS

echo "Starting MITM certificate installation..."

CERT_PATH="%s"
CERT_NAME="%s"
SYSTEM_KEYCHAIN="/Library/Keychains/System.keychain"
USER_KEYCHAIN="$HOME/Library/Keychains/login.keychain-db"

# Check if certificate file exists
if [ ! -f "$CERT_PATH" ]; then
    echo "Error: Certificate file not found"
    exit 1
fi

# Import certificate to system keychain
# Note: We don't remove old certificates - we just add a new one
# Multiple certificates with the same name can coexist
echo "Importing certificate to system keychain..."
if security import "$CERT_PATH" -k "$SYSTEM_KEYCHAIN" -A 2>&1; then
    echo "Certificate imported successfully"
else
    # Check if it already exists (error code 48 or similar)
    if security find-certificate -c "$CERT_NAME" "$SYSTEM_KEYCHAIN" >/dev/null 2>&1; then
        echo "Certificate already exists in keychain, adding another instance..."
        # Force import even if it exists
        security import "$CERT_PATH" -k "$SYSTEM_KEYCHAIN" -A 2>&1 || echo "Note: Certificate may already exist"
    fi
fi

# Verify
if security find-certificate -c "$CERT_NAME" "$SYSTEM_KEYCHAIN" >/dev/null 2>&1; then
    echo "Certificate verified in system keychain!"
else
    echo "Warning: Could not verify certificate installation"
fi
`, certPath, certName)

	return script
}

// buildMacOSRemoveCertScript 构建 macOS 证书移除脚本
func buildMacOSRemoveCertScript() string {
	const certName = "Yakit MITM Root CA"

	script := fmt.Sprintf(`#!/bin/zsh
# MITM Root Certificate Removal Script for macOS

echo "Starting MITM certificate removal..."

CERT_NAME="%s"
TOTAL_REMOVED=0

# List of keychains to check with proper paths
KEYCHAINS=(
    "/Library/Keychains/System.keychain"
    "$HOME/Library/Keychains/login.keychain-db"
)

echo "Searching for certificates named: $CERT_NAME"

# Remove from each keychain - need to loop because there might be multiple certificates
for KEYCHAIN in "${KEYCHAINS[@]}"; do
    if [ ! -f "$KEYCHAIN" ]; then
        continue
    fi
    
    echo "Checking keychain: $(basename $KEYCHAIN)"
    
    # Keep removing until no more certificates found
    REMOVED_FROM_THIS=0
    while true; do
        # Try to delete one certificate
        if security delete-certificate -c "$CERT_NAME" "$KEYCHAIN" 2>/dev/null; then
            REMOVED_FROM_THIS=$((REMOVED_FROM_THIS + 1))
            TOTAL_REMOVED=$((TOTAL_REMOVED + 1))
        else
            # No more certificates to delete from this keychain
            break
        fi
    done
    
    if [ $REMOVED_FROM_THIS -gt 0 ]; then
        echo "  ✓ Removed $REMOVED_FROM_THIS certificate(s) from $(basename $KEYCHAIN)"
    fi
done

if [ $TOTAL_REMOVED -eq 0 ]; then
    echo "No certificates found to remove"
else
    echo "Successfully removed $TOTAL_REMOVED certificate(s) in total"
fi
`, certName)

	return script
}
