package pprofutils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// StartCPUProfile 开始 CPU 采样并将结果写入 profile 文件（导出名为 pprof.StartCPUProfile）
// 该调用会阻塞直到采样时长结束（由 pprof.timeout 或 pprof.ctx 控制，默认 15 秒）
//
// 参数:
//   - opts: 可选项，如 pprof.cpuProfilePath / pprof.timeout / pprof.ctx / pprof.onCPUProfileStarted / pprof.onCPUProfileFinished
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// prof = file.Join(os.TempDir(), "cpu_demo.prof")
// pprof.StartCPUProfile(pprof.cpuProfilePath(prof), pprof.timeout(1))
// println(file.IsExisted(prof))   // OUT: true
// assert file.IsExisted(prof), "StartCPUProfile should write a CPU profile file"
// file.Remove(prof)
// ```
func StartCPUProfile(opts ...Option) error {
	c := NewConfig()
	for _, opt := range opts {
		opt(c)
	}

	if c.cpuProfileFile == "" {
		c.cpuProfileFile = filepath.Join(consts.GetDefaultYakitBaseTempDir(), fmt.Sprintf("cpu-%v.prof", utils.DatetimePretty2()))
	}

	fd, err := os.Create(c.cpuProfileFile)
	if err != nil {
		return utils.Errorf("create cpu profile file failed: %s", err)
	}

	isStarted := utils.NewAtomicBool()

	defer func() {
		fd.Close()
		if c.onCPUProfileFinished == nil {
			return
		}
		if isStarted.IsSet() {
			c.onCPUProfileFinished(c.cpuProfileFile, nil)
		} else {
			c.onCPUProfileFinished(c.cpuProfileFile, utils.Error("cpu profile is not started"))
		}
	}()

	if c.onCPUProfileStarted != nil {
		c.onCPUProfileStarted(c.cpuProfileFile)
	}

	if c.ctx == nil {
		c.ctx = utils.TimeoutContextSeconds(15)
	} else {
		_, ok := c.ctx.Deadline()
		if !ok {
			log.Info("context deadline is not set, use default 15s")
			c.ctx = utils.TimeoutContextSeconds(15)
		}
	}

	// start cpu profile
	select {
	case <-c.ctx.Done():
		return utils.Errorf("context is done")
	default:
	}

	err = pprof.StartCPUProfile(fd)
	if err != nil {
		return utils.Errorf("start cpu profile failed: %s", err)
	}
	isStarted.Set()
	if c.onCPUProfileStarted != nil {
		c.onCPUProfileStarted(c.cpuProfileFile)
	}
	defer pprof.StopCPUProfile()
	<-c.ctx.Done()
	return nil
}
