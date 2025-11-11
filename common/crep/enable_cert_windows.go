//go:build windows

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

// AddMITMRootCertIntoSystem 将 MITM 根证书添加到 Windows 系统信任库
// 使用 certutil 命令行工具操作证书存储
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
	script := buildWindowsInstallCertScript(tmpFile)

	log.Info("installing MITM root certificate into Windows system trust store")
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

// WithdrawMITMRootCertFromSystem 从 Windows 系统信任库中移除 MITM 根证书
func WithdrawMITMRootCertFromSystem() error {
	ctx := context.Background()
	executor := privileged.NewExecutor("Remove MITM Root Certificate")

	// 构建移除脚本
	script := buildWindowsRemoveCertScript()

	log.Info("removing MITM root certificate from Windows system trust store")
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

// buildWindowsInstallCertScript 构建 Windows 证书安装脚本
// 使用 certutil 和 PowerShell 来操作证书
func buildWindowsInstallCertScript(certPath string) string {
	// Windows 路径需要反斜杠
	certPath = filepath.ToSlash(certPath)

	script := fmt.Sprintf(`@echo off
REM MITM Root Certificate Installation Script for Windows
setlocal enabledelayedexpansion

echo Starting MITM certificate installation...

set "CERT_PATH=%s"
set "CERT_NAME=Yaklang MITM CA"

REM Check if certificate file exists
if not exist "%%CERT_PATH%%" (
    echo Error: Certificate file not found: %%CERT_PATH%%
    exit /b 1
)

echo Certificate file: %%CERT_PATH%%

REM Check if certificate already exists
certutil -store -user Root "%%CERT_NAME%%" >nul 2>&1
if %%ERRORLEVEL%% equ 0 (
    echo Certificate already exists in user trust store, removing old version...
    certutil -delstore -user Root "%%CERT_NAME%%" >nul 2>&1
)

certutil -store Root "%%CERT_NAME%%" >nul 2>&1
if %%ERRORLEVEL%% equ 0 (
    echo Certificate already exists in system trust store, removing old version...
    certutil -delstore Root "%%CERT_NAME%%" >nul 2>&1
)

REM Import certificate into Root store (Trusted Root Certification Authorities)
echo Importing certificate into system trust store...
certutil -addstore Root "%%CERT_PATH%%"

if %%ERRORLEVEL%% equ 0 (
    echo [+] Certificate installed and trusted successfully!
) else (
    echo [-] Failed to install certificate
    exit /b 1
)

REM Verify installation
certutil -store Root "%%CERT_NAME%%" >nul 2>&1
if %%ERRORLEVEL%% equ 0 (
    echo [+] Certificate verification passed
) else (
    echo [-] Certificate verification failed
    exit /b 1
)

echo MITM certificate installation completed successfully!
exit /b 0
`, certPath)

	return script
}

// buildWindowsRemoveCertScript 构建 Windows 证书移除脚本
func buildWindowsRemoveCertScript() string {
	script := `@echo off
REM MITM Root Certificate Removal Script for Windows
setlocal enabledelayedexpansion

echo Starting MITM certificate removal...

set "CERT_NAME=Yaklang MITM CA"
set "FOUND=0"

REM Check and remove from user Root store
certutil -store -user Root "%CERT_NAME%" >nul 2>&1
if %ERRORLEVEL% equ 0 (
    echo Found certificate in user trust store
    certutil -delstore -user Root "%CERT_NAME%"
    set "FOUND=1"
    echo [+] Certificate removed from user trust store
)

REM Check and remove from system Root store
certutil -store Root "%CERT_NAME%" >nul 2>&1
if %ERRORLEVEL% equ 0 (
    echo Found certificate in system trust store
    certutil -delstore Root "%CERT_NAME%"
    set "FOUND=1"
    echo [+] Certificate removed from system trust store
)

if %FOUND% equ 0 (
    echo Certificate not found in trust stores
    exit /b 0
)

REM Verify removal
certutil -store Root "%CERT_NAME%" >nul 2>&1
if %ERRORLEVEL% neq 0 (
    certutil -store -user Root "%CERT_NAME%" >nul 2>&1
    if %ERRORLEVEL% neq 0 (
        echo [+] Certificate removal verified
    ) else (
        echo [-] Certificate still exists in user store after removal
        exit /b 1
    )
) else (
    echo [-] Certificate still exists in system store after removal
    exit /b 1
)

echo MITM certificate removal completed successfully!
exit /b 0
`

	return script
}
