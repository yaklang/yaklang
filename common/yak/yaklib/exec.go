package yaklib

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"syscall"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/google/shlex"
)

func _execStringToCommand(ctx context.Context, s string) (*exec.Cmd, error) {
	cmds, err := shlex.Split(s)
	if err != nil {
		return nil, utils.Errorf("parse string to cmd args failed: %s, reason: %v", s, err)
	}
	if cmds == nil {
		return nil, utils.Errorf("error system cmd: %v", s)
	}

	if len(cmds) > 1 {
		return exec.CommandContext(ctx, cmds[0], cmds[1:]...), nil
	} else {
		return exec.CommandContext(ctx, cmds[0]), nil
	}
}

// 执行系统命令
func _execSystem(ctx context.Context, i string) ([]byte, error) {
	s, err := _execStringToCommand(ctx, i)
	if err != nil {
		return nil, err
	}
	raw, err := s.CombinedOutput()
	if err != nil {
		return nil, utils.Errorf("system[%v] failed: %v", i, err)
	}
	return raw, err
}

func _execSystemBatch(i string, opts ...poolOpt) {
	config := &_execPoolConfig{
		concurrent: 20,
	}

	for _, opt := range opts {
		opt(config)
	}

	if config.concurrent <= 0 {
		config.concurrent = 20
	}

	swg := utils.NewSizedWaitGroup(config.concurrent)
	defer swg.Wait()

	for _, cmdRaw := range _fuzz(i) {
		cmdRaw := cmdRaw
		swg.Add()
		go func() {
			defer swg.Done()

			log.Infof("start exec: %v", cmdRaw)
			ctx := context.Background()
			if config.timeout > 0 {
				ctx = utils.TimeoutContext(config.timeout)
			}
			raw, err := _execSystem(ctx, cmdRaw)
			if err != nil {
				log.Infof("exec[%v] failed: %v", cmdRaw, err)
				return
			}

			if config.callback != nil {
				config.callback(cmdRaw, raw)
			}
		}()
	}
}

func _checkExecCrash(c *exec.Cmd) (bool, error) {
	sysType := runtime.GOOS
	if sysType == "windows" {
		return true, errors.New("Unspport Windows now")
	}
	processState := c.ProcessState
	status := processState.Sys().(syscall.WaitStatus)
	if status.Signaled() {
		signal := status.Signal()
		if signal == syscall.SIGSEGV || signal == syscall.SIGABRT {
			return true, nil
		}
	}
	if status.Stopped() {
		signal := status.StopSignal()
		if signal == syscall.SIGSEGV || signal == syscall.SIGABRT {
			return true, nil
		}
	}
	return false, nil
}

// ////////////////////////////////////////////////////
type _execPoolConfig struct {
	concurrent int
	timeout    time.Duration
	callback   func(cmd string, results []byte)
}

type poolOpt func(c *_execPoolConfig)

func _execConcurrent(i int) poolOpt {
	return func(c *_execPoolConfig) {
		c.concurrent = i
	}
}

func _execTimeout(i float64) poolOpt {
	return func(c *_execPoolConfig) {
		c.timeout = utils.FloatSecondDuration(i)
	}
}

func _execSetCallback(f func(string, []byte)) poolOpt {
	return func(c *_execPoolConfig) {
		c.callback = f
	}
}

func _execWatchStdout(i string, timeout float64, f func(raw []byte) bool) error {
	defer func() {
		if err := recover(); err != nil {
			log.Error(err)
		}
	}()

	rootCtx, cancel := context.WithCancel(utils.TimeoutContext(utils.FloatSecondDuration(timeout)))
	cmd, err := _execStringToCommand(rootCtx, i)
	if err != nil {
		return utils.Errorf("create system command[%v] failed: %v", i, err)
	}
	reader, err := cmd.StdoutPipe()
	if err != nil {
		return utils.Errorf("create[%v] stdout pipe failed: %s", i, err)
	}

	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Error(err)
			}
		}()
		defer cancel()
		utils.ReadWithContextTickCallback(
			context.Background(), reader, f, time.Second,
		)
	}()

	err = cmd.Run()
	if err != nil {
		return utils.Errorf("exec %v failed: %s", i, err.Error())
	}
	return nil
}

func _execWatchStderr(i string, timeout float64, f func(raw []byte) bool) error {
	defer func() {
		if err := recover(); err != nil {
			log.Error(err)
		}
	}()

	rootCtx, cancel := context.WithCancel(utils.TimeoutContext(utils.FloatSecondDuration(timeout)))
	_ = cancel
	cmd, err := _execStringToCommand(rootCtx, i)
	if err != nil {
		return utils.Errorf("create system command[%v] failed: %v", i, err)
	}
	reader, err := cmd.StderrPipe()
	if err != nil {
		return utils.Errorf("create[%v] stderr pipe failed: %s", i, err)
	}
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Error(err)
			}
		}()
		defer cancel()
		utils.ReadWithContextTickCallback(
			context.Background(), reader, f, time.Second,
		)
	}()

	err = cmd.Run()
	if err != nil {
		return utils.Errorf("exec %v failed: %s", i, err.Error())
	}
	return nil
}

// 系统命令执行导出接口
var ExecExports = map[string]interface{}{
	"CommandContext": _execStringToCommand,
	"Command": func(i string) (*exec.Cmd, error) {
		return _execStringToCommand(context.Background(), i)
	},
	//检查是否crash
	"CheckCrash": _checkExecCrash,

	// 批量命令执行
	"SystemBatch": _execSystemBatch,

	// 带上下文的基础命令执行
	"SystemContext": _execSystem,

	// 基础命令执行
	"System": func(i string) ([]byte, error) {
		return _execSystem(context.Background(), i)
	},

	// 监控输出
	"WatchStdout": _execWatchStdout,
	"WatchOutput": _execWatchStdout,
	"WatchStderr": _execWatchStderr,

	"timeout":    _execTimeout,
	"callback":   _execSetCallback,
	"concurrent": _execConcurrent,
}
