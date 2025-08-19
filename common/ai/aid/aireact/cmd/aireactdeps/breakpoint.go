package aireactdeps

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// handleRequestBreakpoint 处理断点功能 - 在AI交互前暂停
func handleRequestBreakpoint(prompt string) {
	fmt.Printf("\n" + strings.Repeat("=", 80) + "\n")
	fmt.Printf("🛑 BREAKPOINT: AI Interaction Paused\n")
	fmt.Printf(strings.Repeat("=", 80) + "\n")
	fmt.Printf("PROMPT TO BE SENT:\n")
	fmt.Printf(strings.Repeat("-", 40) + "\n")
	fmt.Printf("%s\n", prompt)
	fmt.Printf(strings.Repeat("-", 40) + "\n")
	fmt.Printf("\nControls:\n")
	fmt.Printf("  y/Y/Enter  - Continue with AI request\n")
	fmt.Printf("  e/q/Q      - Exit program\n")
	fmt.Printf("  Ctrl+C     - Exit program\n")
	fmt.Print("\nPress Enter to continue or type command: ")

	// 设置断点状态以指示我们正在等待断点输入
	gs := GetGlobalState()
	gs.SetBreakpointWaiting(true)

	// 为Ctrl+C设置信号处理器
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// 等待来自全局输入通道的输入而不是创建新的扫描器
	// 这避免了与globalInputReader的冲突
	select {
	case input := <-gs.UserInput:
		input = strings.TrimSpace(strings.ToLower(input))
		switch input {
		case "", "y", "yes", "continue":
			fmt.Printf("✅ Continuing with AI request...\n")
			fmt.Printf(strings.Repeat("=", 80) + "\n\n")
		case "e", "q", "exit", "quit":
			fmt.Printf("🚪 Exiting as requested by user\n")
			os.Exit(0)
		default:
			fmt.Printf("🤷 Unknown command '%s', continuing with AI request...\n", input)
			fmt.Printf(strings.Repeat("=", 80) + "\n\n")
		}
	case sig := <-sigChan:
		fmt.Printf("\n🚪 Received signal %v, exiting...\n", sig)
		os.Exit(0)
	case <-time.After(60 * time.Second): // 60秒超时
		fmt.Printf("\n⏰ Timeout after 60 seconds, continuing with AI request...\n")
		fmt.Printf(strings.Repeat("=", 80) + "\n\n")
	}

	// 完成时清除断点状态
	gs.SetBreakpointWaiting(false)
}

// handleResponseBreakpoint 处理断点功能 - 在AI交互后暂停以检查响应
func handleResponseBreakpoint(resp *aicommon.AIResponse) {
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

	fmt.Printf(strings.Repeat("-", 40) + "\n")
	fmt.Printf("\nControls:\n")
	fmt.Printf("  y/Y/Enter  - Continue processing\n")
	fmt.Printf("  e/q/Q      - Exit program\n")
	fmt.Printf("  Ctrl+C     - Exit program\n")
	fmt.Print("\nPress Enter to continue or type command: ")

	// 设置断点状态以指示我们正在等待断点输入
	gs := GetGlobalState()
	gs.SetBreakpointWaiting(true)

	// 为Ctrl+C设置信号处理器
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// 等待来自全局输入通道的输入而不是创建新的扫描器
	// 这避免了与globalInputReader的冲突
	select {
	case input := <-gs.UserInput:
		input = strings.TrimSpace(strings.ToLower(input))
		switch input {
		case "", "y", "yes", "continue":
			fmt.Printf("✅ Continuing with response processing...\n")
			fmt.Printf(strings.Repeat("=", 80) + "\n\n")
		case "e", "q", "exit", "quit":
			fmt.Printf("🚪 Exiting as requested by user\n")
			os.Exit(0)
		default:
			fmt.Printf("🤷 Unknown command '%s', continuing with response processing...\n", input)
			fmt.Printf(strings.Repeat("=", 80) + "\n\n")
		}
	case sig := <-sigChan:
		fmt.Printf("\n🚪 Received signal %v, exiting...\n", sig)
		os.Exit(0)
	case <-time.After(60 * time.Second): // 60秒超时
		fmt.Printf("\n⏰ Timeout after 60 seconds, continuing with response processing...\n")
		fmt.Printf(strings.Repeat("=", 80) + "\n\n")
	}

	// 完成时清除断点状态
	gs.SetBreakpointWaiting(false)
}
