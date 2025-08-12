package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var debugMode = false

func main() {
	log.Info("Starting ReAct CLI Demo")

	// Initialize database and configurations
	err := initializeDatabase()
	if err != nil {
		log.Errorf("Failed to initialize database: %v", err)
		// Continue anyway, as some features may still work
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Info("Received interrupt signal, shutting down...")
		cancel()
		os.Exit(0)
	}()

	// Create AI callback with real-time streaming display
	aiCallback := aid.AIChatToAICallbackType(func(msg string, opts ...aispec.AIConfigOption) (string, error) {
		// Add stream handlers to show real-time output
		opts = append(opts,
			aispec.WithStreamHandler(func(reader io.Reader) {
				// Show raw AI stream in real-time
				showRawStreamOutput(reader)
			}),
			aispec.WithReasonStreamHandler(func(reader io.Reader) {
				// Show reasoning stream if available
				showReasonStreamOutput(reader)
			}),
		)

		// The database initialization has already loaded API keys into environment variables
		// so ai.Chat will automatically find and use them
		return ai.Chat(msg, opts...)
	})

	// For debugging, let's create a wrapper to see what's happening
	debugAICallback := func(config *aid.Config, req *aid.AIRequest) (*aid.AIResponse, error) {
		if debugMode {
			log.Infof("AI Request: %s", req.GetPrompt())
		}
		resp, err := aiCallback(config, req)
		if err != nil {
			if debugMode {
				log.Errorf("AI callback error: %v", err)
			}
			return nil, err
		}
		if debugMode {
			log.Infof("AI callback succeeded")
		}
		return resp, nil
	}

	// Create ReAct instance with configuration
	react, err := aireact.NewReAct(
		aireact.WithContext(ctx),
		aireact.WithAICallback(debugAICallback),
		aireact.WithDebug(false), // Default to false, can be toggled
		aireact.WithMaxIterations(5),
		aireact.WithMaxThoughts(3),
		aireact.WithMaxActions(3),
		aireact.WithTemperature(0.7, 0.3),
		aireact.WithEventHandler(func(event *ypb.AIOutputEvent) {
			// Handle output events with enhanced streaming display
			switch event.Type {
			case "react_thought":
				// Display thinking process with typewriter effect
				fmt.Print("💭 ")
				typewriterPrint(string(event.Content))
				fmt.Println()
			case "react_action":
				fmt.Printf("🔧 %s\n", string(event.Content))
			case "react_observation":
				fmt.Printf("👀 %s\n", string(event.Content))
			case "react_result":
				fmt.Printf("✅ 完成: %s\n", extractResultContent(string(event.Content)))
			case "react_error":
				fmt.Printf("❌ 错误: %s\n", string(event.Content))
			case "react_info":
				if debugMode {
					fmt.Printf("ℹ️  %s\n", string(event.Content))
				} else {
					// Show important info messages even in non-debug mode
					content := string(event.Content)
					if strings.Contains(content, "正在") || strings.Contains(content, "准备") ||
						strings.Contains(content, "生成") || strings.Contains(content, "执行") {
						fmt.Printf("ℹ️  %s\n", content)
					}
				}
			case "react_iteration":
				if debugMode {
					fmt.Printf("🔄 %s\n", string(event.Content))
				}
			default:
				if debugMode {
					fmt.Printf("[%s] %s\n", strings.ToUpper(event.Type), string(event.Content))
				}
			}
		}),
		// Add some basic tools
		aireact.WithTools(createBasicTools()...),
	)
	if err != nil {
		log.Errorf("Failed to create ReAct instance: %v", err)
		os.Exit(1)
	}

	// Create input and output channels
	inputChan := chanx.NewUnlimitedChan[*ypb.AITriageInputEvent](ctx, 10)
	defer inputChan.Close()

	outputChan, err := react.UnlimitedInvoke(inputChan)
	if err != nil {
		log.Errorf("Failed to start ReAct: %v", err)
		os.Exit(1)
	}

	// Start output handler that properly drains the channel
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		for {
			select {
			case event, ok := <-outputChan:
				if !ok {
					return
				}
				_ = event // Events are handled by the event handler
			case <-ctx.Done():
				return
			}
		}
	}()

	// Interactive CLI loop
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("ReAct CLI 已就绪。输入您的问题 (输入 'exit' 退出，'/debug' 切换调试模式):")
	fmt.Print("> ")

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			log.Info("Context cancelled, exiting")
			return
		default:
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			fmt.Print("> ")
			continue
		}

		if input == "exit" || input == "quit" {
			log.Info("User requested exit")
			cancel()
			// Wait for output handler to finish gracefully
			select {
			case <-outputDone:
			case <-time.After(time.Second):
			}
			return
		}

		if input == "/debug" {
			debugMode = !debugMode
			if debugMode {
				fmt.Println("🐛 调试模式已开启")
				log.SetLevel(log.DebugLevel)
			} else {
				fmt.Println("🐛 调试模式已关闭")
				log.SetLevel(log.InfoLevel)
			}
			// Update ReAct debug settings
			react.UpdateDebugMode(debugMode)
			fmt.Print("> ")
			continue
		}

		// Reset streaming state for new request
		streamingMutex.Lock()
		streamDisplayed = false
		streamingMutex.Unlock()

		// Send user input to ReAct
		event := &ypb.AITriageInputEvent{
			IsFreeInput: true,
			FreeInput:   input,
		}

		fmt.Print("🤔 处理中")

		// Show activity spinner while waiting
		go showActivitySpinner()

		inputChan.SafeFeed(event)

		// Wait for the response to complete before showing next prompt
		waitForResponseCompletion(outputChan, ctx)

		// Stop spinner and show next prompt
		stopActivitySpinner()
		fmt.Print("\n> ")
	}

	if err := scanner.Err(); err != nil {
		log.Errorf("Scanner error: %v", err)
	}
}

// waitForResponseCompletion waits for the AI response to complete
func waitForResponseCompletion(outputChan chan *ypb.AIOutputEvent, ctx context.Context) {
	responseCompleted := false

	for !responseCompleted {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-outputChan:
			if !ok {
				return // Channel closed
			}

			// Check if this indicates the response is complete
			switch event.Type {
			case "react_result":
				responseCompleted = true
			case "react_error":
				responseCompleted = true
			}
		case <-time.After(30 * time.Second): // Timeout after 30 seconds
			fmt.Println("\n⚠️  响应超时，请重试")
			responseCompleted = true
		}
	}
}

// extractResultContent extracts the actual result from the JSON result
func extractResultContent(content string) string {
	// Try to extract "result" field from JSON
	if strings.Contains(content, `"result"`) {
		start := strings.Index(content, `"result":"`)
		if start != -1 {
			start += 10 // length of `"result":"`
			end := strings.Index(content[start:], `"`)
			if end != -1 {
				return content[start : start+end]
			}
		}
	}
	return content
}

// streamingState tracks the current streaming state
var (
	streamingActive   = false
	streamingMutex    sync.Mutex
	currentStreamLine = ""
	streamStartTime   time.Time
	streamCharCount   = 0
	streamDisplayed   = false // Track if we've already shown streaming output for this request

	// Activity spinner state
	spinnerActive = false
	spinnerStop   = make(chan bool, 1)
	spinnerMutex  sync.Mutex
)

// showRawStreamOutput displays the raw AI stream in real-time
func showRawStreamOutput(reader io.Reader) {
	streamingMutex.Lock()
	// Check if we've already displayed streaming output for this request
	if streamDisplayed {
		streamingMutex.Unlock()
		// Just consume the stream without displaying
		io.Copy(io.Discard, reader)
		return
	}

	// Stop the spinner first
	stopActivitySpinner()

	if !streamingActive {
		streamingActive = true
		streamDisplayed = true
		streamStartTime = time.Now()
		streamCharCount = 0
		fmt.Print("【流式输出】")
	}
	streamingMutex.Unlock()

	buffer := make([]byte, 1)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			char := string(buffer[:n])
			streamingMutex.Lock()
			streamCharCount++

			// Control display: only show first 80 chars, then show dots
			if streamCharCount <= 80 {
				// Filter out newlines and control characters for compact display
				if char != "\n" && char != "\r" && char != "\t" {
					fmt.Print(char)
				}
			} else if streamCharCount == 81 {
				fmt.Print("...")
			}
			streamingMutex.Unlock()
		}
		if err != nil {
			break
		}
	}

	streamingMutex.Lock()
	streamingActive = false
	elapsed := time.Since(streamStartTime)
	fmt.Printf(" [%d字符, %.1fs] ✓\n", streamCharCount, elapsed.Seconds())
	streamingMutex.Unlock()
}

// showReasonStreamOutput displays reasoning stream
func showReasonStreamOutput(reader io.Reader) {
	if debugMode {
		fmt.Print("\n【推理流】")
		io.Copy(os.Stdout, reader)
		fmt.Print(" ✓\n")
	}
}

// showActivitySpinner shows a spinning activity indicator
func showActivitySpinner() {
	spinnerMutex.Lock()
	if spinnerActive {
		spinnerMutex.Unlock()
		return
	}
	spinnerActive = true
	spinnerMutex.Unlock()

	spinners := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0

	for {
		select {
		case <-spinnerStop:
			spinnerMutex.Lock()
			spinnerActive = false
			spinnerMutex.Unlock()
			return
		default:
			fmt.Printf("\r🤔 处理中 %s", spinners[i%len(spinners)])
			i++
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// stopActivitySpinner stops the activity spinner
func stopActivitySpinner() {
	spinnerMutex.Lock()
	if spinnerActive {
		select {
		case spinnerStop <- true:
		default:
		}
	}
	spinnerMutex.Unlock()

	// Wait a bit for spinner to stop
	time.Sleep(50 * time.Millisecond)
	fmt.Print("\r                    \r") // Clear the spinner line
}

// typewriterPrint prints text with a typewriter effect
func typewriterPrint(text string) {
	// Skip typewriter effect in debug mode to avoid cluttering logs
	if debugMode {
		fmt.Print(text)
		return
	}

	// Print characters one by one with small delay
	for _, char := range text {
		fmt.Print(string(char))
		time.Sleep(20 * time.Millisecond) // Adjust speed as needed
	}
}

// createBasicTools creates some basic tools for demonstration
func createBasicTools() []*aitool.Tool {
	tools := make([]*aitool.Tool, 0)

	// Simple calculator tool
	calculatorTool, err := aitool.New(
		"calculator",
		aitool.WithDescription("Performs basic arithmetic calculations"),
		aitool.WithStringParam("input",
			aitool.WithParam_Description("Mathematical expression to calculate"),
			aitool.WithParam_Required(true),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			expression := params.GetString("input")
			if expression == "" {
				return "Please provide a mathematical expression", nil
			}

			// Simple calculator logic (for demo purposes)
			// In a real implementation, you'd use a proper expression evaluator
			log.Infof("Calculator received: %s", expression)
			return fmt.Sprintf("Calculated result for '%s': [This is a demo response]", expression), nil
		}),
	)
	if err == nil {
		tools = append(tools, calculatorTool)
	}

	// Echo tool for testing
	echoTool, err := aitool.New(
		"echo",
		aitool.WithDescription("Echoes back the input text"),
		aitool.WithStringParam("input",
			aitool.WithParam_Description("Text to echo back"),
			aitool.WithParam_Required(true),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			input := params.GetString("input")
			return fmt.Sprintf("Echo: %s", input), nil
		}),
	)
	if err == nil {
		tools = append(tools, echoTool)
	}

	// Time tool
	timeTool, err := aitool.New(
		"current_time",
		aitool.WithDescription("Gets the current date and time"),
		aitool.WithStringParam("format",
			aitool.WithParam_Description("Time format (optional)"),
			aitool.WithParam_Required(false),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			format := params.GetString("format")
			if format == "" {
				format = "2006-01-02 15:04:05"
			}
			return fmt.Sprintf("Current time: %s", time.Now().Format(format)), nil
		}),
	)
	if err == nil {
		tools = append(tools, timeTool)
	}

	return tools
}

// initializeDatabase initializes the Yakit database and configurations
func initializeDatabase() error {
	log.Info("Initializing Yakit database and configurations...")

	// Initialize Yakit database (project and profile)
	consts.InitializeYakitDatabase("", "", "")

	// Initialize CVE database (optional, don't fail if it doesn't work)
	_, err := consts.InitializeCVEDatabase()
	if err != nil {
		log.Warnf("Failed to initialize CVE database: %v", err)
	}

	// Call post-init database functions (network config, etc.)
	err = yakit.CallPostInitDatabase()
	if err != nil {
		log.Warnf("Failed to call post-init database functions: %v", err)
		return err
	}

	// Load AI provider configurations from database
	err = loadAIProvidersFromDatabase()
	if err != nil {
		log.Warnf("Failed to load AI providers from database: %v", err)
		// Don't return error, continue with default AI configuration
	}

	log.Info("Database and configurations initialized successfully")
	return nil
}

// loadAIProvidersFromDatabase loads AI provider configurations from yakit database
func loadAIProvidersFromDatabase() error {
	log.Info("AI provider configurations will be loaded automatically from database")
	// The yakit.CallPostInitDatabase() function has already loaded API keys
	// and other configurations into environment variables
	// AI gateways will automatically pick them up when making requests
	return nil
}
