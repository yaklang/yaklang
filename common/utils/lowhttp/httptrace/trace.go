// Package httptrace records opt-in AI HTTP diagnostics without interpreting the
// provider protocol. Captures contain credentials and prompts; never enable it
// implicitly or send its contents back to the model.
package httptrace

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

type Scope struct {
	Sequence  int64  `json:"checkpoint_sequence,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	RuntimeID string `json:"runtime_id,omitempty"`
	CallID    string `json:"call_id"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Attempt   int    `json:"attempt,omitempty"`
}

type contextKey struct{}
type settings struct {
	Scope
	Enabled   bool
	Directory string
}

func config(ctx context.Context) settings {
	if ctx != nil {
		if s, ok := ctx.Value(contextKey{}).(settings); ok {
			return s
		}
	}
	return settings{}
}
func WithScope(ctx context.Context, scope Scope) context.Context {
	s := config(ctx)
	s.Scope = scope
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, s)
}
func GetScope(ctx context.Context) Scope { return config(ctx).Scope }
func Enabled(ctx context.Context) bool   { return config(ctx).Enabled }
func WithEnabled(ctx context.Context, enabled bool) context.Context {
	s := config(ctx)
	s.Enabled = enabled
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, s)
}

// WithDirectory is useful for an isolated embedded engine or test. It does not
// enable recording. The desktop exporter reads DefaultDirectory.
func WithDirectory(ctx context.Context, dir string) context.Context {
	s := config(ctx)
	s.Directory = dir
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, s)
}
func DefaultDirectory() string {
	return filepath.Join(consts.GetDefaultAISpaceDir(), ".model-network-debug")
}
func bucket(scope Scope) string {
	identity := "session:" + scope.SessionID
	if scope.SessionID == "" {
		identity = "runtime:" + scope.RuntimeID
		if scope.RuntimeID == "" {
			identity = "call:" + scope.CallID
		}
	}
	h := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(h[:])
}

type Metadata struct {
	Scope
	ID                string        `json:"id"`
	Started           time.Time     `json:"started_at"`
	Ended             *time.Time    `json:"ended_at,omitempty"`
	DurationMS        float64       `json:"duration_ms"`
	FirstByteMS       *float64      `json:"first_byte_ms,omitempty"`
	HTTPS             bool          `json:"https"`
	Status            int           `json:"status"`
	ContentType       string        `json:"content_type,omitempty"`
	RetryAfter        string        `json:"retry_after,omitempty"`
	ResponseBytes     int64         `json:"response_bytes"`
	TransportComplete bool          `json:"transport_complete"`
	Error             string        `json:"error,omitempty"`
	CaptureError      string        `json:"capture_error,omitempty"`
	FramingError      string        `json:"framing_error,omitempty"`
	Representation    string        `json:"representation"`
	AdditionalHeaders []HeaderBlock `json:"additional_headers,omitempty"`
}

type HeaderBlock struct {
	Kind    string      `json:"kind"`
	Status  int         `json:"status,omitempty"`
	Headers http.Header `json:"headers"`
}

// HTTP/2 interim responses/trailers cannot be appended to an unchunked body
// without corrupting its representation. Preserve these blocks in metadata.
func (r *Record) AdditionalHeaders(kind string, status int, header http.Header) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	if r.meta.FirstByteMS == nil {
		ms := float64(time.Since(r.meta.Started)) / float64(time.Millisecond)
		r.meta.FirstByteMS = &ms
	}
	r.meta.AdditionalHeaders = append(r.meta.AdditionalHeaders, HeaderBlock{kind, status, header.Clone()})
}

type Record struct {
	mu       sync.Mutex
	meta     Metadata
	dir      string
	response *os.File
	closed   bool
}

func Start(ctx context.Context, request []byte, https bool) *Record {
	s := config(ctx)
	if !s.Enabled {
		return nil
	}
	if s.CallID == "" {
		s.CallID = uuid.NewString()
	}
	root := s.Directory
	if root == "" {
		root = DefaultDirectory()
	}
	r := &Record{meta: Metadata{Scope: s.Scope, ID: uuid.NewString(), Started: time.Now(), HTTPS: https,
		Representation: "HTTP/1 response bytes before decompression; HTTP/2 headers reconstructed plus DATA payload (not TLS or HTTP/2 frames)"}}
	r.dir = filepath.Join(root, bucket(s.Scope), r.meta.ID)
	if err := os.MkdirAll(r.dir, 0700); err != nil {
		log.Warnf("AI network debug: cannot create capture: %v", err)
		return nil
	}
	if err := os.WriteFile(filepath.Join(r.dir, "request.http"), request, 0600); err != nil {
		r.fail(err)
	}
	var err error
	r.response, err = os.OpenFile(filepath.Join(r.dir, "response.http"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		r.fail(err)
	}
	r.save()
	return r
}

// Write is a best-effort mirror. Disk errors must never alter network reads.
// It writes incrementally so long responses are not retained in Go memory.
func (r *Record) Write(p []byte) (int, error) {
	if r == nil {
		return len(p), nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return len(p), nil
	}
	if r.meta.FirstByteMS == nil && len(p) > 0 {
		ms := float64(time.Since(r.meta.Started)) / float64(time.Millisecond)
		r.meta.FirstByteMS = &ms
	}
	if r.response != nil && r.meta.CaptureError == "" {
		n, err := r.response.Write(p)
		r.meta.ResponseBytes += int64(n)
		if err != nil {
			r.fail(err)
		}
	}
	return len(p), nil
}
func (r *Record) fail(err error) {
	if err != nil && r.meta.CaptureError == "" {
		r.meta.CaptureError = err.Error()
		log.Warnf("AI network debug capture %s: %v", r.meta.ID, err)
	}
}
func (r *Record) save() {
	b, err := json.MarshalIndent(r.meta, "", "  ")
	if err != nil {
		r.fail(err)
		return
	}
	tmp := filepath.Join(r.dir, "metadata.tmp")
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		r.fail(err)
		return
	}
	r.fail(os.Rename(tmp, filepath.Join(r.dir, "metadata.json")))
}
func (r *Record) Finish(err error, status int, contentType, retryAfter string, complete bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	now := time.Now()
	r.meta.Ended = &now
	r.meta.DurationMS = float64(now.Sub(r.meta.Started)) / float64(time.Millisecond)
	r.meta.Status, r.meta.ContentType, r.meta.RetryAfter = status, contentType, retryAfter
	r.meta.TransportComplete = complete && err == nil
	if err != nil {
		r.meta.Error = err.Error()
	}
	if r.response != nil {
		r.fail(r.response.Close())
	}
	// lowhttp deliberately tolerates some malformed/truncated HTTP packets. A
	// nil transport error alone must not label their diagnostic capture complete.
	if r.meta.CaptureError == "" {
		if framingErr := validateResponse(filepath.Join(r.dir, "response.http")); framingErr != nil {
			r.meta.FramingError = framingErr.Error()
			r.meta.TransportComplete = false
		}
	}
	r.save()
}

func validateResponse(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	for {
		response, err := http.ReadResponse(reader, nil)
		if err != nil {
			return err
		}
		_, err = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if err != nil {
			return fmt.Errorf("incomplete HTTP body: %w", err)
		}
		if response.StatusCode < 100 || response.StatusCode >= 200 {
			return nil
		}
	}
}

// RemoveSessions removes only the selected sessions' diagnostic files. Call after
// quiescing their runtimes; there is no database copy or configuration migration.
func RemoveSessions(sessions []string) error {
	for _, session := range sessions {
		if session == "" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(DefaultDirectory(), bucket(Scope{SessionID: session}))); err != nil {
			return err
		}
	}
	return nil
}
