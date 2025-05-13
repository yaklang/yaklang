package aibalance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"log"
	"sync"
	"time"
)

// chatJSONChunkWriter handles the streaming of chat completion responses
// It implements chunked transfer encoding for streaming responses
type chatJSONChunkWriter struct {
	notStream bool

	writerClose     io.WriteCloser
	reasonBufWriter *bytes.Buffer
	outputBufWriter *bytes.Buffer
	mu              sync.Mutex

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
		writerClose:     writer,
		reasonBufWriter: bytes.NewBuffer(nil),
		outputBufWriter: bytes.NewBuffer(nil),
		uid:             uid,
		created:         time.Now(),
		model:           model,
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

// buildMessage constructs a full message for streaming responses
// reason: Whether this is a reason message (true) or content message (false)
// content: The actual content to be sent
func (w *chatJSONChunkWriter) buildMessage(reasonContent string, content string) ([]byte, error) {
	r := map[string]any{
		"role": "assistant",
	}
	if reasonContent != "" {
		r["reason_content"] = reasonContent
	}
	r["content"] = content
	result := map[string]any{
		"id":      "chat-ai-balance-" + w.uid,
		"object":  "chat.completion.chunk",
		"created": w.created.Unix(),
		"model":   w.model,
		"choices": []map[string]any{
			{
				"message":       r,
				"index":         0,
				"finish_reason": "stop",
			},
		},
	}
	return json.Marshal(result)
}

// writerWrapper wraps the chatJSONChunkWriter to handle different types of messages
type writerWrapper struct {
	notStream bool
	buf       *bytes.Buffer

	reason bool                 // Whether this is a reason writer
	writer *chatJSONChunkWriter // The underlying chat writer
}

// Write implements io.Writer interface for streaming responses
// It formats the data into chunked transfer encoding format
func (w *writerWrapper) Write(p []byte) (n int, err error) {
	if w.notStream {
		return w.buf.Write(p)
	}

	delta, err := w.writer.buildDelta(w.reason, string(p))
	if err != nil {
		return 0, err
	}
	msg := fmt.Sprintf("data: %s\r\n\r\n", string(delta))
	chunk := fmt.Sprintf("%x\r\n%s\r\n", len(msg), msg)

	w.writer.mu.Lock()
	defer w.writer.mu.Unlock()

	// fmt.Println(string(chunk))
	if _, err := w.writer.writerClose.Write([]byte(chunk)); err != nil {
		return 0, err
	}
	utils.FlushWriter(w.writer.writerClose)

	// Return the length of the original data, not the chunk
	return len(p), nil
}

// GetOutputWriter returns a writer for content messages
func (w *chatJSONChunkWriter) GetOutputWriter() *writerWrapper {
	return &writerWrapper{
		notStream: w.notStream,
		buf:       w.outputBufWriter,
		reason:    false,
		writer:    w,
	}
}

// GetReasonWriter returns a writer for reason messages
func (w *chatJSONChunkWriter) GetReasonWriter() *writerWrapper {
	return &writerWrapper{
		notStream: w.notStream,
		buf:       w.reasonBufWriter,
		reason:    true,
		writer:    w,
	}
}

func (w *chatJSONChunkWriter) WriteError(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	defer utils.FlushWriter(w.writerClose)

	rawmsg := map[string]any{
		"error": err,
	}
	msgBytes, err := json.Marshal(rawmsg)
	if err != nil {
		log.Printf("Failed to marshal error: %v", err)
		return
	}
	msg := fmt.Sprintf("data: %s\r\n\r\n", string(msgBytes))
	chunk := fmt.Sprintf("%x\r\n%s\r\n", len(msg), msg)
	if _, err := w.writerClose.Write([]byte(chunk)); err != nil {
		log.Printf("Failed to write error: %v", err)
	}
}

func (w *chatJSONChunkWriter) GetNotStreamBody() []byte {
	msg, err := w.buildMessage(w.reasonBufWriter.String(), w.outputBufWriter.String())
	if err != nil {
		msg = []byte(utils.Errorf("w.buildMessage failed: %v", err).Error())
	}
	return msg
}

// Close finalizes the streaming response
// It sends the [DONE] marker and closes the underlying writer
func (w *chatJSONChunkWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	defer utils.FlushWriter(w.writerClose)

	if w.notStream {
		return nil
	}

	rawmsg := map[string]any{
		"id":      "chat-ai-balance-" + w.uid,
		"object":  "chat.completion.chunk",
		"created": w.created.Unix(),
		"model":   w.model,
		"choices": []map[string]any{
			{
				"index":         0,
				"finish_reason": "stop",
			},
		},
	}
	msgBytes, err := json.Marshal(rawmsg)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("data: %s\r\n\r\n", string(msgBytes))
	chunk := fmt.Sprintf("%x\r\n%s\r\n", len(msg), msg)

	// fmt.Println(string(chunk))
	if _, err := w.writerClose.Write([]byte(chunk)); err != nil {
		return err
	}

	// write data: [DONE]
	msg = "data: [DONE]\r\n\r\n"
	chunk = fmt.Sprintf("%x\r\n%s\r\n", len(msg), msg)
	// fmt.Println(string(chunk))
	if _, err := w.writerClose.Write([]byte(chunk)); err != nil {
		return err
	}

	// Send chunked encoding end marker
	chunk = "0\r\n\r\n"
	// fmt.Println(string(chunk))
	if _, err := w.writerClose.Write([]byte(chunk)); err != nil {
		return err
	}

	return w.writerClose.Close()
}
