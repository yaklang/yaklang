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

// StartMemoryProfile 在采样时长结束后写入一次堆内存 profile（导出名为 pprof.StartMemoryProfile）
// 该调用会阻塞直到采样时长结束（由 pprof.timeout 或 pprof.ctx 控制，默认 15 秒）
//
// 参数:
//   - opts: 可选项，如 pprof.memProfilePath / pprof.timeout / pprof.ctx / pprof.onMemProfileStarted / pprof.onMemProfileFinished
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// prof = file.Join(os.TempDir(), "mem_demo.prof")
// pprof.StartMemoryProfile(pprof.memProfilePath(prof), pprof.timeout(1))
// println(file.IsExisted(prof))   // OUT: true
// assert file.IsExisted(prof), "StartMemoryProfile should write a memory profile file"
// file.Remove(prof)
// ```
func StartMemoryProfile(opts ...Option) error {
	c := NewConfig()
	for _, opt := range opts {
		opt(c)
	}

	// 设置默认的内存profile文件路径，如果没有通过Option指定
	if c.memProfileFile == "" {
		c.memProfileFile = filepath.Join(consts.GetDefaultYakitBaseTempDir(), fmt.Sprintf("mem-%v.prof", utils.DatetimePretty2()))
	}

	// 创建文件用于写入内存profile
	fd, err := os.Create(c.memProfileFile)
	if err != nil {
		return utils.Errorf("create memory profile file failed: %s", err)
	}
	defer fd.Close()

	if c.onMemProfileStarted != nil {
		c.onMemProfileStarted(c.memProfileFile)
	}

	// 设置context，如果没有通过Option指定
	if c.ctx == nil {
		c.ctx = utils.TimeoutContextSeconds(15)
	} else {
		_, ok := c.ctx.Deadline()
		if !ok {
			// 如果context没有设置deadline，则使用默认的15秒
			log.Info("context deadline is not set, use default 15s")
			c.ctx = utils.TimeoutContextSeconds(15)
		}
	}

	// 在context结束前，阻塞等待
	<-c.ctx.Done()

	// Context结束，进行内存采样
	if err := pprof.WriteHeapProfile(fd); err != nil {
		return utils.Errorf("write memory profile failed: %s", err)
	}

	if c.onMemProfileFinished != nil {
		c.onMemProfileFinished(c.memProfileFile, nil)
	}

	return nil
}
