package privileged

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/hpcloud/tail"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/sys/windows"
)

// TOKEN_ELEVATION 结构用于存储令牌的提升信息
type TOKEN_ELEVATION struct {
	TokenIsElevated uint32
}

// isPrivileged 检测当前进程是否以管理员权限运行
// 使用 Windows Token API 来准确判断进程的提升状态
func isPrivileged() bool {
	// 方法1: 检查 Token Elevation（推荐方法）
	// 这个方法检查当前进程的访问令牌是否已提升
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)
	if err != nil {
		log.Errorf("failed to open process token: %v", err)
		// 如果无法打开令牌，尝试备用方法
		return isPrivilegedFallback()
	}
	defer token.Close()

	// 获取令牌的提升信息
	var elevation TOKEN_ELEVATION
	var returnedLen uint32
	err = windows.GetTokenInformation(
		token,
		windows.TokenElevation,
		(*byte)(unsafe.Pointer(&elevation)),
		uint32(unsafe.Sizeof(elevation)),
		&returnedLen,
	)
	if err != nil {
		log.Errorf("failed to get token elevation info: %v", err)
		// 如果无法获取提升信息，尝试备用方法
		return isPrivilegedFallback()
	}

	// TokenIsElevated 非零表示进程已提升（以管理员权限运行）
	return elevation.TokenIsElevated != 0
}

// isPrivilegedFallback 备用的权限检测方法
// 当主方法失败时使用，通过尝试打开物理驱动器来判断
func isPrivilegedFallback() bool {
	// 尝试打开物理驱动器，只有管理员才能打开
	fp, err := os.Open("\\\\.\\PHYSICALDRIVE0")
	if err != nil {
		return false
	}
	fp.Close()
	return true
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

	// 如果当前进程已经具备管理员权限，直接执行命令
	if isPrivileged() {
		// 直接通过 cmd 执行命令
		cmder := exec.CommandContext(ctx, "cmd", "/C", cmd)

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

	// 需要提权，使用 UAC（User Account Control）
	// 创建临时批处理文件来执行命令
	tempFileDir := os.TempDir()
	token := utils.RandStringBytes(20)
	batName := filepath.Join(tempFileDir, fmt.Sprintf("windows-uac-prompt-%v.bat", token))

	stdoutFile := filepath.Join(tempFileDir, "stdout-"+token+".txt")
	stderrFile := filepath.Join(tempFileDir, "stderr-"+token+".txt")
	exitCodeFile := filepath.Join(tempFileDir, "exitcode-"+token+".txt")

	// 确保清理临时文件
	defer func() {
		os.RemoveAll(stdoutFile)
		os.RemoveAll(stderrFile)
		os.RemoveAll(exitCodeFile)
		os.RemoveAll(batName)
	}()

	// 构建批处理脚本
	var batLines []string
	batLines = append(batLines, "@echo off")

	// 添加命令和输出重定向
	batLines = append(batLines, "")
	batLines = append(batLines, fmt.Sprintf("call :sub > %v 2> %v", strconv.Quote(stdoutFile), strconv.Quote(stderrFile)))
	batLines = append(batLines, `exit /b`)
	batLines = append(batLines, "")
	batLines = append(batLines, ":sub")
	batLines = append(batLines, cmd)
	batLines = append(batLines, "echo %errorlevel% > "+exitCodeFile)
	batLines = append(batLines, "exit /b")

	// 写入批处理文件
	fp, err := os.OpenFile(batName, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		return nil, utils.Errorf("failed to create bat file: %v", err)
	}
	_, err = fp.Write([]byte(strings.Join(batLines, "\n")))
	fp.Close()
	if err != nil {
		return nil, utils.Errorf("failed to write bat file: %v", err)
	}

	log.Infof("execute privileged command via UAC: %s", utils.ShrinkString(cmd, 100))

	// 使用 PowerShell 的 Start-Process 以管理员身份运行批处理文件
	// -Verb RunAs 会触发 UAC 提示
	psCmd := exec.CommandContext(ctx, "powershell.exe", "start-process", "-verb", "runas",
		"-windowstyle", "hidden", batName)

	// 启动 PowerShell 命令
	if err := psCmd.Start(); err != nil {
		return nil, utils.Wrapf(err, "failed to start UAC prompt")
	}

	// 在进程启动后，调用 BeforePrivilegedProcessExecute 回调（如果设置了）
	// 注意：这个回调会在 UAC 对话框出现后立即调用，而不是等待用户授权
	if config.BeforePrivilegedProcessExecute != nil {
		config.BeforePrivilegedProcessExecute()
	}

	// 等待 PowerShell 命令完成（这只是等待 UAC 对话框关闭）
	_ = psCmd.Wait()

	// 如果不需要收集输出，直接返回
	if config.DiscardStdoutStderr {
		// 给一些时间让批处理文件执行完成
		time.Sleep(500 * time.Millisecond)
		return nil, nil
	}

	// 收集输出
	// 使用 tail 来读取输出文件，因为批处理可能还在执行
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	// 等待输出文件创建并读取内容
	// 最多等待 30 秒
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		// 检查退出码文件是否存在，表示命令已完成
		if exists, _ := utils.PathExists(exitCodeFile); exists {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 读取 stdout
	if exists, _ := utils.PathExists(stdoutFile); exists {
		t, err := tail.TailFile(stdoutFile, tail.Config{Follow: false})
		if err == nil {
			for line := range t.Lines {
				if line.Text != "" {
					stdoutBuf.WriteString(line.Text)
					stdoutBuf.WriteByte('\n')
				}
			}
		}
	}

	// 读取 stderr
	if exists, _ := utils.PathExists(stderrFile); exists {
		t, err := tail.TailFile(stderrFile, tail.Config{Follow: false})
		if err == nil {
			for line := range t.Lines {
				if line.Text != "" {
					stderrBuf.WriteString(line.Text)
					stderrBuf.WriteByte('\n')
				}
			}
		}
	}

	// 合并输出
	var combinedOutput bytes.Buffer
	combinedOutput.Write(stdoutBuf.Bytes())
	combinedOutput.Write(stderrBuf.Bytes())

	// 检查退出码
	if exists, _ := utils.PathExists(exitCodeFile); exists {
		exitCodeData, _ := os.ReadFile(exitCodeFile)
		if exitCodeData != nil {
			exitCode, _ := strconv.Atoi(strings.TrimSpace(string(exitCodeData)))
			if exitCode != 0 {
				return combinedOutput.Bytes(), utils.Errorf("command exited with code %d", exitCode)
			}
		}
	}

	return combinedOutput.Bytes(), nil
}
