package pprofutils

import (
	"github.com/yaklang/yaklang/common/log"
	"sync"
)

var Exports = map[string]any{
	"StartCPUAndMemoryProfile": StartCPUAndMemoryProfile,
	"StartCPUProfile":          StartCPUProfile,
	"StartMemoryProfile":       StartMemoryProfile,
	"AutoAnalyzeFile":          AutoAnalyzeFile,

	"cpuProfilePath":       WithCPUProfileFile,
	"memProfilePath":       WithMemProfileFile,
	"ctx":                  WithContext,
	"onCPUProfileFinished": WithOnCPUProfileFinished,
	"onCPUProfileStarted":  WithOnCPUProfileStarted,
	"onMemProfileStarted":  WithOnMemProfileStarted,
	"onMemProfileFinished": WithOnMemProfileFinished,
	"timeout":              WithTimeout,
	"callback":             WithFinished,
}

func StartCPUAndMemoryProfile(opts ...Option) error {
	// 并行开始CPU profiling
	wg := new(sync.WaitGroup)
	wg.Add(2)

	errs := make(chan error, 2)
	go func() {
		defer func() {
			wg.Done()
		}()
		err := StartCPUProfile(opts...)
		errs <- err
		if err != nil {
			log.Errorf("CPU Profile error: %v", err)
		}
	}()

	go func() {
		defer func() {
			wg.Done()
		}()
		// 在CPU profiling结束后开始Memory profiling
		err := StartMemoryProfile(opts...)
		if err != nil {
			log.Errorf("Memory Profile error: %v", err)
		}
	}()

	wg.Wait()
	return nil
}
