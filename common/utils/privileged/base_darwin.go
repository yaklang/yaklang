package privileged

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed darwin_applescript.tmpl
var darwinAppleScriptTemplate string

//go:embed yakit_icon.png
var yakitIconData []byte

func isPrivileged() bool {
	return os.Geteuid() == 0
}

var (
	iconTempFilePath string
	iconReleaseOnce  *utils.Once
)

func init() {
	iconReleaseOnce = utils.NewOnce()
}

// releaseIconToTemp 将内嵌的图标释放到 Yakit 基础目录，只会执行一次
func releaseIconToTemp() (string, error) {
	var err error
	iconReleaseOnce.Do(func() {
		// 使用 Yakit 基础目录存储图标
		yakitBaseDir := consts.GetDefaultYakitBaseDir()
		iconPath := filepath.Join(yakitBaseDir, "yakit-icon.png")

		// 检查文件是否已经存在
		if exists, _ := utils.PathExists(iconPath); exists {
			// 文件已存在，直接使用
			iconTempFilePath = iconPath
			return
		}

		// 确保目录存在
		if mkdirErr := os.MkdirAll(yakitBaseDir, 0755); mkdirErr != nil {
			err = utils.Errorf("failed to create yakit base dir: %v", mkdirErr)
			return
		}

		// 写入图标数据
		if writeErr := os.WriteFile(iconPath, yakitIconData, 0644); writeErr != nil {
			err = utils.Errorf("failed to write icon data: %v", writeErr)
			return
		}

		iconTempFilePath = iconPath
	})

	if err != nil {
		return "", err
	}

	if iconTempFilePath == "" {
		return "", utils.Errorf("icon temp file path is empty")
	}

	return iconTempFilePath, nil
}

type Executor struct {
	AppName       string
	AppIcon       string // 图标文件路径，如果为空则使用内嵌的 yakit 图标
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

	// 如果当前进程已经具备 root 权限，直接执行命令而不通过 AppleScript
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
		// 这与 AppleScript 路径的行为保持一致：在特权进程真正启动后才调用
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

	if config.Title == "" {
		config.Title = p.AppName
	}

	if config.Prompt == "" {
		if config.Title != "" && config.Description != "" {
			config.Prompt = fmt.Sprintf("[%v] %v", config.Title, config.Description)
		} else if config.Title != "" {
			config.Prompt = fmt.Sprintf("[%v]", config.Title)
		} else if config.Description != "" {
			config.Prompt = config.Description
		}
	}

	if config.Prompt == "" {
		config.Prompt = p.DefaultPrompt
	}

	// 确定要使用的图标路径
	iconPath := p.AppIcon
	if iconPath == "" {
		// 使用内嵌图标，释放到临时文件
		tempIconPath, iconErr := releaseIconToTemp()
		if iconErr == nil {
			iconPath = tempIconPath
		}
		// 如果释放失败，iconPath 保持为空，AppleScript 将使用默认图标
	}

	// 准备模板参数
	skipConfirm := "0"
	if config.SkipConfirmDialog {
		skipConfirm = "1"
	}

	script, err := utils.RenderTemplate(darwinAppleScriptTemplate, map[string]string{
		"Title":       hex.EncodeToString([]byte(config.Title)),
		"Description": hex.EncodeToString([]byte(config.Description)),
		"Command":     hex.EncodeToString([]byte(cmd)),
		"Prompt":      hex.EncodeToString([]byte(config.Prompt)),
		"IconPath":    hex.EncodeToString([]byte(iconPath)),
		"SkipConfirm": skipConfirm,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render AppleScript template: %v", err)
	}

	cmder := exec.CommandContext(ctx, "osascript", "-e", script)

	// 如果设置了 BeforePrivilegedProcessExecute 回调，我们需要监控 osascript 的输出
	// 以捕获 "PRIVILEGED_PROCESS_START" 标志
	if config.BeforePrivilegedProcessExecute != nil {
		// 创建管道来捕获 stderr（osascript 的 log 输出到 stderr）
		stderrPipe, err := cmder.StderrPipe()
		if err != nil {
			return nil, utils.Wrapf(err, "failed to create stderr pipe")
		}

		// 如果需要收集输出，也创建 stdout 管道
		var stdoutBuf bytes.Buffer
		if !config.DiscardStdoutStderr {
			cmder.Stdout = &stdoutBuf
		}

		// 启动命令
		if err := cmder.Start(); err != nil {
			return nil, utils.Wrapf(err, "failed to start osascript")
		}

		// 在 goroutine 中扫描 stderr 寻找启动标志
		startFlagDetected := make(chan struct{})
		go func() {
			scanner := bufio.NewScanner(stderrPipe)
			for scanner.Scan() {
				line := scanner.Text()
				// 检查是否包含启动标志
				if strings.Contains(line, "PRIVILEGED_PROCESS_START") {
					close(startFlagDetected)
					break
				}
			}
			// 继续读取剩余内容，避免管道阻塞
			for scanner.Scan() {
			}
		}()

		// 等待启动标志或超时
		select {
		case <-startFlagDetected:
			// 检测到启动标志，调用回调
			if config.BeforePrivilegedProcessExecute != nil {
				config.BeforePrivilegedProcessExecute()
			}
		case <-time.After(30 * time.Second):
			// 超时，可能用户取消了授权
			cmder.Process.Kill()
			return nil, utils.Errorf("timeout waiting for privileged process to start")
		case <-ctx.Done():
			// 上下文取消
			cmder.Process.Kill()
			return nil, ctx.Err()
		}

		// 等待命令完成
		err = cmder.Wait()
		if err != nil {
			if utils.MatchAllOfSubString(strings.ToLower(err.Error()), "sig", "killed") {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_ = exec.CommandContext(ctx, "osascript", "-e", `display dialog "Authorization failed due to inactivity or abnormal exit(KILL signal). Please retry if needed." with title "Authorization Failed" buttons {"OK"} default button "OK" with icon caution`).Run()
			}
			return stdoutBuf.Bytes(), utils.Wrapf(err, "run osascript->'%v' failed", utils.ShrinkString(cmd, 30))
		}

		if config.DiscardStdoutStderr {
			return nil, nil
		}
		return stdoutBuf.Bytes(), nil
	}

	// 没有 BeforeExecute 回调的情况，使用原来的简单逻辑
	if config.DiscardStdoutStderr {
		cmder.Stdout = nil
		cmder.Stderr = nil
		err = cmder.Run()
		if err != nil {
			if utils.MatchAllOfSubString(strings.ToLower(err.Error()), "sig", "killed") {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_ = exec.CommandContext(ctx, "osascript", "-e", `display dialog "Authorization failed due to inactivity or abnormal exit(KILL signal). Please retry if needed." with title "Authorization Failed" buttons {"OK"} default button "OK" with icon caution`).Run()
			}
			return nil, utils.Wrapf(err, "run osascript->'%v' failed", utils.ShrinkString(cmd, 30))
		}
		return nil, nil
	}

	// 默认行为：收集所有输出
	var out bytes.Buffer
	cmder.Stdout = &out
	cmder.Stderr = &out
	err = cmder.Run()
	if err != nil {
		if utils.MatchAllOfSubString(strings.ToLower(err.Error()), "sig", "killed") {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = exec.CommandContext(ctx, "osascript", "-e", `display dialog "Authorization failed due to inactivity or abnormal exit(KILL signal). Please retry if needed." with title "Authorization Failed" buttons {"OK"} default button "OK" with icon caution`).Run()
		}
		return out.Bytes(), utils.Wrapf(err, "run osascript->'%v' failed, output: %v\nreason:", utils.ShrinkString(cmd, 30), out.String())
	}

	return out.Bytes(), nil
}
