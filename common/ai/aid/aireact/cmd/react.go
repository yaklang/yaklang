package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Global state management (simplified - most queue logic moved to ReAct)
var (
	globalUserInput      = make(chan string, 100) // Channel for all user input (increased buffer)
	waitingForReview     = false                  // Flag to indicate if we're waiting for review input
	reviewOptions        []reviewOption           // Current review options
	reviewMutex          sync.Mutex               // Mutex to protect review state
	currentReviewEventID string                   // Current review event ID

	// Breakpoint state management
	waitingForBreakpoint = false    // Flag to indicate if we're waiting for breakpoint input
	breakpointMutex      sync.Mutex // Mutex to protect breakpoint state
)

// reviewOption represents a review choice option
type reviewOption struct {
	value  string
	prompt string
}

var debugMode = false
var breakpointEnabled = false

// displayQueueInfo 显示 ReAct 队列信息
func displayQueueInfo(reactInstance *aireact.ReAct) {
	// 使用标准的 AIInputEvent 发送同步请求
	event := &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aireact.SYNC_TYPE_QUEUE_INFO,
	}

	err := reactInstance.SendInputEvent(event)
	if err != nil {
		fmt.Printf("Failed to get queue info: %v\n", err)
		return
	}
	fmt.Println("Queue info request sent - check output events for details")
}

// displayTimelineInfo 显示 ReAct 时间线信息
func displayTimelineInfo(reactInstance *aireact.ReAct, limit int) {
	// 使用标准的 AIInputEvent 发送同步请求
	var syncJsonInput string
	if limit > 0 {
		params := map[string]interface{}{
			"limit": limit,
		}
		if paramsJson, err := json.Marshal(params); err == nil {
			syncJsonInput = string(paramsJson)
		}
	}

	event := &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aireact.SYNC_TYPE_TIMELINE,
		SyncJsonInput: syncJsonInput,
	}

	err := reactInstance.SendInputEvent(event)
	if err != nil {
		fmt.Printf("Failed to get timeline info: %v\n", err)
		return
	}
	fmt.Printf("Timeline info request sent (limit: %d) - check output events for details\n", limit)
}

func main() {
	// Command line flags
	var (
		language    = flag.String("lang", "zh", "Response language (zh for Chinese, en for English)")
		query       = flag.String("query", "", "One-time query mode (exits after response)")
		debug       = flag.Bool("debug", false, "Enable debug mode")
		noInteract  = flag.Bool("no-interact", false, "Disable interactive tool review mode (auto-approve all tools)")
		breakpoint  = flag.Bool("breakpoint", false, "Enable breakpoint mode (pause before/after each AI interaction for inspection)")
		breakpointB = flag.Bool("b", false, "Enable breakpoint mode (shorthand for --breakpoint)")
	)
	flag.Parse()

	// Combine breakpoint flags and set global variable
	breakpointEnabled = *breakpoint || *breakpointB

	// Interactive mode is enabled by default unless --no-interact is specified
	interactiveMode := !*noInteract

	// Set debug mode from command line flag (independent of breakpoint mode)
	debugMode = *debug
	if debugMode {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug mode enabled")
	}

	if breakpointEnabled {
		log.Info("Breakpoint mode enabled - will pause before/after each AI interaction")
		log.Info("In breakpoint mode, press Enter/y to continue, e/q to exit, or Ctrl+C to terminate")
	}

	// Display mode information
	if interactiveMode {
		log.Info("Interactive tool review mode enabled - will require user approval for each tool use")
	} else {
		log.Info("Non-interactive mode enabled - all tool usage will be automatically approved")
	}

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
	aiCallback := aicommon.AIChatToAICallbackType(func(msg string, opts ...aispec.AIConfigOption) (string, error) {
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
		// Add a longer timeout for complex ReAct processing
		opts = append(opts, aispec.WithTimeout(180)) // 3 minutes timeout for AI requests
		return ai.Chat(msg, opts...)
	})

	// For debugging, let's create a wrapper to see what's happening
	debugAICallback := func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		if debugMode {
			log.Infof("AI Request: %s", req.GetPrompt())
		}

		// Breakpoint functionality - pause before AI interaction
		if breakpointEnabled {
			handleRequestBreakpoint(req.GetPrompt())
		}

		resp, err := aiCallback(config, req)
		if err != nil {
			if debugMode {
				log.Errorf("AI callback error: %v", err)
			}
			return nil, err
		}

		// Breakpoint functionality - pause after AI interaction to inspect response
		// In breakpoint mode, store the response and let stream processing trigger the breakpoint
		if breakpointEnabled {
			streamingMutex.Lock()
			pendingResponse = resp

			streamingMutex.Unlock()
		}

		if debugMode {
			log.Infof("AI callback succeeded")
		}
		return resp, nil
	}

	// Create input and output channels for client-server mode
	inputChan := make(chan *ypb.AIInputEvent, 100)
	outputChan := make(chan *schema.AiOutputEvent, 100)

	// Create ReAct client using aid.Config style
	var reactOptions []aireact.Option
	reactOptions = append(reactOptions,
		aireact.WithContext(ctx),
		aireact.WithAICallback(debugAICallback),
		aireact.WithDebug(debugMode),
		aireact.WithMaxIterations(25),
		aireact.WithLanguage(*language),
		aireact.WithTopToolsCount(100),
		aireact.WithAutoAIReview(true),
		aireact.WithEventHandler(func(e *schema.AiOutputEvent) {
			outputChan <- e
		}),
		// Use buildinaitools system
		aireact.WithBuiltinTools(),
	)

	// Configure tool review based on command line options
	if interactiveMode {
		// Interactive mode - require user approval for each tool
		log.Info("Configuring interactive tool review mode")
		reactOptions = append(reactOptions,
			aireact.WithToolReview(true),
		)
	} else {
		// Non-interactive mode - auto-approve all tools
		log.Info("Configuring non-interactive mode")
		reactOptions = append(reactOptions,
			aireact.WithAutoApproveTools(),
		)
	}

	// Create ReAct instance
	reactInstance, err := aireact.NewReAct(reactOptions...)
	if err != nil {
		log.Errorf("Failed to create ReAct instance: %v", err)
		os.Exit(1)
	}

	// Start input handler to send input events to ReAct event loop
	go func() {
		for {
			select {
			case inputEvent := <-inputChan:
				if err := reactInstance.SendInputEvent(inputEvent); err != nil {
					log.Errorf("Failed to send input event: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Start output handler with client-side event processing
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		for {
			select {
			case event, ok := <-outputChan:
				if !ok {
					return
				}
				// Handle the event using client-side event handler
				if event != nil {
					handleClientEvent(event, inputChan, interactiveMode)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Handle one-time query mode
	if *query != "" {
		handleInitialQuery(reactInstance, *query)
		// Give a moment for the query to be processed before starting interactive mode
		time.Sleep(100 * time.Millisecond)
	}

	// Start interactive CLI loop in background
	go handleInteractiveLoop(reactInstance, ctx)

	// Wait for tasks to complete and keep main thread alive
	waitForTasksAndContinue(ctx)
}

// handleInitialQuery sends the initial query directly to ReAct
func handleInitialQuery(reactInstance *aireact.ReAct, query string) {
	event := &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   query,
	}

	err := reactInstance.SendInputEvent(event)
	if err != nil {
		log.Errorf("Failed to send initial query: %v", err)
	} else {
		log.Infof("Initial query sent to ReAct: %s", query)
	}
}

// handleInteractiveLoop handles continuous user interaction
func handleInteractiveLoop(reactInstance *aireact.ReAct, ctx context.Context) {
	// Start global input reader in background
	go globalInputReader(ctx)

	// Don't show prompt immediately if we have an initial query running
	// The prompt will be shown after the initial query completes
	firstInput := true

	for {
		if debugMode {
			log.Debugf("Interactive loop: waiting for input...")
		}
		select {
		case input := <-globalUserInput:
			if debugMode {
				log.Debugf("Interactive loop received input: '%s'", input)
			}

			// Check if we're waiting for breakpoint input first
			breakpointMutex.Lock()
			if waitingForBreakpoint {
				if debugMode {
					log.Debugf("Processing breakpoint input: '%s'", input)
				}
				// Signal that breakpoint input was received - the breakpoint function will handle it
				waitingForBreakpoint = false
				breakpointMutex.Unlock()
				continue
			}
			breakpointMutex.Unlock()

			// Check if we're waiting for review input (before filtering empty input)
			reviewMutex.Lock()
			if waitingForReview {
				if debugMode {
					log.Debugf("Processing review input: '%s'", input)
				}
				// Always process review input immediately when waiting for review
				// Allow empty input for review (to select default continue)
				processReviewInput(input, reactInstance)
				reviewMutex.Unlock()
				fmt.Print("> ")
				continue
			}
			reviewMutex.Unlock()

			// For non-review input, filter empty input
			if input == "" {
				fmt.Print("> ")
				continue
			}

			if input == "exit" || input == "quit" {
				log.Info("User requested exit")
				os.Exit(0)
			}

			if input == "/debug" {
				debugMode = !debugMode
				if debugMode {
					fmt.Println("[debug]: enabled")
					log.SetLevel(log.DebugLevel)
				} else {
					fmt.Println("[debug]: disabled")
					log.SetLevel(log.InfoLevel)
				}
				fmt.Print("> ")
				continue
			}

			if input == "/queue" {
				displayQueueInfo(reactInstance)
				fmt.Print("> ")
				continue
			}

			if strings.HasSuffix(input, "???") || input == "/status" {
				fmt.Printf("\n=== SYSTEM STATUS ===\n")
				fmt.Printf("Debug mode: %v\n", debugMode)
				reviewMutex.Lock()
				fmt.Printf("Waiting for review: %v\n", waitingForReview)
				fmt.Printf("Review options count: %d\n", len(reviewOptions))
				reviewMutex.Unlock()
				fmt.Printf("====================\n")

				// Force show prompt
				fmt.Printf("\n" + strings.Repeat("=", 60) + "\n")
				fmt.Printf("🎯 Manual prompt trigger! Ready for next question.\n")
				fmt.Printf("ReAct CLI ready. Enter your question (type 'exit' to quit, '/debug' to toggle debug mode, '/queue' to view queue, '/timeline [limit]' to view timeline):\n")
				fmt.Print("> ")
				continue
			}

			if strings.HasPrefix(input, "/breakpoint") || strings.HasPrefix(input, "/bp") {
				breakpointEnabled = true
				continue
			}

			if strings.HasPrefix(input, "/timeline") {
				// Parse optional limit parameter
				parts := strings.Fields(input)
				limit := 20 // Default limit
				if len(parts) > 1 {
					if parsedLimit, err := strconv.Atoi(parts[1]); err == nil && parsedLimit > 0 {
						limit = parsedLimit
					}
				}
				displayTimelineInfo(reactInstance, limit)
				fmt.Print("> ")
				continue
			}

			// Show the interactive prompt if this is the first regular input or if needed after task completion
			if firstInput {
				fmt.Println("ReAct CLI ready. Enter your question (type 'exit' to quit, '/debug' to toggle debug mode, '/queue' to view queue, '/timeline [limit]' to view timeline):")
				firstInput = false
			}

			// Send query directly to ReAct
			if debugMode {
				log.Debugf("Sending regular input to ReAct: '%s'", input)
			}

			event := &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   input,
			}

			err := reactInstance.SendInputEvent(event)
			if err != nil {
				fmt.Printf("Failed to send query: %v\n", err)
			} else {
				fmt.Printf("Query sent to ReAct: %s\n", input)
			}
			fmt.Print("> ")

		case <-ctx.Done():
			log.Info("Context cancelled, exiting interactive loop")
			return
		}
	}
}

// globalInputReader reads from stdin and sends to global channel
func globalInputReader(ctx context.Context) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		input := strings.TrimSpace(scanner.Text())
		if debugMode {
			log.Debugf("Input reader got: '%s'", input)
		}

		if debugMode {
			log.Infof("start to put input into globalUserInput")
		}
		select {
		case globalUserInput <- input:
			// Successfully sent
			if debugMode {
				log.Debugf("Input sent to channel: '%s'", input)
			}
		case <-ctx.Done():
			return
		}
	}

	if err := scanner.Err(); err != nil {
		log.Errorf("Scanner error: %v", err)
	}
}

// waitForTasksAndContinue keeps the main thread alive until context cancellation
func waitForTasksAndContinue(ctx context.Context) {
	<-ctx.Done()
	log.Info("Context done, shutting down")
}

// extractResultContent extracts the actual result from the JSON result and formats it for better readability
func extractResultContent(content string) string {
	// Try to extract "result" field from JSON
	if strings.Contains(content, `"result"`) {
		start := strings.Index(content, `"result":"`)
		if start != -1 {
			start += 10 // length of `"result":"`
			end := strings.Index(content[start:], `"`)
			if end != -1 {
				result := content[start : start+end]
				// Unescape JSON string
				result = strings.ReplaceAll(result, `\"`, `"`)
				result = strings.ReplaceAll(result, `\\`, `\`)
				result = strings.ReplaceAll(result, `\n`, "\n")
				result = strings.ReplaceAll(result, `\t`, "\t")
				return result
			}
		}
	}

	// If it's already human-readable text, return as-is
	return content
}

// streamingState tracks the current streaming state
var (
	streamingActive = false
	streamingMutex  sync.Mutex
	streamStartTime time.Time
	streamCharCount = 0
	streamDisplayed = false // Track if we've already shown streaming output for this request

	// Activity spinner state
	spinnerActive = false
	spinnerStop   = make(chan bool, 1)
	spinnerMutex  sync.Mutex

	// Pending response for breakpoint
	pendingResponse *aicommon.AIResponse
)

// showRawStreamOutput displays the raw AI stream in real-time (or buffered in breakpoint mode)
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

	// In breakpoint mode, collect all content and display at the end
	if breakpointEnabled {
		streamingActive = true
		streamDisplayed = true
		streamStartTime = time.Now()
		streamCharCount = 0
		streamingMutex.Unlock()

		// Collect all content without displaying
		var buffer []byte
		tempBuffer := make([]byte, 1024)
		for {
			n, err := reader.Read(tempBuffer)
			if n > 0 {
				buffer = append(buffer, tempBuffer[:n]...)
				streamCharCount += n
			}
			if err != nil {
				break
			}
		}

		// Display complete content at once
		elapsed := time.Since(streamStartTime)
		content := string(buffer)
		// Clean up content (remove control characters)
		cleanContent := ""
		for _, r := range content {
			if r != '\n' && r != '\r' && r != '\t' && r != '\x00' {
				cleanContent += string(r)
			}
		}

		fmt.Printf("[stream]: %s\n", cleanContent)
		fmt.Printf("[stream]: [%d chars, %.1fs] done\n", streamCharCount, elapsed.Seconds())

		// Mark stream as completed and trigger breakpoint if needed
		markStreamCompleted()
		return
	}

	// Normal real-time streaming mode
	if !streamingActive {
		streamingActive = true
		streamDisplayed = true
		streamStartTime = time.Now()
		streamCharCount = 0
		fmt.Print("[stream]: ")
	}
	streamingMutex.Unlock()

	const maxDisplayWidth = 60 // Maximum display width
	var displayBuffer []rune   // Buffer to store display content
	var byteBuffer []byte      // Buffer to accumulate bytes for UTF-8 decoding

	buffer := make([]byte, 1024) // Read larger chunks for better UTF-8 handling
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			streamingMutex.Lock()
			streamCharCount += n

			// Append to byte buffer
			byteBuffer = append(byteBuffer, buffer[:n]...)

			// Find the last complete UTF-8 character boundary
			validEnd := len(byteBuffer)
			for validEnd > 0 {
				if utf8.ValidString(string(byteBuffer[:validEnd])) {
					break
				}
				validEnd--
			}

			if validEnd > 0 {
				// Convert valid UTF-8 bytes to string
				text := string(byteBuffer[:validEnd])

				// Filter out control characters and add to display buffer
				for _, r := range text {
					if r != '\n' && r != '\r' && r != '\t' && r != '\x00' {
						displayBuffer = append(displayBuffer, r)
					}
				}

				// Keep remaining incomplete bytes for next iteration
				byteBuffer = byteBuffer[validEnd:]

				// Implement scrolling marquee effect
				if len(displayBuffer) > maxDisplayWidth {
					// Keep only the last maxDisplayWidth characters
					displayBuffer = displayBuffer[len(displayBuffer)-maxDisplayWidth:]
				}

				// Clear current line and redraw
				fmt.Print("\r[stream]: ")
				fmt.Print(string(displayBuffer))

				// Add padding to clear any remaining characters
				padding := maxDisplayWidth - len(displayBuffer)
				if padding > 0 {
					fmt.Print(strings.Repeat(" ", padding))
				}
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
	// Clear the line completely before showing final message
	fmt.Print("\r" + strings.Repeat(" ", maxDisplayWidth+20) + "\r")
	fmt.Printf("[stream]: [%d chars, %.1fs] done\n", streamCharCount, elapsed.Seconds())
	streamingMutex.Unlock()

	// Mark stream as completed and trigger breakpoint if needed
	markStreamCompleted()
}

// markStreamCompleted marks the stream as completed and triggers response breakpoint if needed
func markStreamCompleted() {
	streamingMutex.Lock()
	defer streamingMutex.Unlock()

	// If we're in breakpoint mode and have a pending response, trigger the breakpoint
	if breakpointEnabled && pendingResponse != nil {
		// Reset state before calling breakpoint to avoid locks
		resp := pendingResponse
		pendingResponse = nil
		streamingMutex.Unlock()

		handleResponseBreakpoint(resp)

		streamingMutex.Lock()
	}
}

// showReasonStreamOutput displays reasoning stream
func showReasonStreamOutput(reader io.Reader) {
	if debugMode {
		fmt.Print("\n[reasoning]: ")
		io.Copy(os.Stdout, reader)
		fmt.Print(" done\n")
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

// handleRequestBreakpoint handles breakpoint functionality - pauses before AI interaction
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

	// Set breakpoint state to indicate we're waiting for breakpoint input
	breakpointMutex.Lock()
	waitingForBreakpoint = true
	breakpointMutex.Unlock()

	// Set up signal handler for Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Wait for input from the global input channel instead of creating a new scanner
	// This avoids the conflict with globalInputReader
	select {
	case input := <-globalUserInput:
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
	case <-time.After(60 * time.Second): // 60 second timeout
		fmt.Printf("\n⏰ Timeout after 60 seconds, continuing with AI request...\n")
		fmt.Printf(strings.Repeat("=", 80) + "\n\n")
	}

	// Clear breakpoint state when done
	breakpointMutex.Lock()
	waitingForBreakpoint = false
	breakpointMutex.Unlock()
}

// handleResponseBreakpoint handles breakpoint functionality - pauses after AI interaction to inspect response
func handleResponseBreakpoint(resp *aicommon.AIResponse) {
	fmt.Printf("\n" + strings.Repeat("=", 80) + "\n")
	fmt.Printf("🛑 RESPONSE BREAKPOINT: AI Response Received\n")
	fmt.Printf(strings.Repeat("=", 80) + "\n")
	fmt.Printf("AI RESPONSE CONTENT:\n")
	fmt.Printf(strings.Repeat("-", 40) + "\n")

	// Extract and display response content safely
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

	// Set breakpoint state to indicate we're waiting for breakpoint input
	breakpointMutex.Lock()
	waitingForBreakpoint = true
	breakpointMutex.Unlock()

	// Set up signal handler for Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Wait for input from the global input channel instead of creating a new scanner
	// This avoids the conflict with globalInputReader
	select {
	case input := <-globalUserInput:
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
	case <-time.After(60 * time.Second): // 60 second timeout
		fmt.Printf("\n⏰ Timeout after 60 seconds, continuing with response processing...\n")
		fmt.Printf(strings.Repeat("=", 80) + "\n\n")
	}

	// Clear breakpoint state when done
	breakpointMutex.Lock()
	waitingForBreakpoint = false
	breakpointMutex.Unlock()
}

// parseSelectionIndex parses user input as a selection index (1-based) and returns 0-based index, or -1 if invalid
func parseSelectionIndex(input string, maxOptions int) int {
	if len(input) == 1 && input[0] >= '1' && input[0] <= '9' {
		idx := int(input[0] - '1')
		if idx < maxOptions {
			return idx
		}
	}
	return -1
}

// handleClientEvent handles events in client mode using input channel
func handleClientEvent(event *schema.AiOutputEvent, inputChan chan<- *ypb.AIInputEvent, interactiveMode bool) {
	if debugMode {
		content := string(event.Content)
		preview := content
		if len(content) > 100 {
			preview = content[:100] + "..."
		}
		log.Debugf("Handling client event: type=%s, content_preview=%s", event.Type, preview)
	}

	// Handle output events with simplified display
	switch event.Type {
	case schema.EVENT_TYPE_THOUGHT:
		// Display thinking process
		fmt.Printf("[think]: %s\n", string(event.Content))
	case schema.EVENT_TYPE_ACTION:
		fmt.Printf("[action]: %s\n", string(event.Content))
	case schema.EVENT_TYPE_OBSERVATION:
		fmt.Printf("[observe]: %s\n", string(event.Content))
	case schema.EVENT_TYPE_RESULT:
		if debugMode {
			log.Debugf("Processing EVENT_TYPE_RESULT case")
		}
		result := extractResultContent(string(event.Content))
		fmt.Printf("[result]: %s\n", result)
		fmt.Printf("[ai]: final message for current loop\n")

		// Reset review state when ReAct loop completes
		reviewMutex.Lock()
		if waitingForReview {
			waitingForReview = false
			reviewOptions = nil
			currentReviewEventID = ""
		}
		reviewMutex.Unlock()

		// Show prompt for next interaction after task completion
		if debugMode {
			log.Debugf("Task completed, showing prompt after delay...")
		}

		go func() {
			// Add a longer delay to ensure all output is flushed
			time.Sleep(500 * time.Millisecond)

			if debugMode {
				log.Debugf("Displaying task completion prompt now")
			}

			fmt.Print("> ")

			// Force flush the output multiple times
			os.Stdout.Sync()
			os.Stderr.Sync()

			if debugMode {
				log.Debugf("Task completion prompt displayed and flushed")
			}
		}()
	case schema.EVENT_TYPE_STRUCTURED:
		// Handle queue info and timeline events
		content := string(event.Content)
		if strings.Contains(content, "queue_name") {
			fmt.Printf("\n=== REACT QUEUE INFO ===\n")
			fmt.Printf("%s\n", content)
			fmt.Printf("========================\n\n")
		} else if strings.Contains(content, "total_entries") {
			displayFormattedTimeline(content)
		} else if debugMode {
			fmt.Printf("[structured]: %s\n", content)
		}
	case schema.EVENT_TYPE_ITERATION:
		if debugMode {
			fmt.Printf("[iteration]: %s\n", string(event.Content))
		}
	case schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE:
		// Handle tool review events
		if debugMode {
			fmt.Printf("[tool_review]: %s\n", string(event.Content))
		}

		// In interactive mode, handle user interaction
		if interactiveMode {
			handleReviewRequireClient(event, inputChan)
		}
		// In non-interactive mode, this event will be handled by DoWaitAgree auto-approval
	case schema.EVENT_TYPE_STREAM:
		// Always show stream events with scrolling effect
		fmt.Printf("[stream]: %s\n", string(event.Content))
	default:
		if debugMode {
			fmt.Printf("[%s]: %s\n", strings.ToLower(string(event.Type)), string(event.Content))
		}
	}

	// Force trigger prompt if the event type suggests task completion
	if event.Type == schema.EVENT_TYPE_RESULT || strings.Contains(string(event.Content), "final message") {
		if debugMode {
			log.Debugf("Force triggering completion prompt due to event type: %s", event.Type)
		}
		go func() {
			time.Sleep(1 * time.Second) // Longer delay
			os.Stdout.Sync()
		}()
	}
}

// handleReviewRequireClient handles TOOL_USE_REVIEW_REQUIRE events using input channel
func handleReviewRequireClient(event *schema.AiOutputEvent, inputChan chan<- *ypb.AIInputEvent) {
	// Parse the review event content
	var reviewData map[string]interface{}
	if err := json.Unmarshal(event.Content, &reviewData); err != nil {
		log.Errorf("Failed to parse review event: %v", err)
		return
	}

	// Extract information from the event
	eventID := event.GetInteractiveId()
	if eventID == "" {
		log.Errorf("No interactive ID found in review event")
		return
	}

	toolName, _ := reviewData["tool"].(string)
	toolDesc, _ := reviewData["tool_description"].(string)
	selectors, _ := reviewData["selectors"].([]interface{})

	// Display tool information
	fmt.Printf("\n[TOOL REVIEW REQUIRED]\n")
	fmt.Printf("Tool: %s\n", toolName)
	if toolDesc != "" {
		fmt.Printf("Description: %s\n", toolDesc)
	}
	if debugMode {
		if params, ok := reviewData["params"]; ok {
			fmt.Printf("Parameters: %v\n", params)
		}
	}

	// Display selectors if available
	var options []reviewOption
	if len(selectors) > 0 {
		for _, sel := range selectors {
			if selMap, ok := sel.(map[string]interface{}); ok {
				option := reviewOption{
					value:  getString(selMap, "value"),
					prompt: getString(selMap, "prompt"),
				}
				if option.prompt == "" {
					option.prompt = getString(selMap, "prompt_english")
				}
				options = append(options, option)
			}
		}
	}

	// Use default options if none provided
	if len(options) == 0 {
		options = []reviewOption{
			{value: "continue", prompt: "同意工具使用"},
			{value: "wrong_tool", prompt: "工具选择不当"},
			{value: "wrong_params", prompt: "参数不合理"},
			{value: "direct_answer", prompt: "要求AI直接回答"},
		}
	}

	// Display options
	fmt.Printf("\nPlease choose an action:\n")
	for i, option := range options {
		if option.value == "continue" {
			fmt.Printf("  %d. %s - %s (default, press Enter)\n", i+1, option.value, option.prompt)
		} else {
			fmt.Printf("  %d. %s - %s\n", i+1, option.value, option.prompt)
		}
	}

	// Check if continue option exists for prompt message
	hasContinue := false
	for _, option := range options {
		if option.value == "continue" {
			hasContinue = true
			break
		}
	}

	if hasContinue {
		fmt.Printf("Your choice (1-%d, Enter for continue): ", len(options))
	} else {
		fmt.Printf("Your choice (1-%d): ", len(options))
	}

	// Set up review state and wait for global input
	reviewMutex.Lock()
	waitingForReview = true
	reviewOptions = options
	currentReviewEventID = eventID
	reviewMutex.Unlock()

	// Add a timeout mechanism to auto-continue if no input received
	go func(eventID string) {
		time.Sleep(60 * time.Second) // 60 second timeout
		reviewMutex.Lock()
		if waitingForReview && currentReviewEventID == eventID {
			log.Warnf("Review timeout reached, auto-selecting continue")
			waitingForReview = false
			reviewOptions = nil
			currentReviewEventID = ""
			reviewMutex.Unlock()

			// Send continue response directly
			inputEvent := &ypb.AIInputEvent{
				IsInteractiveMessage: true,
				InteractiveId:        eventID,
				InteractiveJSONInput: `{"suggestion": "continue"}`,
			}

			// Try to send via inputChan
			select {
			case inputChan <- inputEvent:
				fmt.Printf("\n[TIMEOUT]: Auto-selected continue after 60 seconds\n> ")
			default:
				log.Errorf("Failed to send timeout input event")
			}
		} else {
			reviewMutex.Unlock()
		}
	}(eventID)

	// The processReviewInput function will handle the actual input when it arrives
}

// processReviewInput processes user input for review selection
func processReviewInput(input string, reactInstance *aireact.ReAct) {
	var selectedValue string

	// Handle empty input (just pressing Enter)
	if strings.TrimSpace(input) == "" {
		// Look for "continue" option first
		for _, option := range reviewOptions {
			if option.value == "continue" {
				selectedValue = "continue"
				fmt.Printf("[REVIEW]: Empty input detected, selecting default: %s\n", selectedValue)
				break
			}
		}
		// If no "continue" option found, use first option
		if selectedValue == "" {
			selectedValue = reviewOptions[0].value
			fmt.Printf("[REVIEW]: Empty input detected, selecting first option: %s\n", selectedValue)
		}
	} else {
		// Try to parse as number first
		if idx := parseSelectionIndex(input, len(reviewOptions)); idx >= 0 {
			selectedValue = reviewOptions[idx].value
		} else {
			// Try to match by value
			for _, option := range reviewOptions {
				if strings.EqualFold(input, option.value) {
					selectedValue = option.value
					break
				}
			}
		}

		// Default to continue if available, otherwise first option
		if selectedValue == "" {
			// Look for "continue" option first
			for _, option := range reviewOptions {
				if option.value == "continue" {
					selectedValue = "continue"
					fmt.Printf("[REVIEW]: Invalid input '%s', defaulting to %s\n", input, selectedValue)
					break
				}
			}
			// If no "continue" option found, use first option
			if selectedValue == "" {
				selectedValue = reviewOptions[0].value
				fmt.Printf("[REVIEW]: Invalid input '%s', defaulting to %s\n", input, selectedValue)
			}
		} else {
			fmt.Printf("[REVIEW]: Selected action: %s\n", selectedValue)
		}
	}

	// Create and send input event to ReAct
	inputEvent := &ypb.AIInputEvent{
		IsInteractiveMessage: true,
		InteractiveId:        currentReviewEventID,
		InteractiveJSONInput: fmt.Sprintf(`{"suggestion": "%s"}`, selectedValue),
	}

	// Send the input event through ReAct
	err := reactInstance.SendInputEvent(inputEvent)
	if err != nil {
		log.Errorf("Failed to send input event: %v", err)
	}

	fmt.Print("Continuing with ReAct processing...\n\n")

	waitingForReview = false
	reviewOptions = nil
	currentReviewEventID = ""
}

// getString safely extracts a string value from a map
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
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

	log.Info("Database and configurations initialized successfully")
	return nil
}

// displayFormattedTimeline 显示格式化的时间线信息
func displayFormattedTimeline(jsonContent string) {
	// 解析JSON内容
	var timelineData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonContent), &timelineData); err != nil {
		log.Errorf("Failed to parse timeline JSON: %v", err)
		fmt.Printf("\n=== REACT TIMELINE ===\n")
		fmt.Printf("%s\n", jsonContent)
		fmt.Printf("======================\n\n")
		return
	}

	// 提取基本信息
	totalEntries, _ := timelineData["total_entries"].(float64)
	limit, _ := timelineData["limit"].(float64)
	entriesData, _ := timelineData["entries"].([]interface{})

	// 显示标题和统计信息
	fmt.Printf("\n")
	fmt.Printf("╔══════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║                                🕐 REACT TIMELINE                             ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║ Total Entries: %-3.0f │ Showing: %-3.0f │ Timeline Activity Overview         ║\n", totalEntries, limit)
	fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════╝\n")

	if len(entriesData) == 0 {
		fmt.Printf("┌─ No timeline entries available\n")
		fmt.Printf("└─ Timeline is empty\n\n")
		return
	}

	// 显示时间线条目
	for i, entryData := range entriesData {
		entryMap, ok := entryData.(map[string]interface{})
		if !ok {
			continue
		}

		// 解析时间戳
		timestampStr, _ := entryMap["timestamp"].(string)
		entryType, _ := entryMap["type"].(string)
		content, _ := entryMap["content"].(string)

		// 解析时间
		var timeStr string
		if timestamp, err := time.Parse(time.RFC3339Nano, timestampStr); err == nil {
			timeStr = timestamp.Format("15:04:05.000")
		} else {
			timeStr = "unknown"
		}

		// 根据类型选择图标和颜色前缀
		var icon, typeDisplay string
		switch entryType {
		case "tool_result":
			icon = "🔧"
			typeDisplay = "TOOL"
		case "user_interaction":
			icon = "👤"
			typeDisplay = "USER"
		case "text":
			icon = "📝"
			typeDisplay = "TEXT"
		default:
			icon = "❓"
			typeDisplay = strings.ToUpper(entryType)
		}

		// 显示连接线
		isLast := i == len(entriesData)-1
		connector := "├─"
		if isLast {
			connector = "└─"
		}

		// 显示主要条目信息
		fmt.Printf("%s[%s] %s %s\n", connector, timeStr, icon, typeDisplay)

		// 处理内容显示
		if content != "" {
			contentLines := utils.ParseStringToRawLines(content)
			for j, line := range contentLines {
				// 限制每行长度避免过宽显示
				if len(line) > 100 {
					line = line[:97] + "..."
				}

				linePrefix := "│    "
				if isLast {
					linePrefix = "     "
				}

				// 对于第一行，显示内容标题
				if j == 0 && len(contentLines) > 1 {
					fmt.Printf("%s┌─ Content:\n", linePrefix)
					fmt.Printf("%s│  %s\n", linePrefix, line)
				} else if j == 0 {
					fmt.Printf("%s━━ %s\n", linePrefix, line)
				} else if j == len(contentLines)-1 && len(contentLines) > 1 {
					fmt.Printf("%s└─ %s\n", linePrefix, line)
				} else {
					fmt.Printf("%s│  %s\n", linePrefix, line)
				}

				// 限制显示行数避免过长输出
				if j >= 8 && len(contentLines) > 10 {
					remaining := len(contentLines) - j - 1
					fmt.Printf("%s└─ ... (%d more lines)\n", linePrefix, remaining)
					break
				}
			}
		}

		// 添加条目间的分隔
		if !isLast {
			fmt.Printf("│\n")
		}
	}

	fmt.Printf("\n")
}
