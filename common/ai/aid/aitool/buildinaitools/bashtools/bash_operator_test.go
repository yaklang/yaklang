package bashtools

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestCreateBashTools(t *testing.T) {
	bashCtx := NewBashSessionContext(context.Background())
	tools, err := CreateBashTools(bashCtx)
	if err != nil {
		t.Fatalf("CreateBashTools failed: %v", err)
	}

	// 检查工具数量
	expectedToolCount := 4 // bash_session_execute, list_bash_sessions, read_bash_session_buffer, close_bash_session
	if len(tools) != expectedToolCount {
		t.Errorf("Expected %d tools, got %d", expectedToolCount, len(tools))
	}

	// 检查工具名称
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.GetName()] = true
	}

	expectedTools := []string{
		"bash_session_execute",
		"list_bash_sessions",
		"read_bash_session_buffer",
		"close_bash_session",
	}

	for _, expectedTool := range expectedTools {
		if !toolNames[expectedTool] {
			t.Errorf("Expected tool %s not found", expectedTool)
		}
	}
}

func TestWindowsCompatibility(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows compatibility test, skipping on non-Windows platform")
	}

	bashCtx := NewBashSessionContext(context.Background())
	tools, err := CreateBashTools(bashCtx)
	if err != nil {
		t.Fatalf("CreateBashTools failed: %v", err)
	}

	// 找到bash_session_execute工具
	var bashTool *aitool.Tool
	for _, tool := range tools {
		if tool.GetName() == "bash_session_execute" {
			bashTool = tool
			break
		}
	}

	if bashTool == nil {
		t.Fatal("bash_session_execute tool not found")
	}

	// 清理测试环境
	defer cleanupTestSessions(bashCtx)

	// 测试Windows cmd命令
	t.Run("WindowsCmdCommands", func(t *testing.T) {
		sessionName := "windows_test_session"

		// 测试基本Windows命令
		commands := []string{
			"echo Windows Test",
			"dir",
			"ver",
			"set TEST_WIN_VAR=windows_value",
			"echo %TEST_WIN_VAR%",
		}

		for i, cmd := range commands {
			params := aitool.InvokeParams{
				"command": cmd,
				"session": sessionName,
				"shell":   "cmd", // 明确指定使用cmd
				"timeout": 10,
			}

			result, err := bashTool.InvokeWithParams(params)
			if err != nil {
				t.Errorf("Windows command %d (%s) failed: %v", i+1, cmd, err)
				continue
			}

			if !result.Success {
				t.Errorf("Windows command %d (%s) execution failed: %s", i+1, cmd, result.Error)
				continue
			}

			t.Logf("Windows command %d (%s) executed successfully", i+1, cmd)
		}
	})

	// 测试PowerShell命令
	t.Run("WindowsPowerShellCommands", func(t *testing.T) {
		sessionName := "powershell_test_session"

		// 测试基本PowerShell命令
		commands := []string{
			"Write-Host 'PowerShell Test'",
			"Get-Location",
			"$env:TEST_PS_VAR = 'powershell_value'",
			"Write-Host $env:TEST_PS_VAR",
		}

		for i, cmd := range commands {
			params := aitool.InvokeParams{
				"command": cmd,
				"session": sessionName,
				"shell":   "powershell", // 明确指定使用powershell
				"timeout": 10,
			}

			result, err := bashTool.InvokeWithParams(params)
			if err != nil {
				t.Errorf("PowerShell command %d (%s) failed: %v", i+1, cmd, err)
				continue
			}

			if !result.Success {
				t.Errorf("PowerShell command %d (%s) execution failed: %s", i+1, cmd, result.Error)
				continue
			}

			t.Logf("PowerShell command %d (%s) executed successfully", i+1, cmd)
		}
	})
}

// cleanupTestSessions 清理测试会话
func cleanupTestSessions(bashCtx *BashSessionContext) {
	bashCtx.sessionsMutex.Lock()
	sessionsToClean := make([]string, 0)

	// 收集需要清理的会话名称
	for name := range bashCtx.sessions {
		if strings.HasPrefix(name, "test_") || strings.HasPrefix(name, "temp_") {
			sessionsToClean = append(sessionsToClean, name)
		}
	}
	bashCtx.sessionsMutex.Unlock()

	// 逐个关闭会话（使用我们改进的closeSession函数）
	for _, name := range sessionsToClean {
		closeSession(bashCtx, name)
	}
}
