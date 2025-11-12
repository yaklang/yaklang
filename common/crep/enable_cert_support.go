package crep

import (
	"os/exec"
	"runtime"
)

// CheckMITMAutoInstallReady 判断当前系统是否满足自动安装 MITM 证书的前置条件
func CheckMITMAutoInstallReady() (bool, string) {
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("pkexec"); err != nil {
			return false, "pkexec not found; install policykit (e.g. sudo apt install policykit-1)"
		}
	default:
		// Windows / macOS 使用各自的 UAC 方案，不需要额外依赖
	}
	return true, ""
}
