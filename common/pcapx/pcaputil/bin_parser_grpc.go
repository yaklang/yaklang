package pcaputil

import (
	"bytes"
	"encoding/binary"
	"strings"
)

// Process the stream object retained before HTTP/2 retires it on END_STREAM.
func (f *binFlow) consumeGRPC(dir int, e *ProtocolEvent, stream *binH2Stream) error {
	h := f.h2
	if h == nil || e.Session == nil {
		return nil
	}
	id, _ := e.Session["Stream ID"].(uint32)
	if stream == nil {
		stream = h.streams[id]
	}
	if stream == nil {
		return nil
	}
	if e.Session["Reset"] == true || e.Session["Exchange Complete"] == true {
		defer func() {
			for d := range stream.grpc {
				h.grpcBuffered -= int64(cap(stream.grpc[d]))
				stream.grpc[d] = nil
			}
		}()
	}
	if stream.grpcFailed {
		e.Session["GRPC State"] = "invalid-message-stream"
		return nil
	}
	if e.Session["Reset"] == true {
		return nil
	}
	headers, _ := e.Session["Headers"].([]map[string]any)
	for _, header := range headers {
		name, _ := header["Name"].(string)
		value, _ := header["Value"].(string)
		if name == "grpc-encoding" {
			stream.grpcEncoding[dir] = value
		}
		if name == "content-type" && (e.Session["Header Kind"] == "request" || e.Session["Header Kind"] == "response") {
			media := strings.ToLower(strings.TrimSpace(strings.SplitN(value, ";", 2)[0]))
			stream.grpcEnabled[dir] = media == "application/grpc" || strings.HasPrefix(media, "application/grpc+")
			if stream.grpcEnabled[dir] {
				e.Session["GRPC Content Type"] = value
			}
		}
	}
	if !stream.grpcEnabled[dir] {
		return nil
	}
	e.Session["GRPC"] = true
	for _, header := range headers {
		name, _ := header["Name"].(string)
		value, _ := header["Value"].(string)
		switch name {
		case "grpc-status":
			e.Session["GRPC Status"] = value
		case "grpc-message":
			e.Session["GRPC Message"] = value
		}
	}
	typ, _ := e.Session["Frame Type"].(byte)
	if typ == 0 {
		payload := grpcDATAPayload(e.Raw)
		old := stream.grpc[dir]
		// At most one partial message plus one bounded HTTP/2 frame is transiently retained.
		size := len(old) + len(payload)
		if size > f.a.config.MaxMessageBytes+f.a.budget.MaxFrameBytes {
			return protocolError(ErrResourceExceeded, "gRPC buffered DATA exceeds limit")
		}
		delta := int64(size - cap(old))
		if err := f.reserveSession(h.sessionStorageBytes() + max(delta, 0)); err != nil {
			return err
		}
		wire := make([]byte, size)
		copy(wire, old)
		copy(wire[len(old):], payload)
		msgs, rest, err := splitGRPCMessages(wire, f.a.budget)
		if err != nil {
			return err
		}
		stream.grpc[dir] = bytes.Clone(rest)
		h.grpcBuffered += int64(cap(stream.grpc[dir]) - cap(old))
		if len(msgs) > 0 {
			total := 0
			for _, m := range msgs {
				stream.grpcIndex[dir]++
				m["Index"] = stream.grpcIndex[dir]
				if m["Compressed"] == true {
					encoding := stream.grpcEncoding[dir]
					if encoding != "gzip" {
						return protocolError(ErrUnsupportedFeature, "gRPC compressed message requires gzip encoding")
					}
					remaining := f.a.config.MaxMessageBytes - total
					if err := f.reserveSession(h.sessionStorageBytes() + int64(f.a.config.MaxMessageBytes)); err != nil {
						return err
					}
					plain, err := DecodeBody(m["Payload"].([]byte), encoding, remaining)
					if err != nil {
						return err
					}
					total += len(plain)
					m["Decoded Payload"], m["Decoded Length"], m["Byte Source"] = plain, len(plain), "decompressed"
				}
				payload := m["Payload"].([]byte)
				if decoded, ok := m["Decoded Payload"].([]byte); ok {
					payload = decoded
				}
				if fields, err := DecodeProtobufWire(payload, f.a.budget.MaxCollectionElements); err == nil {
					m["Protobuf Wire Fields"] = fields
				} else {
					m["Protobuf Wire Status"] = "opaque-or-unsupported"
				}
			}
			e.Session["GRPC Messages"] = msgs
			if err := f.reserveSession(h.sessionStorageBytes()); err != nil {
				return err
			}
		}
	}
	if e.Session["End Stream"] == true && len(stream.grpc[dir]) != 0 {
		return protocolError(ErrMalformedMessage, "gRPC END_STREAM inside a length-prefixed message")
	}
	return nil
}

// The HTTP/2 wire validator already checked the padding and frame length.
func grpcDATAPayload(raw []byte) []byte {
	if len(raw) < 9 || raw[3] != 0 {
		return nil
	}
	payload := raw[9:]
	if raw[4]&8 != 0 {
		payload = payload[1 : len(payload)-int(payload[0])]
	}
	return payload
}

func splitGRPCMessages(w []byte, budget ParserBudget) ([]map[string]any, []byte, error) {
	var out []map[string]any
	for len(w) >= 5 {
		if w[0] > 1 {
			return nil, nil, protocolError(ErrMalformedMessage, "gRPC compressed flag must be 0 or 1")
		}
		n := uint64(binary.BigEndian.Uint32(w[1:5]))
		if n > uint64(budget.MaxMessageBytes) {
			return nil, nil, protocolError(ErrResourceExceeded, "gRPC declared message exceeds limit")
		}
		if n > uint64(len(w)-5) {
			return out, w, nil
		}
		if len(out) >= budget.MaxCollectionElements {
			return nil, nil, protocolError(ErrResourceExceeded, "gRPC message count exceeds limit")
		}
		end := 5 + int(n)
		out = append(out, map[string]any{"Compressed": w[0] == 1, "Length": n, "Payload": bytes.Clone(w[5:end])})
		w = w[end:]
	}
	return out, w, nil
}
