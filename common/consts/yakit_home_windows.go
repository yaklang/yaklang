//go:build windows

package consts

import (
	"os"

	"github.com/yaklang/yaklang/common/log"
	"golang.org/x/sys/windows/registry"
)

func GetRegistryYakitHome() {
	// 如果已经设置了环境变量YAKIT_HOME，则不再从注册表中获取
	if os.Getenv("YAKIT_HOME") != "" {
		return
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer k.Close()

	s, _, err := k.GetStringValue("YAKIT_HOME")
	if err == nil {
		os.Setenv("YAKIT_HOME", s)
		log.Debugf("Set YAKIT_HOME from registry HKCU\\Environment: %s", s)
	}
}
