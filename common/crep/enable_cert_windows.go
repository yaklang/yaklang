//go:build windows

package crep

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/privileged"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"golang.org/x/sys/windows"
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

	// 创建临时文件保存证书。
	// 使用 Yakit 自己管理的基础临时目录而非 os.TempDir()：os.TempDir() 可能
	// 指向含非 ASCII（如中文用户名）或被自定义过的路径，在中文 Win7 的 GBK
	// 代码页下，把这些路径写进批处理脚本时会出现 UTF-8/GBK 编码错乱。
	tmpFile := filepath.Join(consts.GetDefaultYakitBaseTempDir(), "yaklang-mitm-ca.crt")
	err = os.WriteFile(tmpFile, ca, 0644)
	if err != nil {
		return utils.Errorf("failed to write certificate to temp file: %v", err)
	}
	defer os.Remove(tmpFile)

	// 将证书路径转换为 8.3 短路径（纯 ASCII），避免批处理脚本里的非 ASCII 路径
	// 在 cmd 的 OEM 代码页下被错误解码，导致 certutil 找不到证书文件。这是
	// best-effort：若卷上禁用了 8.3 短文件名生成则回退到原始长路径。
	certPathForScript := windowsShortPath(tmpFile)

	// 构建安装脚本
	script := buildWindowsInstallCertScript(certPathForScript)

	log.Info("installing MITM root certificate into Windows system trust store")
	output, err := executor.Execute(ctx, script,
		privileged.WithDescription("Install MITM root certificate for secure traffic inspection"),
		privileged.WithTitle("Install MITM Certificate"),
	)

	decodedOutput := decodeWindowsConsoleOutput(output)
	if err != nil {
		return utils.Errorf("failed to install certificate: %s, output: %s", err, decodedOutput)
	}

	log.Infof("certificate installation output: %s", decodedOutput)
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

	decodedOutput := decodeWindowsConsoleOutput(output)
	if err != nil {
		return utils.Errorf("failed to remove certificate: %s, output: %s", err, decodedOutput)
	}

	log.Infof("certificate removal output: %s", decodedOutput)
	return nil
}

// buildWindowsInstallCertScript 构建 Windows 证书安装脚本
// 使用 certutil 和 PowerShell 来操作证书
func buildWindowsInstallCertScript(certPath string) string {
	// certPath 已是 8.3 短路径（纯 ASCII），保留 Windows 原生反斜杠直接使用。
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

func decodeWindowsConsoleOutput(output []byte) string {
	if len(output) == 0 {
		return ""
	}
	if utf8.Valid(output) {
		return string(output)
	}
	decoded, err := codec.GbkToUtf8(output)
	if err != nil {
		return string(output)
	}
	return string(decoded)
}

// windowsShortPath returns the 8.3 short path form of path. Short paths are
// pure ASCII, which sidesteps the UTF-8/GBK codepage mismatch between Go's
// UTF-8 file paths and cmd.exe's OEM codepage on localized (e.g. Chinese)
// Windows 7: non-ASCII bytes embedded in a batch file (user profile dir, temp
// dir, ...) get mis-decoded by cmd and break certutil.
//
// It is best-effort: if 8.3 name generation is disabled on the volume (via
// fsutil / NtfsDisable8dot3NameCreation) the original long path is returned
// unchanged. The caller must ensure the path exists, since GetShortPathName
// resolves an existing path.
func windowsShortPath(path string) string {
	if path == "" {
		return path
	}
	long, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return path
	}
	n, err := windows.GetShortPathName(long, nil, 0)
	if err != nil || n == 0 {
		return path
	}
	buf := make([]uint16, n)
	n, err = windows.GetShortPathName(long, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		return path
	}
	short := windows.UTF16ToString(buf[:n])
	if short == "" {
		return path
	}
	return short
}
