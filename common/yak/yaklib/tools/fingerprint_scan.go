package tools

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/utils/spacengine/base"

	filter2 "github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pingutil"
)

// fingerprintDropWarnOnce 是 fingerprint 结果因 ctx cancel 而被丢弃时的
// "首次告警" 标记. 进程生命周期内只会触发一次, 避免 cancel 时刷屏.
//
// 关键词: fingerprint outC drop warn once, sync.Once
var fingerprintDropWarnOnce sync.Once

// fingerprintInFlight 统计当前正在执行的 fingerprint inner goroutine 数量,
// 用于运维诊断 (例如 stall 时与历史泄漏数对比, 或在 pprof 之外快速判断是否
// 还有未收敛的扫描). 不影响业务逻辑.
//
// 关键词: fingerprint in-flight 计数, goroutine 可观测性
var fingerprintInFlight atomic.Int64

// GetInFlightFingerprintScans 返回当前正在执行的 fingerprint inner goroutine
// 数量. 主要供运维诊断使用 (从 Go 侧调用, 不通过 yak export 暴露, 避免污染
// 用户面的 servicescan API).
//
// 关键词: GetInFlightFingerprintScans, 可观测性接口
func GetInFlightFingerprintScans() int64 {
	return fingerprintInFlight.Load()
}

// maxOpenPortsPerHost 是单主机"开放端口数"熔断阈值. 在一次扫描中, 如果同一个
// 主机被识别出的开放端口数量达到该阈值, 几乎可以确定这是一个异常目标 (例如
// tarpit / 防火墙 / NAT 设备对所有端口都回 SYN-ACK, 或处于 198.18.0.0/15 这类
// benchmark 保留段的"全开放"主机). 这种主机会让 fingerprint inner goroutine 与
// 日志无限增长, 拖垮整个系统. 命中阈值后直接强制停止整条扫描流, 保护系统健康.
//
// 关键词: scan port 单主机端口熔断阈值, 150 端口, tarpit 防护
const maxOpenPortsPerHost = 150

// hostPortGuard 是 servicescan 的"单主机端口数熔断器". 它统计一次扫描流里每个
// 主机出现的端口数, 当任意主机达到 limit 时触发熔断 (tripped). 上层据此强制
// 停止整条扫描流, 避免对异常目标 (tarpit/全端口响应) 无限扫描刷屏.
//
// 注意: 这里采用"单主机累计端口数"而非严格的"连续端口数"作为判据. 累计计数对
// 多主机交错的结果流更鲁棒, 且异常主机 (单点全开放) 在累计语义下同样会命中,
// 不会漏判; 而严格"连续"语义在多主机交错时容易被打断而失效.
//
// 关键词: servicescan 单主机端口熔断, hostPortGuard, scan port 健康保护
type hostPortGuard struct {
	mu      sync.Mutex
	limit   int
	count   map[string]int
	tripped bool
}

// newHostPortGuard 创建一个熔断器. limit <= 0 表示禁用熔断 (observe 恒返回 false).
func newHostPortGuard(limit int) *hostPortGuard {
	return &hostPortGuard{
		limit: limit,
		count: make(map[string]int),
	}
}

// observe 记录 host 的一次端口出现, 返回 true 表示已触发熔断 (该主机端口数达到
// limit, 或之前已经触发过). 一旦触发, 后续对任何主机的 observe 都返回 true,
// 保证上层能稳定地走到"强制停止"分支.
func (g *hostPortGuard) observe(host string) bool {
	if g == nil || g.limit <= 0 {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.tripped {
		return true
	}
	g.count[host]++
	if g.count[host] >= g.limit {
		g.tripped = true
		return true
	}
	return false
}

func openPortGuardLimit(config *fp.Config) int {
	if config != nil && config.DisableOpenPortGuard {
		return 0
	}
	if config != nil && config.OpenPortGuardLimit > 0 {
		return config.OpenPortGuardLimit
	}
	return maxOpenPortsPerHost
}

// sendMatchResultOrDrop 把 fingerprint 结果送入 outC. 当 ctx 已 cancel
// 时主动放弃, 防止下游已停止消费时 inner goroutine 永久阻塞 (chan send hang).
//
// 这是 fingerprint goroutine 泄漏修复的核心点. 历史问题: 当上游 yak VM 或
// servicescan 调用方在 cancel ctx 后停止 range outC, 但 outC 未关闭 (因为
// inner goroutine 仍在工作), 之前的 `outC <- result` 会永久阻塞, swg.Done
// 永远不调用, swg.Wait/close(outC) 永远不返回, goroutine 永久泄漏.
//
// 关键词: fingerprint goroutine 泄漏修复, outC ctx 短路, drop on cancel
func sendMatchResultOrDrop(ctx context.Context, outC chan<- *fp.MatchResult, result *fp.MatchResult) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case outC <- result:
	case <-ctx.Done():
		fingerprintDropWarnOnce.Do(func() {
			log.Warnf("fingerprint result dropped because ctx cancelled before downstream consumed; subsequent drops will be silent for this process")
		})
	}
}

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

// _scanFingerprint 启动一组并发指纹识别 goroutine, 通过 outC 返回结果.
//
// Context 契约 (重要):
//   - 调用方应该传入一个可被自己取消的 ctx (通过 servicescan.ctx() 注入).
//   - 调用方如果在 outC 关闭之前停止消费 outC, 必须先 cancel ctx, 否则正在
//     发送 result 的 inner goroutine 会通过 sendMatchResultOrDrop 内的
//     ctx.Done() 分支退出.
//   - 当 ctx == nil 时, 走 context.Background(), 退出条件由 fp 内部超时托底.
//
// 关键词: _scanFingerprint ctx 契约, fingerprint goroutine 退出条件
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

	// outC 改为 buffered (容量 = concurrent), 与上游 inner goroutine 数量匹配.
	// 配合 sendMatchResultOrDrop 的 ctx 短路, 构成 "正常路径不阻塞, 异常路径
	// 可解开" 的双保险, 防止 goroutine 泄漏.
	// 关键词: fingerprint outC buffered, goroutine 泄漏防御性叠加
	outCBuf := concurrent
	if outCBuf <= 0 {
		outCBuf = 1
	}
	outC := make(chan *fp.MatchResult, outCBuf)

	// scanCtx / guard 含义同 _scanFromTargetStream. 区别在于: 这里是 TCP 全连接
	// 扫描, 派发循环遍历的是"待探测端口"(attempt)而非"已开放端口", 因此熔断必须按
	// "开放结果"计数, 否则像 1-65535 这种合法全端口扫描会在第 150 个 attempt 就被
	// 误杀. 所以 guard.observe 放在 inner goroutine 判定 result.IsOpen() 之后.
	// 关键词: _scanFingerprint ctx 短路, TCP 扫描按开放端口熔断
	scanCtx, scanCancel := context.WithCancel(ctx)
	guardLimit := openPortGuardLimit(config)
	guard := newHostPortGuard(guardLimit)

	// tripGuard 在 inner goroutine 判定端口开放后调用, 命中阈值则强制停止整条流.
	// 调用方可通过 servicescan.openPortGuardLimit() 调整阈值, 或通过
	// servicescan.disableOpenPortGuard() 显式关闭该保护.
	tripGuard := func(h string) {
		if guard.observe(h) {
			log.Errorf("host [%s] reached scan-port safety threshold (%d open ports in one scan); likely a tarpit/firewall responding on all ports, force stopping the scan to keep the system healthy", h, guardLimit)
			scanCancel()
		}
	}

	go func() {
		// 注意: 这里绝对不能 defer scanCancel(). 派发循环只负责"派发任务", 它结束时
		// 仍有最多 concurrent 个 inner goroutine 在执行 (swg 限流). 若派发一结束就
		// cancel, 会把这些在途扫描连同结果一起取消/丢弃 —— 当端口数 <= 并发时甚至
		// 会取消全部任务, 导致扫描"零结果". scanCancel 只在: (1) 熔断命中时主动调用;
		// (2) swg.Wait 之后 (所有任务真正结束) 调用以释放派生 ctx.
		// 关键词: scanCancel 时机, 不能在派发结束时取消, 在途结果保护
		swg := utils.NewSizedWaitGroup(concurrent)
		portsInt := utils.ParseStringToPorts(port)
	dispatch:
		for _, p := range portsInt {
			for _, hRaw := range utils.ParseStringToHosts(host) {
				// ctx 短路: cancel / 熔断后立即停止派发, 不再继续遍历端口与打印日志.
				if scanCtx.Err() != nil {
					break dispatch
				}
				h := utils.ExtractHost(hRaw)
				if h != hRaw {
					buildinHost, buildinPort, _ := utils.ParseStringToHostPort(hRaw)
					if buildinPort > 0 {
						swg.Add()
						go func() {
							defer swg.Done()
							fingerprintInFlight.Add(1)
							defer fingerprintInFlight.Add(-1)
							if scanCtx.Err() != nil {
								return
							}
							proto, portWithoutProto := utils.ParsePortToProtoPort(buildinPort) // 这里将协议和端口分开，便于后面打印日志
							addr := utils.HostPort(buildinHost, buildinPort)
							if filter.Exist(addr) {
								return
							}
							filter.Insert(addr)
							log.Infof("start task to scan: [%s://%s]", proto, utils.HostPort(buildinHost, portWithoutProto))
							result, err := matcher.MatchWithContext(scanCtx, buildinHost, buildinPort)
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

							if result.IsOpen() {
								tripGuard(buildinHost)
							}
							sendMatchResultOrDrop(scanCtx, outC, result)
						}()
					}
				}

				swg.Add()
				rawPort := p
				rawHost := h
				proto, portWithoutProto := utils.ParsePortToProtoPort(p) // 这里将协议和端口分开，便于后面打印日志
				go func() {
					defer swg.Done()
					fingerprintInFlight.Add(1)
					defer fingerprintInFlight.Add(-1)

					if scanCtx.Err() != nil {
						return
					}
					addr := utils.HostPort(rawHost, rawPort)
					if filter.Exist(addr) {
						return
					}
					filter.Insert(addr)

					//log.Infof("start task to scan: [%s://%s]", proto, utils.HostPort(rawHost, portWithoutProto))
					result, err := matcher.MatchWithContext(scanCtx, rawHost, rawPort)
					if err != nil {
						log.Errorf("failed to scan [%s://%s]: %v", proto, utils.HostPort(rawHost, portWithoutProto), err)
						return
					}

					if result.IsOpen() {
						tripGuard(rawHost)
					}
					sendMatchResultOrDrop(scanCtx, outC, result)
				}()
			}
		}
		go func() {
			swg.Wait()
			scanCancel() // 所有任务结束后再释放派生 ctx
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
//
// Context 契约 (重要):
//   - 通过 servicescan.ctx() 注入的 ctx 会被本函数下游 inner goroutine 用作
//     "发送结果时的退出信号". 调用方在停止 range outC 之前应当 cancel ctx,
//     否则 inner goroutine 会通过 sendMatchResultOrDrop 的 ctx.Done() 分支
//     退出, 结果会被丢弃 (首次丢弃会有一条 log.Warn, 后续静默).
//   - ctx 缺省 (config.Ctx == nil) 时退化为 context.Background(), 仅依赖
//     fp 内部超时托底, 这种情况下 cancel 路径不存在, 建议生产环境显式注入 ctx.
//
// 关键词: _scanFromTargetStream ctx 契约, outC goroutine 泄漏防护
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

	// outC 改为 buffered (容量 = concurrent), 与上游 inner goroutine 数量匹配.
	// 配合 sendMatchResultOrDrop 的 ctx 短路, 构成 "正常路径不阻塞, 异常路径
	// 可解开" 的双保险, 防止 goroutine 泄漏.
	// 关键词: fingerprint outC buffered, goroutine 泄漏防御性叠加
	outCBuf := concurrent
	if outCBuf <= 0 {
		outCBuf = 1
	}
	outC := make(chan *fp.MatchResult, outCBuf)

	// scanCtx 派生自调用方 ctx, 额外承担"内部熔断"职责: 当命中单主机端口数熔断,
	// 或调用方 cancel 时, 通过 scanCancel 让派发循环与所有 inner goroutine 一起
	// 退出, 而不是把 synResults 里剩余目标全部"派发 + 打印"一遍.
	// 关键词: _scanFromTargetStream 派发 ctx 短路, 单主机端口熔断 cancel
	scanCtx, scanCancel := context.WithCancel(ctx)

	// guard 单主机端口数熔断器. 这里 synResults 中的每个元素都是"已开放端口"
	// (synscan 只投递开放端口, SpaceEngine/Ping 等输入也都是已确认的目标),
	// 因此在派发处计数即等价于"单主机开放端口数", 语义正确.
	guardLimit := openPortGuardLimit(config)
	guard := newHostPortGuard(guardLimit)

	go func() {
		// 注意: 不能在这里 defer scanCancel(). 派发循环结束时仍有最多 concurrent 个
		// inner goroutine 在执行, 过早 cancel 会丢弃这些在途结果. scanCancel 只在
		// 熔断命中时主动调用, 以及 swg.Wait 之后调用以释放派生 ctx.
		// 关键词: scanCancel 时机, 在途结果保护
		swg := utils.NewSizedWaitGroup(concurrent)
		for synRes := range synResults {
			// ctx 短路: 历史 bug 的根因之一是这个派发循环从不检查 ctx. cancel 后
			// MatchWithContext / sendMatchResultOrDrop 虽然会快速返回, 但
			// "start task to scan" 日志在调用 MatchWithContext 之前就打印了, 循环
			// 又不提前退出, 于是 cancel 之后仍会把剩余目标全部刷屏. 这里提前 break.
			if scanCtx.Err() != nil {
				break
			}
			// 单主机端口数熔断: 命中阈值视为异常目标 (tarpit/防火墙全端口响应),
			// 强制停止整条扫描流, 保护系统健康. 调用方可通过
			// servicescan.openPortGuardLimit() 调整阈值, 或通过
			// servicescan.disableOpenPortGuard() 显式关闭该保护.
			if guard.observe(synRes.Host) {
				log.Errorf("host [%s] reached scan-port safety threshold (%d open ports in one scan); likely a tarpit/firewall responding on all ports, force stopping the scan to keep the system healthy", synRes.Host, guardLimit)
				scanCancel()
				break
			}
			swg.Add()
			rawPort := synRes.Port
			rawHost := synRes.Host
			proto, portWithoutProto := utils.ParsePortToProtoPort(rawPort) // 这里将协议和端口分开，便于后面打印日志
			go func() {
				defer swg.Done()
				fingerprintInFlight.Add(1)
				defer fingerprintInFlight.Add(-1)

				// 派发到真正执行之间也可能已 cancel, 再次短路, 避免无谓的日志与扫描.
				if scanCtx.Err() != nil {
					return
				}
				log.Infof("start task to scan: [%s://%s]", proto, utils.HostPort(rawHost, portWithoutProto))
				result, err := matcher.MatchWithContext(scanCtx, rawHost, rawPort)
				if err != nil {
					log.Errorf("failed to scan [%s://%s]: %v", proto, utils.HostPort(rawHost, portWithoutProto), err)
					return
				}

				sendMatchResultOrDrop(scanCtx, outC, result)
			}()
		}

		// 退出派发循环后 (无论是正常结束 / cancel / 熔断), 都把 synResults 抽干,
		// 防止上游 producer goroutine 永久阻塞在 `synResults <- r` 上而泄漏.
		// 关键词: synResults drain on exit, producer 不阻塞
		go func() {
			for range synResults {
			}
		}()

		go func() {
			swg.Wait()
			scanCancel() // 所有任务结束后再释放派生 ctx
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

// web servicescan 的配置选项，仅启用 Web 指纹识别(只扫描 Web 服务指纹)
// 在 yak 中通过 servicescan.web 调用
// 返回值:
//   - 一个 servicescan.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：仅使用 web 指纹进行扫描
// result, err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.web())
// die(err)
//
//	for res := range result {
//	    println(res.String())
//	}
//
// ```
func _webOption() fp.ConfigOption {
	v := true
	return func(config *fp.Config) {
		config.OnlyEnableWebFingerprint = v
	}
}

// service servicescan 的配置选项，仅启用服务(nmap)指纹识别，禁用 Web 指纹
// 在 yak 中通过 servicescan.service 调用
// 返回值:
//   - 一个 servicescan.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：仅使用 service 指纹进行扫描
// result, err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.service())
// die(err)
//
//	for res := range result {
//	    println(res.String())
//	}
//
// ```
func _serviceOption() fp.ConfigOption {
	v := true
	return func(config *fp.Config) {
		config.DisableWebFingerprint = v
	}
}

// all servicescan 的配置选项，强制同时启用 Web 与服务(nmap)全部指纹识别
// 在 yak 中通过 servicescan.all 调用
// 返回值:
//   - 一个 servicescan.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用全部指纹进行扫描
// result, err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.all())
// die(err)
//
//	for res := range result {
//	    println(res.String())
//	}
//
// ```
func _allOption() fp.ConfigOption {
	v := true
	return func(config *fp.Config) {
		config.ForceEnableAllFingerprint = v
	}
}

// disableDefaultRule servicescan 的配置选项，禁用内置默认指纹规则(通常配合自定义规则使用)
// 在 yak 中通过 servicescan.disableDefaultRule 调用
// 参数:
//   - b: 可选，是否禁用默认规则，不传时默认为 true
//
// 返回值:
//   - 一个 servicescan.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：禁用默认规则并使用自定义 web 规则
// result, err = servicescan.Scan("127.0.0.1", "80", servicescan.disableDefaultRule(true))
// die(err)
// ```
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

	// 注入可取消 ctx, 调用方应在停止 range 结果 channel 前 cancel ctx,
	// 否则正在向 outC 发送结果的 inner goroutine 会通过 ctx.Done() 分支退出
	// 并产生一条 warn-once 日志 (见 sendMatchResultOrDrop).
	//
	// Example (yak): ctx, cancel = context.WithCancel(context.Background())
	//                defer cancel()
	//                servicescan.Scan("...", "...", servicescan.context(ctx))
	//
	// 关键词: servicescan context 注入, fingerprint cancel
	"context": fp.WithCtx,

	// 整体扫描并发
	"concurrent": fp.WithPoolSize,

	// 单主机开放端口数熔断保护. 默认阈值为 150, 默认保护启用.
	"openPortGuardLimit":   fp.WithOpenPortGuardLimit,
	"disableOpenPortGuard": fp.WithDisableOpenPortGuard,

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
