package aireactdeps

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/cmd/aireactdeps/promptui"
)

// BreakpointOption 定义断点选项
type BreakpointOption struct {
	Value       string
	Description string
}

// handleRequestBreakpoint 处理断点功能 - 在AI交互前暂停，使用 promptui
func handleRequestBreakpoint(prompt string) {
	// 关闭主菜单IO，避免冲突
	if globalEventMonitor := GetGlobalEventMonitor(); globalEventMonitor != nil {
		globalEventMonitor.CloseMenu()
	}

	fmt.Printf("\n" + strings.Repeat("=", 80) + "\n")
	fmt.Printf("🛑 BREAKPOINT: AI Interaction Paused\n")
	fmt.Printf(strings.Repeat("=", 80) + "\n")
	fmt.Printf("PROMPT TO BE SENT:\n")
	fmt.Printf(strings.Repeat("-", 40) + "\n")
	fmt.Printf("%s\n", prompt)
	fmt.Printf(strings.Repeat("-", 40) + "\n\n")

	// 定义选项
	options := []BreakpointOption{
		{Value: "continue", Description: "继续执行 AI 请求"},
		{Value: "exit", Description: "退出程序"},
	}

	// 创建 promptui 选择器
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}?",
		Active:   "▶ {{ .Description | cyan }}",
		Inactive: "  {{ .Description }}",
		Selected: "✓ {{ .Description | green }}",
	}

	promptSelect := promptui.Select{
		Label:     "请选择操作",
		Items:     options,
		Templates: templates,
		Size:      4,
	}

	// 创建一个通道来接收选择结果
	resultChan := make(chan int, 1)
	errChan := make(chan error, 1)

	// 在goroutine中运行prompt
	go func() {
		selectedIndex, _, err := promptSelect.Run()
		if err != nil {
			errChan <- err
		} else {
			resultChan <- selectedIndex
		}
	}()

	// 等待结果（全局信号处理器会处理Ctrl+C）
	select {
	case selectedIndex := <-resultChan:
		selectedOption := options[selectedIndex]
		switch selectedOption.Value {
		case "continue":
			fmt.Printf("✅ 继续执行 AI 请求...\n")
			fmt.Printf(strings.Repeat("=", 80) + "\n\n")
		case "exit":
			fmt.Printf("🚪 用户请求退出\n")
			os.Exit(0)
		}
	case err := <-errChan:
		if err == promptui.ErrInterrupt {
			fmt.Printf("\n🚪 用户中断，正在退出...\n")
			os.Exit(0)
		}
		fmt.Printf("🤷 输入错误，继续执行 AI 请求...\n")
		fmt.Printf(strings.Repeat("=", 80) + "\n\n")
	case <-time.After(60 * time.Second): // 60秒超时
		fmt.Printf("\n⏰ 60秒超时，继续执行 AI 请求...\n")
		fmt.Printf(strings.Repeat("=", 80) + "\n\n")
	}
}

// handleResponseBreakpoint 处理断点功能 - 在AI交互后暂停以检查响应，使用 promptui
func handleResponseBreakpoint(resp *aicommon.AIResponse) {
	// 关闭主菜单IO，避免冲突
	if globalEventMonitor := GetGlobalEventMonitor(); globalEventMonitor != nil {
		globalEventMonitor.CloseMenu()
	}

	fmt.Printf("\n" + strings.Repeat("=", 80) + "\n")
	fmt.Printf("🛑 RESPONSE BREAKPOINT: AI Response Received\n")
	fmt.Printf(strings.Repeat("=", 80) + "\n")
	fmt.Printf("AI RESPONSE CONTENT:\n")
	fmt.Printf(strings.Repeat("-", 40) + "\n")

	// 安全地提取和显示响应内容
	if resp != nil {
		fmt.Printf("✅ Response received successfully\n")
		fmt.Printf("  Type: %T\n", resp)
		fmt.Printf("  Response object exists and is ready for processing\n")
		fmt.Printf("  Note: Actual response content was displayed in the stream above\n")
		fmt.Printf("  The stream has been processed and is now complete\n")
	} else {
		fmt.Printf("❌ Response is nil\n")
	}

	fmt.Printf(strings.Repeat("-", 40) + "\n\n")

	// 定义选项
	options := []BreakpointOption{
		{Value: "continue", Description: "继续处理响应"},
		{Value: "exit", Description: "退出程序"},
	}

	// 创建 promptui 选择器
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}?",
		Active:   "▶ {{ .Description | cyan }}",
		Inactive: "  {{ .Description }}",
		Selected: "✓ {{ .Description | green }}",
	}

	promptSelect := promptui.Select{
		Label:     "请选择操作",
		Items:     options,
		Templates: templates,
		Size:      4,
	}

	// 创建一个通道来接收选择结果
	resultChan := make(chan int, 1)
	errChan := make(chan error, 1)

	// 在goroutine中运行prompt
	go func() {
		selectedIndex, _, err := promptSelect.Run()
		if err != nil {
			errChan <- err
		} else {
			resultChan <- selectedIndex
		}
	}()

	// 等待结果（全局信号处理器会处理Ctrl+C）
	select {
	case selectedIndex := <-resultChan:
		selectedOption := options[selectedIndex]
		switch selectedOption.Value {
		case "continue":
			fmt.Printf("✅ 继续处理响应...\n")
			fmt.Printf(strings.Repeat("=", 80) + "\n\n")
		case "exit":
			fmt.Printf("🚪 用户请求退出\n")
			os.Exit(0)
		}
	case err := <-errChan:
		if err == promptui.ErrInterrupt {
			fmt.Printf("\n🚪 用户中断，正在退出...\n")
			os.Exit(0)
		}
		fmt.Printf("🤷 输入错误，继续处理响应...\n")
		fmt.Printf(strings.Repeat("=", 80) + "\n\n")
	case <-time.After(60 * time.Second): // 60秒超时
		fmt.Printf("\n⏰ 60秒超时，继续处理响应...\n")
		fmt.Printf(strings.Repeat("=", 80) + "\n\n")
	}
}
