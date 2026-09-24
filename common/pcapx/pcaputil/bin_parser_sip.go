package pcaputil

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// binSIP is the M0 session state for RFC 3261. Ports 5060/5061 are never
// consulted. ACK and CANCEL are first-class methods. UDP retransmission is
// the same Call-ID/CSeq/Via-branch seen again before a final response.
type binSIP struct {
	pending        map[sipTxnKey]string
	pendingAt      map[sipTxnKey]time.Time
	inviteStatus   map[sipTxnKey]int
	inviteComplete map[sipTxnKey]int
	completedAt    map[sipTxnKey]time.Time
	seen           map[sipTxnKey]int
	seenAt         map[sipTxnKey]time.Time
	finalResponses map[sipFinalResponseKey]int
	responseAt     map[sipFinalResponseKey]time.Time
	responseDir    map[sipFinalResponseKey]int
	dialogs        map[sipDialogKey]string
	dialogAt       map[sipDialogKey]time.Time
	dialogTxns     map[sipDialogKey]sipTxnKey
	reserveMemory  func(int64) error
}

const (
	sipTransactionStateTTL = 32 * time.Second
	sipDialogStateTTL      = 30 * time.Minute
	sipMaxHeaderBytes      = 32 << 10
)

type sipDialogKey struct{ callID, fromTag, toTag string }

type sipFinalResponseKey struct {
	txn    sipTxnKey
	toTag  string
	status int
}

type sipTxnKey struct {
	callID, branch, method string
	cseq                   uint32
	direction              int
}

var sipCompact = map[string]string{
	"i": "call-id",
	"m": "contact",
	"e": "content-encoding",
	"l": "content-length",
	"c": "content-type",
	"f": "from",
	"s": "subject",
	"k": "supported",
	"t": "to",
	"v": "via",
}

var sipMethods = []string{
	"INVITE", "ACK", "BYE", "CANCEL", "REGISTER", "OPTIONS",
	"INFO", "PRACK", "SUBSCRIBE", "NOTIFY", "UPDATE", "MESSAGE", "REFER", "PUBLISH",
}

func probeSIP(w []byte, limit int) ProbeResult {
	if len(w) == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if !sipLooksLike(w) {
		return ProbeResult{Verdict: ProbeReject}
	}
	line, ok := sipFirstLine(w)
	if !ok {
		if len(w) >= min(limit, 512) {
			return ProbeResult{Verdict: ProbeReject}
		}
		return probeNeed("sip", "2.0", len(w), min(limit, 64))
	}
	if sipRequestLine(line) || sipResponseLine(line) {
		return probeAccept("sip", "2.0", 93)
	}
	return ProbeResult{Verdict: ProbeReject}
}

func sipLooksLike(w []byte) bool {
	if bytes.HasPrefix(w, []byte("SIP/2.0")) {
		return true
	}
	for _, m := range sipMethods {
		if bytes.HasPrefix(w, []byte(m+" ")) {
			return true
		}
	}
	return false
}

func sipFirstLine(w []byte) (string, bool) {
	i := bytes.Index(w, []byte("\r\n"))
	if i < 0 {
		return "", false
	}
	return string(w[:i]), true
}

func sipRequestLine(line string) bool {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 || parts[2] != "SIP/2.0" {
		return false
	}
	ok := false
	for _, m := range sipMethods {
		if parts[0] == m {
			ok = true
			break
		}
	}
	if !ok {
		return false
	}
	if parts[1] == "*" {
		return parts[0] == "OPTIONS"
	}
	// RFC 3261 permits a general absolute URI (for example tel:), not only
	// sip: and sips:. Require an absolute, nonempty URI so a relative HTTP
	// target or malformed escape cannot gain SIP admission by its method.
	for i := 0; i < len(parts[1]); i++ {
		c := parts[1][i]
		if c <= ' ' || c >= 0x7f || c == '<' || c == '>' || c == '"' || c == '\\' {
			return false
		}
		if c == '%' {
			if i+2 >= len(parts[1]) || !sipURIHex(parts[1][i+1]) || !sipURIHex(parts[1][i+2]) {
				return false
			}
			i += 2
		}
	}
	u, err := url.ParseRequestURI(parts[1])
	return err == nil && u.IsAbs() && (u.Opaque != "" || u.Host != "" || u.Path != "")
}

func sipURIHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func sipResponseLine(line string) bool {
	if !strings.HasPrefix(line, "SIP/2.0 ") || len(line) < 12 || line[11] != ' ' {
		return false
	}
	code := line[8:11]
	if code[0] < '1' || code[0] > '6' {
		return false
	}
	return code[1] >= '0' && code[1] <= '9' && code[2] >= '0' && code[2] <= '9'
}

func (f *binFlow) frameSIP(dir int, w []byte) (int, *binSpec, error) {
	s := f.sip
	if s == nil {
		return 0, nil, sessionContext("SIP session was not observed")
	}
	s.reserveMemory = f.reserveSession
	if err := s.reserve(s.retainedBytes()); err != nil {
		return 0, nil, err
	}
	end := bytes.Index(w, []byte("\r\n\r\n"))
	if end < 0 {
		if len(w) > sipMaxHeaderBytes {
			return 0, nil, fmt.Errorf("sip: header exceeds 32 KiB")
		}
		return 0, nil, nil
	}
	if end+4 > sipMaxHeaderBytes {
		return 0, nil, fmt.Errorf("sip: header exceeds 32 KiB")
	}
	cl, err := sipContentLength(w[:end])
	if err != nil {
		return 0, nil, err
	}
	n := end + 4 + cl
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	if err := s.reserve(s.retainedBytes() + s.projectedStateBytes(w[:n], dir, f.a.budget.MaxCollectionElements)); err != nil {
		return 0, nil, err
	}
	return n, f.a.specs["sip/SIP"], nil
}

func (s *binSIP) reserve(target int64) error {
	if s.reserveMemory == nil {
		return nil
	}
	return s.reserveMemory(target)
}

func sipTxnRetainedBytes(key sipTxnKey) int64 {
	return 64 + int64(len(key.callID)+len(key.branch)+len(key.method))
}

func sipDialogRetainedBytes(key sipDialogKey) int64 {
	return 64 + int64(len(key.callID)+len(key.fromTag)+len(key.toTag))
}

func sipFinalRetainedBytes(key sipFinalResponseKey) int64 {
	return 64 + int64(len(key.txn.callID)+len(key.txn.branch)+len(key.txn.method)+len(key.toTag))
}

// retainedBytes charges each retained key copy separately. Several correlation
// maps intentionally share immutable strings; charging every map key's string
// data is conservative and keeps attacker-controlled identifiers inside the
// capture memory budget.
func (s *binSIP) retainedBytes() int64 {
	n := int64(256)
	for key, value := range s.pending {
		n += sipTxnRetainedBytes(key) + int64(len(value))
	}
	for key := range s.pendingAt {
		n += sipTxnRetainedBytes(key)
	}
	for key := range s.inviteStatus {
		n += sipTxnRetainedBytes(key)
	}
	for key := range s.inviteComplete {
		n += sipTxnRetainedBytes(key)
	}
	for key := range s.completedAt {
		n += sipTxnRetainedBytes(key)
	}
	for key := range s.seen {
		n += sipTxnRetainedBytes(key)
	}
	for key := range s.seenAt {
		n += sipTxnRetainedBytes(key)
	}
	for key := range s.finalResponses {
		n += sipFinalRetainedBytes(key)
	}
	for key := range s.responseAt {
		n += sipFinalRetainedBytes(key)
	}
	for key := range s.responseDir {
		n += sipFinalRetainedBytes(key)
	}
	for key, value := range s.dialogs {
		n += sipDialogRetainedBytes(key) + int64(len(value))
	}
	for key := range s.dialogAt {
		n += sipDialogRetainedBytes(key)
	}
	for key, txn := range s.dialogTxns {
		n += sipDialogRetainedBytes(key) + sipTxnRetainedBytes(txn)
	}
	return n
}

// projectedStateBytes reserves key storage before consumeAt mutates correlation
// tables. The frame is already syntax-checked; this computes only the state that
// the valid request/response path can add.
func (s *binSIP) projectedStateBytes(raw []byte, dir, max int) int64 {
	end := bytes.Index(raw, []byte("\r\n\r\n"))
	if end < 0 {
		return 0
	}
	lineEnd := bytes.Index(raw, []byte("\r\n"))
	if lineEnd < 0 {
		return 0
	}
	header, err := sipParseHeaders(raw[:end])
	if err != nil {
		return 0
	}
	callID := header["call-id"]
	seq, method, err := sipParseCSeq(header["cseq"])
	if err != nil || callID == "" {
		return 0
	}
	key := sipTxnKey{callID: callID, branch: strings.Clone(sipParam(header["via"], "branch")), method: strings.Clone(method), cseq: seq, direction: dir}
	max = sessionCollectionLimit(max)
	if sipResponseLine(string(raw[:lineEnd])) {
		requestKey := key
		if dir >= 0 {
			requestKey.direction = 1 - dir
		}
		request, matched := s.pending[requestKey]
		if !matched {
			return 0
		}
		status, _ := strconv.Atoi(string(raw[8:11]))
		if status >= 200 && len(s.finalResponses) >= max {
			return 0 // consumeAt returns the same resource error before mutation.
		}
		var delta int64
		if request == "INVITE" && status >= 100 && status < 200 {
			if _, exists := s.inviteStatus[requestKey]; !exists {
				delta += sipTxnRetainedBytes(requestKey)
			}
		}
		if status >= 200 && len(s.finalResponses) < max {
			finalKey := sipFinalResponseKey{txn: requestKey, toTag: strings.Clone(sipParam(header["to"], "tag")), status: status}
			if _, exists := s.finalResponses[finalKey]; !exists {
				delta += sipFinalRetainedBytes(finalKey)
				if _, exists := s.responseDir[finalKey]; !exists {
					delta += sipFinalRetainedBytes(finalKey)
				}
				delta += sipFinalRetainedBytes(finalKey) // responseAt is retained with live capture timestamps.
			}
		}
		if hasDialog := request == "INVITE" && status >= 180 && status < 300 || request == "BYE" && status >= 200 && status < 300; hasDialog {
			dialog, ok := makeSIPDialogKey(callID, strings.Clone(sipParam(header["from"], "tag")), strings.Clone(sipParam(header["to"], "tag")))
			if ok {
				if _, exists := s.dialogs[dialog]; !exists && len(s.dialogs) < max {
					delta += sipDialogRetainedBytes(dialog) + 8 // compact state value
				}
				if request == "INVITE" {
					if _, exists := s.dialogTxns[dialog]; !exists && len(s.dialogTxns) < max {
						delta += sipDialogRetainedBytes(dialog) + sipTxnRetainedBytes(requestKey)
					}
				}
				if _, exists := s.dialogAt[dialog]; !exists {
					delta += sipDialogRetainedBytes(dialog)
				}
			}
		}
		if request == "INVITE" && status >= 300 {
			if _, exists := s.inviteComplete[requestKey]; !exists {
				delta += sipTxnRetainedBytes(requestKey)
			}
			if _, exists := s.completedAt[requestKey]; !exists {
				delta += sipTxnRetainedBytes(requestKey)
			}
		}
		return delta
	}
	line := string(raw[:lineEnd])
	if !sipRequestLine(line) || s.seen[key] > 0 || len(s.seen) >= max {
		return 0
	}
	delta := sipTxnRetainedBytes(key) // seen
	if _, exists := s.seenAt[key]; !exists {
		delta += sipTxnRetainedBytes(key)
	}
	if method != "ACK" && len(s.pending) < max {
		if _, exists := s.pending[key]; !exists {
			delta += sipTxnRetainedBytes(key)
		}
		if _, exists := s.pendingAt[key]; !exists {
			delta += sipTxnRetainedBytes(key)
		}
	}
	return delta
}

func sipContentLength(header []byte) (int, error) {
	hs, err := sipParseHeaders(header)
	if err != nil {
		return 0, err
	}
	v, ok := hs["content-length"]
	if !ok || v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("sip: invalid Content-Length")
	}
	return n, nil
}

func sipParseHeaders(header []byte) (map[string]string, error) {
	lines := bytes.Split(header, []byte("\r\n"))
	if len(lines) == 0 {
		return nil, fmt.Errorf("sip: empty header")
	}
	out := map[string]string{}
	var last string
	for i, line := range lines {
		if i == 0 {
			continue
		}
		if len(line) == 0 {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if last == "" {
				return nil, fmt.Errorf("sip: folded header without name")
			}
			out[last] = out[last] + " " + string(bytes.TrimSpace(line))
			continue
		}
		name, value, ok := bytes.Cut(line, []byte(":"))
		if !ok {
			return nil, fmt.Errorf("sip: header missing colon")
		}
		key := sipHeaderName(string(name))
		valueText := strings.TrimSpace(string(value))
		if key == "content-length" {
			if previous, exists := out[key]; exists && previous != valueText {
				return nil, protocolError(ErrMalformedMessage, "SIP conflicting Content-Length values")
			}
		}
		out[key] = valueText
		last = key
	}
	return out, nil
}

// validateSIPDatagramLength applies SIP's datagram boundary contract. UDP is
// already message framed by the capture; Content-Length must agree exactly so
// trailing bytes cannot silently become part of the body. An absent length is
// only valid for an empty body.
func validateSIPDatagramLength(wire []byte) error {
	end := bytes.Index(wire, []byte("\r\n\r\n"))
	if end < 0 {
		return protocolError(ErrMalformedMessage, "SIP datagram header is incomplete")
	}
	header, err := sipParseHeaders(wire[:end])
	if err != nil {
		return protocolError(ErrMalformedMessage, "SIP datagram header is invalid")
	}
	bodyLen := len(wire) - end - 4
	value, present := header["content-length"]
	if !present {
		if bodyLen != 0 {
			return protocolError(ErrMalformedMessage, "SIP datagram body requires Content-Length")
		}
		return nil
	}
	length, parseErr := strconv.ParseUint(strings.TrimSpace(value), 10, 31)
	if parseErr != nil || int(length) != bodyLen {
		return protocolError(ErrMalformedMessage, "SIP Content-Length does not match datagram boundary")
	}
	return nil
}

func sipHeaderName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if full, ok := sipCompact[n]; ok {
		return full
	}
	return n
}

func (s *binSIP) consume(raw []byte, max int) (map[string]any, error) {
	return s.consumeAt(raw, max, time.Time{})
}

func (s *binSIP) consumeAt(raw []byte, max int, now time.Time, directions ...int) (map[string]any, error) {
	// Direct parser unit callers may omit the transport direction. Real TCP and
	// UDP flow paths pass their stable canonical direction so response matching
	// cannot accept a message emitted by the request originator.
	dir := -1
	if len(directions) > 0 && (directions[0] == 0 || directions[0] == 1) {
		dir = directions[0]
	}
	if s.pending == nil {
		s.pending = map[sipTxnKey]string{}
	}
	if s.seen == nil {
		s.seen = map[sipTxnKey]int{}
	}
	if s.pendingAt == nil {
		s.pendingAt = map[sipTxnKey]time.Time{}
	}
	if s.inviteStatus == nil {
		s.inviteStatus = map[sipTxnKey]int{}
	}
	if s.inviteComplete == nil {
		s.inviteComplete = map[sipTxnKey]int{}
	}
	if s.completedAt == nil {
		s.completedAt = map[sipTxnKey]time.Time{}
	}
	if s.seenAt == nil {
		s.seenAt = map[sipTxnKey]time.Time{}
	}
	if s.finalResponses == nil {
		s.finalResponses = map[sipFinalResponseKey]int{}
	}
	if s.responseAt == nil {
		s.responseAt = map[sipFinalResponseKey]time.Time{}
	}
	if s.responseDir == nil {
		s.responseDir = map[sipFinalResponseKey]int{}
	}
	if s.dialogs == nil {
		s.dialogs = map[sipDialogKey]string{}
	}
	if s.dialogAt == nil {
		s.dialogAt = map[sipDialogKey]time.Time{}
	}
	if s.dialogTxns == nil {
		s.dialogTxns = map[sipDialogKey]sipTxnKey{}
	}
	if max <= 0 {
		max = 4096
	}
	s.prune(now)
	end := bytes.Index(raw, []byte("\r\n\r\n"))
	if end < 0 {
		return nil, fmt.Errorf("sip: truncated header")
	}
	line := string(raw[:bytes.Index(raw, []byte("\r\n"))])
	hs, err := sipParseHeaders(raw[:end])
	if err != nil {
		return nil, err
	}
	callID := hs["call-id"]
	if callID == "" {
		return nil, protocolError(ErrMalformedMessage, "SIP Call-ID is required")
	}
	cseqN, cseqM, err := sipParseCSeq(hs["cseq"])
	if err != nil {
		return nil, err
	}
	from := hs["from"]
	to := hs["to"]
	via := hs["via"]
	info := map[string]any{
		"Call-ID":       callID,
		"CSeq":          cseqN,
		"CSeq Method":   cseqM,
		"From":          from,
		"To":            to,
		"From Tag":      sipParam(from, "tag"),
		"To Tag":        sipParam(to, "tag"),
		"Via":           via,
		"Via Branch":    sipParam(via, "branch"),
		"Content Type":  hs["content-type"],
		"Context Level": "observed",
	}
	key := sipTxnKey{callID: callID, branch: strings.Clone(sipParam(via, "branch")), method: strings.Clone(strings.ToUpper(cseqM)), cseq: cseqN, direction: dir}
	fromTag := strings.Clone(sipParam(from, "tag"))
	toTag := strings.Clone(sipParam(to, "tag"))
	dialog, hasDialog := makeSIPDialogKey(key.callID, fromTag, toTag)
	body := raw[end+4:]
	if strings.Contains(strings.ToLower(hs["content-type"]), "application/sdp") {
		if sdp, sdpErr := sipParseSDP(body, max); sdpErr != nil {
			return nil, sdpErr
		} else if sdp != nil {
			info["SDP"] = sdp
		}
	}
	if sipResponseLine(line) {
		status, _ := strconv.Atoi(line[8:11])
		info["Packet Name"] = "Response"
		info["Status"] = status
		info["Reason"] = strings.TrimSpace(line[11:])
		if hasDialog {
			info["Dialog Identity"] = sipDialogIdentity(callID, dialog)
		}
		requestKey := key
		if dir >= 0 {
			requestKey.direction = 1 - dir
		}
		// If both peers independently emit the same transaction tuple, the
		// response cannot identify which request it answers. Keep it explicitly
		// ambiguous instead of allowing endpoint/direction collisions to cross-link.
		if dir >= 0 {
			sameDirectionKey := requestKey
			sameDirectionKey.direction = dir
			_, sameDirectionPending := s.pending[sameDirectionKey]
			_, oppositeDirectionPending := s.pending[requestKey]
			if sameDirectionPending && !oppositeDirectionPending {
				markSIPDirectionMismatch(info)
				return info, nil
			}
			if sameDirectionPending && oppositeDirectionPending {
				info["Unmatched"] = true
				info["Association Status"] = "ambiguous-direction"
				info["Context Level"] = "partial"
				return info, nil
			}
		}
		responseKey := sipFinalResponseKey{txn: requestKey, toTag: strings.Clone(sipParam(to, "tag")), status: status}
		if count := s.finalResponses[responseKey]; count > 0 {
			if dir >= 0 {
				if previousDir, known := s.responseDir[responseKey]; known && previousDir >= 0 && previousDir != dir {
					markSIPDirectionMismatch(info)
					return info, nil
				}
			}
			info["Matched Request"] = key.method
			info["Association Status"] = "matched"
			info["Retransmission"] = true
			info["Retransmission Count"] = count
			s.finalResponses[responseKey] = count + 1
			return info, nil
		}
		if want, ok := s.pending[requestKey]; ok {
			if status >= 200 && len(s.finalResponses) >= max {
				return info, protocolError(ErrResourceExceeded, "SIP final-response replay window exceeds budget")
			}
			info["Matched Request"] = want
			info["Association Status"] = "matched"
			if want == "INVITE" && status >= 100 {
				s.inviteStatus[requestKey] = status
			}
			if status >= 200 {
				s.finalResponses[responseKey] = 1
				s.responseDir[responseKey] = dir
				if !now.IsZero() {
					s.responseAt[responseKey] = now
				}
				// A forked INVITE can receive more than one 2xx response with
				// the same transaction branch/CSeq but different To-tags. Keep
				// the transaction until its normal capture-state TTL expires.
				if want != "INVITE" || status >= 300 {
					delete(s.pending, requestKey)
					delete(s.pendingAt, requestKey)
				}
				if want == "INVITE" && status >= 300 {
					s.inviteComplete[requestKey] = status
					delete(s.inviteStatus, requestKey)
					if !now.IsZero() {
						s.completedAt[requestKey] = now
					}
				}
			}
			if want == "INVITE" && hasDialog && status >= 180 && status < 300 {
				if _, exists := s.dialogs[dialog]; !exists && len(s.dialogs) >= max {
					return nil, protocolError(ErrResourceExceeded, "SIP dialog state exceeds budget")
				}
				if _, exists := s.dialogTxns[dialog]; !exists && len(s.dialogTxns) >= max {
					return nil, protocolError(ErrResourceExceeded, "SIP dialog transaction links exceed budget")
				}
				state := "early"
				if status >= 200 {
					state = "confirmed"
				}
				s.dialogs[dialog] = state
				s.dialogTxns[dialog] = requestKey
				if !now.IsZero() {
					s.dialogAt[dialog] = now
				}
				info["Dialog State"] = state
			}
			if want == "BYE" && hasDialog && status >= 200 && status < 300 {
				s.dialogs[dialog] = "terminated"
				if !now.IsZero() {
					s.dialogAt[dialog] = now
				}
				info["Dialog State"] = "terminated"
			}
		} else {
			info["Unmatched"] = true
			info["Association Status"] = "missing-request"
			info["Context Level"] = "partial"
		}
		return info, nil
	}
	if !sipRequestLine(line) {
		return nil, fmt.Errorf("sip: invalid start line")
	}
	method := strings.SplitN(line, " ", 3)[0]
	uri := strings.SplitN(line, " ", 3)[1]
	info["Packet Name"] = method
	info["Method"] = method
	info["URI"] = uri
	if hasDialog {
		info["Dialog Identity"] = sipDialogIdentity(callID, dialog)
		if state, ok := s.dialogs[dialog]; ok {
			info["Dialog State"] = state
		}
	}
	if method == "ACK" {
		invite := key
		invite.method = "INVITE"
		if hasDialog {
			if inviteTxn, ok := s.dialogTxns[dialog]; ok && (key.direction < 0 || inviteTxn.direction < 0 || inviteTxn.direction == key.direction) {
				if state := s.dialogs[dialog]; state == "confirmed" || state == "terminating" || state == "terminated" {
					info["ACK For INVITE"] = "2xx-dialog"
					info["Association Status"] = "invite-2xx-ack"
					info["INVITE CSeq"] = inviteTxn.cseq
				}
			} else if inviteTxn, ok := s.dialogTxns[dialog]; ok && key.direction >= 0 && inviteTxn.direction >= 0 && inviteTxn.direction != key.direction {
				markSIPDirectionMismatch(info)
			}
		}
		if status := s.inviteComplete[invite]; status >= 300 {
			info["ACK For INVITE"] = status
			info["Association Status"] = "non-2xx-invite-ack"
			delete(s.inviteComplete, invite)
			delete(s.completedAt, invite)
		} else if info["Association Status"] == nil {
			if want, ok := s.pending[invite]; ok && want == "INVITE" {
				info["ACK For INVITE"] = "2xx-or-pending"
				info["Association Status"] = "invite-ack"
			}
		}
	}
	if method == "BYE" && hasDialog {
		if _, ok := s.dialogs[dialog]; ok {
			s.dialogs[dialog] = "terminating"
			if !now.IsZero() {
				s.dialogAt[dialog] = now
			}
			info["Dialog State"] = "terminating"
		}
	}
	if n := s.seen[key]; n > 0 {
		info["Retransmission"] = true
		info["Retransmission Count"] = n
		s.seen[key] = n + 1
		return info, nil
	}
	if len(s.seen) >= max {
		return info, protocolError(ErrResourceExceeded, "SIP transaction replay window exceeds budget")
	}
	if hasDialog && (method == "INVITE" || method == "BYE") {
		if _, exists := s.dialogs[dialog]; !exists && len(s.dialogs) >= max {
			return info, protocolError(ErrResourceExceeded, "SIP dialog state exceeds budget")
		}
		if method == "INVITE" {
			if _, exists := s.dialogTxns[dialog]; !exists && len(s.dialogTxns) >= max {
				return info, protocolError(ErrResourceExceeded, "SIP dialog transaction links exceed budget")
			}
		}
	}
	s.seen[key] = 1
	if !now.IsZero() {
		s.seenAt[key] = now
	}
	switch method {
	case "ACK":
		if info["Association Status"] == nil {
			info["Association Status"] = "ack"
		}
	case "CANCEL":
		inv := key
		inv.method = "INVITE"
		if _, ok := s.pending[inv]; ok && s.inviteStatus[inv] < 200 {
			info["Cancels"] = "INVITE"
		}
		fallthrough
	default:
		if method != "ACK" {
			if len(s.pending) >= max {
				return info, protocolError(ErrResourceExceeded, "SIP outstanding transactions exceed budget")
			}
			s.pending[key] = method
			if !now.IsZero() {
				s.pendingAt[key] = now
			}
			info["Outstanding"] = true
		}
	}
	return info, nil
}

func markSIPDirectionMismatch(info map[string]any) {
	info["Unmatched"] = true
	info["Association Status"] = "direction-mismatch"
	info["Context Level"] = "partial"
}

func makeSIPDialogKey(callID, fromTag, toTag string) (sipDialogKey, bool) {
	if callID == "" || fromTag == "" || toTag == "" {
		return sipDialogKey{}, false
	}
	if toTag < fromTag {
		fromTag, toTag = toTag, fromTag
	}
	return sipDialogKey{callID: callID, fromTag: fromTag, toTag: toTag}, true
}

func sipDialogIdentity(callID string, dialog sipDialogKey) map[string]string {
	return map[string]string{"Call-ID": callID, "Tag A": dialog.fromTag, "Tag B": dialog.toTag}
}

func (s *binSIP) prune(now time.Time) {
	if now.IsZero() {
		return
	}
	expired := func(created time.Time) bool {
		return !created.IsZero() && !now.Before(created) && now.Sub(created) >= sipTransactionStateTTL
	}
	for key, created := range s.pendingAt {
		if expired(created) {
			delete(s.pendingAt, key)
			delete(s.pending, key)
			delete(s.inviteStatus, key)
		}
	}
	for key, created := range s.completedAt {
		if expired(created) {
			delete(s.completedAt, key)
			delete(s.inviteComplete, key)
			delete(s.inviteStatus, key)
		}
	}
	for key, created := range s.seenAt {
		if expired(created) {
			delete(s.seenAt, key)
			delete(s.seen, key)
		}
	}
	for key, created := range s.responseAt {
		if expired(created) {
			delete(s.responseAt, key)
			delete(s.finalResponses, key)
			delete(s.responseDir, key)
		}
	}
	for key, created := range s.dialogAt {
		if !created.IsZero() && !now.Before(created) && now.Sub(created) >= sipDialogStateTTL {
			delete(s.dialogAt, key)
			delete(s.dialogs, key)
			delete(s.dialogTxns, key)
		}
	}
	for dialog, txn := range s.dialogTxns {
		if _, exists := s.dialogs[dialog]; !exists {
			delete(s.dialogTxns, dialog)
			if status, exists := s.inviteStatus[txn]; exists && status < 300 {
				delete(s.inviteStatus, txn)
			}
		}
	}
}

func sipParseCSeq(v string) (uint32, string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, "", protocolError(ErrMalformedMessage, "SIP CSeq is required")
	}
	num, meth, ok := strings.Cut(v, " ")
	if !ok {
		return 0, "", protocolError(ErrMalformedMessage, "SIP CSeq must be number and method")
	}
	n, err := strconv.ParseUint(strings.TrimSpace(num), 10, 32)
	if err != nil {
		return 0, "", protocolError(ErrMalformedMessage, "SIP CSeq number is invalid")
	}
	return uint32(n), strings.ToUpper(strings.TrimSpace(meth)), nil
}

func sipParam(v, name string) string {
	low := strings.ToLower(v)
	key := ";" + strings.ToLower(name) + "="
	i := strings.Index(low, key)
	if i < 0 {
		return ""
	}
	rest := v[i+len(key):]
	if j := strings.IndexAny(rest, ";>"); j >= 0 {
		rest = rest[:j]
	}
	return strings.Trim(strings.TrimSpace(rest), `"`)
}

func sipParseSDP(body []byte, maxElements int) (map[string]any, error) {
	if len(body) == 0 || !bytes.HasPrefix(body, []byte("v=")) {
		return nil, nil
	}
	if len(body) > 1<<20 {
		return nil, protocolError(ErrResourceExceeded, "sdp: body exceeds 1 MiB")
	}
	if maxElements <= 0 {
		maxElements = DefaultParserBudget().MaxCollectionElements
	}
	out := map[string]any{}
	var sessionConnection map[string]any
	var media []map[string]any
	var current map[string]any
	lineCount, formatCount, rtpMapCount := 0, 0, 0
	for offset := 0; offset < len(body); {
		lineEnd := bytes.Index(body[offset:], []byte("\r\n"))
		end := len(body)
		if lineEnd >= 0 {
			end = offset + lineEnd
		}
		line := body[offset:end]
		offset = end
		if offset < len(body) {
			offset += 2
		}
		if len(line) == 0 && offset >= len(body) {
			break
		}
		lineCount++
		if lineCount > maxElements {
			return nil, protocolError(ErrResourceExceeded, "sdp: line budget exceeded")
		}
		if len(line) < 2 || line[1] != '=' {
			return nil, protocolError(ErrMalformedMessage, "sdp: invalid line")
		}
		val := string(line[2:])
		switch line[0] {
		case 'v':
			out["Version"] = val
		case 'o':
			out["Origin"] = val
		case 's':
			out["Session Name"] = val
		case 'c':
			connection, err := sipSDPConnection(val)
			if err != nil {
				return nil, err
			}
			if current == nil {
				sessionConnection = connection
				out["Connection"] = connection
			} else {
				current["Connection"] = connection
			}
		case 'm':
			if len(media) >= maxElements {
				return nil, protocolError(ErrResourceExceeded, "sdp: media section budget exceeded")
			}
			parts, fieldsErr := sipSDPFieldsBounded(val, maxElements+3)
			if fieldsErr != nil {
				return nil, fieldsErr
			}
			if len(parts) < 4 {
				return nil, protocolError(ErrMalformedMessage, "sdp: malformed media description")
			}
			portText, portCountText, hasPortCount := strings.Cut(parts[1], "/")
			if hasPortCount && strings.Contains(portCountText, "/") {
				return nil, protocolError(ErrMalformedMessage, "sdp: invalid media port")
			}
			port, err := strconv.Atoi(portText)
			if err != nil || port < 0 || port > 65535 {
				return nil, protocolError(ErrMalformedMessage, "sdp: invalid media port")
			}
			portCount := 1
			if hasPortCount {
				portCount, err = strconv.Atoi(portCountText)
				if err != nil || portCount < 1 || portCount > maxElements {
					return nil, protocolError(ErrMalformedMessage, "sdp: invalid media port count")
				}
			}
			formatCount += len(parts) - 3
			if formatCount > maxElements {
				return nil, protocolError(ErrResourceExceeded, "sdp: payload format budget exceeded")
			}
			formats := append([]string(nil), parts[3:]...)
			payloads := make([]int, 0, len(formats))
			for _, format := range formats {
				payload, parseErr := strconv.Atoi(format)
				if parseErr == nil && payload >= 0 && payload <= 127 {
					payloads = append(payloads, payload)
				}
			}
			current = map[string]any{
				"Line": val, "Type": parts[0], "Port": parts[1], "Port Number": port,
				"Port Count": portCount, "Proto": parts[2], "Format": strings.Join(formats, " "),
				"Formats": formats, "Payload Types": payloads, "Direction": "sendrecv",
				"RTP Maps": []map[string]any{},
			}
			if direction, ok := out["Direction"].(string); ok {
				current["Direction"] = direction
			}
			if sessionConnection != nil {
				current["Connection"] = sessionConnection
			}
			media = append(media, current)
		case 'a':
			if current == nil {
				if val == "sendrecv" || val == "sendonly" || val == "recvonly" || val == "inactive" {
					out["Direction"] = val
				}
				continue
			}
			switch {
			case val == "sendrecv" || val == "sendonly" || val == "recvonly" || val == "inactive":
				current["Direction"] = val
			case val == "rtcp-mux":
				current["RTCP Mux"] = true
				current["RTCP Port Number"] = current["Port Number"]
			case strings.HasPrefix(val, "rtcp:"):
				rtcp, err := sipSDPRTCP(strings.TrimPrefix(val, "rtcp:"), current)
				if err != nil {
					return nil, err
				}
				for k, v := range rtcp {
					current[k] = v
				}
			case strings.HasPrefix(val, "rtpmap:"):
				rtpMapCount++
				if rtpMapCount > maxElements {
					return nil, protocolError(ErrResourceExceeded, "sdp: rtpmap budget exceeded")
				}
				mapping, err := sipSDPRTPMap(strings.TrimPrefix(val, "rtpmap:"))
				if err != nil {
					return nil, err
				}
				maps := current["RTP Maps"].([]map[string]any)
				maps = append(maps, mapping)
				current["RTP Maps"] = maps
			}
		}
	}
	if len(media) > 0 {
		for _, section := range media {
			if section["RTCP Port Number"] != nil && section["RTCP Connection"] == nil {
				if connection, ok := section["Connection"].(map[string]any); ok {
					section["RTCP Connection"] = connection
				}
			}
		}
		out["Media"] = media
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func sipSDPConnection(value string) (map[string]any, error) {
	parts, err := sipSDPFieldsBounded(value, 3)
	if err != nil {
		return nil, err
	}
	if len(parts) != 3 || parts[0] != "IN" || (parts[1] != "IP4" && parts[1] != "IP6") {
		return nil, protocolError(ErrMalformedMessage, "sdp: unsupported connection address family")
	}
	address := strings.SplitN(parts[2], "/", 2)[0]
	ip := net.ParseIP(address)
	if ip == nil || parts[1] == "IP4" && ip.To4() == nil || parts[1] == "IP6" && ip.To4() != nil {
		return nil, protocolError(ErrMalformedMessage, "sdp: invalid connection address")
	}
	return map[string]any{"Network Type": parts[0], "Address Type": parts[1], "Address": ip.String(), "Multicast": ip.IsMulticast(), "Unspecified": ip.IsUnspecified()}, nil
}

func sipSDPRTPMap(value string) (map[string]any, error) {
	parts, err := sipSDPFieldsBounded(value, 2)
	if err != nil {
		return nil, err
	}
	if len(parts) != 2 {
		return nil, protocolError(ErrMalformedMessage, "sdp: malformed rtpmap")
	}
	pt, err := strconv.Atoi(parts[0])
	if err != nil || pt < 0 || pt > 127 {
		return nil, protocolError(ErrMalformedMessage, "sdp: invalid rtpmap payload type")
	}
	codec, clockText, hasClock := strings.Cut(parts[1], "/")
	if !hasClock || codec == "" {
		return nil, protocolError(ErrMalformedMessage, "sdp: invalid rtpmap encoding")
	}
	clockText, channelsText, hasChannels := strings.Cut(clockText, "/")
	if hasChannels && strings.Contains(channelsText, "/") {
		return nil, protocolError(ErrMalformedMessage, "sdp: invalid rtpmap encoding")
	}
	clock, err := strconv.Atoi(clockText)
	if err != nil || clock <= 0 || clock > 1000000 {
		return nil, protocolError(ErrMalformedMessage, "sdp: invalid rtpmap clock rate")
	}
	channels := 1
	if hasChannels {
		channels, err = strconv.Atoi(channelsText)
		if err != nil || channels < 1 || channels > 255 {
			return nil, protocolError(ErrMalformedMessage, "sdp: invalid rtpmap channels")
		}
	}
	return map[string]any{"Payload Type": pt, "Encoding": codec, "Clock Rate": clock, "Channels": channels}, nil
}

func sipSDPRTCP(value string, media map[string]any) (map[string]any, error) {
	parts, err := sipSDPFieldsBounded(value, 4)
	if err != nil {
		return nil, err
	}
	if len(parts) != 1 && len(parts) != 4 {
		return nil, protocolError(ErrMalformedMessage, "sdp: malformed rtcp attribute")
	}
	port, err := strconv.Atoi(parts[0])
	if err != nil || port < 1 || port > 65535 {
		return nil, protocolError(ErrMalformedMessage, "sdp: invalid RTCP port")
	}
	result := map[string]any{"RTCP Port Number": port}
	if len(parts) == 4 {
		connection, err := sipSDPConnection(strings.Join(parts[1:], " "))
		if err != nil {
			return nil, err
		}
		result["RTCP Connection"] = connection
	} else if connection, ok := media["Connection"].(map[string]any); ok {
		result["RTCP Connection"] = connection
	}
	return result, nil
}

func sipSDPFieldsBounded(value string, max int) ([]string, error) {
	if max <= 0 {
		return nil, protocolError(ErrResourceExceeded, "sdp: field budget is empty")
	}
	count := 0
	inField := false
	for _, r := range value {
		space := r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == '\v' || r == '\f'
		if !space && !inField {
			count++
			if count > max {
				return nil, protocolError(ErrResourceExceeded, "sdp: field budget exceeded")
			}
		}
		inField = !space
	}
	return strings.Fields(value), nil
}
