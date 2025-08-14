package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var debugMode = false
var breakpointEnabled = false

func main() {
	// Command line flags
	var (
		language    = flag.String("lang", "zh", "Response language (zh for Chinese, en for English)")
		query       = flag.String("query", "", "One-time query mode (exits after response)")
		debug       = flag.Bool("debug", false, "Enable debug mode")
		interactive = flag.Bool("i", false, "Enable interactive tool review mode (requires user approval for each tool use)")
		breakpoint  = flag.Bool("breakpoint", false, "Enable breakpoint mode (pause before/after each AI interaction for inspection)")
		breakpointB = flag.Bool("b", false, "Enable breakpoint mode (shorthand for --breakpoint)")
	)
	flag.Parse()

	// Combine breakpoint flags and set global variable
	breakpointEnabled = *breakpoint || *breakpointB

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

	if *interactive {
		log.Info("Interactive tool review mode enabled - will require user approval for each tool use")
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
		// Add a longer timeout for complex ReAct processing
		opts = append(opts, aispec.WithTimeout(180)) // 3 minutes timeout for AI requests
		return ai.Chat(msg, opts...)
	})

	// For debugging, let's create a wrapper to see what's happening
	debugAICallback := func(config *aid.Config, req *aid.AIRequest) (*aid.AIResponse, error) {
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
			streamCompleted = false
			streamingMutex.Unlock()
		}

		if debugMode {
			log.Infof("AI callback succeeded")
		}
		return resp, nil
	}

	outputChan := make(chan *schema.AiOutputEvent, 100) // Buffered channel for output events

	// Create tool review handler if interactive mode is enabled
	var reactOptions []aireact.Option
	reactOptions = append(reactOptions,
		aireact.WithContext(ctx),
		aireact.WithAICallback(debugAICallback),
		aireact.WithDebug(debugMode), // Use debug mode from command line flag (independent of breakpoint)
		aireact.WithMaxIterations(5),
		aireact.WithMaxThoughts(3),
		aireact.WithMaxActions(3),
		aireact.WithTemperature(0.7, 0.3),
		aireact.WithLanguage(*language),
		aireact.WithTopToolsCount(20), // Show top 20 tools
		aireact.WithEventHandler(func(e *schema.AiOutputEvent) {
			outputChan <- e
		}),
	)

	// Add interactive tool review if enabled
	if *interactive {
		log.Info("Interactive tool review mode enabled")
		reactOptions = append(reactOptions,
			aireact.WithToolReview(true),
			aireact.WithReviewHandler(createInteractiveReviewHandler()),
		)
	}

	// Define event handler function that can be reused
	eventHandler := func(event *schema.AiOutputEvent) {
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
			result := extractResultContent(string(event.Content))
			fmt.Printf("[result]: %s\n", result)
			fmt.Printf("[ai]: final message for current loop\n")
		case schema.EVENT_TYPE_ITERATION:
			if debugMode {
				fmt.Printf("[iteration]: %s\n", string(event.Content))
			}
		case schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE:
			// Handle tool review events in interactive mode
			if debugMode {
				fmt.Printf("[tool_review]: %s\n", string(event.Content))
			}
		case schema.EVENT_TYPE_STREAM:
			if debugMode {
				fmt.Printf("[stream]: %s\n", string(event.Content))
			}
		case schema.EVENT_TYPE_STRUCTURED:
			if debugMode {
				fmt.Printf("[structured]: %s\n", string(event.Content))
			}
		default:
			if debugMode {
				fmt.Printf("[%s]: %s\n", strings.ToLower(string(event.Type)), string(event.Content))
			}
		}
	}

	// Add event handler
	reactOptions = append(reactOptions,
		aireact.WithEventHandler(eventHandler),
		// Use buildinaitools system instead of hardcoded tools
		aireact.WithBuiltinTools(),
	)

	// Create ReAct instance with all options
	react, err := aireact.NewReAct(reactOptions...)
	if err != nil {
		log.Errorf("Failed to create ReAct instance: %v", err)
		os.Exit(1)
	}

	// Start output handler that properly drains the channel and detects completion
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		for {
			select {
			case event, ok := <-outputChan:
				if !ok {
					return
				}
				// Handle the event using the configured event handler
				if event != nil {
					eventHandler(event)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Handle one-time query mode
	if *query != "" {
		handleSingleQuery(react, *query, ctx)
		return
	}

	// Interactive CLI loop
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("ReAct CLI ready. Enter your question (type 'exit' to quit, '/debug' to toggle debug mode):")
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
				fmt.Println("[debug]: enabled")
				log.SetLevel(log.DebugLevel)
			} else {
				fmt.Println("[debug]: disabled")
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
		streamCompleted = false
		pendingResponse = nil
		streamingMutex.Unlock()

		// Send user input to ReAct
		event := &ypb.AITriageInputEvent{
			IsFreeInput: true,
			FreeInput:   input,
		}

		fmt.Print("[processing]")

		// Show activity spinner while waiting
		go showActivitySpinner()

		react.ProcessInputEvent(event)

		// Stop spinner and show next prompt
		stopActivitySpinner()
		fmt.Print("\n> ")
	}

	if err := scanner.Err(); err != nil {
		log.Errorf("Scanner error: %v", err)
	}
}

// handleSingleQuery handles one-time query mode
func handleSingleQuery(reactChan *aireact.ReAct, query string, ctx context.Context) {
	event := &ypb.AITriageInputEvent{
		IsFreeInput: true,
		FreeInput:   query,
	}

	log.Infof("Processing query: %s", query)
	err := reactChan.ProcessInputEvent(event)
	if err != nil {
		log.Errorf("Failed to process input: %v", err)
	}

	log.Info("Query completed, exiting...")

	// Force exit after single query
	os.Exit(0)
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
	streamingActive   = false
	streamingMutex    sync.Mutex
	currentStreamLine = ""
	streamStartTime   time.Time
	streamCharCount   = 0
	streamDisplayed   = false // Track if we've already shown streaming output for this request
	streamCompleted   = false // Track if stream processing is completed

	// Activity spinner state
	spinnerActive = false
	spinnerStop   = make(chan bool, 1)
	spinnerMutex  sync.Mutex

	// Pending response for breakpoint
	pendingResponse *aid.AIResponse
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

	streamCompleted = true

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
			fmt.Printf("\r[processing] %s", spinners[i%len(spinners)])
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

	// Create scanner for user input
	scanner := bufio.NewScanner(os.Stdin)

	// Set up signal handler for Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Use goroutine to handle input and signals
	inputChan := make(chan string, 1)
	go func() {
		if scanner.Scan() {
			inputChan <- strings.TrimSpace(strings.ToLower(scanner.Text()))
		} else {
			inputChan <- "continue" // Default to continue if scan fails
		}
	}()

	// Wait for either user input or signal
	select {
	case input := <-inputChan:
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

	// Clean up signal handler
	signal.Stop(sigChan)
}

// handleResponseBreakpoint handles breakpoint functionality - pauses after AI interaction to inspect response
func handleResponseBreakpoint(resp *aid.AIResponse) {
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

	// Create scanner for user input
	scanner := bufio.NewScanner(os.Stdin)

	// Set up signal handler for Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Use goroutine to handle input and signals
	inputChan := make(chan string, 1)
	go func() {
		if scanner.Scan() {
			inputChan <- strings.TrimSpace(strings.ToLower(scanner.Text()))
		} else {
			inputChan <- "continue" // Default to continue if scan fails
		}
	}()

	// Wait for either user input or signal
	select {
	case input := <-inputChan:
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

	// Clean up signal handler
	signal.Stop(sigChan)
}

// createInteractiveReviewHandler creates an interactive tool review handler
func createInteractiveReviewHandler() func(reviewInfo *aireact.ToolReviewInfo) {
	return func(reviewInfo *aireact.ToolReviewInfo) {
		// Display tool information to user
		fmt.Printf("\n[TOOL REVIEW REQUIRED]\n")
		fmt.Printf("Tool: %s\n", reviewInfo.Tool.Name)
		fmt.Printf("Description: %s\n", reviewInfo.Tool.Description)
		fmt.Printf("Parameters: %v\n", reviewInfo.Params)
		fmt.Printf("\nPlease choose an action:\n")
		fmt.Printf("  1. continue    - Approve tool use\n")
		fmt.Printf("  2. wrong_tool  - Tool selection is wrong\n")
		fmt.Printf("  3. wrong_params - Parameters are wrong\n")
		fmt.Printf("  4. direct_answer - Skip tool and answer directly\n")
		fmt.Printf("  5. cancel      - Cancel operation\n")
		fmt.Print("Your choice (1-5): ")

		// Read user input
		scanner := bufio.NewScanner(os.Stdin)
		var response *aireact.ToolReviewResponse

		if scanner.Scan() {
			input := strings.TrimSpace(scanner.Text())
			switch input {
			case "1", "continue":
				response = &aireact.ToolReviewResponse{
					Suggestion: "continue",
				}
				fmt.Println("[REVIEW]: Tool use approved")
			case "2", "wrong_tool":
				fmt.Print("Enter suggested tool name (optional): ")
				scanner.Scan()
				suggestedTool := strings.TrimSpace(scanner.Text())
				fmt.Print("Enter search keyword (optional): ")
				scanner.Scan()
				keyword := strings.TrimSpace(scanner.Text())

				response = &aireact.ToolReviewResponse{
					Suggestion:        "wrong_tool",
					SuggestionTool:    suggestedTool,
					SuggestionKeyword: keyword,
				}
				fmt.Println("[REVIEW]: Tool reselection requested")
			case "3", "wrong_params":
				response = &aireact.ToolReviewResponse{
					Suggestion: "wrong_params",
				}
				fmt.Println("[REVIEW]: Parameter modification requested")
			case "4", "direct_answer":
				response = &aireact.ToolReviewResponse{
					Suggestion:     "direct_answer",
					DirectlyAnswer: true,
				}
				fmt.Println("[REVIEW]: Direct answer requested")
			case "5", "cancel":
				response = &aireact.ToolReviewResponse{
					Cancel: true,
				}
				fmt.Println("[REVIEW]: Operation cancelled")
			default:
				// Default to continue if invalid input
				response = &aireact.ToolReviewResponse{
					Suggestion: "continue",
				}
				fmt.Printf("[REVIEW]: Invalid input '%s', defaulting to continue\n", input)
			}
		} else {
			// Default to continue if unable to read
			response = &aireact.ToolReviewResponse{
				Suggestion: "continue",
			}
			fmt.Println("[REVIEW]: Unable to read input, defaulting to continue")
		}

		// Send response back
		select {
		case reviewInfo.ResponseChannel <- response:
			log.Infof("Tool review response sent: %s", response.Suggestion)
		default:
			log.Warnf("Failed to send tool review response - channel may be closed")
		}

		fmt.Print("Continuing with ReAct processing...\n\n")
	}
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
