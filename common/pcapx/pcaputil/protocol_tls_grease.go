package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// RFC8701 reserves these values for proposals which exercise extensibility.
// They can be advertised by CH (and by CR/NST), never selected by a peer.
// In particular, 0x7a7a is reserved, not an ordinary future extension.
func tlsGREASE16(v uint16) bool {
	return byte(v>>8) == byte(v) && v&0x0f0f == 0x0a0a
}

func tlsGREASEALPN(v []byte) bool {
	return len(v) == 2 && tlsGREASE16(binary.BigEndian.Uint16(v))
}

func tlsServerGREASESelection(body []byte, h *tlsHelloView) (string, uint16) {
	version := binary.BigEndian.Uint16(body)
	if v := h.ext[43]; len(v) == 2 {
		version = binary.BigEndian.Uint16(v)
	}
	if tlsGREASE16(version) {
		return "version", version
	}
	if tlsGREASE16(h.suite) {
		return "cipher suite", h.suite
	}
	for _, kind := range h.order {
		if tlsGREASE16(kind) {
			return "extension", kind
		}
	}
	if v := h.ext[51]; version == 0x304 && len(v) >= 2 {
		if group := binary.BigEndian.Uint16(v); tlsGREASE16(group) {
			return "key_share group", group
		}
	}
	if name, err := tlsALPNName(h.ext[16], true); err == nil && tlsGREASEALPN(name) {
		return "ALPN", binary.BigEndian.Uint16(name)
	}
	return "", 0
}

func tlsServerHelloGREASE(body []byte, h *tlsHelloView) error {
	if kind, value := tlsServerGREASESelection(body, h); kind != "" {
		name := "ServerHello"
		if bytes.Equal(body[2:34], tlsRetryRandom[:]) {
			name = "HelloRetryRequest"
		}
		return protocolError(ErrMalformedMessage, fmt.Sprintf("TLS %s selects reserved GREASE %s 0x%04x", name, kind, value))
	}
	return nil
}

// TLSGREASEServerHelloRecordObservation retains a rejected literal. It grants
// no negotiated value or session identity; Offset addresses its complete record.
type TLSGREASEServerHelloRecordObservation struct {
	TLSServerHelloRecordObservation
	SelectionKind     string
	Value             uint16
	Offset            int
	HelloRetryRequest bool
	RefusalReason     string
}

func ObserveTLSGREASEServerHelloRecord(record []byte, maxElements int) (*TLSGREASEServerHelloRecordObservation, error) {
	server, err := ObserveTLSServerHelloRecord(record, maxElements)
	if err != nil {
		return nil, err
	}
	body := record[9:]
	h, err := tlsHello(body, false, maxElements)
	if err != nil {
		return nil, err
	}
	kind, value := tlsServerGREASESelection(body, h)
	if kind == "" {
		return nil, protocolError(ErrContextRequired, "TLS ServerHello has no reserved GREASE selection")
	}
	offset := 9
	extension := uint16(0)
	switch kind {
	case "version":
		if _, present := h.ext[43]; present {
			extension = 43
		}
	case "cipher suite":
		offset = 44 + server.SessionIDLength
	case "extension":
		extension = value
	case "key_share group":
		extension = 51
	case "ALPN":
		extension = 16
	}
	if extension != 0 {
		// The complete Hello has already passed framing/collection admission.
		// Walk its literal vector for both legacy and TLS1.3; the other public
		// extension observer intentionally requires a TLS1.3 declaration.
		found := false
		for at := 9 + len(h.prefix) + 2; at < len(record); at += 4 + int(binary.BigEndian.Uint16(record[at+2:])) {
			if binary.BigEndian.Uint16(record[at:]) == extension {
				offset, found = at, true
				break
			}
		}
		if !found {
			return nil, protocolError(ErrContextRequired, "TLS reserved GREASE selection has no literal extension")
		}
		if kind != "extension" {
			offset += 4
			if kind == "ALPN" {
				offset += 3 // list length and its sole protocol-name length
			}
		}
	}
	return &TLSGREASEServerHelloRecordObservation{TLSServerHelloRecordObservation: *server,
		SelectionKind: kind, Value: value, Offset: offset, HelloRetryRequest: bytes.Equal(body[2:34], tlsRetryRandom[:]),
		RefusalReason: tlsServerHelloGREASE(body, h).Error()}, nil
}

func tlsCertificateGREASE(view *TLSCertificateView) error {
	entries, _ := view.Fields["Certificate Entries"].([]map[string]any)
	for _, entry := range entries {
		extensions, _ := entry["Extensions"].([]map[string]any)
		for _, extension := range extensions {
			if kind, ok := extension["Type"].(uint16); ok && tlsGREASE16(kind) {
				return protocolError(ErrMalformedMessage, fmt.Sprintf("TLS Certificate selects reserved GREASE extension 0x%04x", kind))
			}
		}
	}
	return nil
}
