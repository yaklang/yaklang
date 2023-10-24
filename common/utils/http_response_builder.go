package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
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

func ReadHTTPResponseFromBufioReader(reader *bufio.Reader, req *http.Request) (*http.Response, error) {
	rsp, err := readHTTPResponseFromBufioReader(reader, false, req, nil)
	if err != nil {
		return nil, err
	}
	rsp.Request = req
	return rsp, nil
}

func ReadHTTPResponseFromBufioReaderConn(reader *bufio.Reader, conn net.Conn, req *http.Request) (*http.Response, error) {
	rsp, err := readHTTPResponseFromBufioReader(reader, false, req, conn)
	if err != nil {
		return nil, err
	}
	rsp.Request = req
	return rsp, nil
}

func ReadHTTPResponseFromBytes(raw []byte, req *http.Request) (*http.Response, error) {
	rsp, err := readHTTPResponseFromBufioReader(bufio.NewReader(bytes.NewReader(raw)), true, req, nil)
	if err != nil {
		return nil, err
	}
	rsp.Request = req
	return rsp, nil
}

func readHTTPResponseFromBufioReader(reader *bufio.Reader, fixContentLength bool, req *http.Request, conn net.Conn) (*http.Response, error) {
	var rawPacket = new(bytes.Buffer)

	var rsp = &http.Response{
		Header:           nil,
		Body:             nil,
		ContentLength:    0,
		TransferEncoding: nil,
		Close:            false,
		Uncompressed:     false,
		Trailer:          nil,
		Request:          nil,
		TLS:              nil,
	}
	firstLine, err := BufioReadLine(reader)
	if err != nil {
		return nil, errors.Wrap(err, "read HTTPResponse firstline failed")
	}
	rawPacket.Write(firstLine)
	rawPacket.WriteString(CRLF)

	var statusText string
	rsp.Proto, rsp.StatusCode, statusText, _ = ParseHTTPResponseLine(string(firstLine))
	rsp.Status = fmt.Sprintf("%v %s", rsp.StatusCode, statusText)
	_, after, _ := strings.Cut(rsp.Proto, "/")
	major, minor, _ := strings.Cut(after, ".")
	rsp.ProtoMajor = codec.Atoi(major)
	rsp.ProtoMinor = codec.Atoi(minor)
	if rsp.StatusCode < 100 {
		return nil, Errorf("invalid first line: %v", strconv.Quote(string(firstLine)))
	}

	// header
	var header = make(http.Header)
	var useContentLength = false
	var contentLengthInt = 0
	var useTransferEncodingChunked = false
	var defaultClose = (rsp.ProtoMajor == 1 && rsp.ProtoMinor == 0) || rsp.ProtoMajor < 1

	for {
		lineBytes, err := BufioReadLine(reader)
		if err != nil {
			return nil, errors.Wrap(err, "read HTTPResponse header failed")
		}
		if len(bytes.TrimSpace(lineBytes)) == 0 {
			rawPacket.WriteString(CRLF)
			break
		}
		rawPacket.Write(lineBytes)
		rawPacket.WriteString(CRLF)

		before, after, _ := bytes.Cut(lineBytes, []byte{':'})
		keyStr := string(before)
		valStr := strings.TrimLeftFunc(string(after), unicode.IsSpace)

		if _, isCommonHeader := commonHeader[keyStr]; isCommonHeader {
			keyStr = http.CanonicalHeaderKey(keyStr)
		}

		switch strings.ToLower(keyStr) {
		case "content-length":
			useContentLength = true
			contentLengthInt = codec.Atoi(valStr)
			if contentLengthInt != 0 {
				header.Set(keyStr, valStr)
				rsp.ContentLength = int64(contentLengthInt)
			}
		case "transfer-encoding":
			rsp.TransferEncoding = []string{valStr}
			if strings.EqualFold(valStr, "chunked") {
				useTransferEncodingChunked = true
			}
		case "connection":
			if strings.EqualFold(valStr, "close") {
				defaultClose = true
			} else if strings.EqualFold(valStr, "keep-alive") {
				defaultClose = false
			}
		}
		// add header
		if keyStr == "" {
			continue
		}
		header[keyStr] = append(header[keyStr], valStr)
	}
	rsp.Close = defaultClose
	rsp.Header = header

	var bodyRawBuf = new(bytes.Buffer)
	var peekabledData = new(bytes.Buffer)
	if conn != nil {
		if !useContentLength && !useTransferEncodingChunked {
			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		}
		bt, err := reader.ReadByte()
		if !useContentLength && !useTransferEncodingChunked {
			_ = conn.SetReadDeadline(time.Time{})
		}
		if err != nil {
			rsp.Body = http.NoBody
			if req != nil {
				rsp.Request = req
				httpctx.SetBareResponseBytes(req, rawPacket.Bytes())
			}
			return rsp, err
		} else {
			peekabledData.WriteByte(bt)
		}
	}

	getPeekableReader := func() io.Reader {
		if conn == nil {
			return reader
		}
		return io.MultiReader(peekabledData, reader)
	}

	if fixContentLength || (!useContentLength && !useTransferEncodingChunked) { //在设置修复长度,或者CL合Chunk两者都没有的情况下尽可能的读取
		// by reader
		raw, _ := ReadConnUntilStable(getPeekableReader(), conn, 5*time.Second, 300*time.Millisecond)
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
				bodyRaw, _ := io.ReadAll(io.NopCloser(io.LimitReader(getPeekableReader(), int64(contentLengthInt))))
				rawPacket.Write(bodyRaw)
				bodyRawBuf.Write(bodyRaw)
				if ret := contentLengthInt - len(bodyRaw); ret > 0 {
					bodyRawBuf.WriteString(strings.Repeat("\n", ret))
				}
			} else {
				// chunked
				_, fixed, _, err := codec.HTTPChunkedDecoderWithRestBytes(getPeekableReader())
				rawPacket.Write(fixed)
				if err != nil {
					return nil, errors.Wrap(err, "chunked decoder error")

				}
				bodyRawBuf.Write(fixed)
			}
		} else if !useContentLength && useTransferEncodingChunked {
			// handle chunked
			_, fixed, _, err := codec.HTTPChunkedDecoderWithRestBytes(getPeekableReader())
			rawPacket.Write(fixed)
			if err != nil {
				return nil, errors.Wrap(err, "chunked decoder error")
			}
			if len(fixed) > 0 {
				bodyRawBuf.Write(fixed)
			}
		} else {
			// handle content-length as default
			if contentLengthInt > 0 {
				var bodyRaw, err = io.ReadAll(io.NopCloser(io.LimitReader(getPeekableReader(), int64(contentLengthInt))))
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
	if bodyRawBuf.Len() == 0 {
		rsp.Body = http.NoBody
	} else {
		rsp.Body = io.NopCloser(bodyRawBuf)
	}
	if req != nil {
		httpctx.SetBareResponseBytes(req, rawPacket.Bytes())
	}
	return rsp, nil
}
