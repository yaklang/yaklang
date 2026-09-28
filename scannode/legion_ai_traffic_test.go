package scannode

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"google.golang.org/protobuf/proto"
)

type trafficTestEmitter struct {
	mu     sync.Mutex
	events [][]byte
}

func (e *trafficTestEmitter) Emit(_ string, raw []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, append([]byte(nil), raw...))
}
func (*trafficTestEmitter) Done([]byte)                   {}
func (*trafficTestEmitter) Failed(string, string, []byte) {}
func trafficTestCollector(t *testing.T, endpoint string) *aiTrafficCollector {
	t.Helper()
	return &aiTrafficCollector{binding: aiSessionBinding{Ref: aiSessionCommandRef{SessionID: "session", BindEpoch: 7}, NodeID: "node", NodeSessionID: "node-session", PlatformBearerToken: "node-secret"}, policy: &aiv1.AITrafficCapturePolicy{ProtocolVersion: 1, UploadBaseUrl: endpoint, PacketLimitBytes: 1024, SessionLimitBytes: 4096}, dir: t.TempDir(), client: &http.Client{Timeout: time.Second}, wake: make(chan struct{}, 1), stop: make(chan struct{}), accepting: true, emitter: &trafficTestEmitter{}}
}
func TestResilienceAITrafficUploadRequiresDurableMatchingReceipt(t *testing.T) {
	var attempts [][]byte
	durable := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer node-secret" || r.Header.Get("X-Node-Session-ID") != "node-session" {
			t.Error("missing node credentials")
		}
		raw, _ := io.ReadAll(r.Body)
		attempts = append(attempts, raw)
		upload := new(aiv1.AITrafficUpload)
		if err := proto.Unmarshal(raw, upload); err != nil {
			t.Error(err)
		}
		receipt := &aiv1.AITrafficReceipt{ProtocolVersion: 1, FlowId: upload.Record.FlowId, Phase: upload.Record.Phase, Durable: durable, UploadSha256: trafficSHA(raw), StorageStatus: "stored"}
		body, _ := proto.Marshal(receipt)
		w.Write(body)
	}))
	defer server.Close()
	c := trafficTestCollector(t, server.URL)
	c.enqueue(&aiv1.AITrafficUpload{Record: &aiv1.AITrafficRecord{ProtocolVersion: 1, FlowId: "flow", SessionId: "session", Phase: "terminal", Outcome: "success"}, RawRequest: []byte("secret request")})
	c.flush(context.Background())
	paths, _ := c.pending()
	if len(paths) != 1 {
		t.Fatal("unconfirmed upload was deleted")
	}
	durable = true
	c.flush(context.Background())
	paths, _ = c.pending()
	if len(paths) != 0 {
		t.Fatal("confirmed upload remained queued")
	}
	if len(attempts) != 2 || !bytes.Equal(attempts[0], attempts[1]) {
		t.Fatal("retry changed stable upload identity or packet bytes")
	}
	receipt := filepath.Join(c.dir, "flow.terminal.receipt")
	if _, err := os.Stat(receipt); err != nil {
		t.Fatal("durable receipt not persisted")
	}
	// Simulate a crash after receipt fsync, before unlinking the queue file.
	if err := os.WriteFile(filepath.Join(c.dir, "flow.terminal.pb"), attempts[0], 0600); err != nil {
		t.Fatal(err)
	}
	c.flush(context.Background())
	if len(attempts) != 2 {
		t.Fatal("restart ignored its durable receipt")
	}
	emitter := c.emitter.(*trafficTestEmitter)
	for _, event := range emitter.events {
		if bytes.Contains(event, []byte("secret request")) || bytes.Contains(event, []byte("node-secret")) {
			t.Fatal("raw evidence leaked through metadata events")
		}
	}
}
func TestAITrafficReadsOversizePacketFilesAndTruncatesBytes(t *testing.T) {
	dir := t.TempDir()
	header := []byte("HTTP/1.1 200 OK\r\nContent-Length: 8\r\n\r\n")
	body := []byte("12345678")
	hp, bp := filepath.Join(dir, "headers"), filepath.Join(dir, "body")
	os.WriteFile(hp, header, 0600)
	os.WriteFile(bp, body, 0600)
	want := append(append([]byte(nil), header...), body...)
	raw, size, truncated, err := trafficPacket([]byte("placeholder with local paths"), hp, bp, int64(len(want)-3))
	if err != nil || size != int64(len(want)) || !truncated || !bytes.Equal(raw, want[:len(want)-3]) {
		t.Fatalf("actual packet reconstruction failed: size=%d truncated=%v err=%v raw=%q", size, truncated, err, raw)
	}
}
func TestResilienceAITrafficDrainWaitsForActiveRequestsAndReceipts(t *testing.T) {
	c := trafficTestCollector(t, "http://127.0.0.1:1")
	finish := c.observe(lowhttp.HTTPAttempt{ToolCallID: "tool", AgentID: "agent", TurnID: "turn", StartedAt: time.Now(), Request: []byte("GET /?token=hidden HTTP/1.1\r\nHost: test.invalid\r\nAuthorization: hidden\r\n\r\n")})
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	result := c.drain(ctx, "drain")
	if result.Complete || result.PendingRecords < 2 {
		t.Fatalf("active/unacknowledged request reported complete: %+v", result)
	}
	finish(lowhttp.HTTPAttempt{FinishedAt: time.Now(), Error: context.Canceled})
	paths, _ := c.pending()
	if len(paths) != 2 {
		t.Fatalf("start/terminal queue lost: %d", len(paths))
	}
	for _, path := range paths {
		raw, _ := os.ReadFile(path)
		upload := new(aiv1.AITrafficUpload)
		proto.Unmarshal(raw, upload)
		if upload.Record.TurnId != "turn" || upload.Record.ToolCallId != "tool" {
			t.Fatal("lost turn/tool provenance")
		}
		if upload.Record.Phase == "terminal" && upload.Record.Outcome != "cancelled" {
			t.Fatal("cancelled request outcome lost")
		}
	}
}
func TestAITrafficSessionQuotaPreservesTerminalMetadata(t *testing.T) {
	c := trafficTestCollector(t, "http://unused.invalid")
	c.used = c.policy.SessionLimitBytes
	c.enqueue(&aiv1.AITrafficUpload{Record: &aiv1.AITrafficRecord{FlowId: "flow", Phase: "terminal", Outcome: "success"}, RawRequest: []byte("too much")})
	paths, _ := c.pending()
	raw, _ := os.ReadFile(paths[0])
	upload := new(aiv1.AITrafficUpload)
	proto.Unmarshal(raw, upload)
	if len(upload.RawRequest) != 0 || upload.Record.CaptureError != "local_session_limit" || upload.Record.Outcome != "success" {
		t.Fatal("quota must preserve attempt outcome and explicitly drop evidence")
	}
}
func TestAITrafficRejectsForeignUploadOriginAndUnsupportedVersion(t *testing.T) {
	for _, version := range []uint32{1, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			binding := aiSessionBinding{Ref: aiSessionCommandRef{SessionID: "session"}, PlatformAPIBaseURL: "https://platform.invalid", PlatformBearerToken: "secret", NodeSessionID: "node", TrafficCapture: &aiv1.AITrafficCapturePolicy{ProtocolVersion: version, UploadBaseUrl: "https://foreign.invalid"}}
			if _, err := newAITrafficCollector(binding, nil, t.TempDir()); err == nil {
				t.Fatal("unsafe upload policy accepted")
			}
		})
	}
}
func TestAITrafficMetadataExcludesSensitiveURLAndHeaders(t *testing.T) {
	c := trafficTestCollector(t, "http://unused.invalid")
	finish := c.observe(lowhttp.HTTPAttempt{StartedAt: time.Now(), Request: []byte("GET /private?token=SECRET HTTP/1.1\r\nHost: example.invalid\r\nAuthorization: SECRET\r\nCookie: SECRET\r\n\r\n")})
	finish(lowhttp.HTTPAttempt{FinishedAt: time.Now()})
	paths, _ := c.pending()
	for _, path := range paths {
		raw, _ := os.ReadFile(path)
		upload := new(aiv1.AITrafficUpload)
		proto.Unmarshal(raw, upload)
		metadata := fmt.Sprint(upload.Record)
		if strings.Contains(metadata, "SECRET") || strings.Contains(metadata, "private") {
			t.Fatal("sensitive metadata escaped")
		}
	}
}

func TestResilienceAITrafficAcknowledgedStartRemainsIncompleteAfterRestart(t *testing.T) {
	c := trafficTestCollector(t, "http://unused.invalid")
	// The process died after receiving the start receipt, before persisting any
	// terminal record. An empty pending upload directory must not mean complete.
	if err := os.WriteFile(filepath.Join(c.dir, "flow.started.receipt"), []byte("durable start"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	result := c.drain(ctx, "drain-after-restart")
	if result.Complete || result.PendingRecords != 1 {
		t.Fatalf("orphaned started attempt was hidden: %+v", result)
	}
}
