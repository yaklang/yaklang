package pcaputil

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"strings"
)

// DecodeBody transforms an explicitly selected body under an output budget.
// It never alters captured lengths or authenticates the carrier.
func DecodeBody(data []byte, encoding string, maxBytes int) ([]byte, error) {
	if maxBytes <= 0 || maxBytes > 16<<20 {
		return nil, fmt.Errorf("body budget must be between 1 byte and 16 MiB")
	}
	var r io.ReadCloser
	var err error
	switch strings.ToLower(encoding) {
	case "identity", "":
		if len(data) > maxBytes {
			return nil, protocolError(ErrResourceExceeded, "body output budget")
		}
		return bytes.Clone(data), nil
	case "gzip":
		r, err = gzip.NewReader(bytes.NewReader(data))
	case "deflate":
		r, err = zlib.NewReader(bytes.NewReader(data))
	default:
		return nil, protocolError(ErrUnsupportedFeature, "unsupported body encoding")
	}
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out, err := io.ReadAll(io.LimitReader(r, int64(maxBytes)+1))
	if len(out) > maxBytes {
		return nil, protocolError(ErrResourceExceeded, "decompressed body budget")
	}
	return out, err
}
func (ws *binWebSocket) message(dir int, op byte, payload []byte) ([]byte, error) {
	if ws.compressed[dir] {
		input := append(bytes.Clone(payload), 0, 0, 255, 255, 1, 0, 0, 255, 255)
		reader := flate.NewReaderDict(bytes.NewReader(input), ws.dictionary[dir])
		out, err := io.ReadAll(io.LimitReader(reader, int64(ws.limit)+1))
		reader.Close()
		if len(out) > ws.limit {
			return nil, protocolError(ErrResourceExceeded, "WebSocket decompressed message budget")
		}
		if err != nil {
			return nil, fmt.Errorf("WebSocket deflate: %w", err)
		}
		payload = out
		if !ws.noContext[dir] {
			dict := append(bytes.Clone(ws.dictionary[dir]), payload...)
			if len(dict) > 32768 {
				dict = dict[len(dict)-32768:]
			}
			ws.dictionary[dir] = bytes.Clone(dict)
		}
	}
	ws.compressed[dir] = false
	return payload, nil
}
func wsDeflate(header string, client int) (*binWebSocket, error) {
	ws := &binWebSocket{client: client, phase: "frame"}
	if header == "" {
		return ws, nil
	}
	parts := strings.Split(header, ";")
	if strings.TrimSpace(parts[0]) != "permessage-deflate" {
		return nil, protocolError(ErrUnsupportedFeature, "WebSocket extension profile")
	}
	ws.deflate = true
	seen := map[string]bool{}
	for _, part := range parts[1:] {
		k, v, has := strings.Cut(strings.TrimSpace(part), "=")
		if seen[k] {
			return nil, fmt.Errorf("duplicate WebSocket extension parameter")
		}
		seen[k] = true
		switch k {
		case "client_no_context_takeover":
			if has {
				return nil, fmt.Errorf("WebSocket flag has value")
			}
			ws.noContext[client] = true
		case "server_no_context_takeover":
			if has {
				return nil, fmt.Errorf("WebSocket flag has value")
			}
			ws.noContext[1-client] = true
		case "client_max_window_bits", "server_max_window_bits":
			if !has || strings.Trim(v, "\"") != "15" {
				return nil, protocolError(ErrUnsupportedFeature, "WebSocket window profile requires 15 bits")
			}
		default:
			return nil, protocolError(ErrUnsupportedFeature, "WebSocket extension parameter")
		}
	}
	return ws, nil
}

// Match extension tokens, not substrings. The first profile only supports
// 15-bit windows; a server cannot silently ignore a client's smaller limit.
func validateWSDeflateOffer(offer, response string) error {
	if response == "" {
		return nil
	}
	selected := strings.Split(response, ";")
	if strings.TrimSpace(selected[0]) != "permessage-deflate" {
		return protocolError(ErrUnsupportedFeature, "WebSocket extension profile")
	}
	responseParams := map[string]string{}
	for _, part := range selected[1:] {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		responseParams[k] = strings.Trim(v, "\"")
	}
	for _, alternative := range strings.Split(offer, ",") {
		parts := strings.Split(alternative, ";")
		if strings.TrimSpace(parts[0]) != "permessage-deflate" {
			continue
		}
		params := map[string]string{}
		valid := true
		for _, part := range parts[1:] {
			k, v, has := strings.Cut(strings.TrimSpace(part), "=")
			if _, ok := params[k]; ok {
				valid = false
			}
			params[k] = strings.Trim(v, "\"")
			switch k {
			case "client_max_window_bits":
				if has && params[k] != "15" {
					valid = false
				}
			case "server_max_window_bits":
				if !has || params[k] != "15" {
					valid = false
				}
			case "server_no_context_takeover", "client_no_context_takeover":
				if has {
					valid = false
				}
			default:
				valid = false
			}
		}
		if _, ok := responseParams["client_max_window_bits"]; ok {
			if _, offered := params["client_max_window_bits"]; !offered {
				valid = false
			}
		}
		if _, ok := params["server_no_context_takeover"]; ok {
			if _, selected := responseParams["server_no_context_takeover"]; !selected {
				valid = false
			}
		}
		if _, ok := params["server_max_window_bits"]; ok {
			if responseParams["server_max_window_bits"] != "15" {
				valid = false
			}
		}
		if valid {
			return nil
		}
	}
	return protocolError(ErrUnsupportedFeature, "WebSocket deflate response lacks a supported matching offer")
}
