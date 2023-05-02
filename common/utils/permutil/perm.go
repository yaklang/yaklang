package permutil

import (
	"os"
	"runtime"
	"github.com/yaklang/yaklang/common/utils"
)

func IAmAdmin() bool {
	switch runtime.GOOS {
	case "windows":
		_, err := os.Open("\\\\.\\PHYSICALDRIVE0")
		if err != nil {
			return false
		}
		return true
	default:
		if os.Getuid() == 0 {
			return true
		}

		if os.Geteuid() == 0 {
			return true
		}
	}
	return false
}

func Sudo(cmd string, opt ...SudoOption) error {
	switch runtime.GOOS {
	case "darwin":
		return DarwinSudo(cmd, opt...)
	case "linux":
		return LinuxPKExecSudo(cmd, opt...)
	case "windows":
		return WindowsSudo(cmd, opt...)
	default:
		return utils.Error("not implemented")
	}
}
