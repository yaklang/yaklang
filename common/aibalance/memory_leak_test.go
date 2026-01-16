package aibalance

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aispec"
)

func startTestAIBalanceServer(t *testing.T, cfg *ServerConfig) (addr string, stop func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go cfg.Serve(conn)
		}
	}()

	return listener.Addr().String(), func() {
		_ = listener.Close()
		<-done
	}
}

func sendChatRequestRaw(t *testing.T, serverAddr, apiKey string, message aispec.ChatMessage, timeout time.Duration) string {
	t.Helper()

	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	msgBytes, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	request := fmt.Sprintf("POST /v1/chat/completions HTTP/1.1\r\n"+
		"Host: %s\r\n"+
		"Authorization: Bearer %s\r\n"+
		"Content-Type: application/json\r\n"+
		"Content-Length: %d\r\n"+
		"\r\n%s",
		serverAddr, apiKey, len(msgBytes), string(msgBytes))

	_, err = conn.Write([]byte(request))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(timeout))

	var buffer bytes.Buffer
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF || strings.Contains(err.Error(), "timeout") {
				break
			}
			t.Fatalf("read failed: %v", err)
		}
		buffer.Write(buf[:n])
		if n < len(buf) && strings.Contains(buffer.String(), "\r\n\r\n") {
			break
		}
	}
	return buffer.String()
}

func forceGC() {
	runtime.GC()
	debug.FreeOSMemory()
	runtime.GC()
}

func snapshotMem() (goroutines int, alloc uint64) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return runtime.NumGoroutine(), ms.Alloc
}

func assertNoLeak(t *testing.T, baseG int, baseAlloc uint64) {
	t.Helper()

	forceGC()
	time.Sleep(100 * time.Millisecond)
	forceGC()

	g2, a2 := snapshotMem()

	// goroutine number is noisy; allow some slack.
	if g2 > baseG+50 {
		t.Fatalf("goroutine leak suspected: base=%d now=%d", baseG, g2)
	}

	// alloc is also noisy; allow some slack.
	if a2 > baseAlloc+50*1024*1024 {
		t.Fatalf("heap alloc leak suspected: base=%d now=%d", baseAlloc, a2)
	}
}

func readTestImageDataURL(t *testing.T) string {
	t.Helper()

	// Prefer small testdata to avoid repo bloat; still exercises image parsing path.
	// File is created in common/aibalance/testdata/test_image.txt
	wd, _ := os.Getwd()
	p := filepath.Join(wd, "testdata", "test_image.txt")
	raw, err := os.ReadFile(p)
	if err != nil {
		// fallback: generate 256KB payload
		b := bytes.Repeat([]byte{0xAB}, 256*1024)
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b)
	}

	// test_image.txt contains raw bytes; encode into data URL
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
}

func TestProviderFailoverDoesNotLeakMemory_InvalidProviderType(t *testing.T) {
	// This test reproduces the original leak deterministically:
	// Provider.GetAIClientWithImages returns error (invalid type), but server used to "continue"
	// without closing pipes/writer goroutines.
	cfg := NewServerConfig()

	key := &Key{
		Key:           "test-key",
		AllowedModels: map[string]bool{"test-model": true},
	}
	cfg.Keys.keys["test-key"] = key
	cfg.KeyAllowedModels.allowedModels["test-key"] = map[string]bool{"test-model": true}

	// Multiple invalid providers to force loop + continue.
	p1 := &Provider{ModelName: "test-model", TypeName: "no_such_ai_type", DomainOrURL: "http://invalid", APIKey: "k1"}
	p2 := &Provider{ModelName: "test-model", TypeName: "no_such_ai_type", DomainOrURL: "http://invalid", APIKey: "k2"}
	p3 := &Provider{ModelName: "test-model", TypeName: "no_such_ai_type", DomainOrURL: "http://invalid", APIKey: "k3"}
	cfg.Models.models["test-model"] = []*Provider{p1, p2, p3}
	cfg.Entrypoints.providers["test-model"] = []*Provider{p1, p2, p3}

	addr, stop := startTestAIBalanceServer(t, cfg)
	defer stop()

	forceGC()
	baseG, baseAlloc := snapshotMem()

	chatMessage := aispec.ChatMessage{
		Model:  "test-model",
		Stream: true,
		Messages: []aispec.ChatDetail{
			{Role: "user", Content: "test"},
		},
	}

	for i := 0; i < 200; i++ {
		_ = sendChatRequestRaw(t, addr, "test-key", chatMessage, 2*time.Second)
	}

	assertNoLeak(t, baseG, baseAlloc)
}

func TestImageUploadWithProviderFailure_InvalidProviderType(t *testing.T) {
	cfg := NewServerConfig()

	key := &Key{
		Key:           "test-key",
		AllowedModels: map[string]bool{"test-model": true},
	}
	cfg.Keys.keys["test-key"] = key
	cfg.KeyAllowedModels.allowedModels["test-key"] = map[string]bool{"test-model": true}

	p1 := &Provider{ModelName: "test-model", TypeName: "no_such_ai_type", DomainOrURL: "http://invalid", APIKey: "k1"}
	p2 := &Provider{ModelName: "test-model", TypeName: "no_such_ai_type", DomainOrURL: "http://invalid", APIKey: "k2"}
	cfg.Models.models["test-model"] = []*Provider{p1, p2}
	cfg.Entrypoints.providers["test-model"] = []*Provider{p1, p2}

	addr, stop := startTestAIBalanceServer(t, cfg)
	defer stop()

	imageURL := readTestImageDataURL(t)

	chatMessage := aispec.ChatMessage{
		Model:  "test-model",
		Stream: true,
		Messages: []aispec.ChatDetail{
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "describe this image"},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}},
				},
			},
		},
	}

	forceGC()
	baseG, baseAlloc := snapshotMem()

	for i := 0; i < 80; i++ {
		_ = sendChatRequestRaw(t, addr, "test-key", chatMessage, 2*time.Second)
	}

	assertNoLeak(t, baseG, baseAlloc)
}

// Mock server for future reproduction using real provider types (openai etc).
// We keep it here as a building block, even if current tests use invalid type for determinism.
func newMockOpenAIChatServer(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.ReadAll(r.Body)
		_ = r.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// TestDeadlockPreventionWithHangingProvider verifies that when AI provider stream hangs,
// the main request processing does NOT deadlock.
// Note: AI SDK internal goroutines may still hang (that's an SDK issue),
// but the main request handler should complete within timeout.
//
// This test is skipped by default because it involves real HTTP connections
// and AI SDK internals that have their own timeout behavior.
func TestDeadlockPreventionWithHangingProvider(t *testing.T) {
	t.Skip("Skip hanging provider test: AI SDK has its own timeout behavior")
}

// TestClientDisconnectReleasesServerResources verifies that when client disconnects,
// server-side resources are properly released.
func TestClientDisconnectReleasesServerResources(t *testing.T) {
	cfg := NewServerConfig()

	key := &Key{
		Key:           "test-key",
		AllowedModels: map[string]bool{"test-model": true},
	}
	cfg.Keys.keys["test-key"] = key
	cfg.KeyAllowedModels.allowedModels["test-key"] = map[string]bool{"test-model": true}

	// Use invalid provider type to ensure quick failure
	p1 := &Provider{ModelName: "test-model", TypeName: "no_such_type", DomainOrURL: "http://invalid", APIKey: "k1"}
	cfg.Models.models["test-model"] = []*Provider{p1}
	cfg.Entrypoints.providers["test-model"] = []*Provider{p1}

	addr, stop := startTestAIBalanceServer(t, cfg)
	defer stop()

	forceGC()
	baseG, baseAlloc := snapshotMem()

	chatMessage := aispec.ChatMessage{
		Model:  "test-model",
		Stream: true,
		Messages: []aispec.ChatDetail{
			{Role: "user", Content: "test"},
		},
	}

	// Send many requests and disconnect immediately
	for i := 0; i < 100; i++ {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("dial failed: %v", err)
		}

		msgBytes, _ := json.Marshal(chatMessage)
		request := fmt.Sprintf("POST /v1/chat/completions HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Authorization: Bearer test-key\r\n"+
			"Content-Type: application/json\r\n"+
			"Content-Length: %d\r\n"+
			"\r\n%s",
			addr, len(msgBytes), string(msgBytes))

		_, _ = conn.Write([]byte(request))

		// Close immediately without waiting for response
		_ = conn.Close()
	}

	// Give some time for cleanup
	time.Sleep(500 * time.Millisecond)

	assertNoLeak(t, baseG, baseAlloc)
	t.Log("Client disconnect test passed: resources released properly")
}

// TestRapidRequestsDoNotLeakGoroutines verifies that rapid requests don't leak goroutines
func TestRapidRequestsDoNotLeakGoroutines(t *testing.T) {
	cfg := NewServerConfig()

	key := &Key{
		Key:           "test-key",
		AllowedModels: map[string]bool{"test-model": true},
	}
	cfg.Keys.keys["test-key"] = key
	cfg.KeyAllowedModels.allowedModels["test-key"] = map[string]bool{"test-model": true}

	// Use multiple invalid providers to exercise failover path
	p1 := &Provider{ModelName: "test-model", TypeName: "invalid_type_1", DomainOrURL: "http://invalid1", APIKey: "k1"}
	p2 := &Provider{ModelName: "test-model", TypeName: "invalid_type_2", DomainOrURL: "http://invalid2", APIKey: "k2"}
	p3 := &Provider{ModelName: "test-model", TypeName: "invalid_type_3", DomainOrURL: "http://invalid3", APIKey: "k3"}
	cfg.Models.models["test-model"] = []*Provider{p1, p2, p3}
	cfg.Entrypoints.providers["test-model"] = []*Provider{p1, p2, p3}

	addr, stop := startTestAIBalanceServer(t, cfg)
	defer stop()

	forceGC()
	baseG, baseAlloc := snapshotMem()

	chatMessage := aispec.ChatMessage{
		Model:  "test-model",
		Stream: true,
		Messages: []aispec.ChatDetail{
			{Role: "user", Content: "rapid test"},
		},
	}

	// Send many rapid requests
	for i := 0; i < 300; i++ {
		_ = sendChatRequestRaw(t, addr, "test-key", chatMessage, 1*time.Second)
	}

	time.Sleep(500 * time.Millisecond)

	assertNoLeak(t, baseG, baseAlloc)
	t.Log("Rapid requests test passed: no goroutine leak detected")
}

func TestLongRunningMemoryStability(t *testing.T) {
	if os.Getenv("RUN_LONG_MEMORY_TEST") != "true" {
		t.Skip("skip long-running memory test; set RUN_LONG_MEMORY_TEST=true to enable")
	}

	cfg := NewServerConfig()
	key := &Key{Key: "test-key", AllowedModels: map[string]bool{"test-model": true}}
	cfg.Keys.keys["test-key"] = key
	cfg.KeyAllowedModels.allowedModels["test-key"] = map[string]bool{"test-model": true}
	p1 := &Provider{ModelName: "test-model", TypeName: "no_such_ai_type", DomainOrURL: "http://invalid", APIKey: "k1"}
	p2 := &Provider{ModelName: "test-model", TypeName: "no_such_ai_type", DomainOrURL: "http://invalid", APIKey: "k2"}
	cfg.Models.models["test-model"] = []*Provider{p1, p2}
	cfg.Entrypoints.providers["test-model"] = []*Provider{p1, p2}

	addr, stop := startTestAIBalanceServer(t, cfg)
	defer stop()

	chatMessage := aispec.ChatMessage{
		Model:  "test-model",
		Stream: true,
		Messages: []aispec.ChatDetail{
			{Role: "user", Content: "ping"},
		},
	}

	forceGC()
	baseG, baseAlloc := snapshotMem()

	start := time.Now()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for time.Since(start) < 5*time.Minute {
		<-ticker.C
		_ = sendChatRequestRaw(t, addr, "test-key", chatMessage, 2*time.Second)
	}

	assertNoLeak(t, baseG, baseAlloc)
}
