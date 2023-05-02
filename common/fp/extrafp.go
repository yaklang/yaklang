package fp

import (
	"strings"
	"github.com/yaklang/yaklang/common/log"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/extrafp"
)

// 在这里可以做服务的修正，用来 ifelse 处理各种小问题
func specialCase(result *MatchResult, config *Config) *MatchResult {
	if result == nil {
		return nil
	}

	if result.State == CLOSED || result.Fingerprint == nil {
		return result
	}

	if utils2.MatchAnyOfSubString(result.Fingerprint.Banner, "Server: Proxy", "Unauthorized ...", "Auth Result: Invalid user.") {
		result.Fingerprint.ServiceName = "ccproxy"
		result.Fingerprint.CPEs = append(result.Fingerprint.CPEs, "cpe:2.3:a:*:ccproxy:")
	}

	if utils2.MatchAnyOfSubString(strings.ToLower(result.GetServiceName()), "rdp", "remote_desktop", "remote_desktop_p") {
		addr := utils2.HostPort(result.Target, result.Port)
		verbose, cpe, err := extrafp.RDPVersion(addr, config.ProbeTimeout)
		if err != nil {
			return result
		}
		if verbose != "" {
			log.Infof("extrafp-%v: %v", addr, verbose)
		}
		if result.Fingerprint != nil && len(cpe) > 0 {
			result.Fingerprint.CPEs = append(result.Fingerprint.CPEs, cpe...)
			result.Fingerprint.OperationVerbose = verbose
		}
	}

	return result
}
