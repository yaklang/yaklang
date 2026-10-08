package pcaputil

import (
	"encoding/binary"
	"fmt"
)

type socks5Phase uint8

const (
	socks5Greeting socks5Phase = iota
	socks5MethodReply
	socks5AuthRequest
	socks5AuthReply
	socks5Request
	socks5Reply
	socks5Complete
)

type binSOCKS5 struct {
	client          int
	phase           socks5Phase
	offeredMethods  map[byte]bool
	selectedMethod  byte
	command         byte
	username        string
	authenticated   bool
	connectAccepted bool
	bindReplies     uint8
}

func socks5KnownMethod(method byte) bool {
	return method == 0 || method == 1 || method == 2
}

// probeSOCKS5 admits only a complete client method offer with at least one
// standardized method. A port number or the two-byte version/count prefix is
// not enough to classify a TCP flow.
func probeSOCKS5(w []byte, limit int) ProbeResult {
	if len(w) == 0 || w[0] != 5 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 2 {
		return probeNeed("socks5", "method-offer", len(w), 2)
	}
	n := int(w[1])
	if n == 0 || n+2 > limit {
		return ProbeResult{Verdict: ProbeReject, Reason: "SOCKS5 method offer is empty or exceeds the probe bound"}
	}
	if len(w) < n+2 {
		return probeNeed("socks5", "method-offer", len(w), n+2)
	}
	seen, known := map[byte]bool{}, false
	for _, method := range w[2 : n+2] {
		if method == 0xff || seen[method] {
			return ProbeResult{Verdict: ProbeReject, Reason: "SOCKS5 method offer has a reserved or duplicate method"}
		}
		seen[method] = true
		known = known || socks5KnownMethod(method)
	}
	if !known {
		return ProbeResult{Verdict: ProbeReject, Reason: "SOCKS5 method offer has no standardized method"}
	}
	return probeAccept("socks5", "5", 98)
}

func (f *binFlow) frameSOCKS5(dir int, w []byte) (int, *binSpec, error) {
	s := f.socks5
	if s == nil {
		return 0, nil, sessionContext("SOCKS5 session was not observed")
	}
	client := dir == s.client
	switch s.phase {
	case socks5Greeting:
		if !client {
			return 0, nil, nil
		}
		if len(w) < 2 {
			return 0, nil, nil
		}
		n := int(w[1])
		if w[0] != 5 || n == 0 || n+2 > f.a.config.MaxMessageBytes {
			return 0, nil, protocolError(ErrMalformedMessage, "socks5: invalid method offer header")
		}
		if n+2 > len(w) {
			return 0, nil, nil
		}
		return n + 2, f.spec("socks5", "ClientNegotiation"), nil
	case socks5MethodReply:
		if client {
			return 0, nil, nil
		}
		if len(w) < 2 {
			return 0, nil, nil
		}
		return 2, f.spec("socks5", "ServerNegotiation"), nil
	case socks5AuthRequest:
		if !client {
			return 0, nil, nil
		}
		if len(w) < 2 {
			return 0, nil, nil
		}
		if w[0] != 1 || w[1] == 0 {
			return 0, nil, protocolError(ErrMalformedMessage, "socks5: invalid username/password request header")
		}
		userEnd := 2 + int(w[1])
		if userEnd >= len(w) {
			return 0, nil, nil
		}
		passwordLength := int(w[userEnd])
		if passwordLength == 0 {
			return 0, nil, protocolError(ErrMalformedMessage, "socks5: empty password")
		}
		n := userEnd + 1 + passwordLength
		if n > f.a.config.MaxMessageBytes {
			return 0, nil, protocolError(ErrResourceExceeded, "socks5: authentication request exceeds message limit")
		}
		if n > len(w) {
			return 0, nil, nil
		}
		return n, f.spec("socks5", "AuthRequest"), nil
	case socks5AuthReply:
		if client {
			return 0, nil, nil
		}
		if len(w) < 2 {
			return 0, nil, nil
		}
		return 2, f.spec("socks5", "AuthReply"), nil
	case socks5Request:
		if !client {
			return 0, nil, nil
		}
		n, err := socks5AddressMessageLength(w, false)
		if err != nil || n == 0 {
			return 0, nil, err
		}
		return n, f.spec("socks5", "Request"), nil
	case socks5Reply:
		if client {
			return 0, nil, nil
		}
		n, err := socks5AddressMessageLength(w, true)
		if err != nil || n == 0 {
			return 0, nil, err
		}
		return n, f.spec("socks5", "Replies"), nil
	case socks5Complete:
		return 0, nil, nil
	default:
		return 0, nil, protocolError(ErrFatalSessionError, "socks5: invalid session phase")
	}
}

func socks5AddressMessageLength(w []byte, reply bool) (int, error) {
	if len(w) < 4 {
		return 0, nil
	}
	if w[0] != 5 || w[2] != 0 {
		return 0, protocolError(ErrMalformedMessage, "socks5: invalid address-message header")
	}
	if reply {
		if w[1] > 8 {
			return 0, protocolError(ErrMalformedMessage, "socks5: invalid reply code")
		}
	} else if w[1] < 1 || w[1] > 3 {
		return 0, protocolError(ErrMalformedMessage, "socks5: invalid command")
	}
	var n int
	switch w[3] {
	case 1:
		n = 10
	case 3:
		if len(w) < 5 {
			return 0, nil
		}
		if w[4] == 0 {
			return 0, protocolError(ErrMalformedMessage, "socks5: empty domain address")
		}
		n = 7 + int(w[4])
	case 4:
		n = 22
	default:
		return 0, protocolError(ErrMalformedMessage, "socks5: invalid address type")
	}
	if n > len(w) {
		return 0, nil
	}
	return n, nil
}

func (s *binSOCKS5) consume(dir int, raw []byte, entry string, result map[string]any) (map[string]any, error) {
	if s == nil {
		return nil, sessionContext("SOCKS5 session was not observed")
	}
	client := dir == s.client
	fields, _ := result["fields"].(map[string]any)
	out := map[string]any{"Stage": entry, "Direction": "server"}
	if client {
		out["Direction"] = "client"
	}
	switch entry {
	case "ClientNegotiation":
		if !client || s.phase != socks5Greeting || len(raw) < 2 || raw[0] != 5 || int(raw[1])+2 != len(raw) {
			return nil, protocolError(ErrMalformedMessage, "socks5: unexpected method offer")
		}
		s.offeredMethods = make(map[byte]bool, raw[1])
		for _, method := range raw[2:] {
			s.offeredMethods[method] = true
		}
		s.phase = socks5MethodReply
		out["Method Count"] = len(s.offeredMethods)
	case "ServerNegotiation":
		if client || s.phase != socks5MethodReply || len(raw) != 2 || raw[0] != 5 {
			return nil, protocolError(ErrMalformedMessage, "socks5: unexpected method selection")
		}
		method := raw[1]
		if method == 0xff {
			s.phase = socks5Complete
			out["Accepted"] = false
			break
		}
		if !s.offeredMethods[method] {
			return nil, protocolError(ErrMalformedMessage, "socks5: selected method was not offered")
		}
		s.selectedMethod = method
		out["Selected Method"] = method
		switch method {
		case 0:
			s.phase = socks5Request
		case 2:
			s.phase = socks5AuthRequest
		default:
			return nil, protocolError(ErrUnsupportedFeature, fmt.Sprintf("socks5: authentication method %d is not supported", method))
		}
	case "AuthRequest":
		if !client || s.phase != socks5AuthRequest || len(raw) < 4 || raw[0] != 1 {
			return nil, protocolError(ErrMalformedMessage, "socks5: unexpected authentication request")
		}
		userLength := int(raw[1])
		if userLength == 0 || 3+userLength > len(raw) || int(raw[2+userLength])+3+userLength != len(raw) {
			return nil, protocolError(ErrMalformedMessage, "socks5: invalid authentication lengths")
		}
		s.username = string(raw[2 : 2+userLength])
		s.phase = socks5AuthReply
		out["Username"] = s.username
		out["Password"] = "[redacted]"
		if fields != nil {
			fields["PASSWD"] = "[redacted]"
			fields["UNAME"] = "[redacted]"
			result["fields"] = fields
		}
	case "AuthReply":
		if client || s.phase != socks5AuthReply || len(raw) != 2 || raw[0] != 1 {
			return nil, protocolError(ErrMalformedMessage, "socks5: unexpected authentication reply")
		}
		s.authenticated = raw[1] == 0
		out["Authenticated"] = s.authenticated
		if !s.authenticated {
			s.phase = socks5Complete
		} else {
			s.phase = socks5Request
		}
	case "Request":
		if !client || s.phase != socks5Request || len(raw) < 4 {
			return nil, protocolError(ErrMalformedMessage, "socks5: unexpected request")
		}
		s.command = raw[1]
		out["Command"] = socks5CommandName(s.command)
		out["Destination Address Type"] = raw[3]
		out["Destination Port"] = binary.BigEndian.Uint16(raw[len(raw)-2:])
		switch raw[3] {
		case 1:
			out["Destination Address"] = fmt.Sprintf("%d.%d.%d.%d", raw[4], raw[5], raw[6], raw[7])
		case 3:
			out["Destination Address"] = string(raw[5 : len(raw)-2])
		case 4:
			out["Destination Address"] = fmt.Sprintf("%x:%x:%x:%x:%x:%x:%x:%x", binary.BigEndian.Uint16(raw[4:6]), binary.BigEndian.Uint16(raw[6:8]), binary.BigEndian.Uint16(raw[8:10]), binary.BigEndian.Uint16(raw[10:12]), binary.BigEndian.Uint16(raw[12:14]), binary.BigEndian.Uint16(raw[14:16]), binary.BigEndian.Uint16(raw[16:18]), binary.BigEndian.Uint16(raw[18:20]))
		}
		s.phase = socks5Reply
	case "Replies":
		if client || s.phase != socks5Reply || len(raw) < 4 || raw[0] != 5 {
			return nil, protocolError(ErrMalformedMessage, "socks5: unexpected reply")
		}
		out["Reply"] = raw[1]
		out["Reply Name"] = socks5ReplyName(raw[1])
		if raw[1] != 0 {
			s.phase = socks5Complete
			break
		}
		if s.command == 2 {
			s.bindReplies++
			if s.bindReplies < 2 {
				s.phase = socks5Reply
			} else {
				s.phase = socks5Complete
			}
			return out, nil
		}
		s.phase = socks5Complete
		s.connectAccepted = s.command == 1
	case "":
		return nil, fmt.Errorf("socks5: missing parser entry")
	default:
		return nil, fmt.Errorf("socks5: unsupported parser entry %q", entry)
	}
	return out, nil
}

func socks5CommandName(command byte) string {
	switch command {
	case 1:
		return "CONNECT"
	case 2:
		return "BIND"
	case 3:
		return "UDP ASSOCIATE"
	default:
		return fmt.Sprintf("unknown(%d)", command)
	}
}

func socks5ReplyName(reply byte) string {
	switch reply {
	case 0:
		return "succeeded"
	case 1:
		return "general failure"
	case 2:
		return "connection not allowed"
	case 3:
		return "network unreachable"
	case 4:
		return "host unreachable"
	case 5:
		return "connection refused"
	case 6:
		return "TTL expired"
	case 7:
		return "command unsupported"
	case 8:
		return "address type unsupported"
	default:
		return fmt.Sprintf("unknown(%d)", reply)
	}
}
