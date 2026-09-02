package yakcmds

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const (
	memfitProtocolVersion = 1
	memfitFramePrefix     = "@yak-memfit/1 "
	memfitMaxFrameSize    = 64 << 20
)

var errMemfitNotProtocolFrame = errors.New("not a memfit protocol frame")

// memfitEnvelope is the only value written to the worker's stdout. Keeping the
// transport versioned and prefixed makes the parent resilient to accidental
// stdout writes from dependencies during process initialization.
type memfitEnvelope struct {
	Version int             `json:"v"`
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type memfitStartConfig struct {
	AIType        string `json:"ai_type,omitempty"`
	APIKey        string `json:"api_key,omitempty"`
	Model         string `json:"model,omitempty"`
	BaseURL       string `json:"base_url,omitempty"`
	Workdir       string `json:"workdir,omitempty"`
	Language      string `json:"language,omitempty"`
	ReviewPolicy  string `json:"review_policy"`
	MaxIterations int    `json:"max_iterations,omitempty"`
	Timeout       int    `json:"timeout_seconds,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	Debug         bool   `json:"debug,omitempty"`
}

type memfitInput struct {
	Text string `json:"text"`
}

type memfitReviewUpdate struct {
	Policy string `json:"policy"`
}

// memfitWorkerEvent deliberately mirrors only presentation-safe AiOutputEvent
// fields. Database internals and provider configuration never cross the
// process boundary.
type memfitWorkerEvent struct {
	Type          string `json:"type"`
	NodeID        string `json:"node_id,omitempty"`
	TaskID        string `json:"task_id,omitempty"`
	CallToolID    string `json:"call_tool_id,omitempty"`
	IsSystem      bool   `json:"is_system,omitempty"`
	IsStream      bool   `json:"is_stream,omitempty"`
	IsReason      bool   `json:"is_reason,omitempty"`
	IsJSON        bool   `json:"is_json,omitempty"`
	StreamDelta   string `json:"stream_delta,omitempty"`
	Content       string `json:"content,omitempty"`
	AIService     string `json:"ai_service,omitempty"`
	AIModel       string `json:"ai_model,omitempty"`
	ModelVerbose  string `json:"model_verbose,omitempty"`
	ContentType   string `json:"content_type,omitempty"`
	VizSource     string `json:"viz_source,omitempty"`
	Timestamp     int64  `json:"timestamp,omitempty"`
	DisableMarkup bool   `json:"disable_markdown,omitempty"`
}

type memfitStatus struct {
	Message string `json:"message,omitempty"`
	TaskID  string `json:"task_id,omitempty"`
	Status  string `json:"status,omitempty"`
}

type memfitProtocolWriter struct {
	mu sync.Mutex
	w  *bufio.Writer
}

func newMemfitProtocolWriter(w io.Writer) *memfitProtocolWriter {
	return &memfitProtocolWriter{w: bufio.NewWriter(w)}
}

func (w *memfitProtocolWriter) send(typ, id string, payload any) error {
	var data json.RawMessage
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal memfit %s payload: %w", typ, err)
		}
		data = raw
	}
	envelope := memfitEnvelope{
		Version: memfitProtocolVersion,
		Type:    typ,
		ID:      id,
		Data:    data,
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal memfit envelope: %w", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.w.WriteString(memfitFramePrefix); err != nil {
		return err
	}
	if _, err := w.w.Write(raw); err != nil {
		return err
	}
	if err := w.w.WriteByte('\n'); err != nil {
		return err
	}
	return w.w.Flush()
}

func decodeMemfitEnvelope(line []byte) (memfitEnvelope, error) {
	line = bytes.TrimSpace(line)
	if !bytes.HasPrefix(line, []byte(memfitFramePrefix)) {
		return memfitEnvelope{}, errMemfitNotProtocolFrame
	}
	line = bytes.TrimSpace(bytes.TrimPrefix(line, []byte(memfitFramePrefix)))
	var envelope memfitEnvelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		return memfitEnvelope{}, fmt.Errorf("decode memfit frame: %w", err)
	}
	if envelope.Version != memfitProtocolVersion {
		return memfitEnvelope{}, fmt.Errorf("unsupported memfit protocol version %d", envelope.Version)
	}
	if strings.TrimSpace(envelope.Type) == "" {
		return memfitEnvelope{}, errors.New("memfit frame has no type")
	}
	return envelope, nil
}

func decodeMemfitPayload[T any](envelope memfitEnvelope) (T, error) {
	var result T
	if len(envelope.Data) == 0 {
		return result, nil
	}
	if err := json.Unmarshal(envelope.Data, &result); err != nil {
		return result, fmt.Errorf("decode memfit %s payload: %w", envelope.Type, err)
	}
	return result, nil
}

func newMemfitFrameScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), memfitMaxFrameSize)
	return scanner
}

func validateMemfitStartConfig(config memfitStartConfig) error {
	switch config.ReviewPolicy {
	case "yolo", "ai", "manual":
	default:
		return fmt.Errorf("invalid review policy %q (want yolo, ai, or manual)", config.ReviewPolicy)
	}
	if config.MaxIterations < 0 {
		return errors.New("max iterations cannot be negative")
	}
	if config.Timeout < 0 {
		return errors.New("timeout cannot be negative")
	}
	return nil
}

func redactMemfitSecret(input, secret string) string {
	if secret == "" || input == "" {
		return input
	}
	return strings.ReplaceAll(input, secret, "[REDACTED]")
}

func memfitNowMillis() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}
