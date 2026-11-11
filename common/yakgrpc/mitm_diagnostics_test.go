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
	now := time.Now().Truncate(time.Millisecond)
	tracker.now = func() time.Time { return now }
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
	require.NoError(t, err)
	tracker.requestObserved(req)
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
		tracker.requestObserved(req)
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

func TestGRPCMUSTPASS_MITMV2_DiagnosticsHTTPAndWebSocket(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := NewLocalClient()
	require.NoError(t, err)
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
	go func() {
		defer close(frames)
		for {
			frame, err := stream.Recv()
			if err != nil {
				return
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
	active, err := client.GetMITMDiagnostics(ctx, &ypb.GetMITMDiagnosticsRequest{SessionId: session, View: "active"})
	require.NoError(t, err)
	require.Len(t, active.GetTraces(), 1)
	require.Equal(t, "first_byte", active.GetTraces()[0].GetStage())
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

	upgraded := make(chan net.Conn, 1)
	wsOrigin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn, writer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		_, _ = writer.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n\r\n")
		_ = writer.Flush()
		upgraded <- conn
	}))
	defer wsOrigin.Close()
	wsAddr := wsOrigin.Listener.Addr().String()
	browser, err := net.DialTimeout("tcp", proxyAddr, time.Second)
	require.NoError(t, err)
	defer browser.Close()
	require.NoError(t, browser.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, err = fmt.Fprintf(browser, "GET http://%[1]s/socket HTTP/1.1\r\nHost: %[1]s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n", wsAddr)
	require.NoError(t, err)
	resp, err := http.ReadResponse(bufio.NewReader(browser), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	select {
	case wsConn := <-upgraded:
		defer wsConn.Close()
	case <-ctx.Done():
		t.Fatal("WebSocket origin did not upgrade")
	}
	stats := waitStats(func(s *ypb.MITMPipelineStats) bool { return s.GetDiagnostics().GetWebSocketOnline() == 1 })
	require.Zero(t, stats.GetActiveTotal())
	require.Zero(t, stats.GetUpstreamActive())
	online, err := client.GetMITMDiagnostics(ctx, &ypb.GetMITMDiagnosticsRequest{SessionId: session, View: "websocket"})
	require.NoError(t, err)
	require.Len(t, online.GetTraces(), 1)
	require.Equal(t, "WebSocket", online.GetTraces()[0].GetProtocol())
	require.Equal(t, "websocket_online", online.GetTraces()[0].GetStage())

	require.NoError(t, browser.Close())
	waitStats(func(s *ypb.MITMPipelineStats) bool { return s.GetDiagnostics().GetWebSocketOnline() == 0 })
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
		tracker.requestObserved(req)
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
		tracker.requestObserved(req)
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
