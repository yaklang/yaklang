package systemproxy

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakdns"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

/*
Get returns the current systemwide proxy settings.
*/
var httpEnableRegexp = regexp.MustCompile(`(?i)HTTPEnable\s*:\s*([01])`)
var httpProxyRegexp = regexp.MustCompile(`(?i)HTTPProxy\s*:\s*([^\s\r\n]*)`)
var httpPortRegexp = regexp.MustCompile(`(?i)HTTPPort\s*:\s*([\d]*)`)

func Get() (Settings, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("fetch scutil proxy from darwin failed: %s", err)
		}
	}()
	cmd := exec.Command("scutil", "--proxy")
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return Settings{}, err
	}
	_ = raw
	if result := httpEnableRegexp.FindSubmatch(raw); len(result) > 1 {
		line := strings.TrimSpace(string(result[1]))
		if line == "1" {
			var host string
			var port int
			if hostRaw := httpProxyRegexp.FindSubmatch(raw); len(hostRaw) > 1 {
				addr := hostRaw[1]
				if string(addr) != "" {
					host = string(addr)
				}
			}
			if portraw := httpPortRegexp.FindSubmatch(raw); len(portraw) > 1 {
				portStr := portraw[1]
				if p, e := strconv.ParseInt(string(portStr), 10, 32); e == nil {
					port = int(p)
				}
			}

			if host != "" && port > 0 {
				return Settings{Enabled: true, DefaultServer: utils.HostPort(host, port)}, nil
			}
		}
		return Settings{
			Enabled:       false,
			DefaultServer: "",
		}, nil
	}
	return Settings{}, utils.Errorf("scutil result empty...")
}

/*
Set updates systemwide proxy settings.

// osascript -c 'shell "" with administrator privileges'
// osascript -e 'do shell script "echo 123" with administrator privileges'
*/
func Set(s Settings) error {
	if s.Enabled && s.DefaultServer != "" {
		host, port, err := utils.ParseStringToHostPort(s.DefaultServer)
		if err != nil {
			return err
		}
		if port <= 0 {
			return utils.Errorf("cannot found port for %s", s.DefaultServer)
		}
		if !utils.IsIPv4(host) && !utils.IsIPv6(host) {

			if addr := yakdns.LookupFirst(host, yakdns.WithTimeout(5*time.Second)); addr == "" {
				return utils.Errorf("cannot set proxy for %s for (DNSFailed)", s.DefaultServer)
			}
		}
		raw := fmt.Sprintf(
			`do shell script "networksetup -setwebproxy Wi-Fi %s %d; networksetup -setsecurewebproxy Wi-Fi %s %d; networksetup -setsocksfirewallproxy Wi-Fi \"\" \"\"" with administrator privileges`,
			host, port, host, port,
		)
		err = exec.Command("osascript", "-e", raw).Run()
		if err != nil {
			return utils.Errorf("OSAScript failed for networksetup: proxy %s", s.DefaultServer)
		}
		return nil
	} else {
		result, _ := Get()
		if !result.Enabled {
			return nil
		}

		var err error
		raw := fmt.Sprint(`do shell script "networksetup -setwebproxy Wi-Fi \"\" \"\"; networksetup -setsecurewebproxy Wi-Fi \"\" \"\"; networksetup -setsocksfirewallproxy Wi-Fi \"\" \"\"" with administrator privileges`)
		err = exec.Command("osascript", "-e", raw).Run()
		if err != nil {
			return utils.Errorf("OSAScript failed for networksetup: proxy %s", s.DefaultServer)
		}
	}
	return nil
}
