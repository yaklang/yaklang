package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

type concurrentWriteDetector struct {
	active     atomic.Int32
	overlapped atomic.Bool
	mu         sync.Mutex
	buffer     bytes.Buffer
}

func (w *concurrentWriteDetector) Write(p []byte) (int, error) {
	if w.active.Add(1) > 1 {
		w.overlapped.Store(true)
	}
	defer w.active.Add(-1)

	// Keep the write window open long enough for unsynchronized callers to
	// overlap deterministically.
	time.Sleep(time.Millisecond)

	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.Write(p)
}

func (w *concurrentWriteDetector) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return bytes.Clone(w.buffer.Bytes())
}

func TestStdioServer(t *testing.T) {
	t.Run("Can instantiate", func(t *testing.T) {
		mcpServer := NewMCPServer("test", "1.0.0")
		stdioServer := NewStdioServer(mcpServer)

		if stdioServer.server == nil {
			t.Error("MCPServer should not be nil")
		}
		if stdioServer.errLogger == nil {
			t.Error("errLogger should not be nil")
		}
	})

	t.Run("Can send and receive messages", func(t *testing.T) {
		// Create pipes for stdin and stdout
		stdinReader, stdinWriter := io.Pipe()
		stdoutReader, stdoutWriter := io.Pipe()

		// Create server
		mcpServer := NewMCPServer("test", "1.0.0",
			WithResourceCapabilities(true, true),
		)
		stdioServer := NewStdioServer(mcpServer)
		stdioServer.SetErrorLogger(log.New(io.Discard, "", 0))

		// Create context with cancel
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create error channel to catch server errors
		serverErrCh := make(chan error, 1)

		// Start server in goroutine
		go func() {
			err := stdioServer.Listen(ctx, stdinReader, stdoutWriter)
			if err != nil && err != io.EOF && err != context.Canceled {
				serverErrCh <- err
			}
			close(serverErrCh)
		}()

		// Create test message
		initRequest := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"clientInfo": map[string]interface{}{
					"name":    "test-client",
					"version": "1.0.0",
				},
			},
		}

		// Send request
		requestBytes, err := json.Marshal(initRequest)
		if err != nil {
			t.Fatal(err)
		}
		_, err = stdinWriter.Write(append(requestBytes, '\n'))
		if err != nil {
			t.Fatal(err)
		}

		// Read response
		scanner := bufio.NewScanner(stdoutReader)
		if !scanner.Scan() {
			t.Fatal("failed to read response")
		}
		responseBytes := scanner.Bytes()

		var response map[string]interface{}
		if err := json.Unmarshal(responseBytes, &response); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		// Verify response structure
		if response["jsonrpc"] != "2.0" {
			t.Errorf("expected jsonrpc version 2.0, got %v", response["jsonrpc"])
		}
		if response["id"].(float64) != 1 {
			t.Errorf("expected id 1, got %v", response["id"])
		}
		if response["error"] != nil {
			t.Errorf("unexpected error in response: %v", response["error"])
		}
		if response["result"] == nil {
			t.Error("expected result in response")
		}

		// Clean up
		cancel()
		stdinWriter.Close()
		stdoutWriter.Close()

		// Check for server errors
		if err := <-serverErrCh; err != nil {
			t.Errorf("unexpected server error: %v", err)
		}
	})

	t.Run("Serializes concurrent responses and notifications", func(t *testing.T) {
		stdioServer := NewStdioServer(NewMCPServer("test", "1.0.0"))
		writer := &concurrentWriteDetector{}

		const messageCount = 64
		var wg sync.WaitGroup
		errs := make(chan error, messageCount)
		for i := 0; i < messageCount; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				err := stdioServer.writeResponse(mcp.JSONRPCResponse{
					JSONRPC: mcp.JSONRPC_VERSION,
					ID:      id,
					Result:  map[string]int{"id": id},
				}, writer)
				if err != nil {
					errs <- err
				}
			}(i)
		}
		wg.Wait()
		close(errs)

		for err := range errs {
			t.Fatalf("write response: %v", err)
		}
		if writer.overlapped.Load() {
			t.Fatal("stdio writer was used concurrently")
		}

		scanner := bufio.NewScanner(bytes.NewReader(writer.Bytes()))
		lines := 0
		for scanner.Scan() {
			if !json.Valid(scanner.Bytes()) {
				t.Fatalf("invalid JSON-RPC frame: %q", scanner.Bytes())
			}
			lines++
		}
		if err := scanner.Err(); err != nil {
			t.Fatalf("scan responses: %v", err)
		}
		if lines != messageCount {
			t.Fatalf("got %d response frames, want %d", lines, messageCount)
		}
	})
}
