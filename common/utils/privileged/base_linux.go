package privileged

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	"golang.org/x/sys/unix"
)

func isPrivileged() bool {
	header := unix.CapUserHeader{
		Version: unix.LINUX_CAPABILITY_VERSION_3,
		Pid:     int32(os.Getpid()),
	}
	// data := unix.CapUserData{}
	var data [2]unix.CapUserData
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := unix.Capget(&header, &data[0]); err == nil {
		data[0].Inheritable = (1 << unix.CAP_NET_RAW)

		if err := unix.Capset(&header, &data[0]); err == nil {
			return true
		}
	}
	return os.Geteuid() == 0
}

type Executor struct {
	AppName       string
	AppIcon       string
	DefaultPrompt string
}

func NewExecutor(appName string) *Executor {
	return &Executor{
		AppName:       appName,
		DefaultPrompt: "this operation requires administrator privileges",
	}
}

func (p *Executor) Execute(ctx context.Context, cmd string, opts ...ExecuteOption) ([]byte, error) {
	config := DefaultExecuteConfig()
	for _, opt := range opts {
		opt(config)
	}

	// 如果当前进程已经具备 root 权限，直接执行命令
	if isPrivileged() {
		// 直接通过 shell 执行命令
		cmder := exec.CommandContext(ctx, "sh", "-c", cmd)

		if config.DiscardStdoutStderr {
			cmder.Stdout = nil
			cmder.Stderr = nil
		} else {
			// 收集输出
			var out bytes.Buffer
			cmder.Stdout = &out
			cmder.Stderr = &out
		}

		// 启动命令
		if err := cmder.Start(); err != nil {
			return nil, utils.Wrapf(err, "failed to start command '%v'", utils.ShrinkString(cmd, 30))
		}

		// 在进程启动后，调用 BeforePrivilegedProcessExecute 回调（如果设置了）
		if config.BeforePrivilegedProcessExecute != nil {
			config.BeforePrivilegedProcessExecute()
		}

		// 等待命令完成
		err := cmder.Wait()
		if err != nil {
			if config.DiscardStdoutStderr {
				return nil, utils.Wrapf(err, "run command '%v' failed", utils.ShrinkString(cmd, 30))
			}
			// 从 Stdout 中获取输出
			if out, ok := cmder.Stdout.(*bytes.Buffer); ok {
				return out.Bytes(), utils.Wrapf(err, "run command '%v' failed, output: %v", utils.ShrinkString(cmd, 30), out.String())
			}
			return nil, utils.Wrapf(err, "run command '%v' failed", utils.ShrinkString(cmd, 30))
		}

		if config.DiscardStdoutStderr {
			return nil, nil
		}

		// 从 Stdout 中获取输出
		if out, ok := cmder.Stdout.(*bytes.Buffer); ok {
			return out.Bytes(), nil
		}
		return nil, nil
	}

	// 需要提权，使用 pkexec
	_, err := exec.LookPath("bash")
	if err != nil {
		return nil, utils.Errorf("bash not found: %v", err)
	}

	_, err = exec.LookPath("pkexec")
	if err != nil {
		return nil, utils.Errorf("pkexec not found: %v", err)
	}

	// 构建 pkexec 命令
	// pkexec 会弹出图形化的权限提示对话框
	var lines []string

	// 设置环境变量（如果有）
	// 注意：pkexec 不会传递环境变量，所以我们需要在命令中显式设置

	// 使用 pkexec 执行 bash 命令
	// --disable-internal-agent 禁用内部代理，强制使用图形化认证
	pkexecCmd := fmt.Sprintf(`pkexec --disable-internal-agent bash -c %v`, strconv.Quote(cmd))
	lines = append(lines, pkexecCmd)

	finalCmd := strings.Join(lines, " && ")
	log.Infof("execute privileged command via pkexec: %s", utils.ShrinkString(finalCmd, 100))

	cmder := exec.CommandContext(ctx, "bash", "-c", finalCmd)

	if config.DiscardStdoutStderr {
		cmder.Stdout = nil
		cmder.Stderr = nil
	} else {
		var out bytes.Buffer
		cmder.Stdout = &out
		cmder.Stderr = &out
	}

	// 启动命令
	if err := cmder.Start(); err != nil {
		return nil, utils.Wrapf(err, "failed to start pkexec command")
	}

	// 在进程启动后，调用 BeforePrivilegedProcessExecute 回调（如果设置了）
	// 注意：对于 pkexec，这个回调会在用户授权后立即调用
	if config.BeforePrivilegedProcessExecute != nil {
		config.BeforePrivilegedProcessExecute()
	}

	// 等待命令完成
	err = cmder.Wait()
	if err != nil {
		if config.DiscardStdoutStderr {
			return nil, utils.Wrapf(err, "run pkexec command failed")
		}
		// 从 Stdout 中获取输出
		if out, ok := cmder.Stdout.(*bytes.Buffer); ok {
			return out.Bytes(), utils.Wrapf(err, "run pkexec command failed, output: %v", out.String())
		}
		return nil, utils.Wrapf(err, "run pkexec command failed")
	}

	if config.DiscardStdoutStderr {
		return nil, nil
	}

	// 从 Stdout 中获取输出
	if out, ok := cmder.Stdout.(*bytes.Buffer); ok {
		return out.Bytes(), nil
	}
	return nil, nil
}
