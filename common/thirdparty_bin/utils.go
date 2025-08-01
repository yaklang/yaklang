package thirdparty_bin

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/yaklang/yaklang/common/utils"
)

// GetDefaultDownloadDir 获取默认下载目录
func GetDefaultDownloadDir() (string, error) {
	homeDir, err := utils.GetHomeDir()
	if err != nil {
		return "", err
	}

	downloadDir := filepath.Join(homeDir, ".yaklang", "thirdparty_bin", "downloads")
	return downloadDir, nil
}

// GetDefaultInstallDir 获取默认安装目录
func GetDefaultInstallDir() (string, error) {
	homeDir, err := utils.GetHomeDir()
	if err != nil {
		return "", err
	}

	var installDir string
	switch runtime.GOOS {
	case "windows":
		installDir = filepath.Join(homeDir, ".yaklang", "thirdparty_bin", "bin")
	case "darwin":
		installDir = filepath.Join(homeDir, ".yaklang", "thirdparty_bin", "bin")
	default: // linux
		installDir = filepath.Join(homeDir, ".yaklang", "thirdparty_bin", "bin")
	}

	return installDir, nil
}

// EnsureExecutable 确保文件具有执行权限
func EnsureExecutable(filePath string) error {
	// Windows不需要设置执行权限
	if runtime.GOOS == "windows" {
		return nil
	}

	// Unix-like系统需要设置执行权限
	return os.Chmod(filePath, 0755)
}

// GetFilenameFromURL 从URL中提取文件名
func GetFilenameFromURL(url string) string {
	// 简单的文件名提取，从最后一个'/'之后开始
	if idx := lastIndex(url, "/"); idx >= 0 && idx < len(url)-1 {
		return url[idx+1:]
	}
	return ""
}

// lastIndex 查找字符串中字符的最后一个位置
func lastIndex(s, sep string) int {
	if len(sep) == 0 {
		return len(s)
	}

	for i := len(s) - len(sep); i >= 0; i-- {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}

// CleanupTempFiles 清理临时文件
func CleanupTempFiles(patterns ...string) error {
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}

		for _, match := range matches {
			os.Remove(match) // 忽略错误
		}
	}
	return nil
}
