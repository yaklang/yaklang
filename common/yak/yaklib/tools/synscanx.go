package tools

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/synscanx"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
	"time"
)

func _scanx(targets string, ports string, opts ...synscanx.SynxConfigOption) (chan *synscan.SynScanResult, error) {
	config := synscanx.NewDefaultConfig()
	defer config.ExcludePorts.Close()

	for _, opt := range opts {
		opt(config)
	}

	return do(targets, ports, config)
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
	"Scan": _scanx,

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
}
