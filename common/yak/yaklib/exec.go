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

// CommandContext 创建一个受上下文控制的命令对象（导出名为 exec.CommandContext）
// 命令字符串会按 shell 规则切分参数；上下文取消时会终止整个进程组
//
// 参数:
//   - ctx: 控制命令生命周期的上下文
//   - s: 完整命令字符串（如 "echo hello"）
//
// 返回值:
//   - 命令对象（可调用 CombinedOutput/Run 等方法）
//   - 错误信息（命令解析失败时返回）
//
// Example:
// ```
// cmd = exec.CommandContext(context.Background(), "echo ctx")~
// output = cmd.CombinedOutput()~
// println(string(output))   // OUT: ctx
// assert str.Contains(string(output), "ctx"), "CommandContext output should contain the echoed text"
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

// Command 创建一个命令对象（导出名为 exec.Command）
// 等价于使用 context.Background() 的 CommandContext
//
// 参数:
//   - s: 完整命令字符串（如 "echo hello"）
//
// 返回值:
//   - 命令对象（可调用 CombinedOutput/Run 等方法）
//   - 错误信息（命令解析失败时返回）
//
// Example:
// ```
// cmd = exec.Command("echo hello")~
// output = cmd.CombinedOutput()~
// println(string(output))   // OUT: hello
// assert str.Contains(string(output), "hello"), "Command output should contain the echoed text"
// ```
func command(s string) (*exec.Cmd, error) {
	return commandContext(context.Background(), s)
}

// SystemContext 在指定上下文下执行命令并返回合并的输出（导出名为 exec.SystemContext）
// 同时收集标准输出与标准错误
//
// 参数:
//   - ctx: 控制命令生命周期的上下文
//   - i: 完整命令字符串
//
// 返回值:
//   - 命令的合并输出（stdout+stderr）
//   - 错误信息（命令执行失败时返回）
//
// Example:
// ```
// output = exec.SystemContext(context.Background(), "echo sysctx")~
// println(string(output))   // OUT: sysctx
// assert str.Contains(string(output), "sysctx"), "SystemContext output should contain the echoed text"
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

// System 执行命令并返回合并的输出（导出名为 exec.System）
// 等价于使用 context.Background() 的 SystemContext，同时收集 stdout 与 stderr
//
// 参数:
//   - i: 完整命令字符串
//
// 返回值:
//   - 命令的合并输出（stdout+stderr）
//   - 错误信息（命令执行失败时返回）
//
// Example:
// ```
// output = exec.System("echo systest")~
// println(string(output))   // OUT: systest
// assert str.Contains(string(output), "systest"), "System output should contain the echoed text"
// ```
func system(i string) ([]byte, error) {
	return systemContext(context.Background(), i)
}

// SystemBatch 批量并发执行命令（导出名为 exec.SystemBatch）
// 第一个参数为命令模板，支持 fuzztag 展开为多条命令；其余为可选项，用于配置并发数、超时与结果回调
//
// 参数:
//   - i: 命令模板（支持 fuzztag，如 "echo {{int(1-3)}}"）
//   - opts: 可选项，如 exec.concurrent / exec.timeout / exec.callback
//
// Example:
// ```
// results = make([]string, 0)
// lock = sync.NewMutex()
// exec.SystemBatch("echo batch{{int(1-3)}}",
//
//	exec.timeout(10),
//	exec.concurrent(5),
//	exec.callback(func(cmd, result) {
//	    lock.Lock(); results = append(results, string(result)); lock.Unlock()
//	}),
//
// )
// println(len(results))   // OUT: 3
// assert len(results) == 3, "SystemBatch should run the three expanded commands"
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

// CheckCrash 检查一个已执行完成的命令是否因崩溃信号（SIGSEGV/SIGABRT）而终止（导出名为 exec.CheckCrash）
// 不支持 Windows 系统；需在命令 Run/Wait 之后调用
//
// 参数:
//   - c: 已执行完成的命令对象
//
// 返回值:
//   - 是否检测到崩溃
//   - 错误信息（如在 Windows 上调用）
//
// Example:
// ```
// cmd = exec.Command("echo done")~
// cmd.Run()
// isCrash = exec.CheckCrash(cmd)~
// println(isCrash)   // OUT: false
// assert isCrash == false, "a normally exited command should not be reported as crashed"
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

// concurrent 设置 exec.SystemBatch 的并发执行数，默认为 20（导出名为 exec.concurrent）
// 作为 exec.SystemBatch 的可选项使用
//
// 参数:
//   - i: 并发数
//
// 返回值:
//   - 可传入 exec.SystemBatch 的选项
//
// Example:
// ```
// results = make([]string, 0)
// lock = sync.NewMutex()
// exec.SystemBatch("echo c{{int(1-3)}}",
//
//	exec.concurrent(2),
//	exec.callback(func(cmd, result) { lock.Lock(); results = append(results, string(result)); lock.Unlock() }),
//
// )
// println(len(results))   // OUT: 3
// assert len(results) == 3, "concurrent option should still run all expanded commands"
// ```
func _execConcurrent(i int) execPoolOpt {
	return func(c *_execPoolConfig) {
		c.concurrent = i
	}
}

// timeout 设置 exec.SystemBatch 中每条命令的超时时间，单位为秒（导出名为 exec.timeout）
// 作为 exec.SystemBatch 的可选项使用；超时后该命令被终止
//
// 参数:
//   - i: 超时秒数（支持小数）
//
// 返回值:
//   - 可传入 exec.SystemBatch 的选项
//
// Example:
// ```
// results = make([]string, 0)
// lock = sync.NewMutex()
// exec.SystemBatch("echo t{{int(1-2)}}",
//
//	exec.timeout(10),
//	exec.callback(func(cmd, result) { lock.Lock(); results = append(results, string(result)); lock.Unlock() }),
//
// )
// println(len(results))   // OUT: 2
// assert len(results) == 2, "timeout option should not affect fast commands"
// ```
func _execTimeout(i float64) execPoolOpt {
	return func(c *_execPoolConfig) {
		c.timeout = utils.FloatSecondDuration(i)
	}
}

// callback 设置 exec.SystemBatch 每条命令执行完成后的回调（导出名为 exec.callback）
// 回调第一个参数为执行的命令，第二个参数为该命令的输出结果；作为 exec.SystemBatch 的可选项使用
//
// 参数:
//   - f: 回调函数 func(cmd, result)
//
// 返回值:
//   - 可传入 exec.SystemBatch 的选项
//
// Example:
// ```
// outputs = make([]string, 0)
// lock = sync.NewMutex()
// exec.SystemBatch("echo cb{{int(1-3)}}",
//
//	exec.callback(func(cmd, result) { lock.Lock(); outputs = append(outputs, string(result)); lock.Unlock() }),
//
// )
// println(len(outputs))   // OUT: 3
// assert len(outputs) == 3, "callback should be invoked once per expanded command"
// ```
func _execSetCallback(f func(string, []byte)) execPoolOpt {
	return func(c *_execPoolConfig) {
		c.callback = f
	}
}

// WatchStdout 执行命令并实时监控其标准输出（导出名为 exec.WatchStdout，exec.WatchOutput 为其别名）
// 每当有新输出时调用回调，回调返回 false 可停止监控；适合监控长时间运行命令的输出流
//
// 参数:
//   - i: 完整命令字符串
//   - timeout: 监控超时时间，单位秒
//   - f: 回调函数 func(raw)，返回是否继续监控
//
// 返回值:
//   - 错误信息（命令创建或执行失败时返回）
//
// Example:
// ```
// got = bufio.NewBuffer()
// exec.WatchStdout(`sh -c "echo watchme; sleep 1"`, 8, func(raw) { got.Write(raw); return true })~
// println(str.Contains(got.String(), "watchme"))   // OUT: true
// assert str.Contains(got.String(), "watchme"), "WatchStdout should deliver stdout to the callback"
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

// WatchStderr 执行命令并实时监控其标准错误（导出名为 exec.WatchStderr）
// 每当有新错误输出时调用回调，回调返回 false 可停止监控
//
// 参数:
//   - i: 完整命令字符串
//   - timeout: 监控超时时间，单位秒
//   - f: 回调函数 func(raw)，返回是否继续监控
//
// 返回值:
//   - 错误信息（命令创建或执行失败时返回）
//
// Example:
// ```
// gotErr = bufio.NewBuffer()
// exec.WatchStderr(`sh -c "echo errmsg 1>&2; sleep 1"`, 8, func(raw) { gotErr.Write(raw); return true })
// println(str.Contains(gotErr.String(), "errmsg"))   // OUT: true
// assert str.Contains(gotErr.String(), "errmsg"), "WatchStderr should deliver stderr to the callback"
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
