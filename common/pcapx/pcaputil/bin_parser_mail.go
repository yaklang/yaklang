package pcaputil

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const (
	mailLineMax  = 512
	imapLineMax  = 8192
	mailReplyMax = 64 << 10
)

type binSMTP struct {
	client      int
	pending     []string
	pendingHead int
	maxPending  int
	inData      bool
	startTLS    bool
	encrypted   bool
}

type binIMAP struct {
	pending      map[string]string
	maxPending   int
	pendingBytes int
	literal      [2]int
	literalTail  [2]bool
	startTLS     string
	encrypted    bool
}

type binPOP3 struct {
	pending    string
	pendingArg bool
	multiline  bool
	stls       bool
	encrypted  bool
}

func probeSMTP(w []byte, limit int) ProbeResult {
	if len(w) == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if smtpCommandPrefix(w) {
		if !bytes.Contains(w, []byte("\r\n")) {
			if len(w) >= min(limit, mailLineMax) {
				return ProbeResult{Verdict: ProbeReject}
			}
			return probeNeed("smtp", "rfc5321", len(w), min(limit, 16))
		}
		return probeAccept("smtp", "rfc5321", 92)
	}
	if w[0] >= '2' && w[0] <= '5' {
		line, ok := firstCRLF(w)
		if !ok {
			if smtpBannerSMTP(w) {
				return probeAccept("smtp", "rfc5321", 90)
			}
			if smtpReplyPrefix(w) && len(w) < min(limit, 64) {
				return probeNeed("smtp", "rfc5321", len(w), min(limit, 24))
			}
			return ProbeResult{Verdict: ProbeReject}
		}
		if smtpBannerSMTP(line) {
			return probeAccept("smtp", "rfc5321", 90)
		}
	}
	_ = limit
	return ProbeResult{Verdict: ProbeReject}
}

func probeIMAP(w []byte, limit int) ProbeResult {
	if len(w) == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if w[0] == '*' || imapTagStart(w[0]) {
		line, ok := firstCRLF(w)
		if !ok {
			if len(w) >= min(limit, 64) {
				return ProbeResult{Verdict: ProbeReject}
			}
			if w[0] == '*' && (len(w) == 1 || w[1] == ' ' || bytes.HasPrefix(w, []byte("* O")) || bytes.HasPrefix(w, []byte("* OK"))) {
				return probeNeed("imap", "rfc3501", len(w), min(limit, 16))
			}
			return ProbeResult{Verdict: ProbeReject}
		}
		if imapGreetingOrTagged(line) {
			return probeAccept("imap", "rfc3501", 93)
		}
	}
	return ProbeResult{Verdict: ProbeReject}
}

func probePOP3(w []byte, limit int) ProbeResult {
	if len(w) == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if bytes.HasPrefix(w, []byte("+OK ")) || bytes.HasPrefix(w, []byte("-ERR")) {
		if !bytes.Contains(w, []byte("\r\n")) {
			if len(w) >= min(limit, mailLineMax) {
				return ProbeResult{Verdict: ProbeReject}
			}
			return probeNeed("pop3", "rfc1939", len(w), min(limit, 16))
		}
		return probeAccept("pop3", "rfc1939", 94)
	}
	if bytes.HasPrefix(w, []byte("+OK")) && !bytes.Contains(w, []byte("\r\n")) && len(w) < 8 {
		return probeNeed("pop3", "rfc1939", len(w), 8)
	}
	if pop3CommandPrefix(w) {
		if !bytes.Contains(w, []byte("\r\n")) {
			return probeNeed("pop3", "rfc1939", len(w), min(limit, 8))
		}
		return probeAccept("pop3", "rfc1939", 80)
	}
	return ProbeResult{Verdict: ProbeReject}
}

func smtpCommandPrefix(w []byte) bool {
	for _, cmd := range []string{"EHLO ", "HELO ", "LHLO ", "MAIL FROM:", "RCPT TO:", "DATA\r", "STARTTLS", "RSET", "VRFY ", "EXPN ", "NOOP", "QUIT", "HELP", "AUTH "} {
		if bytes.HasPrefix(bytes.ToUpper(w), []byte(cmd)) {
			return true
		}
	}
	return false
}

func smtpReplyPrefix(w []byte) bool {
	if len(w) < 1 || w[0] < '2' || w[0] > '5' {
		return false
	}
	for i := 1; i < len(w) && i < 3; i++ {
		if w[i] < '0' || w[i] > '9' {
			return false
		}
	}
	return true
}

func smtpBannerSMTP(line []byte) bool {
	if len(line) < 3 || !smtpReplyPrefix(line) {
		return false
	}
	if mailHasToken(line, "FTP") {
		return false
	}
	return mailHasToken(line, "ESMTP") || mailHasToken(line, "SMTP")
}

func imapTagStart(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
}

func imapGreetingOrTagged(line []byte) bool {
	fields := bytes.Fields(line)
	if len(fields) < 2 {
		return false
	}
	cmd := strings.ToUpper(string(fields[1]))
	if bytes.Equal(fields[0], []byte("*")) {
		switch cmd {
		case "OK", "NO", "BAD", "BYE", "PREAUTH", "CAPABILITY", "FLAGS", "LIST", "LSUB", "SEARCH", "STATUS", "NAMESPACE":
			return true
		}
		if len(fields) >= 3 {
			third := strings.ToUpper(string(fields[2]))
			switch third {
			case "EXISTS", "RECENT", "EXPUNGE", "FETCH":
				return true
			}
		}
		return false
	}
	return imapKnownCommand(cmd)
}

func imapKnownCommand(cmd string) bool {
	switch cmd {
	case "CAPABILITY", "NOOP", "LOGOUT", "STARTTLS", "AUTHENTICATE", "LOGIN", "SELECT", "EXAMINE",
		"CREATE", "DELETE", "RENAME", "SUBSCRIBE", "UNSUBSCRIBE", "LIST", "LSUB", "STATUS", "APPEND",
		"CHECK", "CLOSE", "EXPUNGE", "SEARCH", "FETCH", "STORE", "COPY", "UID", "IDLE", "ENABLE", "ID":
		return true
	}
	return false
}

func pop3CommandPrefix(w []byte) bool {
	u := bytes.ToUpper(w)
	for _, cmd := range []string{"CAPA\r", "CAPA ", "STAT\r", "STAT ", "STLS\r", "STLS ", "APOP ", "UIDL", "TOP "} {
		if bytes.HasPrefix(u, []byte(cmd)) {
			return true
		}
	}
	return false
}

func firstCRLF(w []byte) ([]byte, bool) {
	i := bytes.Index(w, []byte("\r\n"))
	if i < 0 {
		return nil, false
	}
	return w[:i], true
}

func mailHasToken(line []byte, token string) bool {
	for _, f := range bytes.Fields(bytes.ToUpper(line)) {
		if string(f) == token {
			return true
		}
	}
	return false
}

func frameCRLFLine(w []byte, max int) (int, error) {
	i := bytes.Index(w, []byte("\r\n"))
	if i < 0 {
		if len(w) > max {
			return 0, fmt.Errorf("line exceeds %d bytes without CRLF", max)
		}
		return 0, nil
	}
	n := i + 2
	if n > max {
		return 0, fmt.Errorf("line exceeds %d bytes", max)
	}
	return n, nil
}

func frameCodeReply(w []byte, maxLine, maxTotal int) (int, error) {
	total := 0
	for lines := 0; lines < 1024 && total < len(w); lines++ {
		rest := w[total:]
		i := bytes.Index(rest, []byte("\r\n"))
		if i < 0 {
			if len(w) > maxTotal {
				return 0, fmt.Errorf("reply exceeds %d bytes", maxTotal)
			}
			return 0, nil
		}
		if i+2 > maxLine || i < 3 {
			return 0, fmt.Errorf("invalid reply line length")
		}
		if rest[0] < '1' || rest[0] > '5' || rest[1] < '0' || rest[1] > '9' || rest[2] < '0' || rest[2] > '9' {
			return 0, fmt.Errorf("invalid reply code")
		}
		if total > 0 && !bytes.Equal(rest[:3], w[:3]) {
			return 0, fmt.Errorf("multiline reply codes differ")
		}
		sep := byte(' ')
		if i > 3 {
			sep = rest[3]
			if sep != ' ' && sep != '-' {
				return 0, fmt.Errorf("invalid reply separator")
			}
		}
		total += i + 2
		if sep != '-' {
			return total, nil
		}
	}
	if len(w) > maxTotal {
		return 0, fmt.Errorf("reply exceeds %d bytes", maxTotal)
	}
	return 0, nil
}

func frameDotBlock(w []byte, max int) (int, error) {
	if bytes.HasPrefix(w, []byte(".\r\n")) {
		return 3, nil
	}
	i := bytes.Index(w, []byte("\r\n.\r\n"))
	if i >= 0 {
		n := i + 5
		if n > max {
			return 0, fmt.Errorf("dot-stuffed body exceeds limit")
		}
		return n, nil
	}
	if len(w) > max {
		return 0, fmt.Errorf("dot-stuffed body exceeds limit")
	}
	return 0, nil
}

func mailFirstWord(line []byte) string {
	line = bytes.TrimRight(line, "\r\n")
	if i := bytes.IndexByte(line, ' '); i >= 0 {
		return strings.ToUpper(string(line[:i]))
	}
	return strings.ToUpper(string(line))
}

func (f *binFlow) frameSMTP(dir int, w []byte) (int, *binSpec, error) {
	s := f.smtp
	if s == nil {
		return 0, nil, sessionContext("SMTP session was not observed")
	}
	if err := f.reserveSession(256 + int64(max(cap(s.pending), 2*(s.pendingCount()+1)))*32); err != nil {
		return 0, nil, err
	}
	if s.encrypted {
		if len(w) == 0 {
			return 0, nil, nil
		}
		return 0, nil, protocolError(ErrEncrypted, "SMTP transport is encrypted after STARTTLS")
	}
	if s.inData && dir == s.client {
		n, err := frameDotBlock(w, f.a.config.MaxMessageBytes)
		if err != nil {
			return 0, nil, err
		}
		if n == 0 || n > len(w) {
			return n, nil, nil
		}
		return n, f.spec("smtp_reply", "SMTPReplyCarrier"), nil
	}
	if len(w) >= 3 && w[0] >= '2' && w[0] <= '5' && w[1] >= '0' && w[1] <= '9' && w[2] >= '0' && w[2] <= '9' {
		n, err := frameCodeReply(w, mailLineMax, mailReplyMax)
		if err != nil {
			return 0, nil, err
		}
		if n == 0 || n > len(w) {
			return n, nil, nil
		}
		return n, f.spec("smtp_reply", "SMTPReply"), nil
	}
	n, err := frameCRLFLine(w, mailLineMax)
	if err != nil {
		return 0, nil, err
	}
	if n == 0 || n > len(w) {
		return n, nil, nil
	}
	return n, f.spec("smtp", "SMTPCommand"), nil
}

func (s *binSMTP) pendingCount() int { return len(s.pending) - s.pendingHead }

func (s *binSMTP) consume(dir int, raw []byte) (map[string]any, error) {
	if s.encrypted {
		return nil, protocolError(ErrEncrypted, "SMTP transport is encrypted after STARTTLS")
	}
	if s.inData && dir == s.client {
		s.inData = false
		out := map[string]any{"Packet Name": "DATA Body", "Role": "command", "Bytes": len(raw)}
		if s.pendingCount() > 0 {
			out["In Reply To"] = s.pending[s.pendingHead]
		}
		return out, nil
	}
	if len(raw) >= 3 && raw[0] >= '2' && raw[0] <= '5' && unicode.IsDigit(rune(raw[1])) && unicode.IsDigit(rune(raw[2])) {
		code, _ := strconv.Atoi(string(raw[:3]))
		out := map[string]any{
			"Packet Name": "Reply", "Role": "reply", "Reply Code": code, "Multiline": bytes.Contains(raw, []byte("-")),
		}
		if s.pendingCount() > 0 {
			out["In Reply To"] = s.pending[s.pendingHead]
			if s.pending[s.pendingHead] == "DATA" && code == 354 {
				s.inData = true
			}
			if s.pending[s.pendingHead] == "STARTTLS" && code == 220 {
				s.encrypted = true
				out["Encrypted"] = true
				out["Protocol Transition"] = "smtp->tls"
			}
			if code != 354 {
				s.pending[s.pendingHead] = ""
				s.pendingHead++
				if s.pendingHead == len(s.pending) {
					s.pending = s.pending[:0]
					s.pendingHead = 0
				}
			}
		}
		if text := smtpReplyText(raw); text != "" {
			out["Reply Text"] = text
		}
		return out, nil
	}
	line := string(bytes.TrimRight(raw, "\r\n"))
	cmd := mailFirstWord(raw)
	if !smtpKnownCommand(cmd) {
		return nil, fmt.Errorf("smtp: unknown command %q", cmd)
	}
	if cmd == "BDAT" || cmd == "AUTH" {
		return nil, protocolError(ErrUnsupportedFeature, "SMTP %s exchange requires an unsupported profile", cmd)
	}
	if s.pendingCount() >= sessionCollectionLimit(s.maxPending) {
		return nil, protocolError(ErrResourceExceeded, "SMTP pending command budget exceeded")
	}
	// Reuse the FIFO storage after replies, clearing references as they retire.
	if len(s.pending) == cap(s.pending) && s.pendingHead > 0 {
		n := copy(s.pending, s.pending[s.pendingHead:])
		clear(s.pending[n:])
		s.pending = s.pending[:n]
		s.pendingHead = 0
	}
	s.pending = append(s.pending, cmd)
	out := map[string]any{"Packet Name": cmd, "Role": "command", "Line": line}
	if cmd == "STARTTLS" {
		s.startTLS = true
	}
	if cmd == "MAIL" || cmd == "RCPT" {
		out["Path"] = strings.TrimSpace(line[len(cmd):])
	}
	_ = dir
	return out, nil
}

func smtpKnownCommand(cmd string) bool {
	switch cmd {
	case "EHLO", "HELO", "LHLO", "MAIL", "RCPT", "DATA", "RSET", "NOOP", "QUIT", "VRFY", "EXPN", "HELP", "AUTH", "STARTTLS", "BDAT":
		return true
	}
	return false
}

func smtpReplyText(raw []byte) string {
	i := bytes.Index(raw, []byte("\r\n"))
	if i < 4 {
		return ""
	}
	return string(bytes.TrimSpace(raw[4:i]))
}

func (f *binFlow) frameIMAP(dir int, w []byte) (int, *binSpec, error) {
	s := f.imap
	if s == nil {
		return 0, nil, sessionContext("IMAP session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(s.pending)+1)*96 + int64(s.pendingBytes)); err != nil {
		return 0, nil, err
	}
	if s.encrypted {
		if len(w) == 0 {
			return 0, nil, nil
		}
		return 0, nil, protocolError(ErrEncrypted, "IMAP transport is encrypted after STARTTLS")
	}
	if n := s.literal[dir]; n > 0 {
		if len(w) < n {
			return n, nil, nil
		}
		return n, f.a.specs["imap/IMAP"], nil
	}
	n, err := frameCRLFLine(w, imapLineMax)
	if err != nil {
		return 0, nil, err
	}
	if n == 0 || n > len(w) {
		return n, nil, nil
	}
	if err := f.reserveSession(256 + int64(len(s.pending)+1)*96 + int64(s.pendingBytes+n)); err != nil {
		return 0, nil, err
	}
	// Both literal forms have a line header followed by exactly n opaque bytes.
	// Reject oversized declarations before buffering their payload.
	if size, _, ok := imapLiteralSuffix(w[:n-2]); ok && (size > f.a.config.MaxMessageBytes || size > f.a.budget.MaxFrameBytes) {
		return 0, nil, protocolError(ErrResourceExceeded, "IMAP literal exceeds message/frame budget")
	}
	return n, f.a.specs["imap/IMAP"], nil
}

func imapLiteralSuffix(line []byte) (int, bool, bool) {
	i := bytes.LastIndexByte(line, '{')
	if i < 0 || line[len(line)-1] != '}' {
		return 0, false, false
	}
	inner := line[i+1 : len(line)-1]
	plus := false
	if len(inner) > 0 && inner[len(inner)-1] == '+' {
		plus = true
		inner = inner[:len(inner)-1]
	}
	if len(inner) == 0 {
		return 0, false, false
	}
	for _, digit := range inner {
		if digit < '0' || digit > '9' {
			return 0, false, false
		}
	}
	n, err := strconv.Atoi(string(inner))
	if err != nil || n < 0 {
		return 0, false, false
	}
	return n, plus, true
}

func (s *binIMAP) consume(dir int, raw []byte) (map[string]any, error) {
	if s.encrypted {
		return nil, protocolError(ErrEncrypted, "IMAP transport is encrypted after STARTTLS")
	}
	if n := s.literal[dir]; n > 0 {
		s.literal[dir] = 0
		s.literalTail[dir] = true
		return map[string]any{"Packet Name": "Literal", "Role": "literal", "Bytes": n}, nil
	}
	line := string(bytes.TrimRight(raw, "\r\n"))
	if s.literalTail[dir] {
		s.literalTail[dir] = false
		if line == ")" || line == "" {
			return map[string]any{"Packet Name": "Literal End", "Role": "literal-end", "Line": line}, nil
		}
	}
	fields := strings.Fields(line)
	if len(fields) > 0 && fields[0] == "+" {
		return map[string]any{"Tag": "+", "Role": "continuation", "Packet Name": "Continuation", "Line": line}, nil
	}
	if len(fields) < 2 {
		return nil, fmt.Errorf("imap: missing tag or command")
	}
	tag, cmd := fields[0], strings.ToUpper(fields[1])
	out := map[string]any{"Tag": tag, "Command": cmd, "Line": line}
	if size, plus, ok := imapLiteralSuffix(bytes.TrimSuffix(raw, []byte("\r\n"))); ok {
		s.literal[dir] = size
		s.literalTail[dir] = size == 0
		out["Literal Size"] = size
		out["Non-Synchronizing Literal"] = plus
	}
	if tag == "*" {
		out["Role"] = "untagged"
		out["Packet Name"] = "Untagged " + cmd
		if len(fields) >= 3 && imapKnownCommand(strings.ToUpper(fields[2])) {
			out["Command"] = strings.ToUpper(fields[2])
			out["Packet Name"] = "Untagged " + strings.ToUpper(fields[2])
		}
		return out, nil
	}
	if tag == "+" {
		out["Role"] = "continuation"
		out["Packet Name"] = "Continuation"
		return out, nil
	}
	switch cmd {
	case "OK", "NO", "BAD":
		out["Role"] = "tagged-reply"
		out["Packet Name"] = "Tagged " + cmd
		if req, ok := s.pending[tag]; ok {
			out["In Reply To"] = req
			delete(s.pending, tag)
			s.pendingBytes -= len(tag) + len(req)
			if req == "STARTTLS" && cmd == "OK" {
				s.encrypted = true
				out["Encrypted"] = true
				out["Protocol Transition"] = "imap->tls"
			}
		} else {
			out["Association"] = "missing-request"
		}
	default:
		if !imapKnownCommand(cmd) {
			return nil, fmt.Errorf("imap: unknown command %q", cmd)
		}
		out["Role"] = "command"
		out["Packet Name"] = cmd
		if s.pending == nil {
			s.pending = map[string]string{}
		}
		if _, exists := s.pending[tag]; exists {
			return nil, protocolError(ErrDesynchronized, "IMAP command reused an outstanding tag")
		}
		if len(s.pending) >= sessionCollectionLimit(s.maxPending) {
			return nil, protocolError(ErrResourceExceeded, "IMAP pending tag budget exceeded")
		}
		// Avoid retaining an entire command line (possibly credentials or a literal
		// declaration) through a short tag substring.
		s.pending[strings.Clone(tag)] = strings.Clone(cmd)
		s.pendingBytes += len(tag) + len(cmd)
		if cmd == "STARTTLS" {
			s.startTLS = tag
		}
	}
	return out, nil
}

func (f *binFlow) framePOP3(w []byte) (int, *binSpec, error) {
	s := f.pop3
	if s == nil {
		return 0, nil, sessionContext("POP3 session was not observed")
	}
	if err := f.reserveSession(256); err != nil {
		return 0, nil, err
	}
	if s.encrypted {
		if len(w) == 0 {
			return 0, nil, nil
		}
		return 0, nil, protocolError(ErrEncrypted, "POP3 transport is encrypted after STLS")
	}
	if s.multiline {
		n, err := frameDotBlock(w, f.a.config.MaxMessageBytes)
		if err != nil {
			return 0, nil, err
		}
		if n == 0 || n > len(w) {
			return n, nil, nil
		}
		return n, f.a.specs["pop3/POP3"], nil
	}
	n, err := frameCRLFLine(w, mailLineMax)
	if err != nil {
		return 0, nil, err
	}
	if n == 0 || n > len(w) {
		return n, nil, nil
	}
	return n, f.a.specs["pop3/POP3"], nil
}

func (s *binPOP3) consume(raw []byte) (map[string]any, error) {
	if s.encrypted {
		return nil, protocolError(ErrEncrypted, "POP3 transport is encrypted after STLS")
	}
	if s.multiline {
		s.multiline = false
		out := map[string]any{"Packet Name": "Multiline", "Role": "reply"}
		if s.pending != "" {
			out["In Reply To"] = s.pending
			s.pending = ""
		}
		return out, nil
	}
	line := string(bytes.TrimRight(raw, "\r\n"))
	word := mailFirstWord(raw)
	switch {
	case strings.HasPrefix(line, "+OK") || strings.HasPrefix(line, "-ERR"):
		ok := strings.HasPrefix(line, "+OK")
		out := map[string]any{"Packet Name": map[bool]string{true: "+OK", false: "-ERR"}[ok], "Role": "reply", "Line": line, "Status": map[bool]string{true: "+OK", false: "-ERR"}[ok]}
		if s.pending != "" {
			out["In Reply To"] = s.pending
			if ok && pop3Multiline(s.pending, s.pendingArg) {
				s.multiline = true
			} else if ok && s.pending == "STLS" {
				s.encrypted = true
				out["Encrypted"] = true
				out["Protocol Transition"] = "pop3->tls"
				s.pending = ""
			} else {
				s.pending = ""
			}
		}
		return out, nil
	default:
		cmd := word
		if !pop3Known(cmd) {
			return nil, fmt.Errorf("pop3: unknown command %q", cmd)
		}
		if cmd == "AUTH" && len(strings.Fields(line)) > 1 {
			return nil, protocolError(ErrUnsupportedFeature, "POP3 SASL exchange requires an unsupported profile")
		}
		s.pending = cmd
		s.pendingArg = len(strings.Fields(line)) > 1
		if cmd == "STLS" {
			s.stls = true
		}
		return map[string]any{"Packet Name": cmd, "Role": "command", "Line": line}, nil
	}
}

func pop3Known(cmd string) bool {
	switch cmd {
	case "USER", "PASS", "STAT", "LIST", "RETR", "DELE", "NOOP", "RSET", "QUIT", "UIDL", "APOP", "CAPA", "AUTH", "STLS", "TOP":
		return true
	}
	return false
}

func pop3Multiline(cmd string, hasArg bool) bool {
	switch cmd {
	case "CAPA", "RETR", "TOP":
		return true
	case "LIST", "UIDL":
		return !hasArg
	}
	return false
}
