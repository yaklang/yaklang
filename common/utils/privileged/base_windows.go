package privileged

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

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

func startWindowsElevatedBatch(ctx context.Context, batName string, waitForExit bool) (*exec.Cmd, error) {
	args := []string{
		"-NoProfile",
		"-NonInteractive",
		"Start-Process",
		"-FilePath", batName,
		"-Verb", "RunAs",
		"-WindowStyle", "Hidden",
		"-ErrorAction", "Stop",
	}
	if waitForExit {
		args = append(args, "-Wait")
	}

	psCmd := exec.CommandContext(ctx, "powershell.exe", args...)
	if err := psCmd.Start(); err != nil {
		return nil, err
	}
	return psCmd, nil
}

func readWindowsTempFile(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return raw, nil
}

func cleanupWindowsTempFile(path string) {
	if path == "" {
		return
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Warnf("failed to remove temporary Windows UAC file %s: %v", path, err)
	}
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
	tempFileDir := consts.GetDefaultYakitBaseTempDir()
	token := utils.RandStringBytes(20)
	batName := filepath.Join(tempFileDir, fmt.Sprintf("windows-uac-prompt-%v.bat", token))

	waitForExit := !config.DiscardStdoutStderr
	stdoutTarget := "NUL"
	stderrTarget := "NUL"

	var stdoutFile string
	var stderrFile string
	var exitCodeFile string
	if waitForExit {
		stdoutFile = filepath.Join(tempFileDir, "stdout-"+token+".txt")
		stderrFile = filepath.Join(tempFileDir, "stderr-"+token+".txt")
		exitCodeFile = filepath.Join(tempFileDir, "exitcode-"+token+".txt")
		stdoutTarget = strconv.Quote(stdoutFile)
		stderrTarget = strconv.Quote(stderrFile)
	}

	// 确保清理临时文件
	cleanupBatchOnReturn := true
	defer func() {
		cleanupWindowsTempFile(stdoutFile)
		cleanupWindowsTempFile(stderrFile)
		cleanupWindowsTempFile(exitCodeFile)
		if cleanupBatchOnReturn {
			cleanupWindowsTempFile(batName)
		}
	}()

	// 构建批处理脚本
	batLines := buildWindowsUACBatchLines(cmd, stdoutTarget, stderrTarget, exitCodeFile, waitForExit, !waitForExit)

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
	// 在需要收集输出时使用 -Wait 等待真实子进程退出；
	// 对于丢弃输出的常驻特权进程，保留快速返回语义，避免阻塞调用方。
	psCmd, err := startWindowsElevatedBatch(ctx, batName, waitForExit)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to start UAC prompt")
	}

	// 在进程启动后，调用 BeforePrivilegedProcessExecute 回调（如果设置了）
	// 注意：这个回调会在 UAC 对话框出现后立即调用，而不是等待用户授权
	if config.BeforePrivilegedProcessExecute != nil {
		config.BeforePrivilegedProcessExecute()
	}

	waitErr := psCmd.Wait()

	// 如果不需要收集输出，直接返回
	if config.DiscardStdoutStderr {
		if waitErr != nil {
			return nil, utils.Wrapf(waitErr, "failed to complete UAC prompt")
		}
		cleanupBatchOnReturn = false
		// 给一些时间让批处理文件执行完成
		time.Sleep(500 * time.Millisecond)
		return nil, nil
	}

	stdoutRaw, err := readWindowsTempFile(stdoutFile)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read stdout temp file")
	}
	stderrRaw, err := readWindowsTempFile(stderrFile)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read stderr temp file")
	}

	var combinedOutput bytes.Buffer
	combinedOutput.Write(stdoutRaw)
	combinedOutput.Write(stderrRaw)

	if waitErr != nil {
		return combinedOutput.Bytes(), utils.Wrapf(waitErr, "failed to complete UAC prompt")
	}

	// 检查退出码
	exitCodeData, err := readWindowsTempFile(exitCodeFile)
	if err != nil {
		return combinedOutput.Bytes(), utils.Wrapf(err, "failed to read exit code temp file")
	}
	if len(bytes.TrimSpace(exitCodeData)) == 0 {
		return combinedOutput.Bytes(), utils.Errorf("privileged command finished without exit code")
	}
	exitCode, _ := strconv.Atoi(strings.TrimSpace(string(exitCodeData)))
	if exitCode != 0 {
		return combinedOutput.Bytes(), utils.Errorf("command exited with code %d", exitCode)
	}

	return combinedOutput.Bytes(), nil
}
