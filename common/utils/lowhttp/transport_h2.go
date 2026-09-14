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

// h2Transport implements Transport for HTTP/2.
//
// It delegates connection management to H2ConnPool and stream lifecycle to
// http2ClientConn.  The transport handles stale-connection retries internally
// (bounded by maxReconnectTimes) and reports protocol-downgrade conditions
// (ALPN mismatch, server preface timeout) so the orchestration layer can fall
// back to HTTP/1.1.
type h2Transport struct {
	pool *LowHttpConnPool // owning pool (for h2Pool access and connPool default)
}

// NewH2Transport returns a Transport that executes requests over HTTP/2.
// pool must be non-nil; it provides the H2ConnPool used for connection reuse.
func NewH2Transport(pool *LowHttpConnPool) Transport {
	if pool == nil {
		pool = DefaultLowHttpConnPool
	}
	return &h2Transport{pool: pool}
}

func (t *h2Transport) RoundTrip(ctx context.Context, tr *transportRequest) (*transportResult, error) {
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

	// Downgrade: ALPN did not negotiate h2, or preface failed.
	if !entry.IsH2() {
		// The entry's conn is an H1 connection; the orchestration layer will
		// detect the downgrade and retry with H1 transport.
		entry.conn.Close()
		return nil, ErrProtocolNotAvailable
	}

	h2Conn := entry.alt
	if h2Conn == nil {
		return nil, utils.Error("h2 transport: conn h2 processor is nil")
	}

	h2Stream, err := h2Conn.newStream(tr.reqIns, tr.packet, tr.option)
	if err != nil {
		if err == CreateStreamAfterGoAwayErr {
			h2Conn.retire()
			if canReconnect(err) {
				goto RECONNECT
			}
		}
		return nil, err
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		h2Stream.abort()
		return nil, ctxErr
	}

	currentRPS.Add(1)
	serverStart := time.Now()
	h2Stream.SetReadFirstFrameCallback(func() { tr.traceInfo.ServerTime = time.Since(serverStart) })

	if err := h2Stream.doRequest(); err != nil && !errors.Is(err, errH2UploadAborted) {
		h2Stream.abort()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if err == CreateStreamAfterGoAwayErr {
			h2Conn.retire()
			if canReconnect(err) {
				goto RECONNECT
			}
		}
		return nil, err
	}

	timeout := tr.timeout
	resp, responsePacket, err := h2Stream.waitResponse(ctx, timeout)
	_ = resp
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		// Check for protocol-level downgrade conditions.
		if shouldDowngradeH2(err) {
			return nil, err
		}
		if h2RequestCanRetry(tr.reqIns, err) && (tr.option.bodyStreamReaderHandled == nil || !tr.option.bodyStreamReaderHandled.IsSet()) {
			if canReconnect(err) {
				goto RECONNECT
			}
		}
		return nil, err
	}

	if tr.reqIns != nil {
		httpctx.SetBareResponseBytes(tr.reqIns, responsePacket)
	}

	remoteAddr := ""
	if h2Conn.conn != nil {
		remoteAddr = h2Conn.conn.RemoteAddr().String()
	}

	return &transportResult{
		rawBytes:     responsePacket,
		remoteAddr:   remoteAddr,
		portIsOpen:   true,
		firstResponse: &resp,
	}, nil
}

func (t *h2Transport) CanRetry(req *http.Request, err error) bool {
	return h2RequestCanRetry(req, err)
}

func (t *h2Transport) ShouldDowngrade(err error) bool {
	return shouldDowngradeH2(err)
}

