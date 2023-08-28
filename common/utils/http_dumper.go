package utils

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"net/http"
	"strings"
)

var _noContentLengthHeader = map[string]bool{
	"GET":     true,
	"HEAD":    true,
	"DELETE":  true,
	"OPTIONS": true,
	"CONNECT": true,
	"get":     true,
	"head":    true,
	"delete":  true,
	"options": true,
	"connect": true,
}

func ShouldRemoveZeroContentLengthHeader(s string) bool {
	_, ok := _noContentLengthHeader[s]
	return ok
}

const CRLF = "\r\n"

func getHeaderValueAll(header http.Header, key string) string {
	return strings.Join(getHeaderValueList(header, key), ", ")
}

func getHeaderValue(header http.Header, key string) string {
	vals := getHeaderValueList(header, key)
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func getHeaderValueList(header http.Header, key string) []string {
	if header == nil {
		return nil
	}
	cKey := http.CanonicalHeaderKey(key)
	if key == cKey {
		return []string{header.Get(key)}
	}

	v1, _ := header[key]
	v2, _ := header[cKey]
	vals := make([]string, 0, len(v1)+len(v2))
	var m = map[string]any{}
	for _, v := range [][]string{v1, v2} {
		for _, i := range v {
			if i == "" {
				continue
			}
			if _, ok := m[i]; ok {
				continue
			}
			m[i] = i
			vals = append(vals, i)
		}
	}
	return vals
}

func shrinkHeader(header http.Header, key string) {
	values := getHeaderValueList(header, key)
	delete(header, key)
	delete(header, http.CanonicalHeaderKey(key))
	if len(values) > 0 {
		header[http.CanonicalHeaderKey(key)] = values
	}
}

func DumpHTTPResponse(rsp *http.Response) ([]byte, error) {
	return nil, Error("not implemented")
}

// DumpHTTPRequest dumps http request to bytes
// **NO NOT HANDLE SMUGGLE HERE!**
// Transfer-Encoding is handled vai req.TransferEncoding / req.Header["Transfer-Encoding"]
// Content-Length is handled vai req.ContentLength / req.Header["Content-Length"]
// if Transfer-Encoding existed, check body chunked? if not, encode it
// if Transfer-Encoding and Content-Length existed at same time, use transfer-encoding
func DumpHTTPRequest(req *http.Request, loadBody bool) ([]byte, error) {
	if req == nil {
		return nil, Error("nil request")
	}
	var (
		h2                      bool
		transferEncodingChunked bool
		contentLengthExisted    bool
		contentLengthInt        int64
	)
	_ = contentLengthInt
	if len(req.TransferEncoding) > 0 {
		for _, v := range req.TransferEncoding {
			if v == "chunked" {
				transferEncodingChunked = true
				break
			}
		}
	}

	if req.Header.Get("Transfer-Encoding") == "chunked" {
		transferEncodingChunked = true
	}
	if te2, haveTransferEncoding := req.Header["transfer-encoding"]; haveTransferEncoding {
		if strings.Contains(strings.Join(te2, ", "), "chunked") {
			transferEncodingChunked = true
		}
	}

	if req.ProtoMajor == 2 || strings.HasPrefix(req.Proto, "HTTP/2") {
		h2 = true
	}

	if ret := getHeaderValue(req.Header, "content-length"); ret != "" || req.ContentLength > 0 {
		contentLengthExisted = true
		if ret != "" {
			contentLengthInt = int64(Atoi(ret))
		} else {
			contentLengthInt = req.ContentLength
		}
	}

	var buf bytes.Buffer
	buf.WriteString(req.Method)
	buf.WriteString(" ")
	if req.RequestURI == "" {
		buf.WriteString(req.URL.RequestURI())
	} else {
		buf.WriteString(req.RequestURI)
	}
	buf.WriteString(" ")
	if h2 {
		req.Proto = "HTTP/2.0"
	} else {
		req.Proto = fmt.Sprint("HTTP/", req.ProtoMajor, ".", req.ProtoMinor)
	}
	buf.WriteString(req.Proto)
	buf.WriteString(CRLF)

	// handle host
	buf.WriteString("Host: ")
	if ret := getHeaderValue(req.Header, "host"); ret == "" {
		if req.Host != "" {
			buf.WriteString(req.Host)
		} else if req.URL.Host != "" {
			buf.WriteString(req.URL.Host)
		}
	} else {
		buf.WriteString(ret)
	}
	buf.WriteString(CRLF)
	shrinkHeader(req.Header, "content-type")

	for k := range req.Header {
		switch strings.ToLower(k) {
		case "host", "content-length", "transfer-encoding":
			continue
		}
		val := getHeaderValueAll(req.Header, k)
		buf.WriteString(k)
		buf.WriteString(": ")
		buf.WriteString(val)
		buf.WriteString(CRLF)
	}

	if req.Body == nil {
		req.Body = http.NoBody
	}
	rawBody, _ := io.ReadAll(req.Body)
	haveBody := len(rawBody) > 0
	// handle cl / te
	if transferEncodingChunked {
		buf.WriteString("Transfer-Encoding: chunked\r\n")
		// check body is chunked or not
		// if not, encode it
		if haveBody {
			decoded, fixed, _ := codec.ReadHTTPChunkedDataWithFixed(rawBody)
			if len(decoded) == 0 {
				rawBody = codec.HTTPChunkedEncode(rawBody)
			} else {
				rawBody = fixed
			}
		}
	} else {
		if haveBody || !ShouldRemoveZeroContentLengthHeader(req.Method) || contentLengthExisted {
			buf.WriteString("Content-Length: ")
			buf.WriteString(fmt.Sprint(len(rawBody)))
			buf.WriteString(CRLF)
		}
	}

	buf.WriteString(CRLF)
	if loadBody {
		buf.Write(rawBody)
	}
	return buf.Bytes(), nil
}
