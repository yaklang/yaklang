package pprofutils

import (
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"path/filepath"
	"runtime/pprof"
)

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

