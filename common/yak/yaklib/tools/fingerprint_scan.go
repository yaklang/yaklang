package tools

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/yaklang/yaklang/common/utils/spacengine/base"

	filter2 "github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pingutil"
)

// Scan servicescan 库使用的端口扫描类型的方式为全连接扫描，用于对连接目标进行精准的扫描，相比 synscan 库的单纯扫描，servicescan 库会尝试获取精确指纹信息以及 CPE 信息
// @param {string} target 目标地址，支持 CIDR 格式，支持 192.168.1.1-100 格式
// @param {string} port 端口，支持 1-65535、1,2,3、1-100,200-300 格式
// @param {ConfigOption} [opts] servicescan 扫描参数
// @return {chan *MatchResult} 返回结果
// Example:
// ```
// ch, err = servicescan.Scan("127.0.0.1", "22-80,443,3389")  // 开始扫描，函数会立即返回一个错误和结果管道
// die(err) // 如果错误非空则报错
// for result := range ch { // 通过遍历管道的形式获取管道中的结果
//
//	   if result.IsOpen() { // 获取到的结果是一个结构体，可以调用IsOpen方法判断该端口是否打开
//	       println(result.String()) // 输出结果，调用String方法获取可读字符串
//	       println(result.GetCPEs()) // 查看 CPE 结果
//	   }
//	}
//
// ```
func scanFingerprint(target string, port string, opts ...fp.ConfigOption) (chan *fp.MatchResult, error) {
	config := fp.NewConfig(opts...)
	return _scanFingerprint(config.Ctx, config, 50, target, port)
}

// ScanOne servicescan 单体扫描，同步扫描一个目标，主机+端口
// @param {string} target 目标地址
// @param {int} port 端口
// @param {ConfigOption} [opts] servicescan 扫描参数
// @return {MatchResult} 返回结果
// Example:
// ```
// result, err = servicescan.ScanOne("127.0.0.1", "22-80,443,3389")  // 开始扫描，函数会立即返回一个错误和结果
// die(err) // 如果错误非空则报错
// if result.IsOpen() { // 获取到的结果是一个结构体，可以调用IsOpen方法判断该端口是否打开
//
//	    println(result.String()) // 输出结果，调用String方法获取可读字符串
//	    println(result.GetCPEs()) // 查看 CPE 结果
//	}
//
// ```
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
		portsInt := utils.ParseStringToPorts(port)
		for _, p := range portsInt {
			for _, hRaw := range utils.ParseStringToHosts(host) {
				h := utils.ExtractHost(hRaw)
				if h != hRaw {
					buildinHost, buildinPort, _ := utils.ParseStringToHostPort(hRaw)
					if buildinPort > 0 {
						swg.Add()
						go func() {
							defer swg.Done()
							proto, portWithoutProto := utils.ParsePortToProtoPort(buildinPort) // 这里将协议和端口分开，便于后面打印日志
							addr := utils.HostPort(buildinHost, buildinPort)
							if filter.Exist(addr) {
								return
							}
							filter.Insert(addr)
							log.Infof("start task to scan: [%s://%s]", proto, utils.HostPort(buildinHost, portWithoutProto))
							result, err := matcher.MatchWithContext(ctx, buildinHost, buildinPort)
							if err != nil {
								if len(portsInt) <= 0 {
									if strings.Contains(fmt.Sprint(err), "excludeHosts/Ports") {
										return
									}
								} else {
									if strings.Contains(fmt.Sprint(err), "filtered by servicescan") {
										return
									}
								}
								log.Errorf("failed to scan [%s://%s]: %v", proto, utils.HostPort(buildinHost, portWithoutProto), err)
								return
							}

							outC <- result
						}()
					}
				}

				swg.Add()
				rawPort := p
				rawHost := h
				proto, portWithoutProto := utils.ParsePortToProtoPort(p) // 这里将协议和端口分开，便于后面打印日志
				go func() {
					defer swg.Done()

					addr := utils.HostPort(rawHost, rawPort)
					if filter.Exist(addr) {
						return
					}
					filter.Insert(addr)

					//log.Infof("start task to scan: [%s://%s]", proto, utils.HostPort(rawHost, portWithoutProto))
					result, err := matcher.MatchWithContext(ctx, rawHost, rawPort)
					if err != nil {
						log.Errorf("failed to scan [%s://%s]: %v", proto, utils.HostPort(rawHost, portWithoutProto), err)
						return
					}

					outC <- result
				}()
			}
		}
		go func() {
			swg.Wait()
			filter.Close()
			close(outC)
		}()
	}()

	return outC, nil
}

// ScanFromPing 从 ping.Scan 的结果中进行指纹识别
// @param {chan *pingutil.PingResult} res ping.Scan 的结果
// @param {string} ports 端口，支持 1-65535、1,2,3、1-100,200-300 格式
// @param {ConfigOption} [opts] synscan 扫描参数
// @return {chan *MatchResult} 返回结果
// Example:
// ```
// pingResult, err = ping.Scan("192.168.1.1/24") // 先进行存活探测
// die(err)
// fpResults, err := servicescan.ScanFromPing(pingResult, "22-80,443,3389") // 将ping中拿到的结果传入servicescan中进行指纹扫描
// die(err) // 如果错误非空则报错
// for result := range fpResults { // 通过遍历管道的形式获取管道中的结果，一旦有结果返回就会执行循环体的代码
//
//	   println(result.String()) // 输出结果，调用String方法获取可读字符串
//	}
//
// ```
func _scanFromPingUtils(res chan *pingutil.PingResult, ports string, opts ...fp.ConfigOption) (chan *fp.MatchResult, error) {
	synResults := make(chan *synscan.SynScanResult, 1000)
	portsInt := utils.ParseStringToPorts(ports)
	go func() {
		defer close(synResults)
		for result := range res {
			if !result.Ok {
				log.Debugf("%v is may not alive.", result.IP)
				continue
			}

			hostRaw, portRaw, _ := utils.ParseStringToHostPort(result.IP)
			if portRaw > 0 {
				synResults <- &synscan.SynScanResult{Host: hostRaw, Port: portRaw}
			}

			// log.Errorf("%v is alive", result.IP)
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

// ScanFromSynResult / ScanFromSpaceEngine 从 synscan.Scan 或者 spacengine.Query 的结果中进行指纹识别
// @param {interface{}} res synscan.Scan 或者 spacengine.Query 的结果
// @param {scanOpt} [opts] synscan 扫描参数
// @return {chan *MatchResult} 返回结果
// Example:
// ```
// ch, err = synscan.Scan("127.0.0.1", "22-80,443,3389")  // 开始扫描，函数会立即返回一个错误和结果管道
// die(err) // 如果错误非空则报错
// fpResults, err := servicescan.ScanFromSynResult(ch) // 将synscan中拿到的结果传入servicescan中进行指纹扫描
// die(err) // 如果错误非空则报错
// for result := range fpResults { // 通过遍历管道的形式获取管道中的结果，一旦有结果返回就会执行循环体的代码
//
//	   println(result.String()) // 输出结果，调用String方法获取可读字符串
//	}
//
// res, err := spacengine.ShodanQuery(Apikey,query)
// die(err) // 如果错误非空则报错
// fpResults, err := servicescan.ScanFromSpaceEngine(res) // 将spacengine中拿到的结果传入servicescan中进行指纹扫描
// die(err) // 如果错误非空则报错
// for result := range fpResults { // 通过遍历管道的形式获取管道中的结果，一旦有结果返回就会执行循环体的代码
//
//	   println(result.String()) // 输出结果，调用String方法获取可读字符串
//	}
//
// ```
func _scanFromTargetStream(res interface{}, opts ...fp.ConfigOption) (chan *fp.MatchResult, error) {
	synResults := make(chan *synscan.SynScanResult, 1000)

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
				case *base.NetSpaceEngineResult:
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
		case chan *base.NetSpaceEngineResult:
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
		case []*base.NetSpaceEngineResult:
			for _, r := range ret {
				host, port, err := utils.ParseStringToHostPort(r.Addr)
				if err != nil {
					continue
				}
				synResults <- &synscan.SynScanResult{
					Host: host, Port: port,
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
	ctx := config.Ctx
	if config.Ctx == nil {
		ctx = context.Background()
	}

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
			proto, portWithoutProto := utils.ParsePortToProtoPort(rawPort) // 这里将协议和端口分开，便于后面打印日志
			go func() {
				defer swg.Done()

				log.Infof("start task to scan: [%s://%s]", proto, utils.HostPort(rawHost, portWithoutProto))
				result, err := matcher.MatchWithContext(ctx, rawHost, rawPort)
				if err != nil {
					log.Errorf("failed to scan [%s://%s]: %v", proto, utils.HostPort(rawHost, portWithoutProto), err)
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

// proto servicescan 的配置选项，用于指定扫描协议
// @param {...interface{}} [proto] 协议，例如：tcp、udp，可选参数，不传入参数默认为 tcp
// @return {ConfigOption} 返回配置选项
// Example:
// ```
// result,err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.proto(["tcp","udp"]...)) // 使用 TCP 和 UDP 进行扫描
// die(err) // 如果错误非空则报错
// for res := range result { // 通过遍历管道的形式获取管道中的结果，一旦有结果返回就会执行循环体的代码
//
//	   println(res.String()) // 输出结果，调用String方法获取可读字符串
//	}
//
// ```
func _protoOption(proto ...interface{}) fp.ConfigOption {
	return fp.WithTransportProtos(fp.ParseStringToProto(proto...)...)
}

// web servicescan 的配置选项，用于指定扫描指纹的类型为 web
// @return {ConfigOption} 返回配置选项
// Example:
// ```
// result,err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.web()) // 使用 web 指纹进行扫描
// die(err) // 如果错误非空则报错
// for res := range result { // 通过遍历管道的形式获取管道中的结果，一旦有结果返回就会执行循环体的代码
//
//	   println(res.String()) // 输出结果，调用String方法获取可读字符串
//	}
//
// ```
func _webOption() fp.ConfigOption {
	return func(config *fp.Config) {
		config.OnlyEnableWebFingerprint = true
	}
}

// service servicescan 的配置选项，用于指定扫描指纹的类型为 service
// @return {ConfigOption} 返回配置选项
// Example:
// ```
// result,err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.service()) // 使用 service 指纹进行扫描
// die(err) // 如果错误非空则报错
// for res := range result { // 通过遍历管道的形式获取管道中的结果，一旦有结果返回就会执行循环体的代码
//
//	   println(res.String()) // 输出结果，调用String方法获取可读字符串
//	}
//
// ```
func _serviceOption() fp.ConfigOption {
	return func(config *fp.Config) {
		config.DisableWebFingerprint = true
	}
}

// all servicescan 的配置选项，用于指定扫描指纹的类型为 web 和 service
// @return {ConfigOption} 返回配置选项
// Example:
// ```
// result,err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.all()) // 使用 web 和 service 指纹进行扫描
// die(err) // 如果错误非空则报错
// for res := range result { // 通过遍历管道的形式获取管道中的结果，一旦有结果返回就会执行循环体的代码
//
//	   println(res.String()) // 输出结果，调用String方法获取可读字符串
//	}
//
// ```
func _allOption() fp.ConfigOption {
	return func(config *fp.Config) {
		config.ForceEnableAllFingerprint = true
	}
}

func _disableDefaultFingerprint(b ...bool) fp.ConfigOption {
	return func(config *fp.Config) {
		if len(b) == 0 {
			config.DisableDefaultFingerprint = true
		} else {
			config.DisableDefaultFingerprint = utils.GetLastElement(b)
		}
	}
}

// disableWebScanConnPool servicescan 的配置选项，用于禁用 web 扫描的连接池
// @param {bool} b 是否禁用连接池，默认为 false
// @return {ConfigOption} 返回配置选项
// ```
// result,err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.disableWebScanConnPool(true)) // 禁用 web 扫描的连接池
// die(err) // 如果错误非空则报错
// for res := range result { // 通过遍历管道的形式获取管道中的结果，一旦有结果返回就会执行循环体的代码
//
//	   println(res.String()) // 输出结果，调用String方法获取可读
//	   println(res.String()) // 输出结果，调用String方法获取可读字符串
//	}
//
// ```
func _disableWebScanConnPool(b bool) fp.ConfigOption {
	return func(config *fp.Config) {
		config.WebScanDisableConnPool = b
	}
}

var FingerprintScanExports = map[string]interface{}{
	"Scan":                scanFingerprint,
	"ScanOne":             scanOneFingerprint,
	"ScanFromSynResult":   _scanFromTargetStream,
	"ScanFromSpaceEngine": _scanFromTargetStream,
	"ScanFromPing":        _scanFromPingUtils,

	"proto": _protoOption,

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
	"onOpen":        fp.WithOnPortOpenCallback,
	"onFinish":      fp.WithOnFinished,

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

	// 是否使用 debugLog
	"debugLog": fp.WithDebugLog,

	// 指定选择扫描目标协议：指开启 web 服务扫描
	"web": _webOption,

	// 开启 nmap 规则库
	"service": _serviceOption,

	// 全部服务扫描
	"all": _allOption,

	// 选择指纹规则组
	"withRuleGroupAll": fp.WithFingerprintRuleGroupAll,
	"withRuleGroup":    fp.WithFingerprintRuleGroup,

	"disableDefaultRule": _disableDefaultFingerprint,

	"disableWebScanConnPool": _disableWebScanConnPool,
}
