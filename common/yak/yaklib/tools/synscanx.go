package tools

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/synscanx"
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
	scanner, err := synscanx.NewScannerx(ctx, config)
	if err != nil {
		cancel()
		return nil, err
	}
	scanner.Cancel = cancel
	sendDoneSignal := make(chan struct{})

	_ = sendDoneSignal
	targetCh := make(chan *synscanx.SynxTarget, 16)
	resultCh := make(chan *synscan.SynScanResult, 1000)

	go func() {
		err := scanner.Scan(sendDoneSignal, targetCh, resultCh)
		if err != nil {
			close(resultCh)
			log.Errorf("scan failed: %s", err)
		}
	}()

	// 生产者
	go scanner.SubmitTask(targets, ports, targetCh)

	return resultCh, nil
}
