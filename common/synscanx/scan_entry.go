package synscanx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
)

// Scan is the single SYN/UDP scan entry used by the Yak synscan library and
// the synscan CLIs. The older synscan.Scanner plus hybridscan path is gone.
func Scan(ctx context.Context, targets, ports string, opts ...SynxConfigOption) (chan *synscan.SynScanResult, error) {
	config := NewDefaultConfig()
	if ctx != nil {
		config.Ctx = ctx
	}
	for _, opt := range opts {
		if opt != nil {
			opt(config)
		}
	}
	return ScanWithConfig(targets, ports, config)
}

// ScanWithConfig runs one scan with a config that already has its options applied.
func ScanWithConfig(targets, ports string, config *SynxConfig) (chan *synscan.SynScanResult, error) {
	if config == nil {
		config = NewDefaultConfig()
	}
	if config.Ctx == nil {
		config.Ctx = context.Background()
	}
	ctx := config.Ctx

	sample := chooseRouteSample(targets)
	if sample == "" {
		return nil, errors.New("empty target")
	}
	scanner, err := NewScannerx(ctx, sample, config)
	if err != nil {
		return nil, err
	}
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

// chooseRouteSample picks a non-loopback address when the target list has one,
// so the route lookup does not bind the scan to the loopback interface.
func chooseRouteSample(targets string) string {
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
