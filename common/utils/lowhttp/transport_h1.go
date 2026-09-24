package lowhttp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	errorspkg "github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

// h1Transport implements transport for HTTP/1.1.
//
// It supports two connection modes:
//   - Pooled: requests are dispatched through persistConn's read/write loops.
//   - Direct: a one-off connection is dialed, written, and read inline.
//
// A stale pooled connection is closed and reported as reconnectError to the
// orchestration layer, which owns the H1 retry policy.
type h1Transport struct {
	pool *LowHttpConnPool
}

// newH1Transport returns a transport that executes requests over HTTP/1.1.
// pool may be nil; it defaults to DefaultLowHttpConnPool.
func newH1Transport(pool *LowHttpConnPool) transport {
	if pool == nil {
		pool = DefaultLowHttpConnPool
	}
	return &h1Transport{pool: pool}
}

func (t *h1Transport) RoundTrip(ctx context.Context, tr *transportRequest) (*transportResult, error) {
	requestPacket := tr.packet
	connPool := tr.connPool
	if connPool == nil {
		connPool = t.pool
	}
	withConnPool := connPool != nil && tr.usePool
	conn := tr.h1Conn
	tr.h1Conn = nil // transfer ownership to this one attempt
	if conn != nil {
		withConnPool = false
	}
	var err error

	if conn == nil {
		if withConnPool {
			conn, err = connPool.getIdleConn(ctx, tr.cacheKey, tr.dialOpts...)
		} else {
			conn, err = dialXWithContext(ctx, tr.originAddr, tr.dialOpts...)
		}
	}

	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		// old version proxy fallback
		errMsg := err.Error()
		if !containsNoProxyAvailable(errMsg) {
			return nil, err
		}
		// Build the forward-proxy packet before dialing so a malformed request
		// cannot leave a newly opened proxy connection behind.
		requestPacket, err = BuildLegacyProxyRequest(requestPacket, tr.option.Https)
		if err != nil {
			return nil, err
		}
		conn, err = t.tryLegacyProxy(ctx, tr)
		if err != nil {
			return nil, err
		}
		// A forward-proxy request includes Connection: close. Never put this
		// connection in the pool, including when the request fails midway.
		defer conn.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return &transportResult{remoteAddr: conn.RemoteAddr().String(), portIsOpen: true}, ctxErr
		}
		return t.roundTripDirect(ctx, tr, conn, requestPacket)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		conn.Close()
		return &transportResult{remoteAddr: conn.RemoteAddr().String(), portIsOpen: true}, ctxErr
	}

	if withConnPool {
		return t.roundTripPooled(ctx, tr, conn, requestPacket)
	}
	return t.roundTripDirect(ctx, tr, conn, requestPacket)
}

// containsNoProxyAvailable checks if the error message indicates no proxy
// is available.
func containsNoProxyAvailable(msg string) bool {
	return strings.Contains(msg, `no proxy available`)
}

// Extra bytes after the declared request body may be another pipelined request.
// Its response can arrive after the first response has already been parsed.
func hasExtraH1RequestBytes(packet []byte) bool {
	_, body := SplitHTTPHeadersAndBodyFromPacketView(packet)
	if len(body) == 0 {
		return false
	}
	contentLength := GetHTTPPacketHeader(packet, "Content-Length")
	if contentLength == "" {
		return !strings.Contains(strings.ToLower(GetHTTPPacketHeader(packet, "Transfer-Encoding")), "chunked")
	}
	length, err := strconv.ParseInt(strings.TrimSpace(contentLength), 10, 64)
	return err != nil || length < 0 || int64(len(body)) > length
}

func (t *h1Transport) tryLegacyProxy(ctx context.Context, tr *transportRequest) (net.Conn, error) {
	noProxyDial := make([]netx.DialXOption, len(tr.dialOpts), len(tr.dialOpts)+1)
	copy(noProxyDial, tr.dialOpts)
	noProxyDial = append(noProxyDial, netx.DialX_WithDisableProxy(true))
	merged := append([]string{}, tr.option.Proxy...)
	for _, basicProxy := range merged {
		if !utils.IsHttpOrHttpsUrl(basicProxy) {
			continue
		}
		addr := utils.ExtractHostPort(basicProxy)
		conn, err := dialXWithContext(ctx, addr, noProxyDial...)
		if err == nil {
			return conn, nil
		}
	}
	return nil, utils.Error("no proxy available")
}

// roundTripPooled executes an H1 request through the connection pool.
func (t *h1Transport) roundTripPooled(ctx context.Context, tr *transportRequest, conn net.Conn, requestPacket []byte) (*transportResult, error) {
	option := tr.option
	reqIns := tr.reqIns

	pc, ok := conn.(*persistConn)
	if !ok {
		return nil, utils.Error("h1 transport: pooled conn is not a persistConn")
	}

	writeErrCh := make(chan error, 2)
	if option.BeforeDoRequest != nil {
		requestPacket = option.BeforeDoRequest(requestPacket)
	}

	resc := make(chan responseInfo, 1)
	pc.reqCh <- requestAndResponseCh{
		reqPacket:   requestPacket,
		ch:          resc,
		reqInstance: reqIns,
		option:      option,
		writeErrCh:  writeErrCh,
	}
	pc.writeCh <- writeRequest{reqPacket: requestPacket, ch: writeErrCh, reqInstance: reqIns, options: option}

	var rawBytes []byte
	var firstResponse *http.Response
	select {
	case re := <-resc:
		if re.err != nil && len(rawBytes) == 0 {
			if pc.shouldRetryRequest(re.err) {
				pc.closeConn(re.err)
				// Signal reconnect to caller
				return nil, &reconnectError{err: re.err}
			}
			return nil, re.err
		}
		firstResponse = re.resp
		rawBytes = re.respBytes
		tr.traceInfo.ServerTime = re.info.ServerTime
		if option != nil && option.BodyStreamReaderHandler != nil {
			if option.bodyStreamReaderHandled != nil {
				option.bodyStreamReaderHandled.Set()
			}
		}
	case <-ctx.Done():
		pc.closeConn(ctx.Err())
		return nil, ctx.Err()
	case <-pc.ctx.Done():
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if pc.closed == nil {
			return nil, utils.Error("BUG: closeCh but closed is nil")
		}
		if pc.shouldRetryRequest(pc.closed) {
			return nil, &reconnectError{err: pc.closed}
		}
		return nil, pc.closed
	}

	return &transportResult{
		rawBytes:       rawBytes,
		firstResponse:  firstResponse,
		multiResponses: nil,
		remoteAddr:     conn.RemoteAddr().String(),
		portIsOpen:     true,
	}, nil
}

// roundTripDirect executes an H1 request on a one-off (non-pooled) connection.
func (t *h1Transport) roundTripDirect(ctx context.Context, tr *transportRequest, conn net.Conn, requestPacket []byte) (result *transportResult, resultErr error) {
	option := tr.option
	reqIns := tr.reqIns
	timeout := tr.timeout

	var responseRaw bytes.Buffer
	partial := &transportResult{remoteAddr: conn.RemoteAddr().String(), portIsOpen: true}
	defer func() {
		if resultErr != nil && result == nil {
			partial.rawBytes = responseRaw.Bytes()
			result = partial
		}
	}()

	if conn != nil {
		readConnEndCtx, readConnEnd := context.WithCancel(ctx)
		defer readConnEnd()
		go func() {
			<-readConnEndCtx.Done()
			conn.Close()
		}()
	}

	if option.BeforeDoRequest != nil {
		requestPacket = option.BeforeDoRequest(requestPacket)
	}

	if reqIns != nil {
		httpctx.SetBareRequestBytes(reqIns, requestPacket)
	}
	currentRPS.Add(1)

	if option.EnableRandomChunked {
		chunkSender, err := option.GetOrCreateChunkSender()
		if err != nil {
			return nil, errorspkg.Wrap(err, "get or create chunk sender failed")
		}
		err = chunkSender.Send(requestPacket, conn)
		if err != nil {
			return nil, errorspkg.Wrap(err, "write request failed")
		}
	} else {
		_, err := conn.Write(requestPacket)
		if err != nil {
			return nil, errorspkg.Wrap(err, "write request failed")
		}
	}

	if option.DefaultBufferSize <= 0 {
		option.DefaultBufferSize = 4096
	}

	var mirrorWriter io.Writer = &responseRaw
	var finishStreamHandler func()
	defer func() {
		if finishStreamHandler != nil {
			finishStreamHandler()
		}
	}()

	// BodyStreamReaderHandler for non-pool connection
	if option != nil && option.BodyStreamReaderHandler != nil {
		reader, writer := utils.NewBufPipe(nil)
		streamHandlerDone := make(chan struct{})
		streamBodyReaderCh := make(chan io.ReadCloser, 1)
		finished := false
		finishStreamHandler = func() {
			if finished {
				return
			}
			finished = true
			writer.Close()
			waitStreamHandlerDone(streamHandlerDone, streamBodyReaderCh, 2*time.Second, "non-pool stream handler")
		}
		go func() {
			bodyReader, bodyWriter := utils.NewBufPipe(nil)
			streamBodyReaderCh <- bodyReader
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("BodyStreamReaderHandler panic: %v", r)
				}
				bodyWriter.Close()
				close(streamHandlerDone)
			}()

			packetReader := bufio.NewReader(reader)
			responseHeader := bytes.NewBufferString("")
			var responseHeaderWriter io.Writer = responseHeader
			if option.NoBodyBuffer {
				responseHeaderWriter = io.MultiWriter(responseHeaderWriter, &responseRaw)
			}

			var headerErr error
			for {
				line, err := utils.BufioReadLine(packetReader)
				if err != nil {
					headerErr = err
					if err != io.EOF {
						log.Errorf("BodyStreamReaderHandler read response failed: %s", err)
					}
					bodyWriter.Close()
					break
				}
				responseHeaderWriter.Write(line)
				responseHeaderWriter.Write([]byte("\r\n"))
				if len(line) == 0 {
					go func() {
						io.Copy(bodyWriter, packetReader)
						bodyWriter.Close()
					}()
					break
				}
			}
			if headerErr != nil {
				log.Warnf("BodyStreamReaderHandler read response header failed: %s", headerErr)
			} else {
				if option.bodyStreamReaderHandled != nil {
					option.bodyStreamReaderHandled.Set()
				}
				option.BodyStreamReaderHandler(responseHeader.Bytes(), bodyReader)
			}
		}()

		if option.NoBodyBuffer {
			mirrorWriter = writer
		} else {
			rawWriter := io.Writer(&responseRaw)
			if option.AutoDetectSSE {
				rawWriter = &responseRawCaptureWriter{
					dst:           &responseRaw,
					req:           reqIns,
					autoDetectSSE: true,
				}
			}
			mirrorWriter = io.MultiWriter(rawWriter, writer)
		}
	}

	httpResponseReader := bufio.NewReaderSize(io.TeeReader(conn, mirrorWriter), option.DefaultBufferSize)
	if timeout > 0 && option != nil && !option.ExtendReadDeadline {
		hardTimeoutTimer := time.AfterFunc(timeout, func() {
			_ = conn.SetReadDeadline(time.Now().Add(-1 * time.Second))
		})
		defer hardTimeoutTimer.Stop()
	}

	// Read response
	serverTimeStart := time.Now()
	_ = conn.SetReadDeadline(serverTimeStart.Add(timeout))
	firstByte, err := httpResponseReader.Peek(1)
	if err != nil {
		return nil, errorspkg.Wrap(err, "read first byte failed")
	}

	if firstByte[0] == 0x15 {
		tlsHeader, err := httpResponseReader.Peek(6)
		if err == nil && bytes.Equal(tlsHeader, []byte("\x15\x03\x01\x00\x02\x02")) {
			return nil, utils.Errorf("tls record header error detected... raw: %v", spew.Sdump(tlsHeader))
		}
	}

	tr.traceInfo.ServerTime = time.Since(serverTimeStart)

	var firstResponse *http.Response
	if option.DiscardIntermediateResponseBody {
		firstResponse, err = utils.ReadHTTPResponseMetadataFromBufioReader(httpResponseReader, reqIns, responseRaw.Grow)
	} else {
		firstResponse, err = utils.ReadHTTPResponseFromBufioReader(httpResponseReader, reqIns)
	}
	if err != nil {
		log.Warnf("[lowhttp] read response failed: %s", err)
	}
	if utils.HTTPResponseHasDiscardedIntermediateBody(firstResponse) {
		firstResponse.Body = http.NoBody
	}

	// Digest auth (401 retry)
	if tr.option != nil && firstResponse != nil && firstResponse.StatusCode == http.StatusUnauthorized {
		if authHeader := IGetHeader(firstResponse, "WWW-Authenticate"); len(authHeader) > 0 {
			if auth := GetHttpAuth(authHeader[0], tr.option); auth != nil {
				authReq, authErr := auth.Authenticate(conn, tr.option)
				if authErr == nil {
					_, wErr := conn.Write(authReq)
					responseRaw.Reset()
					if wErr != nil {
						return nil, errorspkg.Wrap(wErr, "write request failed")
					}
					// Re-read response after auth
					_ = conn.SetReadDeadline(time.Now().Add(timeout))
					if tr.option.DiscardIntermediateResponseBody {
						firstResponse, err = utils.ReadHTTPResponseMetadataFromBufioReader(httpResponseReader, reqIns, responseRaw.Grow)
					} else {
						firstResponse, err = utils.ReadHTTPResponseFromBufioReader(httpResponseReader, reqIns)
					}
					if err != nil {
						log.Warnf("[lowhttp] read response after auth failed: %s", err)
					}
				}
			}
		}
	}

	responseBodySize := httpctx.GetResponseBodySize(reqIns)
	_ = responseBodySize // caller reads from httpctx

	var multiResponses []*http.Response
	var isMultiResponses bool
	respClose := false
	if firstResponse != nil {
		respClose = firstResponse.Close
		multiResponses = append(multiResponses, firstResponse)
	}

	noFixContentLength := tr.preserveLength
	if firstResponse == nil || respClose {
		if len(responseRaw.Bytes()) == 0 {
			return nil, errorspkg.Wrap(err, "empty result.")
		} else {
			// Drain any remaining bytes from the bufio reader through the
			// TeeReader so that responseRaw captures the full wire packet.
			// Without this, bytes buffered in httpResponseReader but not yet
			// consumed by ReadHTTPResponseFromBufioReader are lost.
			stableTimeout := timeout
			if respClose && timeout < 1*time.Second {
				stableTimeout = 1 * time.Second
			}
			restBytes, _ := utils.ReadUntilStable(httpResponseReader, conn, stableTimeout, 300*time.Millisecond)
			if len(restBytes) > 0 {
				if len(restBytes) > 256 {
					restBytes = restBytes[:256]
				}
				log.Warnf("unhandled rest data in connection: %#v ...", string(restBytes))
			}
		}
	} else {
		firstResponse.Request = reqIns
		for noFixContentLength && !tr.option.NoReadMultiResponse {
			nextResponse, nextErr := utils.ReadHTTPResponseFromBufioReaderConn(httpResponseReader, conn, nil)
			var nextRespClose bool
			if nextResponse != nil {
				nextRespClose = nextResponse.Close
			}
			if nextErr != nil || nextRespClose {
				if errors.Is(nextErr, io.EOF) || errors.Is(nextErr, io.ErrUnexpectedEOF) {
					break
				}
				// Preserve the remaining wire bytes for malformed or closing
				// pipeline responses without delaying ordinary keep-alive replies.
				stableTimeout := timeout
				if nextRespClose && stableTimeout < time.Second {
					stableTimeout = time.Second
				}
				restBytes, _ := utils.ReadUntilStable(httpResponseReader, conn, stableTimeout, 300*time.Millisecond)
				if len(restBytes) > 0 {
					if len(restBytes) > 256 {
						restBytes = restBytes[:256]
					}
					log.Warnf("unhandled rest data in connection: %#v ...", string(restBytes))
				}
				break
			}
			if nextResponse != nil {
				multiResponses = append(multiResponses, nextResponse)
				isMultiResponses = true
			}
		}
	}

	if firstResponse != nil && !respClose && hasExtraH1RequestBytes(requestPacket) {
		// A pipelined response may not have arrived by the time the first one
		// finishes. Only wait for it when the request has extra wire bytes.
		_, _ = utils.ReadUntilStable(httpResponseReader, conn, 500*time.Millisecond, 300*time.Millisecond)
	}

	// Complete the stream handler before exposing responseRaw: with NoBodyBuffer
	// it is the handler that writes the response headers into that buffer.
	if finishStreamHandler != nil {
		finishStreamHandler()
	}

	return &transportResult{
		rawBytes:        responseRaw.Bytes(),
		firstResponse:   firstResponse,
		multiResponses:  multiResponses,
		isMultiResponse: isMultiResponses,
		remoteAddr:      conn.RemoteAddr().String(),
		portIsOpen:      true,
	}, nil
}

func (t *h1Transport) ShouldDowngrade(err error) bool {
	return false
}

// reconnectError wraps an error to signal that the caller should reconnect
// and retry the request.
type reconnectError struct{ err error }

func (e *reconnectError) Error() string { return fmt.Sprintf("reconnect: %v", e.err) }
func (e *reconnectError) Unwrap() error { return e.err }

// isReconnectError reports whether err is a reconnect signal.
func isReconnectError(err error) bool {
	var re *reconnectError
	return errors.As(err, &re)
}

func reconnectErrorUnwrap(err error) error {
	var re *reconnectError
	if errors.As(err, &re) {
		return re.err
	}
	return err
}
