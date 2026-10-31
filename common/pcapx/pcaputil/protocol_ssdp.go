package pcaputil

import (
	"bytes"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func (a *binParser) decodeSSDPDatagram(e *ProtocolEvent, w []byte, explicit bool) bool {
	candidate := bytes.HasPrefix(w, []byte("NOTIFY * HTTP/1.1\r\n")) || bytes.HasPrefix(w, []byte("M-SEARCH * HTTP/1.1\r\n"))
	if !candidate && bytes.HasPrefix(w, []byte("HTTP/1.1 200 ")) {
		lower := bytes.ToLower(w[:min(len(w), 65507)])
		candidate = bytes.Contains(lower, []byte("\r\nst:")) && bytes.Contains(lower, []byte("\r\nusn:"))
	}
	if !explicit && !candidate {
		return false
	}
	fields, err := stream_parser.DecodeSSDPMessage(w)
	e.Protocol, e.Profile, e.Admission, e.Completeness = "ssdp", "uda-discovery-core", "validated-discovery-headers", "message"
	e.Rule, e.Entry = "application-layer.ssdp", "SSDP"
	if explicit {
		e.Admission = "explicit-decode-as"
	}
	if err != nil && len(w) > 65507 {
		err = protocolError(ErrResourceExceeded, "SSDP datagram exceeds bounded profile")
	} else if err != nil && !bytes.Contains(w, []byte("\r\n\r\n")) {
		err = protocolError(ErrNeedMore, "SSDP datagram header block incomplete; no cross-datagram reassembly")
	}
	if err == nil {
		e.Session = fields
		e.semanticFields = cloneSession(fields)
	}
	a.finishProtocolDatagram(e, w, nil, err)
	if err == nil {
		e.Summary = "SSDP " + fields["Message Kind"].(string)
	}
	return true
}
