package lowhttp

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
	"strconv"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

const _autoUnzipMaxDecodedBodyBytes = 32 << 20

type _contentAlgo uint8

const (
	_contentAlgoNone _contentAlgo = iota
	_contentAlgoGzip
	_contentAlgoBrotli
	_contentAlgoZstd
	_contentAlgoZlib
	_contentAlgoDeflateRaw
)

type _unzipPacketEncodingConfig struct {
	enableMagic     bool
	conservative    bool
	maxDecodedBytes int
}

type _unzipPacketEncodingOption func(*_unzipPacketEncodingConfig)

func _defaultUnzipPacketEncodingConfig() _unzipPacketEncodingConfig {
	return _unzipPacketEncodingConfig{
		enableMagic:     true,
		conservative:    true,
		maxDecodedBytes: _autoUnzipMaxDecodedBodyBytes,
	}
}

func _withUnzipPacketEncodingEnableMagic(v bool) _unzipPacketEncodingOption {
	return func(c *_unzipPacketEncodingConfig) { c.enableMagic = v }
}

func _withUnzipPacketEncodingConservative(v bool) _unzipPacketEncodingOption {
	return func(c *_unzipPacketEncodingConfig) { c.conservative = v }
}

func _withUnzipPacketEncodingMaxDecodedBytes(v int) _unzipPacketEncodingOption {
	return func(c *_unzipPacketEncodingConfig) { c.maxDecodedBytes = v }
}

// PacketEncodingState records the removed encoding headers so that the packet can be
// re-encoded after being edited in plain form (used by manual hijack).
type PacketEncodingState struct {
	ContentEncoding  string // original Content-Encoding header value (without key)
	TransferEncoding string // original Transfer-Encoding header value (without key), when chunked
	WasChunked       bool
	hadContentEncHdr bool
	hadChunkedTEHdr  bool
	detectedAlgo     _contentAlgo // actual decoded algorithm (by header or magic), if any
}

// AutoUnzipPacketEncoding 是一个辅助函数，用于将 HTTP 报文中的传输/内容编码“解开”，以便前端展示/编辑。
//
// - 支持处理 Transfer-Encoding: chunked（会自动反分块，并移除相关头）
// - 支持处理 Content-Encoding（如 gzip/br/zstd/zlib/deflate），优先按 header 解码；header 缺失时会尝试用 magic number 判断（如 gzip/zstd/zlib）
// - 失败时保持保守：返回 (raw, nil, false)，避免破坏原始报文
//
// 该函数通常与 AutoZipPacketEncoding 配对使用：前端编辑 plain 报文后，服务端可用 state 将其重新压回原始编码形态。
//
// Example:
// ```
// raw := []byte(`HTTP/1.1 200 OK
// Transfer-Encoding: chunked
//
// 5
// hello
// 0
//
// `)
// plain, state, ok = poc.AutoUnzipPacketEncoding(raw)
// // plain 的 body 变为 "hello"，并移除了 Transfer-Encoding: chunked
// ```
func AutoUnzipPacketEncoding(raw []byte) (plain []byte, state *PacketEncodingState, ok bool) {
	return _unzipPacketEncodingInternal(raw, _defaultUnzipPacketEncodingConfig())
}

func _unzipPacketEncodingInternal(raw []byte, cfg _unzipPacketEncodingConfig, opt ..._unzipPacketEncodingOption) (plain []byte, state *PacketEncodingState, ok bool) {
	for _, o := range opt {
		if o != nil {
			o(&cfg)
		}
	}

	var (
		encoding         string
		transferEncoding string
		isChunked        bool
		hadCEHeader      bool
		hadChunkedTE     bool
		buf              bytes.Buffer
	)

	_, body := SplitHTTPPacket(raw,
		func(method string, requestUri string, proto string) error {
			buf.WriteString(method + " " + requestUri + " " + proto + CRLF)
			return nil
		},
		func(proto string, code int, codeMsg string) error {
			buf.WriteString(proto + " " + strconv.Itoa(code) + " " + codeMsg + CRLF)
			return nil
		},
		func(line string) string {
			k, v := SplitHTTPHeader(line)
			switch strings.ToLower(strings.TrimSpace(k)) {
			case "content-encoding":
				hadCEHeader = true
				encoding = strings.TrimSpace(v)
				return ""
			case "transfer-encoding":
				if utils.IContains(v, "chunked") {
					isChunked = true
					hadChunkedTE = true
					transferEncoding = strings.TrimSpace(v)
					return ""
				}
			}
			buf.WriteString(line + CRLF)
			return line
		},
	)
	buf.WriteString(CRLF)

	// Unchunk first.
	unchunkedApplied := false
	if isChunked {
		unchunked, chunkErr := codec.HTTPChunkedDecode(body)
		if unchunked != nil {
			body = unchunked
			unchunkedApplied = true
		} else {
			if chunkErr == nil {
				body = []byte{}
				unchunkedApplied = true
			} else if cfg.conservative {
				return raw, nil, false
			}
		}
	}

	detected := _contentAlgoNone
	contentDecoded := false

	// Decode content-encoding (try header first, optionally fallback to magic).
	if strings.TrimSpace(encoding) != "" {
		if decoded, algo, decodedOK := _decodeByHeaderOrMagic(encoding, body, cfg.enableMagic, cfg.maxDecodedBytes); decodedOK {
			body = decoded
			detected = algo
			contentDecoded = true
		} else if cfg.conservative {
			return raw, nil, false
		}
	} else {
		if cfg.enableMagic {
			algo := _detectByMagic(body)
			if algo != _contentAlgoNone {
				if decoded, decodedOK := _decodeBody(algo, body, cfg.maxDecodedBytes); decodedOK {
					body = decoded
					detected = algo
					contentDecoded = true
				} else if cfg.conservative {
					return raw, nil, false
				}
			}
		}
	}

	didWork := unchunkedApplied || contentDecoded || hadCEHeader || hadChunkedTE
	if !didWork {
		return raw, nil, false
	}

	plain = ReplaceHTTPPacketBody(buf.Bytes(), body, false)
	state = &PacketEncodingState{
		ContentEncoding:  encoding,
		TransferEncoding: transferEncoding,
		WasChunked:       isChunked,
		hadContentEncHdr: hadCEHeader,
		hadChunkedTEHdr:  hadChunkedTE,
		detectedAlgo:     detected,
	}
	return plain, state, didWork
}

// AutoZipPacketEncoding re-encodes an edited plain packet back to its original encoding,
// using state returned by AutoUnzipPacketEncoding.
//
// If the packet already contains Content-Encoding / Transfer-Encoding(chunked), it will use
// the user's headers (and still normalize Content-Length / chunking).
func AutoZipPacketEncoding(plain []byte, state *PacketEncodingState) (encoded []byte, ok bool) {
	if state == nil {
		return plain, false
	}

	var (
		firstLine string
		headers   []string
		body      []byte

		userCE string
		userTE string
	)

	_, body = SplitHTTPPacketEx(plain, nil, nil, func(fl string) error {
		firstLine = fl
		return nil
	}, func(line string) string {
		k, v := SplitHTTPHeader(line)
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "content-encoding":
			userCE = strings.TrimSpace(v)
			return ""
		case "transfer-encoding":
			if utils.IContains(v, "chunked") {
				userTE = strings.TrimSpace(v)
			}
			return ""
		case "content-length":
			return ""
		default:
			headers = append(headers, line)
			return line
		}
	})
	if firstLine == "" {
		return plain, false
	}

	ceHeader := strings.TrimSpace(userCE)
	writeCEHeader := ceHeader != ""
	if ceHeader == "" && state.hadContentEncHdr {
		ceHeader = strings.TrimSpace(state.ContentEncoding)
		writeCEHeader = ceHeader != ""
	}

	algo := _contentAlgoNone
	if ceHeader != "" {
		algo = _algoFromHeader(ceHeader, state.detectedAlgo)
	} else if state.detectedAlgo != _contentAlgoNone {
		// header absent originally and user didn't set it: keep header absent but re-encode to original algo.
		algo = state.detectedAlgo
	}

	te := strings.TrimSpace(userTE)
	if te == "" && state.WasChunked {
		te = strings.TrimSpace(state.TransferEncoding)
		if te == "" {
			te = "chunked"
		}
	}
	chunked := te != "" && utils.IContains(te, "chunked")

	if body == nil {
		body = []byte{}
	}

	// Encode (content-encoding) then chunk (transfer-encoding).
	if algo != _contentAlgoNone {
		encBody, did := _encodeBody(algo, body)
		if !did {
			return plain, false
		}
		body = encBody
	}

	if chunked {
		body = codec.HTTPChunkedEncode(body)
	}

	var buf bytes.Buffer
	buf.WriteString(firstLine + CRLF)
	for _, h := range headers {
		buf.WriteString(h + CRLF)
	}
	if writeCEHeader {
		buf.WriteString("Content-Encoding: " + ceHeader + CRLF)
	}
	if chunked {
		buf.WriteString("Transfer-Encoding: " + te + CRLF)
	} else {
		buf.WriteString("Content-Length: " + strconv.Itoa(len(body)) + CRLF)
	}
	buf.WriteString(CRLF)
	buf.Write(body)
	return buf.Bytes(), true
}

func _readAllLimited(r io.Reader, max int) ([]byte, bool) {
	maxInt := int(^uint(0) >> 1)
	if max <= 0 || max >= maxInt {
		raw, err := io.ReadAll(r)
		if err != nil {
			return nil, false
		}
		return raw, true
	}

	raw, err := io.ReadAll(io.LimitReader(r, int64(max)+1))
	if err != nil {
		return nil, false
	}
	if len(raw) > max {
		return nil, false
	}
	return raw, true
}

func _isZlibHeader(body []byte) bool {
	// zlib header: CMF/FLG, see RFC 1950.
	if len(body) < 2 {
		return false
	}
	cmf := body[0]
	flg := body[1]
	if cmf&0x0F != 8 { // deflate
		return false
	}
	if (int(cmf)<<8+int(flg))%31 != 0 {
		return false
	}
	// FDICT set means preset dictionary is required; we can't decode without it.
	if flg&0x20 != 0 {
		return false
	}
	return true
}

func _detectByMagic(body []byte) _contentAlgo {
	if len(body) >= 3 && body[0] == 0x1f && body[1] == 0x8b && body[2] == 0x08 {
		return _contentAlgoGzip
	}
	if len(body) >= 4 && bytes.Equal(body[:4], []byte{0x28, 0xB5, 0x2F, 0xFD}) {
		return _contentAlgoZstd
	}
	if _isZlibHeader(body) {
		return _contentAlgoZlib
	}
	return _contentAlgoNone
}

func _decodeByHeaderOrMagic(contentEncoding string, bodyRaw []byte, enableMagic bool, maxDecoded int) (decoded []byte, algo _contentAlgo, ok bool) {
	encoding := strings.TrimSpace(contentEncoding)
	candidates := _candidatesFromHeader(encoding, bodyRaw)
	for _, a := range candidates {
		if out, ok := _decodeBody(a, bodyRaw, maxDecoded); ok {
			return out, a, true
		}
	}
	if enableMagic {
		if a := _detectByMagic(bodyRaw); a != _contentAlgoNone {
			if out, ok := _decodeBody(a, bodyRaw, maxDecoded); ok {
				return out, a, true
			}
		}
	}
	return nil, _contentAlgoNone, false
}

func _candidatesFromHeader(contentEncoding string, bodyRaw []byte) []_contentAlgo {
	switch true {
	case utils.IContains(contentEncoding, "gzip"):
		return []_contentAlgo{_contentAlgoGzip}
	case utils.IContains(contentEncoding, "br"):
		return []_contentAlgo{_contentAlgoBrotli}
	case utils.IContains(contentEncoding, "zstd"):
		return []_contentAlgo{_contentAlgoZstd}
	case utils.IContains(contentEncoding, "zlib"):
		return []_contentAlgo{_contentAlgoZlib}
	case utils.IContains(contentEncoding, "deflate"):
		// Many servers send "deflate" but actually mean zlib-wrapped DEFLATE.
		// Use magic to pick preferred order.
		if _isZlibHeader(bodyRaw) {
			return []_contentAlgo{_contentAlgoZlib, _contentAlgoDeflateRaw}
		}
		return []_contentAlgo{_contentAlgoDeflateRaw, _contentAlgoZlib}
	default:
		return nil
	}
}

func _decodeBody(algo _contentAlgo, bodyRaw []byte, maxDecoded int) (finalResult []byte, ok bool) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("handle content-encoding decode failed! reason: %s", err)
			finalResult = bodyRaw
			ok = false
		}
	}()

	switch algo {
	case _contentAlgoGzip:
		r, err := gzip.NewReader(bytes.NewReader(bodyRaw))
		if err != nil {
			return bodyRaw, false
		}
		defer r.Close()
		if out, ok := _readAllLimited(r, maxDecoded); ok {
			return out, true
		}
		return bodyRaw, false
	case _contentAlgoBrotli:
		r := brotli.NewReader(bytes.NewReader(bodyRaw))
		if out, ok := _readAllLimited(r, maxDecoded); ok {
			return out, true
		}
		return bodyRaw, false
	case _contentAlgoZstd:
		r, err := zstd.NewReader(bytes.NewReader(bodyRaw))
		if err != nil {
			return bodyRaw, false
		}
		defer r.Close()
		if out, ok := _readAllLimited(r, maxDecoded); ok {
			return out, true
		}
		return bodyRaw, false
	case _contentAlgoZlib:
		r, err := zlib.NewReader(bytes.NewReader(bodyRaw))
		if err != nil {
			return bodyRaw, false
		}
		defer r.Close()
		if out, ok := _readAllLimited(r, maxDecoded); ok {
			return out, true
		}
		return bodyRaw, false
	case _contentAlgoDeflateRaw:
		r := flate.NewReader(bytes.NewReader(bodyRaw))
		defer r.Close()
		if out, ok := _readAllLimited(r, maxDecoded); ok {
			return out, true
		}
		return bodyRaw, false
	default:
		return bodyRaw, false
	}
}

func _algoFromHeader(contentEncoding string, fallback _contentAlgo) _contentAlgo {
	// Normalize identity / unsupported encodings.
	if utils.IContains(contentEncoding, "identity") || utils.IContains(contentEncoding, "*") {
		return _contentAlgoNone
	}
	switch true {
	case utils.IContains(contentEncoding, "gzip"):
		return _contentAlgoGzip
	case utils.IContains(contentEncoding, "br"):
		return _contentAlgoBrotli
	case utils.IContains(contentEncoding, "zstd"):
		return _contentAlgoZstd
	case utils.IContains(contentEncoding, "zlib"):
		return _contentAlgoZlib
	case utils.IContains(contentEncoding, "deflate"):
		// Prefer the detected original algorithm when possible (zlib vs raw deflate).
		if fallback == _contentAlgoZlib || fallback == _contentAlgoDeflateRaw {
			return fallback
		}
		return _contentAlgoZlib
	default:
		return _contentAlgoNone
	}
}

func _encodeBody(algo _contentAlgo, bodyRaw []byte) (finalResult []byte, encoded bool) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("handle content-encoding encode failed! reason: %s", err)
			finalResult = bodyRaw
			encoded = false
		}
	}()

	switch algo {
	case _contentAlgoGzip:
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		_, err := w.Write(bodyRaw)
		if err != nil {
			_ = w.Close()
			return bodyRaw, false
		}
		if err := w.Close(); err != nil {
			return bodyRaw, false
		}
		return buf.Bytes(), true
	case _contentAlgoBrotli:
		var buf bytes.Buffer
		w := brotli.NewWriter(&buf)
		_, err := w.Write(bodyRaw)
		if err != nil {
			_ = w.Close()
			return bodyRaw, false
		}
		if err := w.Close(); err != nil {
			return bodyRaw, false
		}
		return buf.Bytes(), true
	case _contentAlgoZstd:
		var buf bytes.Buffer
		w, err := zstd.NewWriter(&buf)
		if err != nil {
			return bodyRaw, false
		}
		_, err = w.Write(bodyRaw)
		if err != nil {
			_ = w.Close()
			return bodyRaw, false
		}
		if err := w.Close(); err != nil {
			return bodyRaw, false
		}
		return buf.Bytes(), true
	case _contentAlgoZlib:
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		_, err := w.Write(bodyRaw)
		if err != nil {
			_ = w.Close()
			return bodyRaw, false
		}
		if err := w.Close(); err != nil {
			return bodyRaw, false
		}
		return buf.Bytes(), true
	case _contentAlgoDeflateRaw:
		var buf bytes.Buffer
		w, err := flate.NewWriter(&buf, flate.DefaultCompression)
		if err != nil {
			return bodyRaw, false
		}
		_, err = w.Write(bodyRaw)
		if err != nil {
			_ = w.Close()
			return bodyRaw, false
		}
		if err := w.Close(); err != nil {
			return bodyRaw, false
		}
		return buf.Bytes(), true
	default:
		return bodyRaw, false
	}
}
