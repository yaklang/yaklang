package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func main() {
	log.Info("Starting ReAct CLI Demo")

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
	}()

	// Create AI callback using the proper AIChatToAICallbackType wrapper
	aiCallback := aid.AIChatToAICallbackType(ai.Chat)

	// Create ReAct instance with configuration
	react, err := aireact.NewReAct(
		aireact.WithContext(ctx),
		aireact.WithAICallback(aiCallback),
		aireact.WithDebug(true),
		aireact.WithMaxIterations(5),
		aireact.WithMaxThoughts(3),
		aireact.WithMaxActions(3),
		aireact.WithTemperature(0.7, 0.3),
		aireact.WithEventHandler(func(event *ypb.AIOutputEvent) {
			// Handle output events
			fmt.Printf("[%s] %s\n", strings.ToUpper(event.Type), string(event.Content))
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

	// Start output handler
	go func() {
		for event := range outputChan {
			// Events are already handled by the event handler
			// This is just to keep the channel draining
			_ = event
		}
	}()

	// Interactive CLI loop
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("ReAct CLI is ready. Type your questions (type 'exit' to quit):")
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
			return
		}

		// Send user input to ReAct
		event := &ypb.AITriageInputEvent{
			IsFreeInput: true,
			FreeInput:   input,
		}

		inputChan.SafeFeed(event)

		// Give some time for processing
		time.Sleep(time.Millisecond * 100)
		fmt.Print("> ")
	}

	if err := scanner.Err(); err != nil {
		log.Errorf("Scanner error: %v", err)
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
