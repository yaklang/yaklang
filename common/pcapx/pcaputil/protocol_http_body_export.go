package pcaputil

import (
	"bytes"
	"encoding/binary"
)

// HTTP2DataFragment is one admitted HTTP/2 DATA frame's contribution to a
// request or response body. The caller owns Payload.
type HTTP2DataFragment struct {
	StreamID  uint64
	EndStream bool
	Payload   []byte
}

// HTTP2DataFragment returns the app-visible bytes and END_STREAM flag of one
// validated DATA frame. Padding is removed; HPACK and frame authentication are
// validated by the protocol session before this event is published.
func (e *ProtocolEvent) HTTP2DataFragment() (HTTP2DataFragment, bool) {
	if e == nil || e.Protocol != "http2" && e.Protocol != "grpc" || len(e.Raw) < 9 || e.Raw[3] != 0 || e.Status == "malformed" {
		return HTTP2DataFragment{}, false
	}
	payload := grpcDATAPayload(e.Raw)
	if payload == nil {
		payload = []byte{}
	}
	return HTTP2DataFragment{
		StreamID:  uint64(binary.BigEndian.Uint32(e.Raw[5:9]) & 0x7fffffff),
		EndStream: e.Raw[4]&1 != 0,
		Payload:   bytes.Clone(payload),
	}, true
}

// HTTP2HeaderFragment describes a validated request or response header block.
// CONTINUATION frames are folded into the completed HEADERS observation by the
// session parser and do not independently identify a body direction.
type HTTP2HeaderFragment struct {
	StreamID  uint64
	Kind      string
	EndStream bool
}

func (e *ProtocolEvent) HTTP2HeaderFragment() (HTTP2HeaderFragment, bool) {
	if e == nil || e.Protocol != "http2" && e.Protocol != "grpc" || len(e.Raw) < 9 || e.Raw[3] != 1 || e.Status == "malformed" || e.Session == nil {
		return HTTP2HeaderFragment{}, false
	}
	kind, _ := e.Session["Header Kind"].(string)
	if kind != "request" && kind != "response" {
		return HTTP2HeaderFragment{}, false
	}
	return HTTP2HeaderFragment{
		StreamID:  uint64(binary.BigEndian.Uint32(e.Raw[5:9]) & 0x7fffffff),
		Kind:      kind,
		EndStream: e.Raw[4]&1 != 0,
	}, true
}

func (e *ProtocolEvent) HTTP2StreamReset() (uint64, bool) {
	if e == nil || e.Protocol != "http2" && e.Protocol != "grpc" || len(e.Raw) < 9 || e.Raw[3] != 3 {
		return 0, false
	}
	return uint64(binary.BigEndian.Uint32(e.Raw[5:9]) & 0x7fffffff), true
}
