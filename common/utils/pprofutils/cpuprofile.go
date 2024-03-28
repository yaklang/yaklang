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
		if isStarted.IsSet() && c.onCPUProfileFinished != nil {
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
