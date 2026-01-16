package aibalance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bufpipe"
)

// chatJSONChunkWriter handles the streaming of chat completion responses
// It implements chunked transfer encoding for streaming responses
type chatJSONChunkWriter struct {
	notStream bool

	wg              *sync.WaitGroup
	writerClose     io.WriteCloser
	reasonBufWriter *bytes.Buffer
	outputBufWriter *bytes.Buffer
	mu              sync.Mutex
	closed          bool // Track if writer has been closed to prevent double-close

	uid     string    // Unique identifier for the chat session
	created time.Time // Timestamp when the chat session was created
	model   string    // Name of the AI model being used
}

// NewChatJSONChunkWriter creates a new chat JSON chunk writer
// writer: The underlying writer to write the chunks to
// uid: Unique identifier for the chat session
// model: Name of the AI model being used
func NewChatJSONChunkWriter(writer io.WriteCloser, uid string, model string) *chatJSONChunkWriter {
	pr, pw := bufpipe.NewPipe()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer func() {
			wg.Done()
		}()
		//io.Copy(writer, io.TeeReader(pr, os.Stdout))
		io.Copy(writer, pr)
		utils.FlushWriter(writer)
	}()
	return &chatJSONChunkWriter{
		wg:              wg,
		writerClose:     pw,
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

	w.writer.mu.Lock()
	defer w.writer.mu.Unlock()
	buf := bytes.Buffer{}
	buf.WriteString("data: ")
	buf.Write(delta)
	buf.WriteString("\n\n")
	w.writer.writerClose.Write([]byte(fmt.Sprintf("%x\r\n", buf.Len())))
	w.writer.writerClose.Write(buf.Bytes())
	w.writer.writerClose.Write([]byte("\r\n"))
	//msg := fmt.Sprintf("data: %s\r\n\r\n", string(delta))
	//chunk := fmt.Sprintf("%x\r\n%s\r\n", len(msg), msg)
	//
	//// fmt.Println(string(chunk))
	//if _, err := w.writer.writerClose.Write([]byte(chunk)); err != nil {
	//	return 0, err
	//}
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
	msg := fmt.Sprintf("data: %s\n\n", string(msgBytes))
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

func (w *chatJSONChunkWriter) Wait() {
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("wait group err: %v", err)
		}
	}()
	w.wg.Wait()
}

// Close finalizes the streaming response
// It sends the [DONE] marker and closes the underlying writer
// Safe to call multiple times - subsequent calls are no-op
func (w *chatJSONChunkWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Prevent double-close
	if w.closed {
		return nil
	}
	w.closed = true

	defer utils.FlushWriter(w.writerClose)

	if w.notStream {
		// Even for non-stream, we need to close the writer to release resources
		return w.writerClose.Close()
	}
	log.Info("start to close ChatJsonChunkWriter")

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
		// Still need to close the writer even on error
		w.writerClose.Close()
		return err
	}
	msg := fmt.Sprintf("data: %s\n\n", string(msgBytes))
	chunk := fmt.Sprintf("%x\r\n%s\r\n", len(msg), msg)

	// fmt.Println(string(chunk))
	if _, err := w.writerClose.Write([]byte(chunk)); err != nil {
		w.writerClose.Close()
		return err
	}

	// write data: [DONE]
	msg = "data: [DONE]\n\n"
	chunk = fmt.Sprintf("%x\r\n%s\r\n", len(msg), msg)
	// fmt.Println(string(chunk))
	if _, err := w.writerClose.Write([]byte(chunk)); err != nil {
		w.writerClose.Close()
		return err
	}

	// Send chunked encoding end marker
	chunk = "0\r\n\r\n"
	// fmt.Println(string(chunk))
	if _, err := w.writerClose.Write([]byte(chunk)); err != nil {
		w.writerClose.Close()
		return err
	}

	return w.writerClose.Close()
}
