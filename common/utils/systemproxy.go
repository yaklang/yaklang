package utils

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/utils/systemproxy"
)

func GetProxyFromEnv() string {
	if setting, err := systemproxy.Get(); err == nil && setting.Enabled && setting.DefaultServer != "" {
		return setting.DefaultServer
	}
	for _, k := range []string{
		"YAK_PROXY", "yak_proxy",
		"HTTP_PROXY", "http_proxy",
		"HTTPS_PROXY", "https_proxy",
		"all_proxy", "all_proxy",
		"proxy", "proxy",
	} {
		if p := strings.Trim(os.Getenv(k), `"`); p != "" {
			return FixProxy(p)
		}
	}
	return ""
}

func FixProxy(i string) string {
	if i == "" {
		return ""
	}

	if !strings.Contains(i, "://") {
		host, port, _ := ParseStringToHostPort(i)
		host = strings.Trim(host, `"' \r\n:`)
		if host != "" && port > 0 {
			return fmt.Sprintf("http://%v:%v", host, port)
		}
	}
	return i
}
