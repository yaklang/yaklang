package pcaputil

import (
	"bytes"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const imapSASLLineMax = stream_parser.IMAPSASLMaxBytes

// Only an observed AUTHENTICATE establishes this context. Keep no credentials
// in the connection state. One peer challenge authorizes exactly one response.
type imapSASL struct {
	tag, mechanism                 string
	client                         int
	waiting, canceled, quarantined bool
	challenges, responses          int
	commandID, challengeID         uint64
}

func imapSASLMechanism(s string) bool {
	if len(s) == 0 || len(s) > 20 {
		return false
	}
	for _, b := range []byte(s) {
		if !(b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_') {
			return false
		}
	}
	return true
}

func imapSASLNoSecurityLayer(mechanism string) bool {
	// RFC 4616, RFC 5802 and RFC 7628 explicitly define no application-data
	// security layer. Unknown mechanisms are never assumed to have none.
	switch mechanism {
	case "PLAIN", "SCRAM-SHA-1", "SCRAM-SHA-1-PLUS", "SCRAM-SHA-256", "SCRAM-SHA-256-PLUS", "OAUTHBEARER", "OAUTH10A":
		return true
	}
	return false
}

func (s *binIMAP) needsNativeSASL(dir int, raw []byte) bool {
	if s.protectionUncertain != "" {
		return true
	}
	f := bytes.Fields(raw)
	if owner := s.idle; owner != nil && (dir == owner.client || bytes.HasPrefix(raw, []byte("+")) || len(f) > 0 && string(f[0]) == owner.tag) {
		return true
	}
	if len(f) >= 2 && (bytes.EqualFold(f[1], []byte("NAMESPACE")) || bytes.EqualFold(f[1], []byte("IDLE"))) {
		return true
	}
	if len(f) > 0 && s.namespaces[string(f[0])] != nil {
		return true
	}
	if a := s.sasl; a != nil && (dir == a.client || bytes.HasPrefix(raw, []byte("+")) || len(f) > 0 && string(f[0]) == a.tag) {
		return true
	}
	return len(f) >= 2 && bytes.EqualFold(f[1], []byte("AUTHENTICATE"))
}

func (s *binIMAP) saslBase(stage string) map[string]any {
	o := map[string]any{"Profile": "imap-sasl-observed", "SASL Stage": stage, "Authentication Verified": false, "Peer Identity Verified": false, "Security Layer Verified": false}
	if a := s.sasl; a != nil {
		o["Tag"], o["Mechanism"] = a.tag, a.mechanism
		o["Challenge Count"], o["Response Count"] = a.challenges, a.responses
	}
	return o
}

func (s *binIMAP) saslRefusal(stage, reason string) (map[string]any, bool, error) {
	o := s.saslBase(stage)
	o["Error Scope"], o["Packet Name"] = "message", "SASL refused transport value"
	return o, true, protocolError(ErrMalformedMessage, "IMAP SASL: %s", reason)
}

func (s *binIMAP) consumeSASL(dir int, raw []byte, fields []string) (map[string]any, bool, error) {
	if s.protectionUncertain != "" {
		o := s.saslBase("protection-unknown")
		o["Mechanism"], o["Packet Name"] = s.protectionUncertain, "SASL protection context required"
		return o, true, protocolError(ErrContextRequired, "IMAP SASL mechanism %s may install a security layer; protected record context was not observed", s.protectionUncertain)
	}
	a := s.sasl
	if a != nil {
		if dir == a.client {
			if !a.waiting || a.canceled || a.quarantined {
				a.quarantined, a.waiting = true, false
				return s.saslRefusal("unexpected-response", "client data has no outstanding server challenge")
			}
			info, err := stream_parser.DecodeIMAPSASLValue(raw, "response")
			if err != nil {
				a.quarantined, a.waiting = true, false
				return s.saslRefusal("invalid-response", err.Error())
			}
			a.waiting = false
			a.responses++
			a.canceled, _ = info["Canceled"].(bool)
			o := s.saslBase("response")
			o["Packet Name"], o["Role"], o["Transport Value"] = "SASL Response", "sasl-response", info
			if a.canceled {
				o["SASL Stage"], o["Packet Name"] = "cancel", "SASL Cancellation"
			}
			return o, true, nil
		}
		if bytes.HasPrefix(raw, []byte("+")) {
			if a.waiting || a.canceled || a.quarantined {
				a.quarantined, a.waiting = true, false
				return s.saslRefusal("unexpected-challenge", "server challenge overlaps a pending, canceled or refused exchange")
			}
			info, err := stream_parser.DecodeIMAPSASLValue(raw, "challenge")
			if err != nil {
				a.quarantined = true
				return s.saslRefusal("invalid-challenge", err.Error())
			}
			a.waiting = true
			a.challenges++
			o := s.saslBase("challenge")
			o["Packet Name"], o["Role"], o["Transport Value"] = "SASL Challenge", "sasl-challenge", info
			return o, true, nil
		}
		if len(fields) >= 2 && fields[0] == a.tag {
			result := strings.ToUpper(fields[1])
			if result != "OK" && result != "NO" && result != "BAD" {
				return s.saslRefusal("invalid-result", "matching server tag has no completion status")
			}
			o := s.saslBase("result")
			o["Packet Name"], o["Role"], o["Command"], o["In Reply To"] = "Tagged "+result, "tagged-reply", result, "AUTHENTICATE"
			o["Server Result Observed"], o["Exchange Canceled"], o["Exchange Had Invalid Value"] = result, a.canceled, a.quarantined
			// A server OK is an observed statement, not verified credentials.
			if result == "OK" && !imapSASLNoSecurityLayer(a.mechanism) {
				s.protectionUncertain = strings.Clone(a.mechanism)
				o["Subsequent Protection"] = "unknown-mechanism-security-layer"
			} else if result == "OK" {
				o["Subsequent Protection"] = "mechanism-defines-no-security-layer"
			}
			if command, ok := s.pending[a.tag]; ok {
				delete(s.pending, a.tag)
				s.pendingBytes -= len(a.tag) + len(command)
			}
			s.sasl = nil
			return o, true, nil
		}
		// Untagged server information and other tags do not steal this owner.
		return nil, false, nil
	}
	if len(fields) < 2 || !strings.EqualFold(fields[1], "AUTHENTICATE") {
		return nil, false, nil
	}
	if !bytes.HasSuffix(raw, []byte("\r\n")) {
		return s.saslRefusal("invalid-command", "complete CRLF command required")
	}
	words := strings.Split(string(raw[:len(raw)-2]), " ")
	if (len(words) != 3 && len(words) != 4) || words[0] == "" || !strings.EqualFold(words[1], "AUTHENTICATE") || !imapSASLMechanism(words[2]) {
		return s.saslRefusal("invalid-command", "expected tag AUTHENTICATE mechanism and optional initial value")
	}
	for _, b := range []byte(words[0]) {
		if b <= 32 || b >= 127 || strings.ContainsRune("(){%*\"\\+", rune(b)) {
			return s.saslRefusal("invalid-command", "invalid authentication tag")
		}
	}
	if s.literal[0] > 0 || s.literal[1] > 0 || s.literalTail[0] || s.literalTail[1] {
		return s.saslRefusal("invalid-command", "authentication overlaps a literal")
	}
	if _, exists := s.pending[words[0]]; exists {
		return nil, true, protocolError(ErrDesynchronized, "IMAP command reused an outstanding tag")
	}
	if len(s.pending) >= sessionCollectionLimit(s.maxPending) {
		return nil, true, protocolError(ErrResourceExceeded, "IMAP pending tag budget exceeded")
	}
	var initial map[string]any
	if len(words) == 4 {
		var err error
		initial, err = stream_parser.DecodeIMAPSASLValue([]byte(words[3]), "initial-value")
		if err != nil {
			return s.saslRefusal("invalid-command", err.Error())
		}
	}
	if s.pending == nil {
		s.pending = map[string]string{}
	}
	tag := strings.Clone(words[0])
	s.pending[tag] = "AUTHENTICATE"
	s.pendingBytes += len(tag) + len("AUTHENTICATE")
	s.sasl = &imapSASL{tag: tag, mechanism: strings.Clone(words[2]), client: dir}
	o := s.saslBase("command")
	o["Packet Name"], o["Role"], o["Command"] = "AUTHENTICATE", "command", "AUTHENTICATE"
	if initial != nil {
		// The standalone value's spans start at zero; the event owns the whole
		// command. Translate encoded spans, never derived decoded payloads.
		if ranges, ok := initial["Wire Field Ranges"].(map[string]any); ok {
			if span, ok := ranges["Encoded Value"].([]uint64); ok {
				offset := uint64(len(raw) - 2 - len(words[3]))
				ranges["Encoded Value"] = []uint64{span[0] + offset, span[1] + offset}
			}
		}
		o["Initial Response"] = initial
	}
	return o, true, nil
}

func (f *binFlow) consumeIMAP(dir int, e *ProtocolEvent) error {
	s := f.imap
	before := s.sasl
	beforeIdle := s.idle
	words := bytes.Fields(e.Raw)
	var beforeNamespace *imapNamespaceRequest
	if len(words) > 0 {
		beforeNamespace = s.namespaces[string(words[0])]
	}
	e.nativeIMAPObservation = true
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	if before == nil && f.tcpClientKnown && dir != f.tcpClientDir && len(words) >= 2 && bytes.EqualFold(words[1], []byte("AUTHENTICATE")) {
		e.Session, _, _ = s.saslRefusal("invalid-command", "AUTHENTICATE came from the observed TCP server")
		e.nativeSessionMessage = true
		e.semanticSession = cloneSession(e.Session)
		return protocolError(ErrMalformedMessage, "IMAP SASL AUTHENTICATE came from the observed TCP server")
	}
	if f.tcpClientKnown && len(words) >= 2 && (bytes.EqualFold(words[1], []byte("NAMESPACE")) || bytes.EqualFold(words[1], []byte("IDLE"))) {
		response := bytes.Equal(words[0], []byte("*"))
		if response && dir == f.tcpClientDir || !response && dir != f.tcpClientDir {
			out, _, err := imapExtensionError("imap-extension-observed", "extension command/response contradicts observed TCP role")
			e.Session = out
			e.nativeSessionMessage = true
			e.semanticSession = cloneSession(e.Session)
			return err
		}
	}
	var err error
	e.Session, err = s.consume(dir, e.Raw)
	if e.Session != nil && (e.Session["Profile"] == "imap-namespace-observed" || e.Session["Profile"] == "imap-idle-observed") {
		e.nativeSessionMessage = true
		e.Profile, _ = e.Session["Profile"].(string)
		if err == nil {
			if e.Session["Profile"] == "imap-idle-observed" {
				owner := beforeIdle
				if owner == nil {
					owner = s.idle
				}
				if owner != nil {
					switch e.Session["IDLE Stage"] {
					case "command":
						owner.commandID = e.ID
					case "continuation":
						e.ResponseTo, owner.continuationID = owner.commandID, e.ID
					case "done":
						e.ResponseTo = owner.continuationID
					case "result":
						e.ResponseTo = owner.commandID
					}
					e.TransactionID = owner.commandID
				}
			} else {
				switch e.Session["Namespace Stage"] {
				case "command":
					s.namespaces[string(words[0])].id = e.ID
					e.TransactionID = e.ID
				case "result":
					if beforeNamespace != nil {
						e.ResponseTo, e.TransactionID = beforeNamespace.id, beforeNamespace.id
					}
				case "response":
					var candidates []*imapNamespaceRequest
					for _, r := range s.namespaces {
						if r.client != dir {
							candidates = append(candidates, r)
						}
					}
					e.Session["Association"] = "request-not-observed"
					if len(candidates) == 1 {
						e.ResponseTo, e.TransactionID = candidates[0].id, candidates[0].id
						e.Session["Association"] = "one-outstanding-opposite-direction-namespace"
					} else if len(candidates) > 1 {
						e.Session["Association"] = "ambiguous-outstanding-namespace"
					}
				}
			}
		}
	}
	if e.Session != nil && e.Session["Profile"] == "imap-sasl-observed" {
		e.nativeSessionMessage = true
		e.Profile = "imap-sasl-observed"
		if err == nil {
			a := before
			if a == nil {
				a = s.sasl
			}
			if a != nil {
				switch e.Session["SASL Stage"] {
				case "command":
					a.commandID = e.ID
				case "challenge":
					e.ResponseTo, a.challengeID = a.commandID, e.ID
				case "response", "cancel":
					e.ResponseTo = a.challengeID
				case "result":
					e.ResponseTo = a.commandID
				}
				e.TransactionID = a.commandID
			}
		}
	}
	e.semanticSession = cloneSession(e.Session)
	return err
}
