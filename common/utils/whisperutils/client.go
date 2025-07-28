package whisperutils

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// LocalWhisperClient is a client for a local whisper server.
type LocalWhisperClient struct {
	baseURL string
}

// NewLocalWhisperClient creates a new client.
// It will try to connect to a server managed by a WhisperManager if available,
// otherwise it will default to localhost:8080.
func NewLocalWhisperClient(opts ...func(*LocalWhisperClient)) (*LocalWhisperClient, error) {
	// default to localhost:8080, but can be overridden
	client := &LocalWhisperClient{baseURL: "http://127.0.0.1:8080"}
	for _, opt := range opts {
		opt(client)
	}
	return client, nil
}

// WithBaseURL sets the base URL for the client.
func WithBaseURL(url string) func(*LocalWhisperClient) {
	return func(c *LocalWhisperClient) {
		c.baseURL = url
	}
}

// Transcribe sends an audio file to the whisper server for transcription.
func (c *LocalWhisperClient) Transcribe(filePath string) (*TranscriptionProcessor, error) {
	// 1. Create a buffer to store our multipart form data
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	// 2. Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	// 3. Create a new form-data header with the file field
	fw, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("error creating form file: %w", err)
	}
	if _, err = io.Copy(fw, file); err != nil {
		return nil, fmt.Errorf("error copying file to form: %w", err)
	}

	// 4. Add other form fields
	_ = writer.WriteField("response_format", "verbose_json")
	_ = writer.WriteField("task", "transcribe")

	// 5. Close the multipart writer. This is important as it writes the trailing boundary.
	writer.Close()

	// 6. Build the HTTP request using lowhttp
	url := c.baseURL + "/inference"
	req, err := http.NewRequest("POST", url, &b)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 7. Use lowhttp.HTTP to send the request
	httpOpts := []lowhttp.LowhttpOpt{
		lowhttp.WithRequest(req),
		lowhttp.WithConnectTimeout(10 * time.Second),
		lowhttp.WithTimeoutFloat(1800),
	}
	resp, err := lowhttp.HTTP(httpOpts...)
	if err != nil {
		return nil, fmt.Errorf("error sending request via lowhttp: %w", err)
	}

	// 8. Process the response
	_, body := lowhttp.SplitHTTPPacketFast(resp.RawPacket)
	processor, err := NewProcessor(body)
	if err != nil {
		return nil, fmt.Errorf("error processing transcription response: %w", err)
	}

	return processor, nil
}
