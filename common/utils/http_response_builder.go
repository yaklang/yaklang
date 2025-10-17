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
	"unicode"

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
	rsp, err := readHTTPResponseFromBufioReader(reader, false, req, nil)
	if err != nil {
		return nil, err
	}
	rsp.Request = req
	return rsp, nil
}

type FileOpenerType func(s string) (*os.File, error)

var (
	tempFileOpener    FileOpenerType
	constsTempFileDir = filepath.Join(GetHomeDirDefault("."), "yakit-projects", "temp")
)

func RegisterTempFileOpener(dialer FileOpenerType) {
	tempFileOpener = dialer
}

func OpenTempFile(s string) (*os.File, error) {
	if tempFileOpener != nil {
		return tempFileOpener(s)
	}

	if !IsDir(constsTempFileDir) {
		_ = os.MkdirAll(constsTempFileDir, 0o755)
	}
	return os.OpenFile(filepath.Join(constsTempFileDir, s), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
}

func ReadHTTPResponseFromBufioReaderConn(reader io.Reader, conn net.Conn, req *http.Request) (*http.Response, error) {
	rsp, err := readHTTPResponseFromBufioReader(reader, false, req, conn)
	if err != nil {
		return nil, err
	}
	rsp.Request = req
	return rsp, nil
}

func ReadHTTPResponseFromBytes(raw []byte, req *http.Request) (*http.Response, error) {
	rsp, err := readHTTPResponseFromBufioReader(bytes.NewReader(raw), true, req, nil)
	if err != nil {
		return nil, err
	}
	rsp.Request = req
	return rsp, nil
}

func readHTTPResponseFromBufioReader(originReader io.Reader, fixContentLength bool, req *http.Request, conn net.Conn) (*http.Response, error) {
	rawPacket := new(bytes.Buffer)
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
	rsp.Proto, rsp.StatusCode, statusText, _ = ParseHTTPResponseLine(string(firstLine))

HandleExpect100Continue:
	// Expect: 100-continue cause the first line is not the real first line
	if rsp.StatusCode == 100 && strings.ToLower(statusText) == "continue" {
		for {
			firstLine, err = ReadLine(headerReader)
			if err != nil {
				return nil, errors.Wrap(err, "read HTTPResponse firstline failed")
			}
			if string(bytes.TrimSpace(firstLine)) == "" {
				continue
			} else {
				break
			}
		}
		rsp.Proto, rsp.StatusCode, statusText, _ = ParseHTTPResponseLine(string(firstLine))
		goto HandleExpect100Continue
	}
	rawPacket.Write(firstLine)
	rawPacket.WriteString(CRLF)
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
		if len(rawHeader) <= 0 {
			rawPacket.WriteString(CRLF)
			return
		}
		rawPacket.Write(rawHeader)
		rawPacket.WriteString(CRLF)

		before, after, _ := bytes.Cut(rawHeader, []byte{':'})
		keyStr := string(before)
		valStr := strings.TrimLeftFunc(string(after), unicode.IsSpace)

		if _, isCommonHeader := commonHeader[keyStr]; isCommonHeader {
			keyStr = http.CanonicalHeaderKey(keyStr)
		}

		lowerKey := strings.ToLower(keyStr)
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

	bodyRawBuf := new(bytes.Buffer)
	if fixContentLength {
		// just for bytes condition
		// by reader
		raw := []byte{}
		if noBodyBuffer {
			io.Copy(io.Discard, bodyReader)
		} else {
			raw, _ = io.ReadAll(io.NopCloser(bodyReader))
		}
		rawPacket.Write(raw)
		if useContentLength && !useTransferEncodingChunked {
			rsp.ContentLength = int64(len(raw))
			shrinkHeader(rsp.Header, "content-length")
			rsp.Header.Set("Content-Length", strconv.Itoa(len(raw)))
		}
		bodyRawBuf.Write(raw)
	} else {
		// by header
		if useContentLength && useTransferEncodingChunked {
			// smuggle...
			log.Debug("content-length and transfer-encoding chunked both exist, try smuggle? use content-length first!")
			if contentLengthInt > 0 {
				// smuggle
				bodyRaw := []byte{}
				if noBodyBuffer {
					io.Copy(io.Discard, io.LimitReader(bodyReader, int64(contentLengthInt)))
				} else {
					bodyRaw, _ = io.ReadAll(io.NopCloser(io.LimitReader(bodyReader, int64(contentLengthInt))))
				}
				rawPacket.Write(bodyRaw)
				bodyRawBuf.Write(bodyRaw)
				if ret := contentLengthInt - len(bodyRaw); ret > 0 {
					bodyRawBuf.WriteString(strings.Repeat("\n", ret))
				}
			} else {
				// chunked
				var fixed []byte
				var err error
				if noBodyBuffer {
					_, _, _, err = codec.HTTPChunkedDecoderWithRestBytes(bodyReader)
				} else {
					_, fixed, _, err = codec.HTTPChunkedDecoderWithRestBytes(bodyReader)
				}
				rawPacket.Write(fixed)
				if err != nil {
					return nil, errors.Wrap(err, "chunked decoder error")
				}
				bodyRawBuf.Write(fixed)
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
			rawPacket.Write(fixed)
			if err != nil {
				return nil, errors.Wrap(err, "chunked decoder error")
			}
			if len(fixed) > 0 {
				bodyRawBuf.Write(fixed)
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
					} else {
						bodyRaw, err = io.ReadAll(io.NopCloser(io.LimitReader(bodyReader, int64(contentLengthInt))))
					}
					rawPacket.Write(bodyRaw)
					if err != nil && err != io.EOF {
						return nil, errors.Wrap(err, "read body error")
					}
					bodyLen := len(bodyRaw)
					bodyRawBuf.Write(bodyRaw)
					bodyRawBuf.WriteString(strings.Repeat("\n", contentLengthInt-bodyLen))
				}
			}
		}
	}
	bodySize := bodyRawBuf.Len()
	if bodySize == 0 {
		rsp.Body = http.NoBody
	} else {
		httpctx.SetResponseBodySize(req, int64(bodySize))
		rsp.Body = io.NopCloser(bodyRawBuf)
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
				fp.Write(bodyRawBuf.Bytes())
				fp.Close()
				httpctx.SetResponseTooLargeBodyFile(req, fp.Name())
			}
		} else {
			httpctx.SetBareResponseBytesForce(req, rawPacket.Bytes())
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
				pushHeaderRawData(append([]byte(CRLF), checkLine...))
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
