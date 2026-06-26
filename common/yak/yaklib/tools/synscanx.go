package tools

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/synscanx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pcapfix"
	"github.com/yaklang/yaklang/common/utils/pingutil"
)

// Scan 使用 SYN 扫描技术进行端口扫描，它不必打开一个完整的TCP连接，只发送一个SYN包，就能做到打开连接的效果，然后等待对端的反应
// 参数:
//   - targets: 目标地址，支持 CIDR 格式
//   - ports: 端口，支持 1-65535、1,2,3、1-100,200-300 格式
//   - opts: 零个或多个 synscan 扫描参数
//
// 返回值:
//   - chan *synscan.SynScanResult: SYN 扫描结果管道，逐个产出开放端口
//   - error: 启动失败时返回错误
//
// <|EXAMPLE_START|> synscan.Scan 的基础 SYN 扫描
// ```
// // 对本机常见端口做 SYN 扫描，遍历结果管道逐个打印开放端口
// res, err = synscan.Scan("127.0.0.1", "22,80,443,3306,8080-8090")
// die(err) // 启动失败(如缺少权限)时停止脚本
// for result := range res {
//     result.Show() // 打印 OPEN: host:port from synscan
// }
// ```
// <|EXAMPLE_END|>
//
// <|EXAMPLE_START|> 自定义发包后的等待时间
// ```
// // SYN 扫描是批量发包后统一等待回包，wait 设置等待秒数(网络差可调大以减少漏报)
// res, err = synscan.Scan("192.168.1.1/24", "1-65535", synscan.wait(5))
// die(err)
// for result := range res {
//     println(f"open: ${result.Host}:${result.Port}")
// }
// ```
// <|EXAMPLE_END|>
//
// <|EXAMPLE_START|> 限速扫描并将开放端口写入文件
// ```
// // 控制发包速率，并把开放端口写入文件，每行带 tcp:// 前缀便于后续处理
// res, err = synscan.Scan("10.0.0.0/24", "80,443",
//     synscan.rateLimit(1, 1000),          // 每 1 毫秒最多发送 1000 个包
//     synscan.outputFile("open_ports.txt"),
//     synscan.outputPrefix("tcp://"),
// )
// die(err)
// for result := range res {
//     result.Show()
// }
// ```
// <|EXAMPLE_END|>
func _scanx(targets string, ports string, opts ...synscanx.SynxConfigOption) (chan *synscan.SynScanResult, error) {
	config := synscanx.NewDefaultConfig()
	for _, opt := range opts {
		opt(config)
	}
	return do(targets, ports, config)
}

// ScanFromPing 对使用 ping.Scan 探测出的存活结果进行端口扫描，需要配合 ping.Scan 使用
// 参数:
//   - res: ping.Scan 的扫描结果管道
//   - ports: 端口，支持 1-65535、1,2,3、1-100,200-300 格式
//   - opts: 零个或多个 synscan 扫描参数
//
// 返回值:
//   - chan *synscan.SynScanResult: SYN 扫描结果管道，逐个产出开放端口
//   - error: 启动失败时返回错误
//
// <|EXAMPLE_START|> 先 ping 探活再做 SYN 端口扫描
// ```
// // 先用 ping 探活，再只对存活主机做 SYN 端口扫描，避免对死主机无谓发包
// pingResult, err = ping.Scan("192.168.1.1/24")
// die(err)
// res, err = synscan.ScanFromPing(pingResult, "22,80,443,3389")
// die(err)
// for result := range res {
//     result.Show()
// }
// ```
// <|EXAMPLE_END|>
func _scanxFromPingUtils(res chan *pingutil.PingResult, ports string, opts ...synscanx.SynxConfigOption) (chan *synscan.SynScanResult, error) {
	config := synscanx.NewDefaultConfig()

	for _, opt := range opts {
		opt(config)
	}
	if config.Ctx == nil {
		config.Ctx = context.Background()
	}

	return doFromPingUtils(_pingutilsToChan(config.Ctx, res), ports, config)

}

func _pingutilsToChan(ctx context.Context, res chan *pingutil.PingResult) chan string {
	if ctx == nil {
		ctx = context.Background()
	}
	c := make(chan string)
	go func() {
		defer close(c)
		hasValidResult := false
		for {
			select {
			case <-ctx.Done():
				return
			case result, ok := <-res:
				if !ok {
					if !hasValidResult {
						select {
						case <-ctx.Done():
						case c <- "":
						}
					}
					return
				}
				if result.Ok {
					hasValidResult = true
					select {
					case <-ctx.Done():
						return
					case c <- result.IP:
					}
				}
			}
		}
	}()
	return c
}

func doFromPingUtils(res chan string, ports string, config *synscanx.SynxConfig) (chan *synscan.SynScanResult, error) {
	if config.Ctx == nil {
		config.Ctx = context.Background()
	}
	ctx := config.Ctx

	// 先获取第一个有效的目标
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var firstTarget string
	var ok bool
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case firstTarget, ok = <-res:
	}
	if !ok || firstTarget == "" {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, utils.Errorf("no valid ping results found")
	}

	// 创建Scanner
	scanner, err := synscanx.NewScannerx(ctx, firstTarget, config)
	if err != nil {
		return nil, err
	}
	scanner.FromPing = true

	inputCh := make(chan string)
	go func() {
		defer close(inputCh)
		// 先发送第一个目标
		select {
		case <-ctx.Done():
			return
		case inputCh <- firstTarget:
		}
		// 转发剩余的有效目标
		for {
			select {
			case <-ctx.Done():
				return
			case target, ok := <-res:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					return
				case inputCh <- target:
				}
			}
		}
	}()

	targetCh := scanner.SubmitTargetFromPing(inputCh, ports)
	resultCh, err := scanner.Scan(targetCh)
	if err != nil {
		log.Errorf("scan failed: %s", err)
		return nil, err
	}
	return resultCh, nil

}

func do(targets, ports string, config *synscanx.SynxConfig) (chan *synscan.SynScanResult, error) {
	if config.Ctx == nil {
		config.Ctx = context.Background()
	}
	ctx := config.Ctx

	log.Debugf("targets: %s", targets)
	sample := chooseScanxRouteSample(targets)
	if sample == "" {
		return nil, utils.Errorf("empty target")
	}
	scanner, err := synscanx.NewScannerx(ctx, sample, config)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := recover(); err != nil {
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	targetCh, err := scanner.SubmitTarget(targets, ports)
	if err != nil {
		return nil, err
	}
	resultCh, err := scanner.Scan(targetCh)
	if err != nil {
		log.Errorf("scan failed: %s", err)
		return nil, err
	}
	return resultCh, nil

}

func chooseScanxRouteSample(targets string) string {
	targetList := utils.ParseStringToHosts(targets)
	if len(targetList) == 0 {
		return ""
	}
	for _, target := range targetList {
		if !utils.IsLoopback(target) {
			return target
		}
	}
	return targetList[0]
}

var SynxPortScanExports = map[string]interface{}{
	"FixPermission": pcapfix.Fix,

	"Scan":         _scanx,
	"ScanFromPing": _scanxFromPingUtils,

	"callback":           synscanx.WithCallback,
	"submitTaskCallback": synscanx.WithSubmitTaskCallback,
	"excludeHosts":       synscanx.WithExcludeHosts,
	"excludePorts":       synscanx.WithExcludePorts,
	"wait":               synscanx.WithWaiting,
	"outputFile":         synscanx.WithOutputFile,
	"outputPrefix":       synscanx.WithOutputFilePrefix,
	"initHostFilter":     synscanx.WithInitFilterHosts,
	"initPortFilter":     synscanx.WithInitFilterPorts,
	"rateLimit":          synscanx.WithRateLimit,
	"concurrent":         synscanx.WithConcurrent,
	"iface":              synscanx.WithIface,
	"shuffle":            synscanx.WithShuffle,
	"maxPorts":           synscanx.WithMaxOpenPorts,

	// context 注入可取消 ctx: cancel 时 syn 发包/提交/结果投递循环都会通过
	// s.ctx.Done() 立刻短路退出 (见 synscanx.Scannerx). 这是让上层 (如 AI 插件)
	// 取消任务时, syn 扫描能尽快停止、不再对异常目标 (tarpit/全端口响应) 持续
	// 发包刷屏的关键. 缺省为 context.Background().
	// 关键词: synscan context 注入, syn 扫描可取消, AI 插件 cancel 传播
	"context": synscanx.WithCtx,
}
