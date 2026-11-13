//go:build !yakit_exclude

package yakgrpc

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func TestMITMDiagnosticsWriteStagesAndPeak(t *testing.T) {
	tracker := newMITMPipelineTracker("storage")
	now := time.Now().Truncate(time.Second)
	tracker.now = func() time.Time { return now }
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	require.NoError(t, err)
	tracker.requestObserved(req, false)
	tracker.requestDispatched(req, false)
	tracker.upstreamCompleted(req, true, 200)
	write := tracker.writeObserver(req)
	write(yakit.HTTPFlowWriteSubmitting, 0, nil)
	write(yakit.HTTPFlowWriteBlocked, 0, nil)
	now = now.Add(75 * time.Millisecond)

	stats := tracker.snapshot(40960, 40960, 37)
	require.EqualValues(t, 1, stats.GetPersistActive())
	require.EqualValues(t, 1, stats.GetDiagnostics().GetPersistBlocked())
	require.EqualValues(t, 75, stats.GetDiagnostics().GetOldestPersistBlockedAgeMs())
	require.EqualValues(t, 37, stats.GetDiagnostics().GetDatabaseWriteTakenUnfinished())

	peakAt := now.UnixMilli()
	write(yakit.HTTPFlowWriteAccepted, 0, nil)
	now = now.Add(30 * time.Millisecond)
	write(yakit.HTTPFlowWriteStarted, 0, nil)
	tracker.responseProcessingFinished(req)
	now = now.Add(12 * time.Millisecond)
	write(yakit.HTTPFlowWriteFinished, 42, nil)

	stats = tracker.snapshot(0, 40960, 0)
	require.Zero(t, stats.GetPersistActive())
	require.Zero(t, stats.GetDiagnostics().GetPersistBlocked())
	require.EqualValues(t, 1, stats.GetPersistedTotal())
	require.EqualValues(t, 128, stats.GetDiagnostics().GetPersistSubmitToStartP95Ms())
	require.EqualValues(t, 32, stats.GetDiagnostics().GetPersistQueueWaitP95Ms())
	require.EqualValues(t, 16, stats.GetDiagnostics().GetPersistWriteP95Ms())
	rows, err := tracker.diagnostics(&ypb.GetMITMDiagnosticsRequest{View: "range", FromUnixMs: peakAt, ToUnixMs: peakAt})
	require.NoError(t, err)
	require.Len(t, rows.GetTraces(), 1)
	detail, err := tracker.diagnostics(&ypb.GetMITMDiagnosticsRequest{View: "detail", TraceId: rows.GetTraces()[0].GetTraceId()})
	require.NoError(t, err)
	require.EqualValues(t, 42, detail.GetDetail().GetFlowId())
	var phases []string
	for _, span := range detail.GetDetail().GetSpans() {
		phases = append(phases, span.GetStage())
	}
	require.Contains(t, phases, "persist_blocked")
	require.Contains(t, phases, "persist_queued")
	require.Contains(t, phases, "persist_write")

	now = now.Add(time.Second)
	tracker.snapshot(0, 40960, 0)
	history, err := tracker.diagnostics(&ypb.GetMITMDiagnosticsRequest{View: "storage"})
	require.NoError(t, err)
	require.Len(t, history.GetStorage(), 2)
	peak, drained := history.GetStorage()[0], history.GetStorage()[1]
	require.Equal(t, peakAt, peak.GetAtUnixMs())
	require.EqualValues(t, 1, peak.GetPersistPending())
	require.EqualValues(t, 1, peak.GetPersistBlocked())
	require.EqualValues(t, 1, peak.GetRequestTotal())
	require.EqualValues(t, 1, peak.GetDispatchTotal())
	require.EqualValues(t, 1, peak.GetUpstreamCompletedTotal())
	require.EqualValues(t, 1, peak.GetPersistEnqueuedTotal())
	require.Zero(t, peak.GetPersistedTotal())
	require.Zero(t, drained.GetPersistPending())
	require.Zero(t, drained.GetPersistBlocked())
	require.EqualValues(t, 1, drained.GetPersistedTotal())

	now = now.Add(181 * time.Second)
	stats = tracker.snapshot(0, 40960, 0)
	require.Zero(t, stats.GetDiagnostics().GetPersistWriteSamples())
	_, err = tracker.diagnostics(&ypb.GetMITMDiagnosticsRequest{View: "detail", TraceId: 1})
	require.Equal(t, codes.NotFound, grpcstatus.Code(err))
}

func TestMITMDiagnosticsWriteCanFinishBeforeSubmitterWakes(t *testing.T) {
	tracker := newMITMPipelineTracker("fast-worker")
	write := tracker.writeObserver(nil)
	write(yakit.HTTPFlowWriteSubmitting, 0, nil)
	write(yakit.HTTPFlowWriteBlocked, 0, nil)
	write(yakit.HTTPFlowWriteStarted, 0, nil)
	write(yakit.HTTPFlowWriteFinished, 0, io.ErrClosedPipe)
	write(yakit.HTTPFlowWriteAccepted, 0, nil)
	stats := tracker.snapshot(0, 1, 0)
	require.Zero(t, stats.GetPersistActive())
	require.Zero(t, stats.GetDiagnostics().GetPersistBlocked())
	require.Zero(t, stats.GetDiagnostics().GetPersistQueueWaitSamples())
	require.EqualValues(t, 1, stats.GetPersistFailedTotal())
	tracker.close()
	write(yakit.HTTPFlowWriteFinished, 42, nil)
	require.Nil(t, tracker.snapshot(0, 1, 0))
}

func TestMITMDiagnosticsBoundsHistoryAndIgnoresLateAttemptEvents(t *testing.T) {
	tracker := newMITMPipelineTracker("trace-samples")
	for range 2 * mitmTraceSampleLimit {
		req, err := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
		require.NoError(t, err)
		tracker.requestObserved(req, false)
		tracker.requestDispatched(req, false)
		observer := httpctx.GetUpstreamObserver(req)
		old := observer.BeginAttempt("HTTP/2")
		old.Phase(httpctx.UpstreamDial)
		next := observer.BeginAttempt("HTTP/1.1")
		next.Phase(httpctx.UpstreamSend)
		old.Phase(httpctx.UpstreamRead)
		next.Phase(httpctx.UpstreamFirstByte)
		observer.Notify(httpctx.UpstreamEvent{Phase: httpctx.UpstreamFailed, Err: context.DeadlineExceeded})
		tracker.responseProcessingFinished(req)
	}
	response, err := tracker.diagnostics(&ypb.GetMITMDiagnosticsRequest{View: "failed", Limit: 20, Offset: 20})
	require.NoError(t, err)
	require.EqualValues(t, mitmTraceSampleLimit, response.GetTotal())
	require.Len(t, response.GetTraces(), 20)
	row := response.GetTraces()[0]
	require.Equal(t, "first_byte", row.GetStage())
	require.EqualValues(t, 2, row.GetAttempts())
	require.Equal(t, "timeout", row.GetErrorCode())
	require.Empty(t, row.GetSpans())

	detail, err := tracker.diagnostics(&ypb.GetMITMDiagnosticsRequest{View: "detail", TraceId: row.GetTraceId()})
	require.NoError(t, err)
	require.NotEmpty(t, detail.GetDetail().GetSpans())
	for _, span := range detail.GetDetail().GetSpans() {
		require.NotEqual(t, "read", span.GetStage())
	}
}

func TestMITMDiagnosticsBrowserScopeBeforePagination(t *testing.T) {
	tracker := newMITMPipelineTracker("browser-scope")
	t.Cleanup(tracker.close)
	now := time.Now()
	tracker.now = func() time.Time { return now }
	dial := func(method, host string) {
		req, err := http.NewRequest(method, "https://"+host+"/", nil)
		require.NoError(t, err)
		tracker.requestObserved(req, false)
		tracker.requestDispatched(req, false)
		httpctx.GetUpstreamObserver(req).BeginAttempt("HTTP/1.1").Phase(httpctx.UpstreamDial)
	}
	for range 24 {
		dial(http.MethodGet, "CONTENT-AUTOFILL.googleapis.com.:443")
	}
	dial(http.MethodPost, "content-autofill.googleapis.com")
	now = now.Add(4 * time.Second)
	dial(http.MethodGet, "content-autofill.googleapis.com.example.invalid")
	dial(http.MethodGet, "other.googleapis.com")
	dial(http.MethodPut, "content-autofill.googleapis.com")
	now = now.Add(3 * time.Second)

	stats := tracker.snapshot(0, 40960, 0)
	require.EqualValues(t, 4, stats.GetVersion())
	require.EqualValues(t, 28, stats.GetActiveTotal())
	require.Len(t, stats.GetDiagnostics().GetHTTPStages(), 1)
	stage := stats.GetDiagnostics().GetHTTPStages()[0]
	require.Equal(t, "dial", stage.GetStage())
	require.EqualValues(t, 28, stage.GetCount())
	require.EqualValues(t, 25, stage.GetBrowserBackgroundCount())
	require.EqualValues(t, 7000, stage.GetOldestAgeMs())
	require.EqualValues(t, 7000, stage.GetOldestBrowserBackgroundAgeMs())
	require.EqualValues(t, 3000, stage.GetOldestOtherAgeMs())

	query := &ypb.GetMITMDiagnosticsRequest{View: "active", Limit: 2}
	for _, tc := range []struct {
		scope, service string
		total          int32
	}{
		{scope: "", service: "chrome_autofill", total: 28},
		{scope: "capture", service: "chrome_autofill", total: 28},
		{scope: "focus", total: 3},
		{scope: "browser", service: "chrome_autofill", total: 25},
	} {
		query.TrafficScope = tc.scope
		rows, err := tracker.diagnostics(query)
		require.NoError(t, err)
		require.Equal(t, tc.total, rows.GetTotal())
		require.EqualValues(t, 25, rows.GetBrowserBackgroundTotal())
		require.Len(t, rows.GetTraces(), 2)
		for _, row := range rows.GetTraces() {
			require.Equal(t, tc.service, row.GetBrowserService())
		}
	}
}

func TestMITMDiagnosticsCaptureScopeKeepsSamplesUnderFilteredLoad(t *testing.T) {
	tracker := newMITMPipelineTracker("capture-scope")
	t.Cleanup(tracker.close)
	now := time.Now()
	tracker.now = func() time.Time { return now }
	waiting := func(host string, decision mitmFilterDecision, phase httpctx.UpstreamPhase) *http.Request {
		req, err := http.NewRequest(http.MethodGet, "http://"+host+"/", nil)
		require.NoError(t, err)
		tracker.requestObserved(req, false)
		if decision.excluded() {
			tracker.observeFilterExclusion(req, decision)
		}
		tracker.requestDispatched(req, false)
		httpctx.GetUpstreamObserver(req).BeginAttempt("HTTP/1.1").Phase(phase)
		return req
	}
	ordinary := waiting("target.invalid", mitmFilterPending, httpctx.UpstreamPoolWait)
	now = now.Add(3 * time.Second)
	tracker.upstreamCompleted(ordinary, true, http.StatusOK)
	tracker.responseMirrored(ordinary, mitmFilterPassed)
	tracker.responseProcessingFinished(ordinary)
	excluded := make([]*http.Request, 2*mitmTraceSampleLimit)
	for i := range excluded {
		excluded[i] = waiting("excluded.invalid", mitmFilterRequestExcluded, httpctx.UpstreamDial)
	}
	now = now.Add(3 * time.Second)
	for _, req := range excluded {
		tracker.upstreamCompleted(req, true, http.StatusOK)
		tracker.responseMirrored(req, mitmFilterRequestExcluded)
		tracker.responseProcessingFinished(req)
	}
	retained, err := tracker.diagnostics(&ypb.GetMITMDiagnosticsRequest{View: "slow", TrafficScope: "capture"})
	require.NoError(t, err)
	require.Len(t, retained.GetTraces(), 1)
	require.Equal(t, "passed", retained.GetTraces()[0].GetFilterDecision())
	all, err := tracker.diagnostics(&ypb.GetMITMDiagnosticsRequest{View: "slow", TrafficScope: "all"})
	require.NoError(t, err)
	require.EqualValues(t, 1+mitmTraceSampleLimit, all.GetTotal())

	for range 100 {
		waiting("excluded.invalid", mitmFilterRequestExcluded, httpctx.UpstreamPoolWait)
	}
	now = now.Add(3 * time.Second)
	waiting("target.invalid", mitmFilterPending, httpctx.UpstreamPoolWait)
	waiting("different-stage.invalid", mitmFilterPending, httpctx.UpstreamDial)
	captured, err := tracker.diagnostics(&ypb.GetMITMDiagnosticsRequest{
		View: "active", Stage: "pool_wait", TrafficScope: "capture", Limit: 1,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, captured.GetTotal())
	require.Equal(t, "target.invalid", captured.GetTraces()[0].GetHost())
	require.Equal(t, "pending", captured.GetTraces()[0].GetFilterDecision())
	all, err = tracker.diagnostics(&ypb.GetMITMDiagnosticsRequest{
		View: "active", Stage: "pool_wait", TrafficScope: "all", Limit: 2,
	})
	require.NoError(t, err)
	require.EqualValues(t, 101, all.GetTotal())
	require.Len(t, all.GetTraces(), 2)
	require.Equal(t, "request_excluded", all.GetTraces()[0].GetFilterDecision())
	_, err = tracker.diagnostics(&ypb.GetMITMDiagnosticsRequest{
		View: "detail", TraceId: all.GetTraces()[0].GetTraceId(), TrafficScope: "capture",
	})
	require.Equal(t, codes.NotFound, grpcstatus.Code(err))
	stats := tracker.snapshot(0, 40960, 0)
	require.EqualValues(t, 102, stats.GetActiveTotal())
	stage := stats.GetDiagnostics().GetHTTPStages()[0]
	require.Equal(t, "pool_wait", stage.GetStage())
	require.EqualValues(t, 101, stage.GetCount())
	require.EqualValues(t, 100, stage.GetFilteredCount())
	require.EqualValues(t, 1, stage.GetInScopeOtherCount())
	require.EqualValues(t, 0, stage.GetOldestInScopeOtherAgeMs())
}

func TestGRPCMUSTPASS_MITMV2_DiagnosticsHTTPAndWebSocket(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, server, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)
	filter := GetMITMFilterManager(server.GetProjectDatabase(), server.GetProfileDatabase())
	filter.Update(&ypb.MITMFilterData{
		ExcludeUri:            []*ypb.FilterDataItem{{MatcherType: "word", Group: []string{"/filtered-request"}}},
		ExcludeMIME:           []*ypb.FilterDataItem{{MatcherType: "word", Group: []string{"image/png"}}},
		FilterBundledStaticJS: true,
	})
	filter.Save()
	streamCtx, stop := context.WithCancel(ctx)
	defer stop()
	stream, err := client.MITMV2(streamCtx)
	require.NoError(t, err)
	port := utils.GetRandomAvailableTCPPort()
	require.NoError(t, stream.Send(&ypb.MITMV2Request{
		Host: "127.0.0.1", Port: uint32(port), SetAutoForward: true, AutoForwardValue: true, DisableSystemProxy: true,
	}))
	require.True(t, waitMITMV2Started(stream, 10*time.Second))
	frames := make(chan *ypb.MITMPipelineStats, 16)
	filterApplied := make(chan struct{}, 1)
	go func() {
		defer close(frames)
		for {
			frame, err := stream.Recv()
			if err != nil {
				return
			}
			if frame.GetGetCurrentHook() {
				filterApplied <- struct{}{}
			}
			if stats := frame.GetPipelineStats(); stats != nil {
				select {
				case frames <- stats:
				case <-streamCtx.Done():
					return
				}
			}
		}
	}()
	waitStats := func(match func(*ypb.MITMPipelineStats) bool) *ypb.MITMPipelineStats {
		t.Helper()
		timer := time.NewTimer(6 * time.Second)
		defer timer.Stop()
		for {
			select {
			case frame, ok := <-frames:
				require.True(t, ok, "MITM stream ended")
				if match(frame) {
					return frame
				}
			case <-timer.C:
				t.Fatal("timed out waiting for MITM diagnostics snapshot")
			}
		}
	}
	session := waitStats(func(stats *ypb.MITMPipelineStats) bool { return stats.GetDiagnostics() != nil }).GetSessionId()

	headers, body := make(chan struct{}), make(chan struct{})
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/filtered-request":
			w.WriteHeader(http.StatusTeapot)
			return
		case "/filtered-mime":
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusTeapot)
			return
		case "/_next/static/app.js":
			w.Header().Set("Content-Type", "application/javascript")
			w.WriteHeader(http.StatusTeapot)
			return
		}
		select {
		case <-headers:
		case <-req.Context().Done():
			return
		}
		w.Header().Set("Content-Length", "2")
		w.(http.Flusher).Flush()
		select {
		case <-body:
			_, _ = io.WriteString(w, "ok")
		case <-req.Context().Done():
		}
	}))
	defer origin.Close()
	defer cancel()
	proxyAddr := utils.HostPort("127.0.0.1", port)
	proxyURL := &url.URL{Scheme: "http", Host: proxyAddr}
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	defer transport.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin.URL+"/slow", nil)
	require.NoError(t, err)
	requestDone := make(chan error, 1)
	go func() {
		resp, err := (&http.Client{Transport: transport}).Do(req)
		if err == nil {
			defer resp.Body.Close()
			_, err = io.Copy(io.Discard, resp.Body)
		}
		requestDone <- err
	}()
	waitForStage := func(stage string, oldest int64) {
		waitStats(func(stats *ypb.MITMPipelineStats) bool {
			for _, item := range stats.GetDiagnostics().GetHTTPStages() {
				if item.GetStage() == stage && item.GetCount() == 1 && item.GetOldestAgeMs() >= oldest {
					return true
				}
			}
			return false
		})
	}
	waitForStage("first_byte", 2000)
	active, err := client.GetMITMDiagnostics(ctx, &ypb.GetMITMDiagnosticsRequest{
		SessionId: session, View: "active", TrafficScope: "capture", Stage: "first_byte",
	})
	require.NoError(t, err)
	require.Len(t, active.GetTraces(), 1)
	require.Equal(t, "first_byte", active.GetTraces()[0].GetStage())
	require.Equal(t, "pending", active.GetTraces()[0].GetFilterDecision())
	require.NotEmpty(t, active.GetTraces()[0].GetConnectionId())
	close(headers)
	waitForStage("read", 0)
	close(body)
	require.NoError(t, <-requestDone)
	waitStats(func(stats *ypb.MITMPipelineStats) bool { return stats.GetPersistedTotal() == 1 })
	slow, err := client.GetMITMDiagnostics(ctx, &ypb.GetMITMDiagnosticsRequest{SessionId: session, View: "slow"})
	require.NoError(t, err)
	require.Len(t, slow.GetTraces(), 1)
	require.NotZero(t, slow.GetTraces()[0].GetFlowId())
	require.Equal(t, "passed", slow.GetTraces()[0].GetFilterDecision())

	for _, tc := range []struct{ path, decision string }{
		{"/filtered-request", "request_excluded"},
		{"/filtered-mime", "response_excluded"},
		{"/_next/static/app.js", "static_js_excluded"},
	} {
		resp, err := (&http.Client{Transport: transport}).Get(origin.URL + tc.path)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusTeapot, resp.StatusCode)
		require.Eventually(t, func() bool {
			failed, err := client.GetMITMDiagnostics(ctx, &ypb.GetMITMDiagnosticsRequest{
				SessionId: session, View: "failed", TrafficScope: "all",
			})
			if err != nil {
				return false
			}
			for _, tr := range failed.GetTraces() {
				if tr.GetFilterDecision() == tc.decision {
					return tr.GetFlowId() == 0
				}
			}
			return false
		}, 3*time.Second, 20*time.Millisecond, "missing trace decision for %s", tc.path)
	}
	failed, err := client.GetMITMDiagnostics(ctx, &ypb.GetMITMDiagnosticsRequest{
		SessionId: session, View: "failed", TrafficScope: "capture",
	})
	require.NoError(t, err)
	require.Empty(t, failed.GetTraces())
	require.EqualValues(t, 1, waitStats(func(s *ypb.MITMPipelineStats) bool {
		return s.GetResponseMirroredTotal() == 4
	}).GetPersistEnqueuedTotal())

	upgraded := make(chan net.Conn, 1)
	handshake := make(chan struct{})
	releaseHandshake := sync.OnceFunc(func() { close(handshake) })
	wsOrigin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/filtered-socket" {
			<-handshake
		}
		conn, writer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		_, _ = writer.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n\r\n")
		_ = writer.Flush()
		upgraded <- conn
	}))
	defer wsOrigin.Close()
	defer releaseHandshake()
	wsAddr := wsOrigin.Listener.Addr().String()
	sendHandshake := func(path, connectionHeader, upgradeHeader string) net.Conn {
		t.Helper()
		browser, err := net.DialTimeout("tcp", proxyAddr, time.Second)
		require.NoError(t, err)
		t.Cleanup(func() { _ = browser.Close() })
		require.NoError(t, browser.SetReadDeadline(time.Now().Add(5*time.Second)))
		_, err = fmt.Fprintf(browser, "GET http://%[1]s%[2]s HTTP/1.1\r\nHost: %[1]s\r\n%[3]s: Upgrade\r\n%[4]s: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n", wsAddr, path, connectionHeader, upgradeHeader)
		require.NoError(t, err)
		return browser
	}
	readUpgrade := func(browser net.Conn) net.Conn {
		t.Helper()
		resp, err := http.ReadResponse(bufio.NewReader(browser), nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
		select {
		case wsConn := <-upgraded:
			t.Cleanup(func() { _ = wsConn.Close() })
			return wsConn
		case <-ctx.Done():
			t.Fatal("WebSocket origin did not upgrade")
			return nil
		}
	}
	browser := sendHandshake("/socket", "Connection", "Upgrade")
	originSocket := readUpgrade(browser)
	stats := waitStats(func(s *ypb.MITMPipelineStats) bool { return s.GetDiagnostics().GetWebSocketOnline() == 1 })
	require.Zero(t, stats.GetActiveTotal())
	require.Zero(t, stats.GetUpstreamActive())
	online, err := client.GetMITMDiagnostics(ctx, &ypb.GetMITMDiagnosticsRequest{SessionId: session, View: "websocket"})
	require.NoError(t, err)
	require.Len(t, online.GetTraces(), 1)
	require.Equal(t, "WebSocket", online.GetTraces()[0].GetProtocol())
	require.Equal(t, "websocket_online", online.GetTraces()[0].GetStage())
	require.Equal(t, "passed", online.GetTraces()[0].GetFilterDecision())

	require.NoError(t, browser.Close())
	require.NoError(t, originSocket.Close())
	waitStats(func(s *ypb.MITMPipelineStats) bool { return s.GetDiagnostics().GetWebSocketOnline() == 0 })

	require.NoError(t, stream.Send(&ypb.MITMV2Request{UpdateFilterWebsocket: true, FilterWebsocket: true}))
	// The subsequent response confirms both stream messages have been processed.
	require.NoError(t, stream.Send(&ypb.MITMV2Request{GetCurrentHook: true}))
	select {
	case <-filterApplied:
	case <-ctx.Done():
		t.Fatal("MITM did not process the WebSocket filter update")
	}
	browser = sendHandshake("/filtered-socket", "connection", "uPgRaDe")
	handshaking := waitStats(func(s *ypb.MITMPipelineStats) bool { return s.GetOldestUpstreamAgeMs() >= 2000 })
	require.EqualValues(t, 1, handshaking.GetDiagnostics().GetWebSocketHandshaking())
	require.Zero(t, handshaking.GetDiagnostics().GetInScopeWebSocketHandshaking())
	excluded, err := client.GetMITMDiagnostics(ctx, &ypb.GetMITMDiagnosticsRequest{
		SessionId: session, View: "active", TrafficScope: "capture",
	})
	require.NoError(t, err)
	require.Empty(t, excluded.GetTraces())
	all, err := client.GetMITMDiagnostics(ctx, &ypb.GetMITMDiagnosticsRequest{
		SessionId: session, View: "active", TrafficScope: "all",
	})
	require.NoError(t, err)
	require.Len(t, all.GetTraces(), 1)
	require.Equal(t, "websocket_excluded", all.GetTraces()[0].GetFilterDecision())
	releaseHandshake()
	originSocket = readUpgrade(browser)
	waitStats(func(s *ypb.MITMPipelineStats) bool { return s.GetDiagnostics().GetWebSocketOnline() == 1 })
	excluded, err = client.GetMITMDiagnostics(ctx, &ypb.GetMITMDiagnosticsRequest{
		SessionId: session, View: "websocket", TrafficScope: "capture",
	})
	require.NoError(t, err)
	require.Empty(t, excluded.GetTraces())
	excluded, err = client.GetMITMDiagnostics(ctx, &ypb.GetMITMDiagnosticsRequest{
		SessionId: session, View: "websocket", TrafficScope: "all",
	})
	require.NoError(t, err)
	require.Len(t, excluded.GetTraces(), 1)
	require.Equal(t, "websocket_excluded", excluded.GetTraces()[0].GetFilterDecision())
	require.Zero(t, excluded.GetTraces()[0].GetFlowId())
	require.NoError(t, browser.Close())
	require.NoError(t, originSocket.Close())
	stop()
	require.Eventually(t, func() bool {
		_, err := client.GetMITMDiagnostics(ctx, &ypb.GetMITMDiagnosticsRequest{SessionId: session, View: "storage"})
		return grpcstatus.Code(err) == codes.NotFound
	}, 3*time.Second, 25*time.Millisecond)
}

func BenchmarkMITMDiagnosticsSnapshot2685(b *testing.B) {
	tracker := newMITMPipelineTracker("benchmark")
	for range 2685 {
		req, _ := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
		tracker.requestObserved(req, false)
		tracker.requestDispatched(req, false)
		httpctx.GetUpstreamObserver(req).BeginAttempt("HTTP/1.1").Phase(httpctx.UpstreamSend)
	}
	b.ResetTimer()
	for range b.N {
		tracker.snapshot(0, 40960, 0)
	}
}

func BenchmarkMITMDiagnosticsLifecycle(b *testing.B) {
	tracker := newMITMPipelineTracker("benchmark")
	req, _ := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	b.ResetTimer()
	for range b.N {
		tracker.requestObserved(req, false)
		tracker.requestDispatched(req, false)
		attempt := httpctx.GetUpstreamObserver(req).BeginAttempt("HTTP/1.1")
		attempt.Phase(httpctx.UpstreamDial)
		attempt.Phase(httpctx.UpstreamSend)
		attempt.Phase(httpctx.UpstreamFirstByte)
		attempt.Phase(httpctx.UpstreamRead)
		tracker.upstreamCompleted(req, true, 200)
		write := tracker.writeObserver(req)
		write(yakit.HTTPFlowWriteSubmitting, 0, nil)
		write(yakit.HTTPFlowWriteAccepted, 0, nil)
		tracker.responseProcessingFinished(req)
		write(yakit.HTTPFlowWriteStarted, 0, nil)
		write(yakit.HTTPFlowWriteFinished, 42, nil)
	}
}
