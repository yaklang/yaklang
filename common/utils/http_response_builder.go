package utils

import (
	"bytes"
	tls "crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	utls2 "github.com/refraction-networking/utls"

	"github.com/pkg/errors"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// ParseHTTPResponseLine parses `HTTP/1.1 200 OK` into its ports
func ParseHTTPResponseLine(line string) (string, int, string, bool) {
	line = strings.TrimSpace(line)

	var proto string
	var code int
	var status string

	blocks := strings.SplitN(line, " ", 3)
	lenOfBlocks := len(blocks)
	if lenOfBlocks > 0 {
		proto = blocks[0]
	}
	if lenOfBlocks > 1 {
		code = codec.Atoi(blocks[1])
	}
	if lenOfBlocks > 2 {
		status = blocks[2]
	}
	return proto, code, status, code != 0
}

func ReadHTTPResponseFromBufioReader(reader io.Reader, req *http.Request) (*http.Response, error) {
	rsp, err := readHTTPResponseFromBufioReader(reader, false, req, nil, true, false, nil, nil)
	if err != nil {
		return nil, err
	}
	rsp.Request = req
	return rsp, nil
}

// ReadHTTPResponseMetadataFromBufioReader consumes a response for callers that
// separately capture the complete wire packet and only need transport metadata
// from this temporary response. For a bounded Content-Length response without
// streaming/large-body callbacks, Body is deliberately not retained while the
// parser-owned final bare response is still stored in req. All other response
// shapes keep the normal parser behavior. prepareBodyCapacity, when non-nil,
// lets the caller reserve
// space in its already-existing wire capture before the body is drained.
func ReadHTTPResponseMetadataFromBufioReader(reader io.Reader, req *http.Request, prepareBodyCapacity func(int)) (*http.Response, error) {
	rsp, err := readHTTPResponseFromBufioReader(reader, false, req, nil, true, true, prepareBodyCapacity, nil)
	if err != nil {
		return nil, err
	}
	rsp.Request = req
	return rsp, nil
}

// ReadSingleHTTPResponseFromBufioReader reads exactly one HTTP response,
// including informational responses. Most callers should use
// ReadHTTPResponseFromBufioReader, which skips non-terminal 1xx responses.
func ReadSingleHTTPResponseFromBufioReader(reader io.Reader, req *http.Request) (*http.Response, error) {
	rsp, err := readHTTPResponseFromBufioReader(reader, false, req, nil, false, false, nil, nil)
	if err != nil {
		return nil, err
	}
	rsp.Request = req
	return rsp, nil
}

type FileOpenerType func(s string) (*os.File, error)

var (
	tempFileOpener FileOpenerType
)

func getDefaultTempFileDir() string {
	if home := os.Getenv("YAKIT_HOME"); home != "" {
		return filepath.Join(home, "temp")
	}
	return filepath.Join(GetHomeDirDefault("."), "yakit-projects", "temp")
}

func RegisterTempFileOpener(dialer FileOpenerType) {
	tempFileOpener = dialer
}

func OpenTempFile(s string) (*os.File, error) {
	if tempFileOpener != nil {
		return tempFileOpener(s)
	}

	tempDir := getDefaultTempFileDir()
	if !IsDir(tempDir) {
		_ = os.MkdirAll(tempDir, 0o755)
	}
	return os.OpenFile(filepath.Join(tempDir, s), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
}

func ReadHTTPResponseFromBufioReaderConn(reader io.Reader, conn net.Conn, req *http.Request) (*http.Response, error) {
	rsp, err := readHTTPResponseFromBufioReader(reader, false, req, conn, true, false, nil, nil)
	if err != nil {
		return nil, err
	}
	rsp.Request = req
	return rsp, nil
}

// ReadHTTPResponseMetadataFromBufioReaderConn is the connection-aware variant
// of ReadHTTPResponseMetadataFromBufioReader.
func ReadHTTPResponseMetadataFromBufioReaderConn(reader io.Reader, conn net.Conn, req *http.Request, prepareBodyCapacity func(int)) (*http.Response, error) {
	rsp, err := readHTTPResponseFromBufioReader(reader, false, req, conn, true, true, prepareBodyCapacity, nil)
	if err != nil {
		return nil, err
	}
	rsp.Request = req
	return rsp, nil
}

// ReadHTTPResponseMetadataFromBufioReaderConnWithBorrowedPacket is the narrow
// connection-pool variant used when the transport already owns a complete wire
// capture. borrowFinalPacket must return an immutable view of the final response
// suffix with exactly the requested size. The view is stored in req without a
// clone; the capture owner must keep it alive and unchanged while req is used.
func ReadHTTPResponseMetadataFromBufioReaderConnWithBorrowedPacket(reader io.Reader, conn net.Conn, req *http.Request, prepareBodyCapacity func(int), borrowFinalPacket func(int) []byte) (*http.Response, error) {
	rsp, err := readHTTPResponseFromBufioReader(reader, false, req, conn, true, true, prepareBodyCapacity, borrowFinalPacket)
	if err != nil {
		return nil, err
	}
	rsp.Request = req
	return rsp, nil
}

func ReadHTTPResponseFromBytes(raw []byte, req *http.Request) (*http.Response, error) {
	rsp, err := readHTTPResponseFromBufioReader(bytes.NewReader(raw), true, req, nil, true, false, nil, nil)
	if err != nil {
		return nil, err
	}
	rsp.Request = req
	return rsp, nil
}

type httpResponseBytesBodyViewReader struct {
	packet []byte
	reader bytes.Reader
}

func newHTTPResponseBytesBodyViewReader(packet []byte) *httpResponseBytesBodyViewReader {
	r := &httpResponseBytesBodyViewReader{packet: packet}
	r.reader.Reset(packet)
	return r
}

func (r *httpResponseBytesBodyViewReader) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *httpResponseBytesBodyViewReader) takeRemainingBodyView() []byte {
	remaining := r.reader.Len()
	offset := len(r.packet) - remaining
	_, _ = r.reader.Seek(0, io.SeekEnd)
	return r.packet[offset:]
}

// ReadHTTPResponseFromBytesWithBodyView parses an immutable complete response
// packet without copying its remaining body. The returned Body retains and
// aliases raw; callers must keep raw unchanged until the response is no longer
// used. ReadHTTPResponseFromBytes keeps its historical independent-Body
// ownership and remains the default for shared or externally mutable packets.
func ReadHTTPResponseFromBytesWithBodyView(raw []byte) (*http.Response, error) {
	rsp, err := readHTTPResponseFromBufioReader(newHTTPResponseBytesBodyViewReader(raw), true, nil, nil, true, false, nil, nil)
	if err != nil {
		return nil, err
	}
	return rsp, nil
}

func responseStatusHasNoBody(statusCode int) bool {
	return statusCode >= 100 && statusCode < 200 ||
		statusCode == http.StatusNoContent ||
		statusCode == http.StatusNotModified
}

// Keep the common MITM body path single-allocation without trusting an
// unbounded peer-provided Content-Length. Larger bodies retain the progressive
// io.ReadAll behavior.
const httpResponseBodyPreallocateLimit = 1 << 20

func readHTTPResponseBodyWithLimit(reader io.Reader, limit int) ([]byte, error) {
	if limit <= 0 {
		return nil, nil
	}
	if limit > httpResponseBodyPreallocateLimit {
		return io.ReadAll(io.LimitReader(reader, int64(limit)))
	}

	body := make([]byte, limit)
	n, err := io.ReadFull(reader, body)
	switch err {
	case nil:
		return body, nil
	case io.EOF, io.ErrUnexpectedEOF:
		return body[:n], nil
	default:
		return body[:n], err
	}
}

func readHTTPResponseBodyToEOF(reader io.Reader) ([]byte, error) {
	if viewReader, ok := reader.(interface{ takeRemainingBodyView() []byte }); ok {
		return viewReader.takeRemainingBodyView(), nil
	}
	if bytesReader, ok := reader.(*bytes.Reader); ok {
		return readHTTPResponseBodyWithLimit(bytesReader, bytesReader.Len())
	}
	return io.ReadAll(reader)
}

func writeHTTPResponseBody(packet *bytes.Buffer, body []byte) {
	if packet == nil || len(body) == 0 {
		return
	}
	if len(body) <= httpResponseBodyPreallocateLimit {
		packet.Grow(len(body))
	}
	_, _ = packet.Write(body)
}

func padHTTPResponseBody(body []byte, size int) []byte {
	if size <= len(body) {
		return body
	}
	bodyLen := len(body)
	if cap(body) >= size {
		body = body[:size]
	} else {
		padded := make([]byte, size)
		copy(padded, body)
		body = padded
	}
	for i := bodyLen; i < size; i++ {
		body[i] = '\n'
	}
	return body
}

// ownedHTTPResponseBody keeps the parser-owned body immutable while allowing
// package-internal dumpers to consume the remaining bytes without allocating a
// second temporary slice. It intentionally exposes only io.ReadCloser and
// io.WriterTo behavior to callers.
type ownedHTTPResponseBody struct {
	data   []byte
	reader *bytes.Reader
}

func newOwnedHTTPResponseBody(body []byte) *ownedHTTPResponseBody {
	return &ownedHTTPResponseBody{data: body, reader: bytes.NewReader(body)}
}

func (b *ownedHTTPResponseBody) Read(p []byte) (int, error) {
	return b.reader.Read(p)
}

func (b *ownedHTTPResponseBody) WriteTo(w io.Writer) (int64, error) {
	return b.reader.WriteTo(w)
}

func (b *ownedHTTPResponseBody) Close() error {
	return nil
}

func (b *ownedHTTPResponseBody) takeRemainingView() []byte {
	offset, _ := b.reader.Seek(0, io.SeekCurrent)
	_, _ = b.reader.Seek(0, io.SeekEnd)
	return b.data[offset:]
}

type discardedIntermediateHTTPResponseBody struct{}

func (discardedIntermediateHTTPResponseBody) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (discardedIntermediateHTTPResponseBody) Close() error {
	return nil
}

// HTTPResponseHasDiscardedIntermediateBody reports whether a metadata-only
// parser consumed the body without retaining it. Transport callers should
// replace the sentinel with http.NoBody before exposing the response.
func HTTPResponseHasDiscardedIntermediateBody(rsp *http.Response) bool {
	if rsp == nil || rsp.Body == nil {
		return false
	}
	_, ok := rsp.Body.(discardedIntermediateHTTPResponseBody)
	return ok
}

func readHTTPResponseFromBufioReader(originReader io.Reader, fixContentLength bool, req *http.Request, conn net.Conn, skipInformational bool, discardIntermediateBody bool, prepareBodyCapacity func(int), borrowFinalPacket func(int) []byte) (*http.Response, error) {
	var rawPacket *bytes.Buffer
	if req != nil {
		rawPacket = new(bytes.Buffer)
	}
	var nobodyReqMethod bool
	if req != nil { // some request method will not have body
		nobodyReqMethod = strings.EqualFold(req.Method, http.MethodHead) ||
			strings.EqualFold(req.Method, http.MethodTrace) ||
			strings.EqualFold(req.Method, http.MethodConnect)
	}

	headerReader := originReader
	rsp := new(http.Response)
	firstLine, err := ReadLine(headerReader)
	if err != nil {
		return nil, errors.Wrap(err, "read HTTPResponse firstline failed")
	}

	var statusText string
	informationalResponses := 0
	for {
		rsp.Proto, rsp.StatusCode, statusText, _ = ParseHTTPResponseLine(string(firstLine))
		if !skipInformational || rsp.StatusCode < 100 || rsp.StatusCode >= 200 || rsp.StatusCode == http.StatusSwitchingProtocols {
			break
		}
		// Only skip well-known interim responses (100/102/103). Other 1xx may be
		// standalone packets (e.g. traffic generators) and must parse as themselves.
		if rsp.StatusCode != 100 && rsp.StatusCode != 102 && rsp.StatusCode != 103 {
			break
		}
		informationalResponses++
		if informationalResponses >= 16 {
			return nil, Error("too many informational HTTP responses")
		}

		// Informational responses have no body. Consume their header section and
		// continue with the next response on the same connection — but only when a
		// real following HTTP response exists. Otherwise rewind and keep this 1xx.
		var headerBlock bytes.Buffer
		canSkip := true
		for {
			line, lineErr := ReadLine(headerReader)
			if lineErr != nil {
				canSkip = false
				if len(bytes.TrimSpace(line)) > 0 {
					headerBlock.Write(line)
				}
				break
			}
			if len(bytes.TrimSpace(line)) == 0 {
				break
			}
			headerBlock.Write(line)
			headerBlock.WriteString(CRLF)
		}
		var nextFirst []byte
		nextFirstHadEOL := false
		if canSkip {
			for {
				line, lineErr := ReadLine(headerReader)
				if len(bytes.TrimSpace(line)) > 0 {
					nextFirst = line
					nextFirstHadEOL = lineErr == nil
					if lineErr != nil {
						canSkip = false
					}
					break
				}
				if lineErr != nil {
					canSkip = false
					break
				}
			}
		}
		if canSkip {
			proto, code, _, parsed := ParseHTTPResponseLine(string(nextFirst))
			if parsed && strings.HasPrefix(proto, "HTTP/") && code >= 100 {
				firstLine = nextFirst
				continue
			}
			canSkip = false
		}
		if !canSkip {
			// Put consumed bytes back so this 1xx can be parsed as the final response.
			var rewind bytes.Buffer
			rewind.Write(headerBlock.Bytes())
			rewind.WriteString(CRLF)
			if len(nextFirst) > 0 {
				rewind.Write(nextFirst)
				if nextFirstHadEOL {
					rewind.WriteString(CRLF)
				}
			}
			headerReader = io.MultiReader(bytes.NewReader(rewind.Bytes()), headerReader)
			break
		}
	}
	if rawPacket != nil {
		rawPacket.Write(firstLine)
		rawPacket.WriteString(CRLF)
	}
	rsp.Status = fmt.Sprintf("%v %s", rsp.StatusCode, statusText)
	_, after, _ := strings.Cut(rsp.Proto, "/")
	major, minor, _ := strings.Cut(after, ".")
	rsp.ProtoMajor = codec.Atoi(major)
	rsp.ProtoMinor = codec.Atoi(minor)
	if rsp.StatusCode < 100 {
		return nil, Errorf("invalid first line: %v", strconv.Quote(string(firstLine)))
	}

	// header
	header := make(http.Header)
	useContentLength := false
	hasEntityHeader := false
	contentLengthInt := 0
	useTransferEncodingChunked := false
	defaultClose := (rsp.ProtoMajor == 1 && rsp.ProtoMinor == 0) || rsp.ProtoMajor < 1

	err = ScanHTTPHeaderWithHeaderFolding(headerReader, func(rawHeader []byte) {
		if rawPacket != nil {
			if len(rawHeader) <= 0 {
				rawPacket.WriteString(CRLF)
			} else {
				rawPacket.Write(rawHeader)
				rawPacket.WriteString(CRLF)
			}
		}
		if len(rawHeader) <= 0 {
			return
		}

		keyStr, lowerKey, valStr := parseOwnedHTTPHeaderLine(rawHeader)
		if ret := httpctx.GetResponseHeaderParsed(req); ret != nil {
			ret(lowerKey, valStr)
		}

		alreadySet := false
		switch lowerKey {
		case "content-length":
			useContentLength = true
			contentLengthInt = codec.Atoi(strings.TrimSpace(valStr))
			if contentLengthInt != 0 {
				header.Set(keyStr, valStr)
				alreadySet = true
				rsp.ContentLength = int64(contentLengthInt)
			}
		case "transfer-encoding":
			rsp.TransferEncoding = []string{valStr}
			if IContains(valStr, "chunked") {
				useTransferEncodingChunked = true
			}
		case "connection":
			if strings.EqualFold(valStr, "close") {
				defaultClose = true
			} else if strings.EqualFold(valStr, "keep-alive") {
				defaultClose = false
			}
		case "x-content-type-options", "content-type", "content-encoding", "content-range", "expires", "content-language":
			hasEntityHeader = true
		}
		// add header
		if keyStr == "" || alreadySet {
			return
		}
		header[keyStr] = append(header[keyStr], valStr)

	}, nil)
	if err != nil {
		return nil, err
	}
	rsp.Close = defaultClose
	rsp.Header = header

	var headerBytes []byte
	if ret := httpctx.GetResponseHeaderWriter(req); ret != nil {
		headerBytes = rawPacket.Bytes()
		_, _ = ret.Write(rawPacket.Bytes())
	}

	noBodyBuffer := httpctx.GetNoBodyBuffer(req)
	discardBoundedBody := discardIntermediateBody &&
		!fixContentLength &&
		!noBodyBuffer &&
		!nobodyReqMethod &&
		useContentLength &&
		!useTransferEncodingChunked &&
		contentLengthInt > 0 &&
		contentLengthInt <= httpResponseBodyPreallocateLimit &&
		httpctx.GetResponseHeaderCallback(req) == nil &&
		!httpctx.GetResponseTooLarge(req)
	if maxContentLength := httpctx.GetResponseMaxContentLength(req); maxContentLength > 0 && contentLengthInt >= maxContentLength {
		discardBoundedBody = false
	}

	var bodyReader io.Reader = originReader
	if ret := httpctx.GetResponseHeaderCallback(req); ret != nil {
		if len(headerBytes) <= 0 {
			headerBytes = rawPacket.Bytes()
		}
		bodyReader, err = ret(rsp, headerBytes, bodyReader)
		if err != nil {
			return nil, Wrapf(err, "get response header callback failed")
		}
	}
	defer func() {
		if ret := httpctx.GetResponseFinishedCallback(req); ret != nil {
			ret()
		}
	}()

	var responseBody []byte
	var discardedBodyBytes int
	if responseStatusHasNoBody(rsp.StatusCode) {
		rsp.ContentLength = 0
	} else if fixContentLength {
		// just for bytes condition
		// by reader
		raw := []byte{}
		if noBodyBuffer {
			io.Copy(io.Discard, bodyReader)
		} else {
			raw, _ = readHTTPResponseBodyToEOF(bodyReader)
		}
		writeHTTPResponseBody(rawPacket, raw)
		if useContentLength && !useTransferEncodingChunked {
			rsp.ContentLength = int64(len(raw))
			shrinkHeader(rsp.Header, "content-length")
			rsp.Header.Set("Content-Length", strconv.Itoa(len(raw)))
		}
		responseBody = raw
	} else {
		// HEAD, TRACE, CONNECT requests should not have response body
		if nobodyReqMethod {
			// HEAD requests should never have a body, even if Transfer-Encoding: chunked is present
			// Skip body reading for HEAD requests to avoid blocking when connection is closed
			if req != nil && strings.EqualFold(req.Method, http.MethodHead) {
				// HEAD request: skip all body reading logic
				// This prevents blocking when server sends Transfer-Encoding: chunked but closes connection without body
			} else {
				// TRACE, CONNECT requests: try to discard body if present
				if useContentLength && contentLengthInt > 0 {
					_, err := io.Copy(io.Discard, io.LimitReader(bodyReader, int64(contentLengthInt)))
					if err != nil && !errors.Is(err, io.EOF) {
						return nil, errors.Wrap(err, "read body error")
					}
				} else {
					_, err := io.Copy(io.Discard, bodyReader)
					if err != nil && !errors.Is(err, io.EOF) {
						return nil, errors.Wrap(err, "read body error")
					}
				}
			}
		} else if useContentLength && useTransferEncodingChunked {
			// smuggle...
			log.Debug("content-length and transfer-encoding chunked both exist, try smuggle? use content-length first!")
			if contentLengthInt > 0 {
				// smuggle
				bodyRaw := []byte{}
				if noBodyBuffer {
					io.Copy(io.Discard, io.LimitReader(bodyReader, int64(contentLengthInt)))
				} else {
					bodyRaw, _ = readHTTPResponseBodyWithLimit(bodyReader, contentLengthInt)
				}
				writeHTTPResponseBody(rawPacket, bodyRaw)
				responseBody = padHTTPResponseBody(bodyRaw, contentLengthInt)
			} else {
				// chunked
				var fixed []byte
				var err error
				if noBodyBuffer {
					_, _, _, err = codec.HTTPChunkedDecoderWithRestBytes(bodyReader)
				} else {
					_, fixed, _, err = codec.HTTPChunkedDecoderWithRestBytes(bodyReader)
				}
				writeHTTPResponseBody(rawPacket, fixed)
				if err != nil {
					return nil, errors.Wrap(err, "chunked decoder error")
				}
				responseBody = fixed
			}
		} else if !useContentLength && useTransferEncodingChunked {
			// handle chunked
			var fixed []byte
			var err error
			if noBodyBuffer {
				_, _, _, err = codec.HTTPChunkedDecoderWithRestBytes(bodyReader)
			} else {
				_, fixed, _, err = codec.HTTPChunkedDecoderWithRestBytes(bodyReader)
			}
			writeHTTPResponseBody(rawPacket, fixed)
			if err != nil {
				return nil, errors.Wrap(err, "chunked decoder error")
			}
			if len(fixed) > 0 {
				responseBody = fixed
			}
		} else {
			// handle content-length as default
			if !nobodyReqMethod { // some request method will not have body
				if !useContentLength && rsp.StatusCode == http.StatusOK && hasEntityHeader {
					contentLengthInt = 100 * 1000 // no cl ,but maybe has body ,give 100k
				}
				if contentLengthInt > 0 {
					bodyRaw := []byte{}
					if noBodyBuffer {
						io.Copy(io.Discard, io.LimitReader(bodyReader, int64(contentLengthInt)))
					} else if discardBoundedBody {
						if prepareBodyCapacity != nil {
							prepareBodyCapacity(contentLengthInt)
						}
						discardReader := bodyReader
						if rawPacket != nil && borrowFinalPacket == nil {
							rawPacket.Grow(contentLengthInt)
							discardReader = io.TeeReader(bodyReader, rawPacket)
						}
						var copied int64
						copied, err = io.Copy(io.Discard, io.LimitReader(discardReader, int64(contentLengthInt)))
						discardedBodyBytes = int(copied)
					} else {
						bodyRaw, err = readHTTPResponseBodyWithLimit(bodyReader, contentLengthInt)
					}
					writeHTTPResponseBody(rawPacket, bodyRaw)
					if err != nil && err != io.EOF {
						return nil, errors.Wrap(err, "read body error")
					}
					if !discardBoundedBody {
						responseBody = padHTTPResponseBody(bodyRaw, contentLengthInt)
					}
				}
			}
		}
	}
	bodySize := len(responseBody)
	if discardBoundedBody {
		// Preserve the temporary response's historical padded Content-Length
		// accounting even though its Body is not retained.
		bodySize = contentLengthInt
	}
	if bodySize > 0 {
		httpctx.SetResponseBodySize(req, int64(bodySize))
	}
	if discardBoundedBody {
		rsp.Body = discardedIntermediateHTTPResponseBody{}
	} else if len(responseBody) == 0 {
		rsp.Body = http.NoBody
	} else {
		rsp.Body = newOwnedHTTPResponseBody(responseBody)
	}
	if req != nil {
		// set too large if greater than max content length
		maxContentLength := httpctx.GetResponseMaxContentLength(req)
		if maxContentLength > 0 && bodySize > maxContentLength {
			httpctx.SetResponseTooLarge(req, true)
		}

		if httpctx.GetResponseTooLarge(req) {
			httpctx.SetBareResponseBytes(req, headerBytes)
			uid := ksuid.New().String()
			suffix := fmt.Sprintf(`%v_%v`, time.Now().Format(DatetimePretty()), uid)
			fp, _ := OpenTempFile(fmt.Sprintf("large-response-header-%v.txt", suffix))
			if fp != nil {
				fp.Write(headerBytes)
				fp.Close()
				httpctx.SetResponseTooLargeHeaderFile(req, fp.Name())
			}
			fp, _ = OpenTempFile(fmt.Sprintf("large-response-body-%v.txt", suffix))
			if fp != nil {
				fp.Write(responseBody)
				fp.Close()
				httpctx.SetResponseTooLargeBodyFile(req, fp.Name())
			}
		} else {
			if discardBoundedBody && borrowFinalPacket != nil {
				finalPacketSize := rawPacket.Len() + discardedBodyBytes
				borrowedPacket := borrowFinalPacket(finalPacketSize)
				if len(borrowedPacket) != finalPacketSize {
					return nil, Errorf("invalid borrowed HTTP response packet: got %d bytes, want %d", len(borrowedPacket), finalPacketSize)
				}
				httpctx.SetBareResponseBytesForceBorrowed(req, borrowedPacket)
			} else {
				// rawPacket is local to this parser and is not accessed after the
				// return. Transfer it to httpctx without cloning; rsp.Body owns a
				// separate responseBody allocation.
				httpctx.SetBareResponseBytesForceOwned(req, rawPacket.Bytes())
			}
		}
	}
	return rsp, nil
}

type flusher interface {
	Flush() error
}

type flusher2 interface {
	Flush()
}

type flusher3 interface {
	Flush() (int, error)
}

type AutoFlushWriter struct {
	w io.Writer
}

func (w *AutoFlushWriter) Write(data []byte) (int, error) {
	n, err := w.w.Write(data)
	if err != nil {
		return n, err
	}
	FlushWriter(w.w)
	return n, nil
}

func WriterAutoFlush(writer io.Writer) *AutoFlushWriter {
	return &AutoFlushWriter{
		w: writer,
	}
}

func CloseConnSafe(conn net.Conn) {
	FlushWriter(conn)
	CloseWrite(conn)
	go func() {
		time.Sleep(50 * time.Millisecond)
		if err := conn.Close(); err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Errorf("failed to close connection: %v", err)
		}
	}()
}

func FlushWriter(writer io.Writer) {
	if f, ok := writer.(flusher); ok {
		err := f.Flush()
		if err != nil {
			log.Warnf("flush writer failed: %s", err)
		}
	} else if f, ok := writer.(flusher2); ok {
		f.Flush()
	} else if f, ok := writer.(flusher3); ok {
		_, err := f.Flush()
		if err != nil {
			log.Warnf("flush writer failed: %s", err)
		}
	}
}

func CloseWrite(i any) {
	switch ret := i.(type) {
	case interface{ CloseWrite() error }:
		if err := ret.CloseWrite(); err != nil {
			log.Errorf("close write failed: %s", err)
		}
		return
	case interface{ CloseWrite() }:
		ret.CloseWrite()
		return
	}
}

func CallGeneralClose(closer any) {
	if IsNil(closer) {
		return
	}
	switch ret := closer.(type) {
	case interface{ Close() error }:
		ret.Close()
	case interface{ Close() }:
		ret.Close()
	case interface{ Cancel() }:
		ret.Cancel()
	case interface{ Cancel() error }:
		ret.Cancel()
	}
}

func TCPNoDelay(i net.Conn) {
	if i == nil {
		return
	}
	if tcpConn, ok := i.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
		// disable write buffer
		_ = tcpConn.SetWriteBuffer(0)
	} else if tlsConn, ok := i.(*tls.Conn); ok {
		netc := tlsConn.NetConn()
		if tc, ok := netc.(*net.TCPConn); ok {
			tc.SetNoDelay(true)
			tc.SetWriteBuffer(0)
		}
	} else if utlsConn, ok := i.(*utls2.Conn); ok {
		netc := utlsConn.NetConn()
		if tc, ok := netc.(*net.TCPConn); ok {
			tc.SetNoDelay(true)
			tc.SetWriteBuffer(0)
		}
	}
}

const (
	CommonHeaderStat string = "common-header"
	HeaderCheckStat         = "header-Check"
)

func ScanHTTPHeaderWithHeaderFolding(reader io.Reader, headerCallback func(rawHeader []byte), prefix []byte) error {
	var headerRawCache []byte
	var currentSata = CommonHeaderStat
	var headerFoldingPrefix = make([]byte, 0)

	setHeaderFoldingPrefix := func(foldingPrefix []byte) {
		headerFoldingPrefix = foldingPrefix
	}

	setCurrentStat := func(stat string) {
		currentSata = stat
	}

	pushHeaderRawData := func(raw []byte) {
		if len(headerRawCache) == 0 {
			// ReadLine returns a distinct allocation for each line. Retain that
			// allocation directly until the following line proves whether this
			// header is folded, instead of copying every ordinary header once
			// more into headerRawCache.
			headerRawCache = raw
			return
		}
		headerRawCache = append(headerRawCache, raw...)
	}

	emitHeaderRaw := func() {
		if headerCallback != nil {
			headerCallback(headerRawCache)
		}
		headerRawCache = make([]byte, 0)
	}

	defer emitHeaderRaw()

	trimPrefix := func(raw []byte) []byte {
		minLen := Min(len(prefix), len(raw))
		i := 0
		for ; i < minLen; i++ {
			if raw[i] != prefix[i] {
				break
			}
		}
		return raw[i:]
	}

	for {
		lineBytes, err := ReadLine(reader)
		if err != nil && err != io.EOF {
			return errors.Wrap(err, "read HTTPResponse header failed")
		}
		lineBytes = trimPrefix(lineBytes)
	Retry:
		switch currentSata {
		case CommonHeaderStat:
			if len(lineBytes) == 0 {
				return nil
			}
			for i, b := range lineBytes {
				if b != ' ' && b != '\t' {
					setHeaderFoldingPrefix(lineBytes[:i])
					break
				}
			}
			pushHeaderRawData(lineBytes)
			setCurrentStat(HeaderCheckStat)
		case HeaderCheckStat:
			checkLine := bytes.TrimPrefix(lineBytes, headerFoldingPrefix)
			if len(checkLine) > 0 && (checkLine[0] == ' ' || checkLine[0] == '\t') {
				headerRawCache = append(headerRawCache, '\r', '\n')
				headerRawCache = append(headerRawCache, checkLine...)
			} else {
				emitHeaderRaw()
				setCurrentStat(CommonHeaderStat)
				goto Retry
			}
		}
	}
}

func ScanHTTPHeaderSimple(reader io.Reader, headerCallback func(rawHeader []byte), prefix []byte) error {
	emitHeaderRaw := func(raw []byte) {
		if headerCallback != nil {
			headerCallback(raw)
		}
	}
	trimPrefix := func(raw []byte) []byte {
		minLen := Min(len(prefix), len(raw))
		i := 0
		for ; i < minLen; i++ {
			if raw[i] != prefix[i] {
				break
			}
		}
		return raw[i:]
	}

	for {
		lineBytes, err := ReadLine(reader)
		if err != nil && err != io.EOF {
			return errors.Wrap(err, "read HTTPResponse header failed")
		}
		lineBytes = trimPrefix(lineBytes)
		if len(bytes.TrimSpace(lineBytes)) == 0 {
			emitHeaderRaw(nil)
			return nil
		}
		emitHeaderRaw(lineBytes)
	}
}

func ScanHTTPHeader(reader io.Reader, headerCallback func(rawHeader []byte), prefix []byte, isResp bool) error {
	if isResp {
		return ScanHTTPHeaderWithHeaderFolding(reader, headerCallback, prefix)
	}
	return ScanHTTPHeaderSimple(reader, headerCallback, prefix)
}
