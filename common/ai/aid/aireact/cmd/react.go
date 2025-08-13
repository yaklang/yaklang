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
	"github.com/yaklang/yaklang/common/utils/chanx"
	_ "github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var debugMode = false

func main() {
	// Command line flags
	var (
		language = flag.String("lang", "zh", "Response language (zh for Chinese, en for English)")
		query    = flag.String("query", "", "One-time query mode (exits after response)")
		debug    = flag.Bool("debug", false, "Enable debug mode")
	)
	flag.Parse()

	// Set debug mode from command line flag
	debugMode = *debug
	if debugMode {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug mode enabled")
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
		aireact.WithDebug(debugMode), // Use debug mode from command line flag
		aireact.WithMaxIterations(5),
		aireact.WithMaxThoughts(3),
		aireact.WithMaxActions(3),
		aireact.WithTemperature(0.7, 0.3),
		aireact.WithLanguage(*language),
		aireact.WithTopToolsCount(20), // Show top 20 tools
		aireact.WithEventHandler(func(event *ypb.AIOutputEvent) {
			// Handle output events with simplified display
			switch event.Type {
			case "react_thought":
				// Display thinking process
				fmt.Printf("[think]: %s\n", string(event.Content))
			case "react_action":
				fmt.Printf("[action]: %s\n", string(event.Content))
			case "react_observation":
				fmt.Printf("[observe]: %s\n", string(event.Content))
			case "react_result":
				fmt.Printf("[result]: %s\n", extractResultContent(string(event.Content)))
				fmt.Printf("[ai]: final message for current loop\n")
			case "react_error":
				fmt.Printf("[error]: %s\n", string(event.Content))
			case "react_info":
				if debugMode {
					fmt.Printf("[info]: %s\n", string(event.Content))
				} else {
					// Show important info messages even in non-debug mode
					content := string(event.Content)
					if strings.Contains(content, "preparing") || strings.Contains(content, "generating") ||
						strings.Contains(content, "executing") || strings.Contains(content, "tool") {
						fmt.Printf("[info]: %s\n", content)
					}
				}
			case "react_iteration":
				if debugMode {
					fmt.Printf("[iteration]: %s\n", string(event.Content))
				}
			default:
				if debugMode {
					fmt.Printf("[%s]: %s\n", strings.ToLower(event.Type), string(event.Content))
				}
			}
		}),
		// Use buildinaitools system instead of hardcoded tools
		aireact.WithBuiltinTools(),
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

	// Channel for signaling response completion
	responseCompleteChan := make(chan struct{}, 1)

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
				// Check for completion events before they're processed by event handler
				if event.Type == "react_result" || event.Type == "react_error" {
					select {
					case responseCompleteChan <- struct{}{}:
					default: // Don't block if channel is full
					}
				}
				// Events are handled by the event handler configured in ReAct
			case <-ctx.Done():
				return
			}
		}
	}()

	// Handle one-time query mode
	if *query != "" {
		handleSingleQuery(*query, inputChan, responseCompleteChan, ctx)
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
		streamingMutex.Unlock()

		// Send user input to ReAct
		event := &ypb.AITriageInputEvent{
			IsFreeInput: true,
			FreeInput:   input,
		}

		fmt.Print("[processing]")

		// Show activity spinner while waiting
		go showActivitySpinner()

		inputChan.SafeFeed(event)

		// Wait for the response to complete before showing next prompt
		waitForResponseCompletion(responseCompleteChan, ctx)

		// Stop spinner and show next prompt
		stopActivitySpinner()
		fmt.Print("\n> ")
	}

	if err := scanner.Err(); err != nil {
		log.Errorf("Scanner error: %v", err)
	}
}

// handleSingleQuery handles one-time query mode
func handleSingleQuery(query string, inputChan *chanx.UnlimitedChan[*ypb.AITriageInputEvent], responseCompleteChan chan struct{}, ctx context.Context) {
	event := &ypb.AITriageInputEvent{
		IsFreeInput: true,
		FreeInput:   query,
	}

	log.Infof("Processing query: %s", query)
	inputChan.SafeFeed(event)

	// Wait for response completion
	waitForResponseCompletion(responseCompleteChan, ctx)

	log.Info("Query completed, exiting...")

	// Force exit after single query
	os.Exit(0)
}

// waitForResponseCompletion waits for the AI response to complete
func waitForResponseCompletion(responseCompleteChan chan struct{}, ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-responseCompleteChan:
		// Response completed successfully
		return
	case <-time.After(200 * time.Second): // Timeout after 200 seconds (longer than AI timeout of 180s)
		fmt.Println("\n[timeout]: response timeout, please retry")
		return
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
