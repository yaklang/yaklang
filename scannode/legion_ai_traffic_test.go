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
	"google.golang.org/protobuf/encoding/protojson"
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

func trafficReceiptTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		upload := new(aiv1.AITrafficUpload)
		if err = proto.Unmarshal(raw, upload); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		receipt := &aiv1.AITrafficReceipt{ProtocolVersion: 1, FlowId: upload.Record.FlowId, Phase: upload.Record.Phase, Durable: true, UploadSha256: trafficSHA(raw), StorageStatus: "stored"}
		body, _ := proto.Marshal(receipt)
		_, _ = w.Write(body)
	}))
}

func waitForAITrafficDrainStart(t *testing.T, c *aiTrafficCollector) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		accepting := c.accepting
		c.mu.Unlock()
		if !accepting {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("terminal path did not start traffic drain")
}

func TestResilienceAITrafficAutomaticTerminalWaitsForReceipts(t *testing.T) {
	for _, kind := range []string{"done", "failed", "root_plan"} {
		t.Run(kind, func(t *testing.T) {
			bridge, fakeJS, driver := newTestAISessionBridge(t)
			if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, validAISessionBindCommand())); err != nil {
				t.Fatal(err)
			}
			driver.mu.Lock()
			emitter := driver.emitters[0].(*managedAISessionRuntimeEmitter)
			driver.mu.Unlock()
			server := trafficReceiptTestServer(t)
			defer server.Close()
			c := trafficTestCollector(t, server.URL)
			c.binding.Ref = emitter.runtime.ref
			c.emitter = emitter
			c.enqueue(&aiv1.AITrafficUpload{Record: &aiv1.AITrafficRecord{ProtocolVersion: 1, FlowId: "flow", SessionId: c.binding.Ref.SessionID, Phase: "terminal", Outcome: "success"}})
			// The worker is intentionally withheld until the pre-receipt assertions.
			defer c.flush(context.Background())
			emitter.runtime.mu.Lock()
			emitter.runtime.trafficCollector = c
			emitter.runtime.executionMode = "single_run"
			emitter.runtime.mu.Unlock()
			resetPublishedMessages(fakeJS)
			done := make(chan struct{})
			go func() {
				defer close(done)
				switch kind {
				case "done":
					emitter.DoneTurn("turn", []byte(`{"status":"done"}`))
				case "failed":
					emitter.FailTurn("turn", "runtime_failed", "failed", nil)
				case "root_plan":
					emitter.Emit("ai_runtime", []byte(`{"type":"end_plan_and_execution","content_json":{}}`))
				}
			}()
			waitForAITrafficDrainStart(t, c)
			if publishedMessageCount(fakeJS) != 0 || !hasAISessionRuntime(bridge.aiRuntime, c.binding.Ref.SessionID) {
				t.Fatal("terminal publication or runtime removal preceded the durable receipt")
			}
			c.flush(context.Background())
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("automatic terminal did not finish after receipt")
			}
			if hasAISessionRuntime(bridge.aiRuntime, c.binding.Ref.SessionID) {
				t.Fatal("runtime retained after receipt and terminal publication")
			}
			select {
			case <-c.stop:
			default:
				t.Fatal("collector worker was not stopped")
			}
			fakeJS.mu.Lock()
			defer fakeJS.mu.Unlock()
			drainIndex, terminalIndex := -1, -1
			var lastSequence uint64
			for index, msg := range fakeJS.publish {
				if msg.Subject == "legion.event."+legionEventAISessionDone || msg.Subject == "legion.event."+legionEventAISessionFailed {
					terminalIndex = index
				}
				if msg.Subject != "legion.event."+legionEventAISessionEvent {
					continue
				}
				event := new(aiv1.AISessionEvent)
				if err := proto.Unmarshal(msg.Data, event); err != nil {
					t.Fatal(err)
				}
				if event.Seq <= lastSequence {
					t.Fatal("terminal traffic events were published with decreasing sequence")
				}
				lastSequence = event.Seq
				if event.EventType == "ai_traffic_drain" {
					result := new(aiv1.AITrafficDrainResult)
					if err := protojson.Unmarshal(event.PayloadJson, result); err != nil || !result.Complete {
						t.Fatalf("missing completed coverage: %v %v", result, err)
					}
					drainIndex = index
				}
				if kind == "root_plan" && event.EventType == "ai_runtime" {
					terminalIndex = index
				}
			}
			if drainIndex < 0 || terminalIndex <= drainIndex {
				t.Fatal("coverage completion did not precede terminal publication")
			}
		})
	}
}

func TestResilienceAITrafficManagerDrainsBeforeWorkspaceCleanup(t *testing.T) {
	server := trafficReceiptTestServer(t)
	defer server.Close()
	c := trafficTestCollector(t, server.URL)
	c.enqueue(&aiv1.AITrafficUpload{Record: &aiv1.AITrafficRecord{ProtocolVersion: 1, FlowId: "flow", SessionId: "session", Phase: "terminal"}})
	defer c.flush(context.Background())
	manager := newAISessionRuntimeManager(nil)
	ref := aiSessionCommandRef{SessionID: "session", CommandID: "turn", BindEpoch: 7, OwnerUserID: "owner"}
	cleaned := make(chan struct{})
	runtime := &aiSessionRuntime{ref: ref, bindEpoch: 7, terminalCommandID: "turn", terminalKind: "auto", trafficCollector: c, codeWorkspace: &legionCodeWorkspaceRuntime{cleanup: func() error { close(cleaned); return nil }}}
	manager.sessions[ref.SessionID] = runtime
	done := make(chan error, 1)
	go func() { done <- manager.CompleteTerminal(ref, "auto") }()
	waitForAITrafficDrainStart(t, c)
	if !hasAISessionRuntime(manager, ref.SessionID) {
		t.Fatal("manager removed runtime before traffic receipt")
	}
	select {
	case <-cleaned:
		t.Fatal("workspace was cleaned before traffic receipt")
	default:
	}
	c.flush(context.Background())
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("manager cleanup deadlocked after receipt")
	}
	select {
	case <-cleaned:
	default:
		t.Fatal("workspace cleanup was skipped")
	}
}

func TestAITrafficCombinedPacketBudgetRetainsBothHeaders(t *testing.T) {
	c := trafficTestCollector(t, "http://unused.invalid")
	c.policy.PacketLimitBytes = aiTrafficDefaultPacketLimit
	c.policy.SessionLimitBytes = aiTrafficDefaultSessionLimit
	requestHeader := []byte("POST /upload HTTP/1.1\r\nHost: example.invalid\r\nContent-Type: application/octet-stream\r\n\r\n")
	responseHeader := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\n\r\n")
	request := append(append([]byte(nil), requestHeader...), bytes.Repeat([]byte("q"), (6<<20)-len(requestHeader))...)
	response := append(append([]byte(nil), responseHeader...), bytes.Repeat([]byte("s"), (6<<20)-len(responseHeader))...)
	finish := c.observe(lowhttp.HTTPAttempt{StartedAt: time.Now(), Request: request})
	finish(lowhttp.HTTPAttempt{FinishedAt: time.Now(), Request: request, Response: response})
	paths, _ := filepath.Glob(filepath.Join(c.dir, "*.terminal.pb"))
	if len(paths) != 1 {
		t.Fatalf("terminal record count=%d", len(paths))
	}
	raw, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	upload := new(aiv1.AITrafficUpload)
	if err = proto.Unmarshal(raw, upload); err != nil {
		t.Fatal(err)
	}
	if len(upload.RawRequest)+len(upload.RawResponse) != aiTrafficDefaultPacketLimit {
		t.Fatal("combined flow exceeded or wasted packet budget")
	}
	if len(raw) > aiTrafficDefaultPacketLimit+(128<<10) {
		t.Fatal("upload would exceed the platform HTTP request-body limit")
	}
	if !bytes.HasPrefix(upload.RawRequest, requestHeader) || !bytes.HasPrefix(upload.RawResponse, responseHeader) {
		t.Fatal("body allocation displaced one side's headers")
	}
	if upload.Record.RequestSizeBytes != 6<<20 || upload.Record.ResponseSizeBytes != 6<<20 || !upload.Record.RequestTruncated || !upload.Record.ResponseTruncated || upload.Record.CaptureError != "packet_limit" {
		t.Fatalf("original sizes or combined truncation reason lost: %v", upload.Record)
	}
}

func TestAITrafficCombinedBudgetExplainsOversizeHeaders(t *testing.T) {
	request := []byte("GET / HTTP/1.1\r\nHost: example.invalid\r\nX-Padding: " + strings.Repeat("r", 700) + "\r\n\r\nrequest-body")
	response := []byte("HTTP/1.1 200 OK\r\nX-Padding: " + strings.Repeat("s", 700) + "\r\n\r\nresponse-body")
	keptRequest, keptResponse, reason := trafficBoundPacketPair(request, response, 1024)
	if len(keptRequest)+len(keptResponse) > 1024 || len(keptRequest) == 0 || len(keptResponse) == 0 || reason != "packet_headers_limit" {
		t.Fatalf("oversize headers were not explicitly bounded: req=%d rsp=%d reason=%s", len(keptRequest), len(keptResponse), reason)
	}
	if !bytes.HasPrefix(keptRequest, []byte("GET / HTTP/1.1\r\n")) || !bytes.HasPrefix(keptResponse, []byte("HTTP/1.1 200 OK\r\n")) {
		t.Fatal("one side lost its first line")
	}
	if bytes.Contains(keptRequest, []byte("request-body")) || bytes.Contains(keptResponse, []byte("response-body")) {
		t.Fatal("body retained while headers did not fit")
	}
}

func TestAITrafficCombinedBudgetReallocatesUnusedBodySpace(t *testing.T) {
	request := []byte("GET / HTTP/1.1\r\nHost: example.invalid\r\n\r\n")
	response := []byte("HTTP/1.1 200 OK\r\n\r\n" + strings.Repeat("b", 4096))
	keptRequest, keptResponse, reason := trafficBoundPacketPair(request, response, 1024)
	if !bytes.Equal(request, keptRequest) || len(keptRequest)+len(keptResponse) != 1024 || reason != "packet_limit" {
		t.Fatal("unused request body quota was not allocated to response")
	}
}

func TestAITrafficConfigHonorsIncreasedLimitsAndHardCaps(t *testing.T) {
	for _, test := range []struct {
		name                                      string
		configured, defaultLimit, hardLimit, want uint64
	}{
		{"packet default", 0, aiTrafficDefaultPacketLimit, aiTrafficMaxPacketLimit, 10 << 20},
		{"larger packet", 32 << 20, aiTrafficDefaultPacketLimit, aiTrafficMaxPacketLimit, 32 << 20},
		{"packet hard cap", 128 << 20, aiTrafficDefaultPacketLimit, aiTrafficMaxPacketLimit, 64 << 20},
		{"session default", 0, aiTrafficDefaultSessionLimit, aiTrafficMaxSessionLimit, 1 << 30},
		{"larger session", 8 << 30, aiTrafficDefaultSessionLimit, aiTrafficMaxSessionLimit, 8 << 30},
		{"session hard cap", 128 << 30, aiTrafficDefaultSessionLimit, aiTrafficMaxSessionLimit, 64 << 30},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := trafficCaptureLimit(test.configured, test.defaultLimit, test.hardLimit); got != test.want {
				t.Fatalf("limit=%d want=%d", got, test.want)
			}
		})
	}
}

func TestAITrafficSingleHeaderBeyondPacketLimitIsExplicit(t *testing.T) {
	c := trafficTestCollector(t, "http://unused.invalid")
	request := []byte("GET / HTTP/1.1\r\nHost: example.invalid\r\nX-Padding: " + strings.Repeat("r", 2048) + "\r\n\r\n")
	finish := c.observe(lowhttp.HTTPAttempt{StartedAt: time.Now(), Request: request})
	finish(lowhttp.HTTPAttempt{FinishedAt: time.Now(), Request: request})
	paths, _ := filepath.Glob(filepath.Join(c.dir, "*.terminal.pb"))
	if len(paths) != 1 {
		t.Fatal("missing terminal upload")
	}
	raw, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	upload := new(aiv1.AITrafficUpload)
	if err = proto.Unmarshal(raw, upload); err != nil {
		t.Fatal(err)
	}
	if len(upload.RawRequest) != int(c.policy.PacketLimitBytes) || upload.Record.CaptureError != "packet_headers_limit" || !upload.Record.RequestTruncated {
		t.Fatal("single oversized header was mistaken for body truncation")
	}
}
