package client

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

func compileTestServer(outputPath string) error {
	cmd := exec.Command(
		"go",
		"build",
		"-o",
		outputPath,
		"../testdata/mockstdio_server.go",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("compilation failed: %v\nOutput: %s", err, output)
	}
	return nil
}

func TestStdioMCPClient(t *testing.T) {
	// Compile mock server
	mockServerPath := filepath.Join(os.TempDir(), "mockstdio_server")
	if err := compileTestServer(mockServerPath); err != nil {
		t.Fatalf("Failed to compile mock server: %v", err)
	}
	defer os.Remove(mockServerPath)

	client, err := NewStdioMCPClient(mockServerPath, []string{})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	t.Run("Initialize", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		request := mcp.InitializeRequest{}
		request.Params.ProtocolVersion = "1.0"
		request.Params.ClientInfo = mcp.Implementation{
			Name:    "test-client",
			Version: "1.0.0",
		}
		request.Params.Capabilities = mcp.ClientCapabilities{
			Roots: &struct {
				ListChanged bool `json:"listChanged,omitempty"`
			}{
				ListChanged: true,
			},
		}

		result, err := client.Initialize(ctx, request)
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		if result.ServerInfo.Name != "mock-server" {
			t.Errorf(
				"Expected server name 'mock-server', got '%s'",
				result.ServerInfo.Name,
			)
		}
	})

	t.Run("Ping", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := client.Ping(ctx)
		if err != nil {
			t.Errorf("Ping failed: %v", err)
		}
	})

	t.Run("ListResources", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		request := mcp.ListResourcesRequest{}
		result, err := client.ListResources(ctx, request)
		if err != nil {
			t.Errorf("ListResources failed: %v", err)
		}

		if len(result.Resources) != 1 {
			t.Errorf("Expected 1 resource, got %d", len(result.Resources))
		}
	})

	t.Run("ReadResource", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		request := mcp.ReadResourceRequest{}
		request.Params.URI = "test://resource"

		result, err := client.ReadResource(ctx, request)
		if err != nil {
			t.Errorf("ReadResource failed: %v", err)
		}

		if len(result.Contents) != 1 {
			t.Errorf("Expected 1 content item, got %d", len(result.Contents))
		}
	})

	t.Run("Subscribe and Unsubscribe", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Test Subscribe
		subRequest := mcp.SubscribeRequest{}
		subRequest.Params.URI = "test://resource"
		err := client.Subscribe(ctx, subRequest)
		if err != nil {
			t.Errorf("Subscribe failed: %v", err)
		}

		// Test Unsubscribe
		unsubRequest := mcp.UnsubscribeRequest{}
		unsubRequest.Params.URI = "test://resource"
		err = client.Unsubscribe(ctx, unsubRequest)
		if err != nil {
			t.Errorf("Unsubscribe failed: %v", err)
		}
	})

	t.Run("ListPrompts", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		request := mcp.ListPromptsRequest{}
		result, err := client.ListPrompts(ctx, request)
		if err != nil {
			t.Errorf("ListPrompts failed: %v", err)
		}

		if len(result.Prompts) != 1 {
			t.Errorf("Expected 1 prompt, got %d", len(result.Prompts))
		}
	})

	t.Run("GetPrompt", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		request := mcp.GetPromptRequest{}
		request.Params.Name = "test-prompt"

		result, err := client.GetPrompt(ctx, request)
		if err != nil {
			t.Errorf("GetPrompt failed: %v", err)
		}

		if len(result.Messages) != 1 {
			t.Errorf("Expected 1 message, got %d", len(result.Messages))
		}
	})

	t.Run("ListTools", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		request := mcp.ListToolsRequest{}
		result, err := client.ListTools(ctx, request)
		if err != nil {
			t.Errorf("ListTools failed: %v", err)
		}

		if len(result.Tools) != 1 {
			t.Errorf("Expected 1 tool, got %d", len(result.Tools))
		}
	})

	t.Run("CallTool", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		request := mcp.CallToolRequest{}
		request.Params.Name = "test-tool"
		request.Params.Arguments = map[string]interface{}{
			"param1": "value1",
		}

		result, err := client.CallTool(ctx, request)
		if err != nil {
			t.Errorf("CallTool failed: %v", err)
		}

		if len(result.Content) != 1 {
			t.Errorf("Expected 1 content item, got %d", len(result.Content))
		}
	})

	t.Run("SetLevel", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		request := mcp.SetLevelRequest{}
		request.Params.Level = mcp.LoggingLevelInfo

		err := client.SetLevel(ctx, request)
		if err != nil {
			t.Errorf("SetLevel failed: %v", err)
		}
	})

	t.Run("Complete", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		request := mcp.CompleteRequest{}
		request.Params.Ref = mcp.PromptReference{
			Type: "ref/prompt",
			Name: "test-prompt",
		}
		request.Params.Argument.Name = "test-arg"
		request.Params.Argument.Value = "test-value"

		result, err := client.Complete(ctx, request)
		if err != nil {
			t.Errorf("Complete failed: %v", err)
		}

		if len(result.Completion.Values) != 1 {
			t.Errorf(
				"Expected 1 completion value, got %d",
				len(result.Completion.Values),
			)
		}
	})
}
