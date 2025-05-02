package aibalance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httputil"
	"sync"
	"time"
)

type chatJSONChunkWriter struct {
	writerClose io.WriteCloser
	mu          sync.Mutex

	uid     string
	created time.Time
	model   string
}

func NewChatJSONChunkWriter(writer io.WriteCloser, uid string, model string) *chatJSONChunkWriter {
	return &chatJSONChunkWriter{
		writerClose: httputil.NewChunkedWriter(writer),
		uid:         uid,
		created:     time.Now(),
		model:       model,
	}
}
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
				"delta":         map[string]any{outputField: content},
				"index":         0,
				"finish_reason": "stop",
			},
		},
	}
	return json.Marshal(result)
}

type writerWrapper struct {
	reason bool
	writer *chatJSONChunkWriter
}

func (w *writerWrapper) Write(p []byte) (n int, err error) {
	w.writer.mu.Lock()
	defer w.writer.mu.Unlock()

	delta, err := w.writer.buildDelta(w.reason, string(p))
	if err != nil {
		return 0, err
	}
	msg := fmt.Sprintf("data: %s\r\n\r\n", string(delta))
	return w.writer.writerClose.Write([]byte(msg))
}

func (w *chatJSONChunkWriter) GetOutputWriter() *writerWrapper {
	return &writerWrapper{
		reason: false,
		writer: w,
	}
}

func (w *chatJSONChunkWriter) GetReasonWriter() *writerWrapper {
	return &writerWrapper{
		reason: true,
		writer: w,
	}
}

func (w *chatJSONChunkWriter) Close() error {
	defer w.writerClose.Close()

	msg := "data: [DONE]\r\n\r\n"
	_, err := w.writerClose.Write([]byte(msg))
	if err != nil {
		return err
	}
	return nil
}
