package lowhttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

// h2Transport implements transport for HTTP/2.
//
// It delegates connection management to H2ConnPool and stream lifecycle to
// http2ClientConn.  The transport handles stale-connection retries internally
// (bounded by maxReconnectTimes) and reports protocol-downgrade conditions
// (ALPN mismatch, server preface timeout) so the orchestration layer can fall
// back to HTTP/1.1.
type h2Transport struct {
	pool *LowHttpConnPool // owning pool (for h2Pool access and connPool default)
}

// newH2Transport returns a transport that executes requests over HTTP/2.
// A nil pool uses DefaultLowHttpConnPool.
func newH2Transport(pool *LowHttpConnPool) transport {
	if pool == nil {
		pool = DefaultLowHttpConnPool
	}
	return &h2Transport{pool: pool}
}

func (t *h2Transport) RoundTrip(ctx context.Context, tr *transportRequest) (*transportResult, error) {
	method, _, _ := GetHTTPPacketFirstLine(tr.packet)
	replayRequest := &http.Request{Method: method}
	if tr.connPool == nil {
		tr.connPool = t.pool
	}
	h2Pool := tr.connPool.h2Pool
	if h2Pool == nil {
		return nil, utils.Error("h2 transport: conn pool has no h2 pool")
	}

	reconnectTimes := 0
	maxReconnects := maxReconnectTimes

	canReconnect := func(err error) bool {
		if reconnectTimes >= maxReconnects {
			log.Warnf("h2 transport: giving up after %d reconnects to %v: %v", reconnectTimes, tr.cacheKey.addr, err)
			return false
		}
		reconnectTimes++
		return true
	}

RECONNECT:
	entry, err := h2Pool.GetOrCreate(ctx, tr.cacheKey, tr.dialOpts)
	if err != nil {
		return nil, err
	}

	// ALPN did not negotiate H2. No request bytes were sent, so transfer this
	// already established H1 socket to the orchestration layer.
	if !entry.IsH2() {
		return &transportResult{
			h1Conn:     entry.conn,
			remoteAddr: entry.conn.RemoteAddr().String(),
			portIsOpen: true,
		}, ErrProtocolNotAvailable
	}
	partial := &transportResult{remoteAddr: entry.conn.RemoteAddr().String(), portIsOpen: true}

	h2Conn := entry.alt
	if h2Conn == nil {
		return partial, utils.Error("h2 transport: conn h2 processor is nil")
	}

	h2Stream, err := h2Conn.newStream(tr.reqIns, tr.packet, tr.option)
	if err != nil {
		if err == CreateStreamAfterGoAwayErr {
			h2Conn.retire()
			if canReconnect(err) {
				goto RECONNECT
			}
		}
		return partial, err
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		h2Stream.abort()
		return partial, ctxErr
	}

	currentRPS.Add(1)
	serverStart := time.Now()
	h2Stream.SetReadFirstFrameCallback(func() { tr.traceInfo.ServerTime = time.Since(serverStart) })

	if err := h2Stream.doRequest(); err != nil && !errors.Is(err, errH2UploadAborted) {
		h2Stream.abort()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return partial, ctxErr
		}
		if err == CreateStreamAfterGoAwayErr {
			h2Conn.retire()
			if canReconnect(err) {
				goto RECONNECT
			}
		}
		return partial, err
	}

	timeout := tr.timeout
	resp, responsePacket, responseStarted, err := h2Stream.waitResponse(ctx, timeout)
	if resp.StatusCode != 0 {
		partial.firstResponse = &resp
		partial.rawBytes = responsePacket
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return partial, ctxErr
		}
		// Check for protocol-level downgrade conditions.
		if shouldDowngradeH2(err) {
			return partial, err
		}
		if !responseStarted && h2RequestCanRetry(replayRequest, err) && (tr.option.bodyStreamReaderHandled == nil || !tr.option.bodyStreamReaderHandled.IsSet()) {
			if canReconnect(err) {
				goto RECONNECT
			}
		}
		return partial, err
	}

	if tr.reqIns != nil {
		httpctx.SetBareResponseBytes(tr.reqIns, responsePacket)
	}

	remoteAddr := ""
	if h2Conn.conn != nil {
		remoteAddr = h2Conn.conn.RemoteAddr().String()
	}

	return &transportResult{
		rawBytes:      responsePacket,
		remoteAddr:    remoteAddr,
		portIsOpen:    true,
		firstResponse: &resp,
	}, nil
}

func (t *h2Transport) ShouldDowngrade(err error) bool {
	return shouldDowngradeH2(err)
}
