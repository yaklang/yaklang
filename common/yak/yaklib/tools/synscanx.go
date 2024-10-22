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
// @param {string} target 目标地址，支持 CIDR 格式
// @param {string} port 端口，支持 1-65535、1,2,3、1-100,200-300 格式
// @param {scanOpt} [opts] synscan 扫描参数
// @return {chan *synscan.SynScanResult} 返回结果
// Example:
// ```
// res, err := synscan.Scan("127.0.0.1", "1-65535") //
// die(err)
//
//	for result := range res {
//	  result.Show()
//	}
//
// ```
func _scanx(targets string, ports string, opts ...synscanx.SynxConfigOption) (chan *synscan.SynScanResult, error) {
	config := synscanx.NewDefaultConfig()
	for _, opt := range opts {
		opt(config)
	}
	return do(targets, ports, config)
}

// ScanFromPing 对使用 ping.Scan 探测出的存活结果进行端口扫描，需要配合 ping.Scan 使用
// @param {chan *PingResult} res ping.Scan 的扫描结果
// @param {string} ports 端口，支持 1-65535、1,2,3、1-100,200-300 格式
// @param {scanOpt} [opts] synscan 扫描参数
// @return {chan *synscan.SynScanResult} 返回结果
// Example:
// ```
// pingResult, err = ping.Scan("192.168.1.1/24") // 先进行存活探测
// die(err)
// res, err = synscan.ScanFromPing(pingResult, "1-65535") // 对存活结果进行端口扫描
// die(err)
//
//	for r := range res {
//	  r.Show()
//	}
//
// ```
func _scanxFromPingUtils(res chan *pingutil.PingResult, ports string, opts ...synscanx.SynxConfigOption) (chan *synscan.SynScanResult, error) {
	config := synscanx.NewDefaultConfig()

	for _, opt := range opts {
		opt(config)
	}

	return doFromPingUtils(pingutilsToChan(res), ports, config)

}

func doFromPingUtils(res chan string, ports string, config *synscanx.SynxConfig) (chan *synscan.SynScanResult, error) {
	if config.Ctx == nil {
		config.Ctx = context.Background()
	}
	ctx := config.Ctx
	// 提取第一个ip
	waitSimpleReady := make(chan struct{})
	var sample string
	inputCh := make(chan string)
	go func() {
		defer close(inputCh)
		for {
			select {
			case v, ok := <-res:
				if !ok {
					return
				}
				if sample == "" {
					sample = v
					waitSimpleReady <- struct{}{}
				}

				inputCh <- v

			case <-ctx.Done():
				return
			}
		}
	}()
	// 等待拿到第一个ip
	<-waitSimpleReady

	// 创建Scanner
	scanner, err := synscanx.NewScannerx(ctx, sample, config)
	if err != nil {
		return nil, err
	}
	scanner.FromPing = true
	targetCh := scanner.SubmitTargetFromPing(inputCh, ports)
	resultCh, err := scanner.Scan(targetCh)
	if err != nil {
		log.Errorf("scan failed: %s", err)
	}
	return resultCh, nil

}

func do(targets, ports string, config *synscanx.SynxConfig) (chan *synscan.SynScanResult, error) {
	if config.Ctx == nil {
		config.Ctx = context.Background()
	}
	ctx := config.Ctx

	log.Debugf("targets: %s", targets)
	sample := utils.ParseStringToHosts(targets)[0]
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
}
