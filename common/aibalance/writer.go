package aibalance

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// chatJSONChunkWriter handles the streaming of chat completion responses
// It implements chunked transfer encoding for streaming responses
type chatJSONChunkWriter struct {
	writerClose io.WriteCloser
	mu          sync.Mutex

	uid     string    // Unique identifier for the chat session
	created time.Time // Timestamp when the chat session was created
	model   string    // Name of the AI model being used
}

// NewChatJSONChunkWriter creates a new chat JSON chunk writer
// writer: The underlying writer to write the chunks to
// uid: Unique identifier for the chat session
// model: Name of the AI model being used
func NewChatJSONChunkWriter(writer io.WriteCloser, uid string, model string) *chatJSONChunkWriter {
	return &chatJSONChunkWriter{
		writerClose: writer,
		uid:         uid,
		created:     time.Now(),
		model:       model,
	}
}

// buildDelta constructs a delta message for streaming responses
// reason: Whether this is a reason message (true) or content message (false)
// content: The actual content to be sent
func (w *chatJSONChunkWriter) buildDelta(reason bool, content string) ([]byte, error) {
	outputField := "content"
	if reason {
		outputField = "reason_content"
	}
	result := map[string]any{
		"id":      "chat-ai-balance-" + w.uid,
		"object":  "chat.completion.chunk",
		"created": w.created.Unix(),
		"model":   w.model,
		"choices": []map[string]any{
			{
				"delta": map[string]any{outputField: content},
				"index": 0,
				//"finish_reason": "stop",
			},
		},
	}
	return json.Marshal(result)
}

// writerWrapper wraps the chatJSONChunkWriter to handle different types of messages
type writerWrapper struct {
	reason bool                 // Whether this is a reason writer
	writer *chatJSONChunkWriter // The underlying chat writer
}

// Write implements io.Writer interface for streaming responses
// It formats the data into chunked transfer encoding format
func (w *writerWrapper) Write(p []byte) (n int, err error) {
	delta, err := w.writer.buildDelta(w.reason, string(p))
	if err != nil {
		return 0, err
	}
	msg := fmt.Sprintf("data: %s\r\n\r\n", string(delta))
	chunk := fmt.Sprintf("%x\r\n%s\r\n", len(msg), msg)

	w.writer.mu.Lock()
	defer w.writer.mu.Unlock()

	if _, err := w.writer.writerClose.Write([]byte(chunk)); err != nil {
		return 0, err
	}

	// Return the length of the original data, not the chunk
	return len(p), nil
}

// GetOutputWriter returns a writer for content messages
func (w *chatJSONChunkWriter) GetOutputWriter() *writerWrapper {
	return &writerWrapper{
		reason: false,
		writer: w,
	}
}

// GetReasonWriter returns a writer for reason messages
func (w *chatJSONChunkWriter) GetReasonWriter() *writerWrapper {
	return &writerWrapper{
		reason: true,
		writer: w,
	}
}

// Close finalizes the streaming response
// It sends the [DONE] marker and closes the underlying writer
func (w *chatJSONChunkWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Send completion marker
	msg := "data: [DONE]\r\n\r\n"
	chunk := fmt.Sprintf("%x\r\n%s\r\n", len(msg), msg)
	if _, err := w.writerClose.Write([]byte(chunk)); err != nil {
		return err
	}

	// Send chunked encoding end marker
	if _, err := w.writerClose.Write([]byte("0\r\n\r\n")); err != nil {
		return err
	}

	return w.writerClose.Close()
}
