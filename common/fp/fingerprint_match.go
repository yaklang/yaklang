package fp

import (
	"context"
	"fmt"
	"github.com/jinzhu/copier"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"net"
	"strings"
)

func (f *Matcher) Match(host string, port int, options ...ConfigOption) (result *MatchResult, err error) {
	return f.MatchWithContext(context.Background(), host, port, options...)
}

func (f *Matcher) MatchWithContext(ctx context.Context, host string, port int, options ...ConfigOption) (result *MatchResult, err error) {
	host = utils2.ExtractHost(host)
	proto, port := utils2.ParsePortToProtoPort(port)
	addr := utils2.HostPort(host, port)

	if f.Config.IsFiltered(host, port) {
		return nil, utils2.Errorf("[IGNORE] %v is filtered by servicescan.excludeHosts/Ports", addr)
	}

	// 是否需要适配 ConfigOption
	var config = NewConfig()
	if len(options) > 0 {
		err := copier.Copy(config, f.Config)
		if err != nil {
			return nil, errors.Errorf("copy config failed: %s", err)
		}

		for _, p := range options {
			p(config)
		}
	} else {
		config = f.Config
	}

	if proto == "udp" {
		config.TransportProtos = []TransportProto{UDP}
	}

	if config.EnableCache {
		result := GetMatchResultCache(addr)
		if result != nil {
			return result, nil
		}
	}

	if config.EnableDatabaseCache {
		result := GetMatchResultDatabaseCache(addr)
		if result != nil {
			return result, nil
		}
	}

	// 设置初始化匹配结果
	result = &MatchResult{
		Target: host,
		Port:   port,
		State:  CLOSED,
		Fingerprint: &FingerprintInfo{
			IP:   host,
			Port: port,
		},
	}

	// 解析需要检测指纹的主机
	ip := net.ParseIP(utils2.FixForParseIP(host))
	if ip == nil {
		log.Debugf("found host:%s is a domain, resolve it to ip", host)
		ipStr := netx.LookupFirst(host, netx.WithTimeout(config.ProbeTimeout))
		if ipStr == "" {
			dataErr := errors.Errorf("resolve %s failed: %s", host, "no available ip")
			result.Reason = dataErr.Error()
			return result, nil
		} else {
			ip = net.ParseIP(ipStr)
		}
		if ip == nil {
			dataErr := errors.Errorf("resolve %s failed: %s", host, "invalid ip addr: "+ipStr)
			result.Reason = dataErr.Error()
			return result, nil
		}
	}

	if config.OnlyEnableWebFingerprint && config.DisableWebFingerprint {
		return nil, errors.Errorf("config confliction for web fingerprint options: %s", "disable/onlyEnable")
	}

	// 指纹识别的顺序也应该注意，7000 以下除了 80-85 和 443 优先 nmap 服务识别
	// 其他优先指纹识别
	webFirst := func() (*MatchResult, error) {
		if !config.DisableWebFingerprint {
			result, err = f.webDetector(result, ctx, config, host, ip, port)
			// 禁用服务扫描
			if config.OnlyEnableWebFingerprint && !config.ForceEnableAllFingerprint {
				return result, err
			}

			if result != nil && result.Fingerprint != nil && result.Fingerprint.HttpFlows != nil {
				return result, nil
			}
			if err != nil {
				//log.Errorf("web detector exec failed: %s", err)
				return nil, err
			}

			if result.State == OPEN {
				return result, nil
			}
		}

		//////////////////////////////////////////////////////////////////////////
		////////////////////////////// 主机指纹识别 ///////////////////////////////
		//////////////////////////////////////////////////////////////////////////
		result2, _ := f.matchWithContext(ctx, ip, port, config)
		result.Merge(result2)
		return result, nil
	}
	serviceFirst := func() (*MatchResult, error) {
		result, err := f.matchWithContext(ctx, ip, port, config)
		if err != nil {
			return nil, err
		}
		if result.GetServiceName() != "" && result.GetServiceName() != "tcp" && !utils2.MatchAllOfRegexp(result.GetServiceName(), "(?i)http") {
			return result, nil
		}

		if result.State == CLOSED {
			return result, nil
		}

		return f.webDetector(result, ctx, config, host, ip, port)
	}

	var matchResult *MatchResult
	portStr := fmt.Sprint(port)
	switch true {
	case config.OnlyEnableWebFingerprint:
		log.Debugf("web-detect first for: %v", utils2.HostPort(host, port))
		matchResult, err = webFirst()
	case config.DisableWebFingerprint:
		log.Debugf("service-detect first for: %v", utils2.HostPort(host, port))
		matchResult, err = serviceFirst()
	case ((port >= 80 && port <= 90) ||
		port == 443 ||
		port >= 7000 ||
		strings.Contains(portStr, "8") || strings.Contains(portStr, "43")) &&
		port <= 30000:
		log.Debugf("web-detect first for: %v", utils2.HostPort(host, port))
		matchResult, err = webFirst()
	default:
		log.Debugf("service-detect first for: %v", utils2.HostPort(host, port))
		matchResult, err = serviceFirst()
	}

	// if port open, check tls...
	if matchResult.State == OPEN && matchResult.Fingerprint != nil {
		matchResult.Fingerprint.TLSInspectResults, _ = netx.TLSInspectTimeout(utils2.HostPort(host, port), 5)
	}

	matchResult.Tidy()
	if matchResult.State == OPEN {
		if config.EnableCache {
			SetMatchResultCache(addr, matchResult)
		}
		if config.EnableDatabaseCache {
			SetMatchResultDatabaseCache(addr, matchResult)
		}
	}
	return matchResult, err
}

//
//func (f *Matcher) pickUpBestMatchResult(results []*MatchResult) (*MatchResult, error) {
//	if len(results) <= 0 {
//		return nil, errors.New("empty match result.")
//	}
//	var tcpOrUdpResult *MatchResult
//	for _, r := range results {
//		if r.Fingerprint.ServiceName == "tcp" || r.Fingerprint.ServiceName == "udp" {
//			tcpOrUdpResult = r
//			continue
//		} else if r.Fingerprint.ServiceName == "http" || strings.HasPrefix(r.Fingerprint.Banner, "HTTP/1.") {
//			return r, nil
//		} else {
//			return r, nil
//		}
//	}
//
//	return tcpOrUdpResult, nil
//}
