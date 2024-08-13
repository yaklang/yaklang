package tools

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/synscanx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pcapfix"
	"github.com/yaklang/yaklang/common/utils/pingutil"
	"sync"
	"time"
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
	count := len(utils.ParseStringToHosts(targets))
	config := synscanx.NewDefaultConfig()
	opts = append(opts, synscanx.TargetCount(count))
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
	processedRes := make(chan string, 16)

	var sample string
	select {
	case sample = <-res:
		processedRes <- sample
	case <-time.After(15 * time.Second):
		return nil, utils.Error("ping timeout")
	}
	ctx, cancel := context.WithCancel(context.Background())

	scanner, err := synscanx.NewScannerx(ctx, sample, config)
	if err != nil {
		cancel()
		return nil, err
	}
	scanner.FromPing = true
	sendDoneSignal := make(chan struct{})

	targetCh := make(chan *synscanx.SynxTarget, 16)
	resultCh := make(chan *synscan.SynScanResult, 1024)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner.SubmitTargetFromPing(processedRes, ports, targetCh)
		close(targetCh)
		<-sendDoneSignal
		close(resultCh)
		log.Infof("send done signal")
	}()

	time.Sleep(100 * time.Millisecond)

	go func() {
		defer wg.Done()

		err := scanner.Scan(sendDoneSignal, targetCh, resultCh)
		if err != nil {
			close(resultCh)
			log.Errorf("scan failed: %s", err)
		}
	}()

	go func() {

		for pingResult := range res {
			processedRes <- pingResult
		}
		close(processedRes)
	}()

	go func() {
		wg.Wait()
		cancel()
	}()

	return resultCh, nil
}

func do(targets, ports string, config *synscanx.SynxConfig) (chan *synscan.SynScanResult, error) {
	ctx, cancel := context.WithCancel(context.Background())
	log.Infof("targets: %s", targets)
	sample := utils.ParseStringToHosts(targets)[0]
	scanner, err := synscanx.NewScannerx(ctx, sample, config)
	if err != nil {
		cancel()
		return nil, err
	}
	sendDoneSignal := make(chan struct{})

	targetCh := make(chan *synscanx.SynxTarget, 16)
	resultCh := make(chan *synscan.SynScanResult, 1024)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner.SubmitTarget(targets, ports, targetCh)
		close(targetCh)
		<-sendDoneSignal
		close(resultCh)
		log.Infof("send done signal")
	}()

	time.Sleep(100 * time.Millisecond)

	go func() {
		defer wg.Done()

		err := scanner.Scan(sendDoneSignal, targetCh, resultCh)
		if err != nil {
			close(resultCh)
			log.Errorf("scan failed: %s", err)
		}
	}()

	go func() {
		wg.Wait()
		cancel()
	}()

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
