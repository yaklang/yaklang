package fp

import (
	"context"
	"net"
	"sync"

	"github.com/jinzhu/copier"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	utils2 "github.com/yaklang/yaklang/common/utils"
)

const webPorts = "1,70,79,80-85,88,113,139,143,280,443,497,505,514,515,540,554,591,620,631,783,888,898,900,901,1026,1080,1042,1214,1220,1234,1314,1344,1503,1610,1611,1830,1900,2001,2002,2030,2064,2160,2306,2396,2525,2715,2869,3000,3002,3052,3128,3280,3372,3531,3689,3872,4000,4444,4567,4660,4711,5000,5427,5060,5222,5269,5280,5432,5800-5803,5900,5985,6103,6346,6544,6600,6699,6969,7002,7007,7070,7100,7402,7776,8000-8010,8080-8085,8088,8118,8181,8530,8880-8888,9000,9001,9030,9050,9080,9090,9999,10000,10001,10005,11371,13013,13666,13722,14534,15000,17988,18264,31337,40193,50000,55555"

func (f *Matcher) log(fmtlog string, items ...any) {
	if !f.Config.DebugLog {
		return
	}
	if len(items) > 0 {
		log.Infof("service-matcher-debug: "+fmtlog, items...)
	} else {
		log.Info("service-matcher-debug: " + fmtlog)
	}
}

var (
	reportOpenMutex = new(sync.Mutex)
)

func (m *Matcher) reportFinished(port *MatchResult) {
	if m.Config.OnFinishedCallback != nil {
		m.Config.OnFinishedCallback(port)
	}
}

func (m *Matcher) reportOpen(port *MatchResult) {
	reportOpenMutex.Lock()
	defer reportOpenMutex.Unlock()

	if m.Config.OpenPortSyncMap == nil {
		m.Config.OpenPortSyncMap = new(sync.Map)
	}
	idt := port.Identifier()

	_, ok := m.Config.OpenPortSyncMap.Load(idt)
	if !ok {
		m.Config.OpenPortSyncMap.Store(idt, struct{}{})
		if utils2.IsNil(m.Config.OnPortOpenCallback) || m.Config.OnPortOpenCallback == nil {
			log.Infof("fast: %v", port.String())
			return
		}
		m.Config.OnPortOpenCallback(port)
	}
}

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
	config := NewConfig()
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
			f.reportFinished(result)
			return result, nil
		}
	}

	if config.EnableDatabaseCache {
		result := GetMatchResultDatabaseCache(addr)
		if result != nil {
			f.reportFinished(result)
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
	f.log("try to parse ip from host: %v", host)
	if ip == nil {
		f.log("found host:%s is a domain, resolve it to ip", host)
		ipStr := netx.LookupFirst(host, netx.WithTimeout(config.ProbeTimeout))
		if ipStr == "" {
			f.log("resolve %v failed: no available ip", host)
			dataErr := errors.Errorf("resolve %s failed: %s", host, "no available ip")
			result.Reason = dataErr.Error()
			f.reportFinished(result)
			return result, nil
		} else {
			f.log("resolved %v to ip: %v", host, ipStr)
			ip = net.ParseIP(ipStr)
		}
		if ip == nil {
			f.log("parse ip failed for %v: invalid ip addr: %v", host, ipStr)
			dataErr := errors.Errorf("resolve %s failed: %s", host, "invalid ip addr: "+ipStr)
			result.Reason = dataErr.Error()
			f.reportFinished(result)
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
			f.log("start web fingerprint detection for %v:%v", host, port)
			result, err = f.webDetector(result, ctx, config, host, ip, port)
			// 禁用服务扫描
			if config.OnlyEnableWebFingerprint && !config.ForceEnableAllFingerprint {
				f.log("only web fingerprint enabled, skip service detection")
				return result, err
			}

			if result != nil && result.Fingerprint != nil && result.Fingerprint.HttpFlows != nil {
				f.log("web fingerprint detection completed with http flows")
				return result, nil
			}
			if err != nil {
				f.log("web detector execution failed: %v", err)
				return nil, err
			}

			if result.State == OPEN {
				f.log("port is open, web fingerprint detection completed")
				return result, nil
			}
		}

		//////////////////////////////////////////////////////////////////////////
		////////////////////////////// 主机指纹识别 ///////////////////////////////
		//////////////////////////////////////////////////////////////////////////
		f.log("start service fingerprint detection for %v:%v", host, port)
		result2, err := f.matchWithContext(ctx, ip, port, host, config)
		if err != nil {
			f.log("service fingerprint detection failed: %v", err)
		} else {
			f.log("service fingerprint detection completed")
		}
		result.Merge(result2)
		return result, nil
	}
	serviceFirst := func() (*MatchResult, error) {
		f.log("start service fingerprint detection for %v:%v", host, port)
		result, err := f.matchWithContext(ctx, ip, port, host, config)
		if err != nil {
			f.log("service fingerprint detection failed: %v", err)
			return nil, err
		}
		f.log("service fingerprint detection completed")

		serviceName := result.GetServiceName()
		f.log("service name detected: %v", serviceName)

		if f.Config.DisableWebFingerprint {
			return result, nil
		}

		if f.Config.DisableWebFingerprint {
			return result, nil
		}

		if f.Config.DisableWebFingerprint {
			return result, nil
		}
		if serviceName != "" && serviceName != "tcp" && serviceName != "ssl" && !utils2.MatchAllOfRegexp(serviceName, "(?i)http") {
			f.log("non-http service detected, skip web detection")
			return result, nil
		}

		if result.State == CLOSED {
			f.log("port is closed, skip web detection")
			return result, nil
		}

		f.log("start web fingerprint detection for %v:%v", host, port)
		return f.webDetector(result, ctx, config, host, ip, port)
	}

	var matchResult *MatchResult
	if config.OnlyEnableWebFingerprint {
		f.log("web-detect first for: %v", utils2.HostPort(host, port))
		matchResult, err = webFirst()
	} else {
		// 使用预定义的端口范围来决定扫描策略
		webPortsFilter := utils2.ParseStringToPorts(webPorts)
		if utils2.IntArrayContains(webPortsFilter, port) {
			f.log("web-detect first for: %v", utils2.HostPort(host, port))
			matchResult, err = webFirst()
		} else {
			// 默认策略，可以根据实际情况调整
			f.log("service-detect first for: %v", utils2.HostPort(host, port))
			matchResult, err = serviceFirst()
		}
	}

	if matchResult == nil || err != nil { // 空指针保护
		f.log("match result is nil or error occurred: %v", err)
		return nil, err
	}

	// if port open, check tls...
	if matchResult.State == OPEN && matchResult.Fingerprint != nil && (len(matchResult.Fingerprint.TLSInspectResults) == 0 && !matchResult.Fingerprint.CheckedTLS) {
		f.log("start TLS inspection for %v", utils2.HostPort(host, port))
		matchResult.Fingerprint.TLSInspectResults, _ = netx.TLSInspectTimeout(utils2.HostPort(host, port), 5)
		f.log("TLS inspection completed")
	}

	f.log("tidying match result")
	matchResult.Tidy()
	if matchResult.State == OPEN {
		if config.EnableCache {
			f.log("saving result to memory cache")
			SetMatchResultCache(addr, matchResult)
		}
		if config.EnableDatabaseCache {
			f.log("saving result to database cache")
			SetMatchResultDatabaseCache(addr, matchResult)
		}
	}
	f.reportFinished(matchResult)
	return matchResult, err
}
