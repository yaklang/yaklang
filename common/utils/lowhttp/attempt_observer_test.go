package lowhttp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHTTPAttemptObserverRecordsRetryRedirectAndIsolation(t *testing.T) {
	var mu sync.Mutex
	var starts, ends []HTTPAttempt
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "/retry")
			w.WriteHeader(302)
			return
		}
		calls++
		if calls == 1 {
			w.WriteHeader(503)
			return
		}
		w.Write([]byte("done"))
	}))
	defer server.Close()
	observer := HTTPAttemptObserver(func(start HTTPAttempt) func(HTTPAttempt) {
		mu.Lock()
		starts = append(starts, start)
		mu.Unlock()
		return func(end HTTPAttempt) { mu.Lock(); ends = append(ends, end); mu.Unlock() }
	})
	unbind := BindHTTPAttemptObserver(WithHTTPAttemptObserver(context.Background(), observer), "tool-observed", "agent-observed")
	defer unbind()
	host := strings.TrimPrefix(server.URL, "http://")
	options := []LowhttpOpt{WithPacketBytes([]byte(fmt.Sprintf("GET /redirect HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", host))), WithRuntimeId("tool-observed"), WithSaveHTTPFlow(false), WithRetryTimes(1), WithRetryInStatusCode([]int{503}), WithRetryWaitTime(time.Millisecond), WithRetryMaxWaitTime(time.Millisecond)}
	response, err := HTTP(options...)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(response.RawPacket), "done") {
		t.Fatalf("unexpected response: %q", response.RawPacket)
	}
	mu.Lock()
	if len(starts) != 3 || len(ends) != 3 {
		t.Fatalf("got starts=%d ends=%d, expected redirect + retry + success", len(starts), len(ends))
	}
	for _, end := range ends {
		if end.ToolCallID != "tool-observed" || end.AgentID != "agent-observed" || end.Error != nil || end.FinishedAt.Before(end.StartedAt) {
			t.Fatalf("invalid source/lifecycle: %+v", end)
		}
	}
	mu.Unlock()
	options = append(options, WithRuntimeId("other-tool"))
	if _, err := HTTP(options...); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ends) != 3 {
		t.Fatal("unrelated runtime was captured")
	}
}

func TestHTTPAttemptObserverRecordsDialFailureAndCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			var terminal *HTTPAttempt
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelled {
				cancel()
			}
			unbind := BindHTTPAttemptObserver(WithHTTPAttemptObserver(context.Background(), func(start HTTPAttempt) func(HTTPAttempt) { return func(end HTTPAttempt) { terminal = &end } }), "failure-tool", "")
			defer unbind()
			_, err := HTTPWithoutRedirect(WithContext(ctx), WithRuntimeId("failure-tool"), WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: "+addr+"\r\n\r\n")), WithConnectTimeout(100*time.Millisecond), WithSaveHTTPFlow(false))
			if err == nil {
				t.Fatal("expected transport failure")
			}
			if terminal == nil || terminal.Error == nil {
				t.Fatal("failed attempt was lost because no HTTPFlow was saved")
			}
		})
	}
}

func TestHTTPAttemptObserverSeparatesAuthenticationRetry(t *testing.T) {
	var ends []HTTPAttempt
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(401)
			return
		}
		w.Write([]byte("authorized"))
	}))
	defer server.Close()
	unbind := BindHTTPAttemptObserver(WithHTTPAttemptObserver(context.Background(), func(HTTPAttempt) func(HTTPAttempt) { return func(end HTTPAttempt) { ends = append(ends, end) } }), "auth-tool", "")
	defer unbind()
	host := strings.TrimPrefix(server.URL, "http://")
	response, err := HTTPWithoutRedirect(WithConnPool(false), WithRuntimeId("auth-tool"), WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: "+host+"\r\n\r\n")), WithUsername("user"), WithPassword("password"), WithSaveHTTPFlow(false))
	if err != nil {
		t.Fatal(err)
	}
	if GetStatusCodeFromResponse(response.RawPacket) != 200 {
		t.Fatal("authentication failed")
	}
	if len(ends) != 2 || GetStatusCodeFromResponse(ends[0].Response) != 401 || GetStatusCodeFromResponse(ends[1].Response) != 200 {
		var statuses []int
		for _, end := range ends {
			statuses = append(statuses, GetStatusCodeFromResponse(end.Response))
		}
		t.Fatalf("challenge and retry were not independently captured: count=%d statuses=%v", len(ends), statuses)
	}
	if strings.Contains(string(ends[0].Request), "Authorization:") || !strings.Contains(string(ends[1].Request), "Authorization:") {
		t.Fatal("captured request did not match the actual authenticated retry")
	}
}

func TestHTTPAttemptObserverOwnsPacketSnapshotsAcrossBufferReuse(t *testing.T) {
	var started, ended HTTPAttempt
	unbind := BindHTTPAttemptObserver(WithHTTPAttemptObserver(context.Background(), func(start HTTPAttempt) func(HTTPAttempt) {
		started = start
		return func(end HTTPAttempt) { ended = end }
	}), "buffer-tool", "")
	defer unbind()
	request := []byte("GET /original HTTP/1.1\r\nHost: example.invalid\r\n\r\n")
	response := []byte("HTTP/1.1 401 Unauthorized\r\nContent-Length: 0\r\n\r\n")
	wantRequest, wantResponse := string(request), string(response)
	tr := &transportRequest{option: &LowhttpExecConfig{RuntimeId: "buffer-tool"}, packet: request}
	finish := observeHTTPAttempt(tr)
	finish(&transportResult{rawBytes: response}, nil)
	// bytes.Buffer.Reset retains its backing array. A later retry writing into
	// it must not rewrite an already completed attempt (or its start packet).
	copy(request, []byte("GET /replaced HTTP/1.1"))
	copy(response, []byte("HTTP/1.1 200 OK          "))
	if string(started.Request) != wantRequest || string(ended.Request) != wantRequest || string(ended.Response) != wantResponse {
		t.Fatalf("transport buffer reuse rewrote captured evidence: start=%q request=%q response=%q", started.Request, ended.Request, ended.Response)
	}
}

func TestHTTPAttemptObserverRetainsPooledPartialResponse(t *testing.T) {
	var terminal HTTPAttempt
	unbind := BindHTTPAttemptObserver(WithHTTPAttemptObserver(context.Background(), func(HTTPAttempt) func(HTTPAttempt) { return func(end HTTPAttempt) { terminal = end } }), "partial-tool", "")
	defer unbind()
	request := &transportRequest{option: &LowhttpExecConfig{RuntimeId: "partial-tool"}, packet: []byte("GET / HTTP/1.1\r\nHost: example.invalid\r\n\r\n")}
	finish := observeHTTPAttempt(request)
	partial := []byte("HTTP/1.1 200 OK\r\nContent-Length: 50\r\n\r\npartial")
	request.attemptResponsePacket = partial
	finish(nil, fmt.Errorf("server closed during body"))
	if string(terminal.Response) != string(partial) || terminal.Error == nil {
		t.Fatal("partial response disappeared on the pooled error path")
	}
}
