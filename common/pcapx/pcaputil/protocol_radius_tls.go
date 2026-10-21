package pcaputil

import (
	"bytes"
	"encoding/binary"
)

// EAP-TLS observations are scoped to this complete outer EAP message. A RADIUS
// endpoint tuple is not an authentication-session identity. Cross-round TLS
// fragments require a proven State/Identifier chain and are never guessed.
func radiusTLSFields(inner map[string]any, code byte, data []byte, max int, note func(string, bool), depths ...int) (bool, error) {
	inner["Type Name"] = "TLS"
	note("eap-tls-authentication-not-verified", false)
	if len(data) == 0 || data[0]&0x80 != 0 && len(data) < 5 {
		return false, protocolError(ErrMalformedMessage, "EAP-TLS flags/length header is truncated")
	}
	flags := data[0]
	included, more := flags&0x80 != 0, flags&0x40 != 0
	start := code == 1 && flags&0x20 != 0
	reserved := flags & 0x3f
	if code == 1 {
		reserved = flags & 0x1f
	}
	body := data[1:]
	t := map[string]any{"Flags": flags, "Length Included": included, "More Fragments": more, "Start": start, "ACK": false, "Reserved Bits": reserved}
	inner["TLS"] = t
	var declared uint32
	if included {
		declared = binary.BigEndian.Uint32(body)
		body = body[4:]
		t["Message Length"] = declared
	}
	t["Data"] = bytes.Clone(body)
	t["ACK"] = len(body) == 0 && !start && !included && !more
	if start && len(body) > 0 {
		note("eap-tls-start-data", true)
		return true, nil
	}
	if included && uint64(declared) < uint64(len(body)) {
		note("eap-tls-message-length", true)
		return true, nil
	}
	if more || included && (int64(declared) != int64(len(body)) || len(body) == 0 && !start) {
		note("eap-tls-fragment-context-required", false)
		return true, protocolError(ErrContextRequired, "EAP-TLS fragments require a proven authentication context")
	}
	if len(body) == 0 {
		t["Record Count"], t["Records"] = 0, []map[string]any{}
		return true, nil
	}
	limits := radiusTLSCertificateLimits{depth: 64, server: code == 1}
	if len(depths) > 0 {
		limits.depth = depths[0]
	}
	err := radiusTLSRecordFields(t, body, max, included && uint64(declared) == uint64(len(body)), limits)
	if p, ok := err.(*ProtocolError); ok {
		switch p.Kind {
		case ErrMalformedMessage:
			note("eap-tls-record-malformed", true)
		case ErrContextRequired:
			note("eap-tls-record-context-required", false)
		case ErrUnsupportedFeature:
			note("eap-tls-record-unimplemented", false)
		}
	}
	return true, err
}

func radiusTLSRecordFields(out map[string]any, data []byte, max int, completeBoundary bool, contexts ...radiusTLSCertificateLimits) error {
	boundaryError := ErrContextRequired
	if completeBoundary {
		boundaryError = ErrMalformedMessage
	}
	// Validate the whole record boundary and count without allocating a list.
	count := 0
	for at := 0; at < len(data); {
		if len(data)-at < 5 {
			return protocolError(boundaryError, "EAP-TLS record boundary is incomplete")
		}
		n := 5 + int(binary.BigEndian.Uint16(data[at+3:]))
		if n > len(data)-at {
			return protocolError(boundaryError, "EAP-TLS record length exceeds observed collection")
		}
		count++
		at += n
	}
	out["Record Count"] = count
	if count > max {
		return protocolError(ErrResourceExceeded, "EAP-TLS record count limit")
	}
	records := make([]map[string]any, 0, count)
	opaque := false
	var handshake, alerts []byte
	var unknown error
	for at := 0; at < len(data); {
		n := 5 + int(binary.BigEndian.Uint16(data[at+3:]))
		w := data[at : at+n]
		at += n
		typ, version := w[0], binary.BigEndian.Uint16(w[1:3])
		p := w[5:]
		encrypted := opaque || typ == 23
		visibility := "unverified-cleartext"
		if encrypted {
			visibility = "encrypted"
		}
		r := map[string]any{"Content Type": typ, "Version": version, "Length": len(p), "Raw": bytes.Clone(w), "Payload": bytes.Clone(p), "Content Visibility": visibility, "Authentication Verified": false}
		records = append(records, r)
		// Future record versions/types retain exact observations without decoding
		// their body using a legacy grammar.
		if version < 0x301 || version > 0x303 || typ < 20 || typ > 23 {
			unknown = protocolError(ErrUnsupportedFeature, "EAP-TLS record type/version requires a separate profile")
			continue
		}
		if encrypted {
			opaque = true
			continue
		}
		if len(p) == 0 {
			return protocolError(ErrContextRequired, "EAP-TLS record cipher context is unproven")
		}
		switch typ {
		case 20:
			if !bytes.Equal(p, []byte{1}) {
				return protocolError(ErrContextRequired, "EAP-TLS ChangeCipherSpec plaintext is unproven")
			}
			opaque = true
		case 21:
			alerts = append(alerts, p...)
		case 22:
			handshake = append(handshake, p...)
		}
	}
	out["Records"] = records
	// Each concatenation is bounded by this already-admitted outer message;
	// it never reserves from the untrusted four-octet EAP declared length.
	if len(handshake) > 0 {
		rows, left, err := radiusTLSHandshakeFields(handshake, max, contexts...)
		out["Handshake Incomplete"] = left > 0
		if left > 0 {
			out["Handshake Buffered Bytes"] = left
		}
		if err != nil {
			if len(rows) > 0 && rows[len(rows)-1]["Type"] == byte(11) {
				out["Handshake Messages"] = rows
			}
			if e, ok := err.(*ProtocolError); ok && e.Kind == ErrUnsupportedFeature {
				unknown = err
			} else {
				return err
			}
		}
		out["Handshake Messages"] = rows
	}
	if len(alerts) > 0 {
		n := len(alerts) / 2
		if n > max {
			return protocolError(ErrResourceExceeded, "EAP-TLS alert count limit")
		}
		rows := make([]map[string]any, 0, n)
		for at := 0; at+1 < len(alerts); at += 2 {
			rows = append(rows, map[string]any{"Level": alerts[at], "Description": alerts[at+1], "Raw": bytes.Clone(alerts[at : at+2])})
		}
		out["Alert Messages"] = rows
		if len(alerts)%2 != 0 {
			out["Alert Buffered Bytes"] = 1
			return protocolError(ErrContextRequired, "EAP-TLS alert needs later authentication-round context")
		}
	}
	if left, ok := out["Handshake Buffered Bytes"].(int); ok && left > 0 {
		return protocolError(ErrContextRequired, "EAP-TLS handshake needs later authentication-round context")
	}
	return unknown
}

func radiusTLSHandshakeFields(w []byte, max int, contexts ...radiusTLSCertificateLimits) ([]map[string]any, int, error) {
	count, at := 0, 0
	for len(w)-at >= 4 {
		n := 4 + tlsU24(w[at+1:])
		if n > len(w)-at {
			break
		}
		at += n
		count++
	}
	left := len(w) - at
	if count > max {
		return nil, left, protocolError(ErrResourceExceeded, "EAP-TLS handshake count limit")
	}
	rows := make([]map[string]any, 0, count)
	var unsupported error
	certificateContext, serverHellos := 0, 0
	for p := 0; p < at; {
		n := 4 + tlsU24(w[p+1:])
		raw := w[p : p+n]
		p += n
		row := map[string]any{"Type": raw[0], "Length": n - 4, "Raw": bytes.Clone(raw), "Body": bytes.Clone(raw[4:])}
		if raw[0] == 1 || raw[0] == 2 {
			hello, err := radiusTLSHelloFields(raw[4:], raw[0] == 1, max)
			if err != nil {
				return nil, left, err
			}
			row["Hello"] = hello
			if raw[0] == 2 {
				serverHellos++
				certificateContext = radiusTLSCertificateContext(hello)
				if serverHellos > 1 {
					certificateContext = 0
				}
			}
		} else if raw[0] == 11 && serverHellos > 0 && len(contexts) > 0 && contexts[0].server {
			rows = append(rows, row)
			limits := radiusTLSCertificateLimits{depth: 64}
			if len(contexts) > 0 {
				limits = contexts[0]
			}
			if certificateContext != 1 || !limits.server {
				kind := ErrContextRequired
				if certificateContext == 2 {
					kind = ErrUnsupportedFeature
				}
				return rows, left, protocolError(kind, "EAP-TLS certificate grammar requires a single observed legacy ServerHello context")
			}
			depth := limits.depth
			if err := radiusTLSCertificateFields(row, raw[4:], max, depth); err != nil {
				return rows, left, err
			}
			continue
		} else {
			unsupported = protocolError(ErrUnsupportedFeature, "EAP-TLS handshake requires a separate semantic profile")
		}
		rows = append(rows, row)
	}
	return rows, left, unsupported
}

func radiusTLSHelloFields(w []byte, client bool, max int) (map[string]any, error) {
	bad := func() (map[string]any, error) {
		return nil, protocolError(ErrContextRequired, "EAP-TLS hello structure/compression plaintext is unproven")
	}
	if len(w) < 35 || w[34] > 32 {
		return bad()
	}
	end := 35 + int(w[34])
	if end > len(w) {
		return bad()
	}
	if binary.BigEndian.Uint16(w) < 0x301 {
		return bad()
	}
	p := end
	cipherStart, cipherN, compStart, compN := 0, 0, 0, 0
	var suite uint16
	var compression byte
	compressionBad := false
	if client {
		if len(w)-p < 2 {
			return bad()
		}
		cipherN = int(binary.BigEndian.Uint16(w[p:]))
		p += 2
		cipherStart = p
		if cipherN < 2 || cipherN%2 != 0 || cipherN > len(w)-p {
			return bad()
		}
		p += cipherN
		if p == len(w) {
			return bad()
		}
		compN = int(w[p])
		p++
		compStart = p
		if compN == 0 || compN > len(w)-p {
			return bad()
		}
		for _, b := range w[p : p+compN] {
			if b != 0 {
				compressionBad = true
			}
		}
		p += compN
	} else {
		if len(w)-p < 3 {
			return bad()
		}
		suite = binary.BigEndian.Uint16(w[p:])
		compression = w[p+2]
		p += 3
		compressionBad = compression != 0
	}
	extStart, extN := p, 0
	if p < len(w) {
		if len(w)-p < 2 || int(binary.BigEndian.Uint16(w[p:])) != len(w)-p-2 {
			return bad()
		}
		p += 2
		extStart = p
		// A bounded allocation-free pass detects duplicate extension IDs before
		// list construction. Maximum work is bounded by the complete RADIUS frame.
		for p < len(w) {
			if len(w)-p < 4 {
				return bad()
			}
			typ, n := binary.BigEndian.Uint16(w[p:]), int(binary.BigEndian.Uint16(w[p+2:]))
			value := p + 4
			if n > len(w)-value {
				return bad()
			}
			for prior := extStart; prior < p; {
				if binary.BigEndian.Uint16(w[prior:]) == typ {
					return bad()
				}
				prior += 4 + int(binary.BigEndian.Uint16(w[prior+2:]))
			}
			p = value + n
			extN++
		}
	}
	if compressionBad {
		return bad()
	}
	if extN > max || client && (cipherN/2 > max || compN > max) {
		return nil, protocolError(ErrResourceExceeded, "EAP-TLS hello vector count limit")
	}
	hello := map[string]any{"Version": binary.BigEndian.Uint16(w), "Random": bytes.Clone(w[2:34]), "Session ID": bytes.Clone(w[35:end])}
	if client {
		suites := make([]uint16, cipherN/2)
		for i := range suites {
			suites[i] = binary.BigEndian.Uint16(w[cipherStart+2*i:])
		}
		hello["Cipher Suites"], hello["Compression Methods"] = suites, bytes.Clone(w[compStart:compStart+compN])
	} else {
		hello["Cipher Suite"], hello["Compression Method"] = suite, compression
	}
	extensions := make([]map[string]any, 0, extN)
	for p := extStart; p < len(w); {
		typ, n := binary.BigEndian.Uint16(w[p:]), int(binary.BigEndian.Uint16(w[p+2:]))
		p += 4
		extensions = append(extensions, map[string]any{"Type": typ, "Raw": bytes.Clone(w[p : p+n])})
		p += n
	}
	hello["Extensions"] = extensions
	return hello, nil
}

// A 0303 record or legacy_version alone is insufficient negotiation evidence.
// This is a structural observation, never proof of a peer's cipher state.
func radiusTLSCertificateContext(h map[string]any) int {
	v, _ := h["Version"].(uint16)
	suite, _ := h["Cipher Suite"].(uint16)
	exts, _ := h["Extensions"].([]map[string]any)
	for _, e := range exts {
		if e["Type"] == uint16(9) || e["Type"] == uint16(20) {
			if !bytes.Equal(e["Raw"].([]byte), []byte{0}) {
				return 0
			}
		}
		if e["Type"] == uint16(43) {
			if v == 0x303 && bytes.Equal(e["Raw"].([]byte), []byte{3, 4}) && suite >= 0x1301 && suite <= 0x1305 {
				return 2
			}
			return 0
		}
	}
	if v != 0x303 {
		return 0
	}
	switch suite {
	case 0x003c:
		return 1
	}
	return 0
}
