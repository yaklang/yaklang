package tools

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	filter2 "yaklang/common/filter"
	"yaklang/common/fp"
	"yaklang/common/log"
	"yaklang/common/synscan"
	"yaklang/common/utils"
	"yaklang/common/utils/pingutil"
	"yaklang/common/utils/spacengine"
)

func scanFingerprint(target string, port string, opts ...fp.ConfigOption) (chan *fp.MatchResult, error) {
	config := fp.NewConfig(opts...)
	return _scanFingerprint(context.Background(), config, 50, target, port)
}

func scanOneFingerprint(target string, port int, opts ...fp.ConfigOption) (*fp.MatchResult, error) {
	config := fp.NewConfig(opts...)
	matcher, err := fp.NewFingerprintMatcher(nil, config)
	if err != nil {
		return nil, err
	}
	return matcher.Match(target, port)
}

func _scanFingerprint(ctx context.Context, config *fp.Config, concurrent int, host, port string) (chan *fp.MatchResult, error) {
	matcher, err := fp.NewDefaultFingerprintMatcher(config)
	if err != nil {
		return nil, err
	}

	log.Infof("start to scan [%s] 's port: %s", host, port)

	if matcher.Config.PoolSize > 0 {
		concurrent = matcher.Config.PoolSize
	}

	filter := filter2.NewFilter()

	outC := make(chan *fp.MatchResult)
	go func() {
		swg := utils.NewSizedWaitGroup(concurrent)
		var portsInt = utils.ParseStringToPorts(port)
		if len(portsInt) <= 0 {
			for _, hRaw := range utils.ParseStringToHosts(host) {
				h := utils.ExtractHost(hRaw)
				if h != hRaw {
					buildinHost, buildinPort, _ := utils.ParseStringToHostPort(hRaw)
					if buildinPort > 0 {
						swg.Add()
						go func() {
							defer swg.Done()
							addr := utils.HostPort(buildinHost, buildinPort)
							if filter.Exist(addr) {
								return
							}
							filter.Insert(addr)
							log.Infof("start task to scan: [%s]", addr)
							result, err := matcher.MatchWithContext(ctx, buildinHost, buildinPort)
							if err != nil {
								if strings.Contains(fmt.Sprint(err), "excludeHosts/Ports") {
									return
								}
								log.Errorf("failed to scan %s: %s", addr, err)
								return
							}

							outC <- result
						}()
					}
				}
			}
		} else {
			for _, p := range portsInt {
				for _, hRaw := range utils.ParseStringToHosts(host) {
					h := utils.ExtractHost(hRaw)
					if h != hRaw {
						buildinHost, buildinPort, _ := utils.ParseStringToHostPort(hRaw)
						if buildinPort > 0 {
							swg.Add()
							go func() {
								defer swg.Done()
								addr := utils.HostPort(buildinHost, buildinPort)
								if filter.Exist(addr) {
									return
								}
								filter.Insert(addr)
								log.Infof("start task to scan: [%s]", addr)
								result, err := matcher.MatchWithContext(ctx, buildinHost, buildinPort)
								if err != nil {
									if strings.Contains(fmt.Sprint(err), "filtered by servicescan") {
										return
									}
									log.Errorf("failed to scan %s: %s", addr, err)
									return
								}

								outC <- result
							}()
						}
					}

					swg.Add()
					rawPort := p
					rawHost := h
					go func() {
						defer swg.Done()

						addr := utils.HostPort(rawHost, rawPort)
						if filter.Exist(addr) {
							return
						}
						filter.Insert(addr)

						log.Infof("start task to scan: [%s]", utils.HostPort(rawHost, rawPort))
						result, err := matcher.MatchWithContext(ctx, rawHost, rawPort)
						if err != nil {
							log.Errorf("failed to scan %s: %s", utils.HostPort(rawHost, rawPort), err)
							return
						}

						outC <- result
					}()
				}
			}
		}
		go func() {
			swg.Wait()
			close(outC)
		}()
	}()

	return outC, nil
}

func _scanFromPingUtils(res chan *pingutil.PingResult, ports string, opts ...fp.ConfigOption) (chan *fp.MatchResult, error) {
	var synResults = make(chan *synscan.SynScanResult, 1000)
	portsInt := utils.ParseStringToPorts(ports)
	go func() {
		defer close(synResults)
		for result := range res {
			if !result.Ok {
				log.Errorf("%v is may not alive.", result.IP)
				continue
			}

			var hostRaw, portRaw, _ = utils.ParseStringToHostPort(result.IP)
			if portRaw > 0 {
				synResults <- &synscan.SynScanResult{Host: hostRaw, Port: portRaw}
			}

			//log.Errorf("%v is alive", result.IP)
			for _, port := range portsInt {
				synResults <- &synscan.SynScanResult{
					Host: result.IP,
					Port: port,
				}
			}
		}
	}()
	return _scanFromTargetStream(synResults, opts...)
}

func _scanFromTargetStream(res interface{}, opts ...fp.ConfigOption) (chan *fp.MatchResult, error) {
	var synResults = make(chan *synscan.SynScanResult, 1000)

	// 生成扫描结果
	go func() {
		defer close(synResults)

		switch ret := res.(type) {
		case chan *synscan.SynScanResult:
			for r := range ret {
				synResults <- r
			}
		case []interface{}:
			for _, r := range ret {
				switch subResult := r.(type) {
				case *synscan.SynScanResult:
					synResults <- subResult
				case synscan.SynScanResult:
					synResults <- &subResult
				case string:
					host, port, err := utils.ParseStringToHostPort(subResult)
					if err != nil {
						continue
					}
					synResults <- &synscan.SynScanResult{
						Host: host,
						Port: port,
					}
				case *spacengine.NetSpaceEngineResult:
					host, port, err := utils.ParseStringToHostPort(subResult.Addr)
					if err != nil {
						continue
					}
					synResults <- &synscan.SynScanResult{
						Host: host,
						Port: port,
					}
				}

			}
		case []*synscan.SynScanResult:
			for _, r := range ret {
				synResults <- r
			}
		case chan *spacengine.NetSpaceEngineResult:
			for r := range ret {
				host, port, err := utils.ParseStringToHostPort(r.Addr)
				if err != nil {
					continue
				}
				synResults <- &synscan.SynScanResult{
					Host: host,
					Port: port,
				}
			}
		case []*spacengine.NetSpaceEngineResult:
			for _, r := range ret {
				host, port, err := utils.ParseStringToHostPort(r.Addr)
				if err != nil {
					continue
				}
				synResults <- &synscan.SynScanResult{
					Host: host,
					Port: port,
				}
			}
		case []string:
			for _, r := range ret {
				host, port, err := utils.ParseStringToHostPort(r)
				if err != nil {
					continue
				}
				synResults <- &synscan.SynScanResult{
					Host: host,
					Port: port,
				}
			}
		default:
			log.Errorf("not a valid param: %v", reflect.TypeOf(res))
		}
	}()

	// 扫描
	config := fp.NewConfig(opts...)
	concurrent := config.PoolSize
	ctx := context.Background()

	matcher, err := fp.NewDefaultFingerprintMatcher(config)
	if err != nil {
		return nil, err
	}

	if matcher.Config.PoolSize > 0 {
		concurrent = matcher.Config.PoolSize
	}

	outC := make(chan *fp.MatchResult)
	go func() {
		swg := utils.NewSizedWaitGroup(concurrent)
		for synRes := range synResults {
			swg.Add()
			rawPort := synRes.Port
			rawHost := synRes.Host
			go func() {
				defer swg.Done()

				log.Infof("start task to scan: [%s]", utils.HostPort(rawHost, rawPort))
				result, err := matcher.MatchWithContext(ctx, rawHost, rawPort)
				if err != nil {
					log.Errorf("failed to scan %s: %s", utils.HostPort(rawHost, rawPort), err)
					return
				}

				outC <- result
			}()
		}

		go func() {
			swg.Wait()
			close(outC)
		}()
	}()

	return outC, nil
}

var FingerprintScanExports = map[string]interface{}{
	"Scan":                scanFingerprint,
	"ScanOne":             scanOneFingerprint,
	"ScanFromSynResult":   _scanFromTargetStream,
	"ScanFromSpaceEngine": _scanFromTargetStream,
	"ScanFromPing":        _scanFromPingUtils,

	"proto": func(proto ...interface{}) fp.ConfigOption {
		return fp.WithTransportProtos(fp.ParseStringToProto(proto...)...)
	},

	// 整体扫描并发
	"concurrent": fp.WithPoolSize,

	"excludePorts": fp.WithExcludePorts,
	"excludeHosts": fp.WithExcludeHosts,

	// 单个请求超时时间
	"probeTimeout": fp.WithProbeTimeoutHumanRead,

	// proxies
	"proxy": fp.WithProxy,

	// 启用缓存
	"cache":         fp.WithCache,
	"databaseCache": fp.WithDatabaseCache,

	// 使用 web 指纹识别规则进行扫描
	"webRule": fp.WithWebFingerprintRule,

	// 可以使用 nmap 的规则进行扫描，也可以写 nmap 规则进行扫描
	"nmapRule": fp.WithNmapRule,

	// nmap 规则筛选，通过稀有度
	"nmapRarityMax": fp.WithRarityMax,

	// 主动发包模式打开
	"active": fp.WithActiveMode,

	// 每个服务最多主动发几个包？
	"maxProbes": fp.WithProbesMax,

	// 主动发包模式下，并发量？
	"maxProbesConcurrent": fp.WithProbesConcurrentMax,

	// 指定选择扫描目标协议：指开启 web 服务扫描
	"web": func() fp.ConfigOption {
		return func(config *fp.Config) {
			config.OnlyEnableWebFingerprint = true
		}
	},

	// 开启 nmap 规则库
	"service": func() fp.ConfigOption {
		return func(config *fp.Config) {
			config.DisableWebFingerprint = true
		}
	},

	// 全部服务扫描
	"all": func() fp.ConfigOption {
		return func(config *fp.Config) {
			config.ForceEnableWebFingerprint = true
		}
	},
}
