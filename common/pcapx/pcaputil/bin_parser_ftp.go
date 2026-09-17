package pcaputil

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type binFTP struct {
	pending   string
	authTLS   bool
	encrypted bool
}

func probeFTP(w []byte, limit int) ProbeResult {
	if len(w) == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if ftpCommandPrefix(w) {
		if !bytes.Contains(w, []byte("\r\n")) {
			if len(w) >= min(limit, mailLineMax) {
				return ProbeResult{Verdict: ProbeReject}
			}
			return probeNeed("ftp", "rfc959", len(w), min(limit, 16))
		}
		return probeAccept("ftp", "rfc959", 91)
	}
	if w[0] == '2' {
		line, ok := firstCRLF(w)
		if !ok {
			if smtpReplyPrefix(w) && len(w) < min(limit, 64) && bytes.Contains(bytes.ToUpper(w), []byte("FTP")) {
				return probeNeed("ftp", "rfc959", len(w), min(limit, 24))
			}
			return ProbeResult{Verdict: ProbeReject}
		}
		if smtpReplyPrefix(line) && mailHasToken(line, "FTP") && !mailHasToken(line, "ESMTP") && !mailHasToken(line, "SMTP") {
			return probeAccept("ftp", "rfc959", 90)
		}
	}
	return ProbeResult{Verdict: ProbeReject}
}

func ftpCommandPrefix(w []byte) bool {
	u := bytes.ToUpper(w)
	for _, cmd := range []string{
		"PASV\r", "PASV ", "PORT ", "EPSV", "EPRT ", "RETR ", "STOR ", "STOU",
		"CWD ", "PWD\r", "XPWD", "SYST\r", "FEAT\r", "FEAT ", "AUTH TLS", "AUTH SSL",
		"PBSZ ", "PROT ", "TYPE ", "MKD ", "RMD ", "DELE ", "RNFR ", "RNTO ",
	} {
		if bytes.HasPrefix(u, []byte(cmd)) {
			return true
		}
	}
	return false
}

func (f *binFlow) frameFTP(w []byte) (int, *binSpec, error) {
	s := f.ftp
	if s == nil {
		return 0, nil, sessionContext("FTP session was not observed")
	}
	if err := f.reserveSession(256); err != nil {
		return 0, nil, err
	}
	if s.encrypted {
		if len(w) == 0 {
			return 0, nil, nil
		}
		return 0, nil, protocolError(ErrEncrypted, "FTP transport is encrypted after AUTH TLS")
	}
	if len(w) >= 3 && w[0] >= '1' && w[0] <= '5' && unicode.IsDigit(rune(w[1])) && unicode.IsDigit(rune(w[2])) {
		n, err := frameCodeReply(w, mailLineMax, mailReplyMax)
		if err != nil {
			return 0, nil, err
		}
		if n == 0 || n > len(w) {
			return n, nil, nil
		}
		return n, f.spec("ftp", "FTP"), nil
	}
	n, err := frameCRLFLine(w, mailLineMax)
	if err != nil {
		return 0, nil, err
	}
	if n == 0 || n > len(w) {
		return n, nil, nil
	}
	return n, f.spec("ftp", "FTPCommand"), nil
}

func (s *binFTP) consume(raw []byte) (map[string]any, error) {
	if s.encrypted {
		return nil, protocolError(ErrEncrypted, "FTP transport is encrypted after AUTH TLS")
	}
	if len(raw) >= 3 && raw[0] >= '1' && raw[0] <= '5' && unicode.IsDigit(rune(raw[1])) && unicode.IsDigit(rune(raw[2])) {
		code, _ := strconv.Atoi(string(raw[:3]))
		out := map[string]any{"Packet Name": "Reply", "Role": "reply", "Reply Code": code, "Multiline": bytes.Contains(raw, []byte("-"))}
		if text := smtpReplyText(raw); text != "" {
			out["Reply Text"] = text
		}
		if s.pending != "" {
			out["In Reply To"] = s.pending
			if s.pending == "AUTH" && (code == 234 || code == 334) && s.authTLS {
				s.encrypted = true
				out["Encrypted"] = true
				out["Protocol Transition"] = "ftp->tls"
			}
			s.pending = ""
		}
		return out, nil
	}
	line := string(bytes.TrimRight(raw, "\r\n"))
	cmd := mailFirstWord(raw)
	if !ftpKnownCommand(cmd) {
		return nil, fmt.Errorf("ftp: unknown command %q", cmd)
	}
	s.pending = cmd
	if cmd == "AUTH" && strings.Contains(strings.ToUpper(line), "TLS") {
		s.authTLS = true
	}
	out := map[string]any{"Packet Name": cmd, "Role": "command", "Line": line}
	return out, nil
}

func ftpKnownCommand(cmd string) bool {
	switch cmd {
	case "USER", "PASS", "ACCT", "CWD", "CDUP", "QUIT", "REIN", "PORT", "PASV", "TYPE", "STRU", "MODE",
		"RETR", "STOR", "STOU", "APPE", "ALLO", "REST", "RNFR", "RNTO", "ABOR", "DELE", "RMD", "MKD",
		"PWD", "LIST", "NLST", "SITE", "SYST", "STAT", "HELP", "NOOP", "FEAT", "OPTS", "AUTH", "PBSZ",
		"PROT", "EPSV", "EPRT", "SIZE", "MDTM", "XPWD":
		return true
	}
	return false
}
