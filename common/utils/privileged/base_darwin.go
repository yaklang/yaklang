package privileged

import (
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

	script, err := utils.RenderTemplate(darwinAppleScriptTemplate, map[string]string{
		"Title":       hex.EncodeToString([]byte(config.Title)),
		"Description": hex.EncodeToString([]byte(config.Description)),
		"Command":     hex.EncodeToString([]byte(cmd)),
		"Prompt":      hex.EncodeToString([]byte(config.Prompt)),
		"IconPath":    hex.EncodeToString([]byte(iconPath)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render AppleScript template: %v", err)
	}

	cmder := exec.CommandContext(ctx, "osascript", "-e", script)
	var out bytes.Buffer
	cmder.Stdout = &out
	cmder.Stderr = &out
	err = cmder.Run()
	if err != nil {
		if utils.MatchAllOfSubString(strings.ToLower(string(err.Error())), "sig", "killed") {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = exec.CommandContext(ctx, "osascript", "-e", `display dialog "Authorization failed due to inactivity or abnormal exit(KILL signal). Please retry if needed." with title "Authorization Failed" buttons {"OK"} default button "OK" with icon caution`).Run()
		}
		return out.Bytes(), utils.Wrapf(err, "run osascript->'%v' failed, output: %v\nreason:", utils.ShrinkString(cmd, 30), out.String())
	}

	return out.Bytes(), nil
}
