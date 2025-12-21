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

	"github.com/yaklang/yaklang/common/utils/shlex"
)

// CommandContext 创建一个受上下文控制的命令结构体，其第一个参数是上下文，第二个参数是要执行的命令
// Example:
// ```
// cmd = exec.CommandContext(context.New(), "ls -al")
// output = cmd.CombineOutput()~
// dump(output)
// ```
func commandContext(ctx context.Context, s string) (*exec.Cmd, error) {
	cmds, err := shlex.Split(s)
	if err != nil {
		return nil, utils.Errorf("parse string to cmd args failed: %s, reason: %v", s, err)
	}
	if cmds == nil {
		return nil, utils.Errorf("error system cmd: %v", s)
	}

	var cmd *exec.Cmd
	if len(cmds) > 1 {
		cmd = exec.CommandContext(ctx, cmds[0], cmds[1:]...)
	} else {
		cmd = exec.CommandContext(ctx, cmds[0])
	}

	// Set up process group for proper cleanup when context is cancelled.
	// This ensures the entire process tree is killed on timeout.
	// Implementation is platform-specific (see exec_unix.go and exec_windows.go).
	setupProcessGroup(cmd)

	return cmd, nil
}

// Command 创建一个命令结构体
// Example:
// ```
// cmd = exec.Command("ls -al")
// output = cmd.CombineOutput()~
// dump(output)
// ```
func command(s string) (*exec.Cmd, error) {
	return commandContext(context.Background(), s)
}

// SystemContext 创建受上下文控制的命令结构体并执行，返回结果与错误
// Example:
// ```
// output, err = exec.SystemContext(context.New(),"ls -al")~
// dump(output)
// ```
func systemContext(ctx context.Context, i string) ([]byte, error) {
	s, err := commandContext(ctx, i)
	if err != nil {
		return nil, err
	}
	raw, err := s.CombinedOutput()
	if err != nil {
		return nil, utils.Errorf("system[%v] failed: %v", i, err)
	}
	return raw, err
}

// System 创建命令结构体并执行，返回结果与错误
// Example:
// ```
// output, err = exec.System("ls -al")~
// dump(output)
// ```
func system(i string) ([]byte, error) {
	return systemContext(context.Background(), i)
}

// SystemBatch 批量执行命令，它的第一个参数为要批量执行的命令(支持 fuzztag )，接下来可以接收零个到多个选项，用于对批量命令执行进行配置，例如设置超时时间，回调函数等
// Example:
// ```
// exec.SystemBatch("ping 192.168.1.{{int(1-100)}}",
// exec.timeout(10),
// exec.concurrent(20),
// exec.callback(func(cmd, result) {
// log.Infof("exec[%v] result: %v", cmd, string(result))
// })
// ```
func systemBatch(i string, opts ...execPoolOpt) {
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
			raw, err := systemContext(ctx, cmdRaw)
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

// CheckCrash 检查命令执行是否发生了崩溃，不支持 Windows 系统，返回值为是否崩溃和错误信息
// Example:
// ```
// cmd = exec.Command("ls -al")~
// isCrash = exec.CheckCrash(cmd)~
// if isCrash {
// // ...
// }
// ```
func checkCrash(c *exec.Cmd) (bool, error) {
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

type execPoolOpt func(c *_execPoolConfig)

// concurrent 是一个选项参数，用于设置批量命令执行的并发数，默认为 20
// Example:
// ```
// exec.SystemBatch("ping 192.168.1.{{int(1-100)}}",
// exec.timeout(10),
// exec.concurrent(20),
// exec.callback(func(cmd, result) {
// log.Infof("exec[%v] result: %v", cmd, string(result))
// })
// ```
func _execConcurrent(i int) execPoolOpt {
	return func(c *_execPoolConfig) {
		c.concurrent = i
	}
}

// timeout 是一个选项参数，用于设置批量命令执行的超时时间，单位为秒
// Example:
// ```
// exec.SystemBatch("ping 192.168.1.{{int(1-100)}}",
// exec.timeout(10),
// exec.concurrent(20),
// exec.callback(func(cmd, result) {
// log.Infof("exec[%v] result: %v", cmd, string(result))
// })
// ```
func _execTimeout(i float64) execPoolOpt {
	return func(c *_execPoolConfig) {
		c.timeout = utils.FloatSecondDuration(i)
	}
}

// callback 是一个选项参数，用于设置批量命令执行的回调函数，回调函数的第一个参数为执行的命令，第二个参数为执行的结果，在回调函数中可以对命令执行结果进行处理
// Example:
// ```
// exec.SystemBatch("ping 192.168.1.{{int(1-100)}}",
// exec.timeout(10),
// exec.concurrent(20),
// exec.callback(func(cmd, result) {
// log.Infof("exec[%v] result: %v", cmd, string(result))
// })
// ```
func _execSetCallback(f func(string, []byte)) execPoolOpt {
	return func(c *_execPoolConfig) {
		c.callback = f
	}
}

// WatchStdout 执行命令并监控标准输出，当标准输出有数据时，会调用回调函数处理数据，回调函数的参数为标准输出的原始数据，返回值为是否继续监控
// Example:
// ```
// exec.WatchStdout("tail -f /tmp/log", 60, func(raw) {
// log.Infof("stdout: %v", string(raw))
// return true
// }
// ```
func execWatchStdout(i string, timeout float64, f func(raw []byte) bool) error {
	defer func() {
		if err := recover(); err != nil {
			log.Error(err)
		}
	}()

	rootCtx, cancel := context.WithCancel(utils.TimeoutContext(utils.FloatSecondDuration(timeout)))
	defer cancel()
	cmd, err := commandContext(rootCtx, i)
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

// WatchStderr 执行命令并监控标准错误，当标准错误有数据时，会调用回调函数处理数据，回调函数的参数为标准错误的原始数据，返回值为是否继续监控
// Example:
// ```
// exec.WatchStderr("tail -f /tmp/log", 60, func(raw) {
// log.Infof("stderr: %v", string(raw))
// return true
// }
// ```
func execWatchStderr(i string, timeout float64, f func(raw []byte) bool) error {
	defer func() {
		if err := recover(); err != nil {
			log.Error(err)
		}
	}()

	rootCtx, cancel := context.WithCancel(utils.TimeoutContext(utils.FloatSecondDuration(timeout)))
	_ = cancel
	cmd, err := commandContext(rootCtx, i)
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
	// 创建命令
	"Command":        command,
	"CommandContext": commandContext,
	// 检查是否crash
	"CheckCrash": checkCrash,

	// 基础命令执行
	"System": system,
	// 带上下文的基础命令执行
	"SystemContext": systemContext,
	// 批量命令执行
	"SystemBatch": systemBatch,

	// 批量命令执行选项
	"timeout":    _execTimeout,
	"callback":   _execSetCallback,
	"concurrent": _execConcurrent,

	// 监控输出
	"WatchStdout": execWatchStdout,
	"WatchOutput": execWatchStdout,
	"WatchStderr": execWatchStderr,
}
