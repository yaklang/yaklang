package yaklib

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dnsutil"
	"time"
)

type _dnsConfig struct {
	timeout    time.Duration
	dnsServers []string
}

type _dnsConfigOpt func(c *_dnsConfig)

func _dnsConfigOpt_WithTimeout(d float64) _dnsConfigOpt {
	return func(c *_dnsConfig) {
		c.timeout = utils.FloatSecondDuration(d)
	}
}

func _dnsConfigOpt_WithDNSServers(servers ...string) _dnsConfigOpt {
	return func(c *_dnsConfig) {
		c.dnsServers = servers
	}
}

func _dnsQueryIP(target string, opts ..._dnsConfigOpt) string {
	var config = &_dnsConfig{
		timeout: 5 * time.Second,
	}

	for _, o := range opts {
		o(config)
	}

	return dnsutil.QueryIP(target, config.timeout, config.dnsServers)
}

func _dnsQueryIPAll(target string, opts ..._dnsConfigOpt) []string {
	var config = &_dnsConfig{
		timeout: 5 * time.Second,
	}

	for _, o := range opts {
		o(config)
	}

	return dnsutil.QueryIPAll(target, config.timeout, config.dnsServers)
}

func _dnsQueryNS(target string, opts ..._dnsConfigOpt) []string {
	var config = &_dnsConfig{
		timeout: 5 * time.Second,
	}

	for _, o := range opts {
		o(config)
	}

	return dnsutil.QueryNS(target, config.timeout, config.dnsServers)
}

func _dnsQueryTxt(target string, opts ..._dnsConfigOpt) []string {

	var config = &_dnsConfig{
		timeout: 5 * time.Second,
	}

	for _, o := range opts {
		o(config)
	}
	return dnsutil.QueryTxt(target, config.timeout, config.dnsServers)
}

func _dnsQueryAxfr(target string, opts ..._dnsConfigOpt) []string {

	var config = &_dnsConfig{
		timeout: 5 * time.Second,
	}

	for _, o := range opts {
		o(config)
	}
	return dnsutil.QueryAXFR(target, config.timeout, config.dnsServers)
}

var DnsExports = map[string]interface{}{
	"QueryIP":    _dnsQueryIP,
	"QueryIPAll": _dnsQueryIPAll,
	"QueryNS":    _dnsQueryNS,
	"QueryTXT":   _dnsQueryTxt,
	"QuertAxfr":  _dnsQueryAxfr,
	"QueryAxfr":  _dnsQueryAxfr,

	"timeout":    _dnsConfigOpt_WithTimeout,
	"dnsServers": _dnsConfigOpt_WithDNSServers,
}
