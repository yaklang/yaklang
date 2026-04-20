package aicommon

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestEmitterWithAIInfoProvider_ResolvesDynamically(t *testing.T) {
	var captured []*schema.AiOutputEvent
	meta := AIEventMeta{}
	emitter := NewEmitter("test-id", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		captured = append(captured, e)
		return e, nil
	})

	scoped := emitter.WithAIInfoProvider(func() AIEventMeta {
		return meta
	})

	if _, err := scoped.EmitInfo("before"); err != nil {
		t.Fatalf("EmitInfo before update failed: %v", err)
	}
	meta = AIEventMeta{
		Service:   "openai",
		ModelName: "gpt-4o-mini",
	}
	if _, err := scoped.EmitInfo("after"); err != nil {
		t.Fatalf("EmitInfo after update failed: %v", err)
	}

	if len(captured) != 2 {
		t.Fatalf("expected 2 events, got %d", len(captured))
	}
	if captured[0].AIService != "" || captured[0].AIModelName != "" {
		t.Fatal("expected first event to be emitted before metadata became available")
	}
	if captured[1].AIService != "openai" || captured[1].AIModelName != "gpt-4o-mini" {
		t.Fatalf("expected provider-backed metadata on second event, got %q:%q", captured[1].AIService, captured[1].AIModelName)
	}
}

func TestAIResponseBindEmitter_UsesDynamicModelInfo(t *testing.T) {
	var captured []*schema.AiOutputEvent
	emitter := NewEmitter("test-id", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		captured = append(captured, e)
		return e, nil
	})

	rsp := NewUnboundAIResponse()
	bound := rsp.BindEmitter(emitter)

	if _, err := bound.EmitInfo("before model info"); err != nil {
		t.Fatalf("EmitInfo before SetModelInfo failed: %v", err)
	}
	rsp.SetModelInfo("openai", "gpt-4o-mini")
	if _, err := bound.EmitInfo("after model info"); err != nil {
		t.Fatalf("EmitInfo after SetModelInfo failed: %v", err)
	}

	if len(captured) != 2 {
		t.Fatalf("expected 2 events, got %d", len(captured))
	}
	if captured[0].AIService != "" || captured[0].AIModelName != "" {
		t.Fatal("expected no AI metadata before response model info is available")
	}
	if captured[1].AIService != "openai" || captured[1].AIModelName != "gpt-4o-mini" {
		t.Fatalf("expected bound emitter to resolve response model info dynamically, got %q:%q", captured[1].AIService, captured[1].AIModelName)
	}
	if captured[1].AIModelVerboseName == "" {
		t.Fatal("expected dynamic binding to include model verbose name")
	}
}

func TestEmitThoughtStream_Truncation(t *testing.T) {
	testCases := []struct {
		name    string
		content string
	}{
		{
			name:    "Simple ASCII",
			content: "This is a simple test message.",
		},
		{
			name:    "Chinese characters",
			content: "这是一个测试消息，包含中文字符。",
		},
		{
			name:    "Mixed content",
			content: "Mixed English and 中文 content here!",
		},
		{
			name:    "Long message",
			content: strings.Repeat("测试Test", 20),
		},
		{
			name:    "Message ending with Chinese",
			content: "Message ending with 中",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var receivedContent strings.Builder
			var mu sync.Mutex
			var wg sync.WaitGroup
			wg.Add(1)

			emitter := NewEmitter("test-id", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
				if e.Type == schema.EVENT_TYPE_STREAM && e.IsStream {
					mu.Lock()
					receivedContent.Write(e.StreamDelta)
					mu.Unlock()
				}
				if e.Type == schema.EVENT_TYPE_STRUCTURED && e.NodeId == "stream-finished" {
					wg.Done()
				}
				return e, nil
			})

			emitter.EmitThoughtStream("task-1", tc.content)

			// Wait for stream to finish with timeout
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				// Success
			case <-time.After(2 * time.Second):
				t.Fatal("Test timed out waiting for stream to finish")
			}

			// Wait for all stream processing
			emitter.WaitForStream()

			mu.Lock()
			received := receivedContent.String()
			mu.Unlock()

			if received != tc.content {
				t.Errorf("Content truncation detected!\nExpected: %q (%d bytes)\nReceived: %q (%d bytes)\nMissing: %q",
					tc.content, len(tc.content),
					received, len(received),
					tc.content[len(received):])
			}
		})
	}
}

func TestTypeWriterCopyWithUTF8Reader(t *testing.T) {
	testCases := []struct {
		name    string
		content string
	}{
		{
			name:    "ASCII only",
			content: "Hello World!",
		},
		{
			name:    "Chinese characters",
			content: "你好世界",
		},
		{
			name:    "Mixed",
			content: "Hello 世界!",
		},
		{
			name:    "Ending with multi-byte",
			content: "Test 测",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the same flow as EmitTextStreamWithTaskIndex
			writer := utils.UTF8Reader(bytes.NewBufferString(tc.content))
			pr, pw := utils.NewPipe()

			var wg sync.WaitGroup
			wg.Add(1)

			// Writer goroutine
			go func() {
				defer pw.Close()
				_, _ = TypeWriterCopy(pw, writer, 500) // Increase speed for testing
				wg.Done()
			}()

			// Reader with UTF8Reader wrapper (like in EmitStreamEvent)
			reader := utils.UTF8Reader(pr)

			var result bytes.Buffer
			_, err := result.ReadFrom(reader)
			if err != nil {
				t.Fatalf("Failed to read: %v", err)
			}

			wg.Wait()

			received := result.String()
			if received != tc.content {
				t.Errorf("Content mismatch!\nExpected: %q (%d bytes)\nReceived: %q (%d bytes)",
					tc.content, len(tc.content),
					received, len(received))
			}
		})
	}
}
