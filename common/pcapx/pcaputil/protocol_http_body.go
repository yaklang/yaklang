package pcaputil

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
)

type HTTPBody struct {
	Data     []byte
	Encoding string
	Trailers http.Header
	Source   ByteSource
}

// DecodeHTTPBody is an explicit bounded transform of a complete HTTP/1 event.
// The returned data owns storage; wire lengths and packet offsets are unchanged.
func (e *ProtocolEvent) DecodeHTTPBody(limit int) (*HTTPBody, error) {
	if limit <= 0 || limit > 16<<20 {
		return nil, fmt.Errorf("HTTP body budget must be 1..16777216")
	}
	if e.Protocol != "http" && e.Protocol != "doh" && e.Protocol != "ipp" {
		return nil, fmt.Errorf("HTTP/1 event required")
	}
	r := bufio.NewReader(bytes.NewReader(e.Raw))
	var body io.ReadCloser
	var header, trailer http.Header
	if bytes.HasPrefix(e.Raw, []byte("HTTP/")) {
		method, _ := e.decodeConfig["httpResponseToMethod"].(string)
		rsp, err := http.ReadResponse(r, &http.Request{Method: method})
		if err != nil {
			return nil, err
		}
		body, header, trailer = rsp.Body, rsp.Header, rsp.Trailer
	} else {
		req, err := http.ReadRequest(r)
		if err != nil {
			return nil, err
		}
		body, header, trailer = req.Body, req.Header, req.Trailer
	}
	defer body.Close()
	raw, err := io.ReadAll(io.LimitReader(body, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > limit {
		return nil, protocolError(ErrResourceExceeded, "HTTP encoded body budget")
	}
	encoding := header.Get("Content-Encoding")
	data, err := DecodeBody(raw, encoding, limit)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	kind := "transfer-decoded"
	if encoding != "" && encoding != "identity" {
		kind = "decompressed"
	}
	return &HTTPBody{Data: data, Encoding: encoding, Trailers: trailer.Clone(), Source: ByteSource{Kind: kind, ParentPDU: e.ID, ParentPDUs: []uint64{e.ID}, SHA256: hex.EncodeToString(sum[:])}}, nil
}
