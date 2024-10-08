package systemproxy

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"runtime"
	"strings"
)

// Settings represents systemwide proxy settings.
type Settings struct {
	// Enabled is true if static (i.e. non-PAC) proxy is enabled
	Enabled bool

	// DefaultServer is the server used for all protocols.
	DefaultServer string
}

// ErrNotImpl error is returned when the current platform isn't supported yet.
var ErrNotImpl = errors.New(fmt.Sprintf("systemproxy not implemented on this platform: %v", runtime.GOOS))

func GetProxyFromEnv() string {
	if setting, err := Get(); err == nil && setting.Enabled && setting.DefaultServer != "" {
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
		host, port, _ := utils.ParseStringToHostPort(i)
		host = strings.Trim(host, `"' \r\n:`)
		if host != "" && port > 0 {
			return fmt.Sprintf("http://%v:%v", host, port)
		}
	}
	return i
}
