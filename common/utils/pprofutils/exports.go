package pprofutils

import (
	"sync"

	"github.com/yaklang/yaklang/common/log"
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

// StartCPUAndMemoryProfile 同时进行 CPU 与内存采样（导出名为 pprof.StartCPUAndMemoryProfile）
// 两类采样并行进行，均受同一组选项控制；该调用会阻塞直到采样时长结束
//
// 参数:
//   - opts: 可选项，如 pprof.cpuProfilePath / pprof.memProfilePath / pprof.timeout / pprof.ctx / pprof.callback 等
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// cpuPath = file.Join(os.TempDir(), "cm_cpu_demo.prof")
// memPath = file.Join(os.TempDir(), "cm_mem_demo.prof")
// finished = 0
// pprof.StartCPUAndMemoryProfile(
//
//	pprof.cpuProfilePath(cpuPath),
//	pprof.memProfilePath(memPath),
//	pprof.timeout(1),
//	pprof.callback(func(p) { finished++ }),
//
// )
// println(file.IsExisted(cpuPath) && file.IsExisted(memPath))   // OUT: true
// assert file.IsExisted(cpuPath) && file.IsExisted(memPath), "should produce both CPU and memory profiles"
// file.Remove(cpuPath); file.Remove(memPath)
// ```
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
