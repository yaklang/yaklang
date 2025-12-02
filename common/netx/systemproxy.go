package netx

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// SystemProxySetting represents systemwide proxy settings.
type SystemProxySetting struct {
	// Enabled is true if static (i.e. non-PAC) proxy is enabled
	Enabled bool

	// DefaultServer is the server used for all protocols.
	DefaultServer string
}

// ErrNotImpl error is returned when the current platform isn't supported yet.
var ErrNotImpl = errors.New(fmt.Sprintf("systemproxy not implemented on this platform: %v", runtime.GOOS))

func GetProxyFromEnv() string {
	if setting, err := GetSystemProxy(); err == nil && setting.Enabled && setting.DefaultServer != "" {
		return setting.DefaultServer
	}
	for _, k := range []string{
		"YAK_PROXY", "yak_proxy",
		"HTTP_PROXY", "http_proxy",
		"HTTPS_PROXY", "https_proxy",
		"all_proxy", "all_proxy",
		"proxy", "proxy",
	} {
		if p := strings.Trim(os.Getenv(k), `"'`); p != "" {
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
		host, port, _ := utils.ParseStringToHostPort(i)
		host = strings.Trim(host, `"' \r\n:`)
		if host != "" && port > 0 {
			return fmt.Sprintf("http://%v:%v", host, port)
		}
		// 如果没有端口，返回空字符串而不是无效的代理地址
		// 这样可以防止无效代理被传递到下游
		return ""
	}

	// 验证带协议的代理地址是否包含端口
	if u := utils.ParseStringToUrl(i); u != nil {
		if u.Port() == "" {
			// 协议存在但没有端口，返回空字符串
			return ""
		}
	}

	return i
}
