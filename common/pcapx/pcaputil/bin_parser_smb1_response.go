package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func smb1ResourceRefusal(err error) error {
	var typed *ProtocolError
	if errors.As(err, &typed) && typed.Kind == ErrResourceExceeded {
		owned := *typed
		return &owned
	}
	return err
}

// SMB_COM_NEGOTIATE and SMB_COM_SESSION_SETUP_ANDX failures use the common
// MS-CIFS 2.2.3.2 WC0/BC0 error layout. It is a complete, framed reply: publish
// the header and status, commit no session state, and keep the flow alive so a
// later retry or command in the same TCP generation is still observable.
func (f *binFlow) consumeSMB1EmptyError(dir int, e *ProtocolEvent) error {
	fields, err := stream_parser.DecodeCIFSEmptyError(e.Raw)
	if err != nil {
		return protocolError(ErrMalformedMessage, "CIFS error reply: %s", err)
	}
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	fields["Error Scope"] = "message"
	fields["Context Eligibility"] = "complete WC0/BC0 error layout; no session, tree or file state committed"
	fields["Association Status"] = "error reply observed; no matched request committed"
	smb1AttachContext(fields, e.Raw[8])
	e.nativeSessionMessage = true
	e.Profile, e.Completeness = "cifs-error-layout", "message"
	e.Session = fields
	e.semanticSession = cloneSession(fields)
	e.semanticFields = cloneSession(fields)
	return nil
}

// An SMB1 command whose body profile is not selected is still exactly framed by
// Direct TCP Length and WordCount/ByteCount. Keep it as an unsupported
// observation with its header and opaque body instead of stopping the stream.
func (f *binFlow) consumeSMB1UnselectedCommand(dir int, e *ProtocolEvent) error {
	if len(e.Raw) < 39 {
		return protocolError(ErrMalformedMessage, "CIFS: truncated SMB1 record")
	}
	wc := e.Raw[36]
	fixed := 39 + 2*int(wc)
	if fixed > len(e.Raw) || int(binary.LittleEndian.Uint16(e.Raw[fixed-2:fixed])) != len(e.Raw)-fixed {
		if e.ID == 0 {
			e.ID = f.a.ids.Add(1)
		}
		fields := map[string]any{
			"Header":      smb1RawHeader(e.Raw),
			"Packet Name": smb1CommandName(e.Raw[8]), "Command Name": smb1CommandName(e.Raw[8]),
			"Response":          e.Raw[13]&0x80 != 0,
			"Profile Status":    "WordCount/ByteCount exceed the complete Direct TCP record",
			"Body Decode Error": "CIFS: WordCount/ByteCount exceed complete record",
			"Error Scope":       "message",
			"State Updated":     false,
		}
		e.nativeSessionMessage = true
		e.Profile, e.Completeness = "cifs-unselected-command-diagnostic", "headers"
		e.Session = fields
		e.semanticSession = cloneSession(fields)
		e.semanticFields = cloneSession(fields)
		return protocolError(ErrMalformedMessage, "CIFS: WordCount/ByteCount exceed complete record")
	}
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	body := map[string]any{
		"Word Count":  wc,
		"Byte Count":  binary.LittleEndian.Uint16(e.Raw[fixed-2 : fixed]),
		"Opaque Body": bytes.Clone(e.Raw[fixed:]),
		"Wire Field Ranges": map[string]any{
			"Word Count":  []any{uint64(36), uint64(37)},
			"Byte Count":  []any{uint64(fixed - 2), uint64(fixed)},
			"Opaque Body": []any{uint64(fixed), uint64(len(e.Raw))},
		},
	}
	fields := map[string]any{
		"Header": smb1RawHeader(e.Raw), "Body": body,
		"Packet Name": smb1CommandName(e.Raw[8]), "Command Name": smb1CommandName(e.Raw[8]),
		"Response":            e.Raw[13]&0x80 != 0,
		"Unselected Command":  true,
		"Profile Status":      "no selected native body profile; Direct TCP Length and the WC/BC envelope are complete",
		"Error Scope":         "message",
		"Context Eligibility": "unassociated observation; no session, tree or file state committed",
	}
	e.nativeSessionMessage = true
	e.Profile, e.Completeness = "cifs-unselected-command", "message"
	smb1AttachContext(fields, e.Raw[8])
	e.Session = fields
	e.semanticSession = cloneSession(fields)
	e.semanticFields = cloneSession(fields)
	return nil
}

func smb1RawHeader(raw []byte) map[string]any {
	header := map[string]any{
		"Protocol ID": binary.LittleEndian.Uint32(raw[4:8]), "Command": raw[8],
		"Status": binary.LittleEndian.Uint32(raw[9:13]), "Flags": raw[13],
		"Flags 2": binary.LittleEndian.Uint16(raw[14:16]), "PID High": binary.LittleEndian.Uint16(raw[16:18]),
		"Security Features": bytes.Clone(raw[18:26]), "Reserved": binary.LittleEndian.Uint16(raw[26:28]),
		"Tree ID": binary.LittleEndian.Uint16(raw[28:30]), "Process ID Low": binary.LittleEndian.Uint16(raw[30:32]),
		"User ID": binary.LittleEndian.Uint16(raw[32:34]), "Multiplex ID": binary.LittleEndian.Uint16(raw[34:36]),
		"Wire Field Ranges": map[string]any{
			"Protocol ID": []any{uint64(4), uint64(8)}, "Command": []any{uint64(8), uint64(9)},
			"Status": []any{uint64(9), uint64(13)}, "Flags": []any{uint64(13), uint64(14)},
			"Flags 2": []any{uint64(14), uint64(16)}, "PID High": []any{uint64(16), uint64(18)},
			"Security Features": []any{uint64(18), uint64(26)}, "Reserved": []any{uint64(26), uint64(28)},
			"Tree ID": []any{uint64(28), uint64(30)}, "Process ID Low": []any{uint64(30), uint64(32)},
			"User ID": []any{uint64(32), uint64(34)}, "Multiplex ID": []any{uint64(34), uint64(36)},
		},
	}
	return header
}

func smb1CommandName(command byte) string {
	switch command {
	case 0x00:
		return "SMB_COM_CREATE"
	case 0x04:
		return "SMB_COM_CLOSE"
	case 0x06:
		return "SMB_COM_DELETE"
	case 0x0e:
		return "SMB_COM_LOCKING_ANDX"
	case 0x22:
		return "SMB_COM_SET_INFORMATION2"
	case 0x23:
		return "SMB_COM_QUERY_INFORMATION2"
	case 0x25:
		return "SMB_COM_TRANSACTION"
	case 0x2e:
		return "SMB_COM_READ_ANDX"
	case 0x2f:
		return "SMB_COM_WRITE_ANDX"
	case 0x32:
		return "SMB_COM_TRANSACTION2"
	case 0x33:
		return "SMB_COM_TRANSACTION2_SECONDARY"
	case 0x72:
		return "SMB_COM_NEGOTIATE"
	case 0x73:
		return "SMB_COM_SESSION_SETUP_ANDX"
	case 0x74:
		return "SMB_COM_LOGOFF_ANDX"
	case 0x75:
		return "SMB_COM_TREE_CONNECT_ANDX"
	case 0x81:
		return "SMB_COM_SEARCH"
	case 0x82:
		return "SMB_COM_FIND"
	case 0xa0:
		return "SMB_COM_NT_TRANSACT"
	case 0xa1:
		return "SMB_COM_NT_TRANSACT_SECONDARY"
	case 0xa2:
		return "SMB_COM_NT_CREATE_ANDX"
	}
	return fmt.Sprintf("SMB_COM_0x%02X", command)
}

// The complete WC17 extended-security response has a 73-byte Direct TCP/SMB
// fixed prefix followed by its uint16 ByteCount (GUID plus opaque token).
func probeSMB1NegotiateExtendedResponse(w []byte, limit int) ProbeResult {
	reject := ProbeResult{Verdict: ProbeReject}
	if len(w) < 8 || w[0] != 0 || binary.LittleEndian.Uint32(w[4:8]) != 0x424d53ff {
		return reject
	}
	n := smb2DirectLength(w)
	total := n + 4
	if n < 85 || total > 65608 || total > limit {
		return reject
	}
	if len(w) > 8 && w[8] != 0x72 || len(w) > 13 && w[13]&0x80 == 0 {
		return reject
	}
	if len(w) >= 13 && binary.LittleEndian.Uint32(w[9:13]) != 0 {
		return reject
	}
	if len(w) >= 16 && binary.LittleEndian.Uint16(w[14:16])&0x800 == 0 {
		return reject
	}
	if len(w) > 36 && w[36] != 17 {
		return reject
	}
	if len(w) >= 60 && binary.LittleEndian.Uint32(w[56:60])&0x80000000 == 0 {
		return reject
	}
	if len(w) > 70 && w[70] != 0 {
		return reject
	}
	if len(w) < 73 {
		return probeNeed("smb", "1-negotiate-extended-response", len(w), 73)
	}
	count := int(binary.LittleEndian.Uint16(w[71:73]))
	if count < 16 || count != n-69 {
		return reject
	}
	if len(w) < total {
		return probeNeed("smb", "1-negotiate-extended-response", len(w), total)
	}
	return probeAccept("smb", "1-negotiate-extended-response", 99)
}

// At most one negotiation request is eligible for selection per TCP
// generation. Repeated application requests and orphan-first observations
// permanently remove that eligibility; retransmissions never reach this layer.
type smb1NegotiateState struct {
	seenRequest, seenOrphan, ambiguous, completed bool
	offer                                         *smb1NegotiateOffer
}

type smb1NegotiateOffer struct {
	id     uint64
	pid    uint32
	mid    uint16
	flags2 uint16
	dir    int
	hash   string
	names  [][]byte
}

func (s *smb1NegotiateState) ownedBytes() int64 {
	if s == nil {
		return 0
	}
	n := int64(128)
	if s.offer != nil {
		n += 128 + int64(len(s.offer.hash)) + int64(cap(s.offer.names))*24
		for _, name := range s.offer.names {
			n += int64(cap(name))
		}
	}
	return n
}

func smb1NegotiateIdentity(fields map[string]any) (uint32, uint16, error) {
	h, ok := fields["Header"].(map[string]any)
	if !ok {
		return 0, 0, fmt.Errorf("CIFS: missing owned header")
	}
	high, a := h["PID High"].(uint16)
	low, b := h["Process ID Low"].(uint16)
	mid, c := h["Multiplex ID"].(uint16)
	if !a || !b || !c {
		return 0, 0, fmt.Errorf("CIFS: missing typed PID/MID")
	}
	return uint32(high)<<16 | uint32(low), mid, nil
}

func (f *binFlow) consumeSMB1Negotiate(dir int, e *ProtocolEvent) (resultErr error) {
	response := e.Entry == "CIFSDirectTCPNegotiateExtendedResponse"
	if response && len(e.Raw) == 39 && e.Raw[36] == 0 && binary.LittleEndian.Uint16(e.Raw[37:39]) == 0 {
		return f.consumeSMB1EmptyError(dir, e)
	}
	// Retain refusal authority before invoking the bounded codec. Caller edits
	// must not turn a resource refusal into a successful generic raw decode.
	e.nativeSessionMessage = true
	e.Profile, e.Completeness = "cifs-negotiate-request", "message"
	if response {
		e.Profile = "cifs-negotiate-extended-response"
	}
	var fields map[string]any
	var err error
	if response {
		fields, err = decodeSMB1NegotiateResponseFields(e.Raw, f.a.budget)
	} else {
		fields, err = decodeSMB1NegotiateFields(e.Raw, f.a.budget)
	}
	if err != nil {
		if _, typed := classifySessionError(err); typed == nil {
			err = protocolError(ErrMalformedMessage, "%s", err)
		}
		return err
	}
	smb1AttachContext(fields, 0x72)
	pid, mid, err := smb1NegotiateIdentity(fields)
	if err != nil {
		return err
	}
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	if f.smb2 == nil {
		return sessionContext("CIFS: carrier state unavailable")
	}
	tx, err := f.beginSMB1Legacy(dir, e, fields)
	if err != nil {
		return err
	}
	defer tx.finish(&resultErr)
	previous := f.smb2.smb1
	var next smb1NegotiateState
	if previous != nil {
		next = *previous
	}
	if f.smb2.smb1Modern {
		next.ambiguous, next.offer = true, nil
	}
	fields["Selection Policy"] = "one observed negotiation per TCP generation; additional requests disable name resolution"
	if !tx.allowed {
		fields["Selection Status"] = tx.reason
		if !response {
			// Even a cross-command collision is an observed additional offer.
			// It cannot bypass the stricter one-negotiation-per-generation policy.
			next.ambiguous, next.offer, next.seenRequest = true, nil, true
		}
	} else if !response {
		if next.ambiguous || next.seenRequest || next.seenOrphan || next.completed {
			next.ambiguous = true
			next.offer = nil
			fields["Selection Status"] = "additional or post-response request; selection context ambiguous"
		} else {
			rows, ok := fields["Dialects"].([]map[string]any)
			if !ok {
				return fmt.Errorf("CIFS: missing owned dialect collection")
			}
			h := fields["Header"].(map[string]any)
			offer := &smb1NegotiateOffer{id: e.ID, pid: pid, mid: mid, dir: dir, flags2: h["Flags 2"].(uint16), names: make([][]byte, len(rows))}
			digest := sha256.Sum256(e.Raw)
			offer.hash = hex.EncodeToString(digest[:])
			for i, row := range rows {
				name, ok := row["Raw Name Bytes"].([]byte)
				if !ok {
					return fmt.Errorf("CIFS: missing owned raw dialect name")
				}
				offer.names[i] = bytes.Clone(name)
			}
			next.offer = offer
			fields["Selection Status"] = "request observed; response required"
		}
		next.seenRequest = true
	} else {
		fields["Selection Status"] = "request context unavailable"
		if next.ambiguous || next.completed {
			fields["Selection Status"] = "negotiation context ambiguous or already completed"
		} else if offer := next.offer; offer != nil {
			if offer.dir == dir || offer.pid != pid || offer.mid != mid {
				fields["Selection Status"] = "response direction or PID/MID does not match observed request"
			} else {
				e.ResponseTo = offer.id
				fields["Matched Request"] = "SMB_COM_NEGOTIATE"
				fields["Request Event ID"], fields["Request PDU SHA256"] = offer.id, offer.hash
				body := fields["Body"].(map[string]any)
				index := int(body["Dialect Index"].(uint16))
				fields["Selection Status"] = "matched request; index or extended-security selection is inconsistent"
				if index < len(offer.names) && bytes.Equal(offer.names[index], []byte("NT LM 0.12")) && offer.flags2&0x800 != 0 {
					fields["Selected Dialect Name"] = "NT LM 0.12"
					fields["Selected Dialect Raw Name Bytes"] = bytes.Clone(offer.names[index])
					fields["Dialect Negotiated"] = true
					fields["Selection Status"] = "observed matching request selection; authentication unverified"
				}
				next.completed = true
				next.offer = nil
			}
		} else {
			next.seenOrphan = true
		}
	}
	if f.smb2.smb1Modern {
		fields["Selection Status"] = "SMB2 carrier already observed; legacy selection unavailable"
	}
	additional := next.ownedBytes() - previous.ownedBytes()
	if err = f.smb2.reserveState(additional); err != nil {
		e.ResponseTo = 0
		return smb1ResourceRefusal(err)
	}
	f.smb2.smb1 = &next
	if err = f.smb2.reserveState(0); err != nil {
		return smb1ResourceRefusal(err)
	}
	e.Session = fields
	e.Profile, e.Completeness = "cifs-negotiate-request", "message"
	if response {
		e.Profile = "cifs-negotiate-extended-response"
	}
	e.nativeSessionMessage = true
	e.semanticFields, e.semanticSession = cloneSession(fields), cloneSession(fields)
	return nil
}
