package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"strings"
	"unicode"
	"unicode/utf8"
)

// binSSH is the M0 session state for RFC 4253 first handshake. After both
// NEWKEYS messages the transport is encrypted; ciphertext is not interpreted
// as login, channel, or shell. Port 22 is never consulted.
type binSSH struct {
	client             int
	clientKnown        bool
	preLines, preBytes int
	banner             [2]bool
	ident              [2]string
	kex                [2]sshKexLists
	haveKEX            [2]bool
	newkeys            [2]bool
	selected           map[string]string
	encrypted          bool
	guessPending       [2]bool
	compatIdentProof   [2]bool
	identWarning       [2]string
}

type sshKexLists struct {
	Kex, HostKey, EncC2S, EncS2C, MACC2S, MACS2C, CompC2S, CompS2C string
}

func probeSSH(w []byte, limit int) ProbeResult {
	if len(w) == 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	prefix := []byte("SSH-")
	if bytes.HasPrefix(prefix, w) && len(w) < 4 {
		return probeNeed("ssh", "2.0", len(w), 4)
	}
	if !bytes.HasPrefix(w, prefix) {
		return ProbeResult{Verdict: ProbeReject}
	}
	if i := bytes.IndexByte(w, '\n'); i >= 0 {
		if _, _, err := sshIdentification(w[:i+1]); err != nil {
			if _, _, warning, observedErr := sshObservedIdentification(w[:i+1]); observedErr != nil || warning == "" {
				return ProbeResult{Verdict: ProbeReject, Protocol: "ssh", Reason: "unsupported identification"}
			}
			// A nonconformant software spelling is not sufficient admission.
			// Require a full initial KEXINIT, including all mandatory fields.
			body := w[i+1:]
			want := i + 1 + 16
			if len(body) >= 4 {
				n := uint64(binary.BigEndian.Uint32(body)) + 4
				if n < 16 || n > 35000 || n%8 != 0 || len(body) >= 6 && body[5] != 20 {
					return ProbeResult{Verdict: ProbeReject, Protocol: "ssh", Reason: "nonconformant identification lacks a valid initial KEXINIT"}
				}
				want = i + 1 + int(n)
				if len(body) >= int(n) && want <= limit {
					if _, err := stream_parser.DecodeSSHInitialPacket(body[:int(n)], "transport"); err == nil {
						return probeAccept("ssh", "observed-nonconformant-identification/2.0", 95)
					}
					return ProbeResult{Verdict: ProbeReject, Protocol: "ssh", Reason: "invalid initial KEXINIT after nonconformant identification"}
				}
			}
			return probeNeed("ssh", "unconfirmed-nonconformant-identification", len(w), min(limit, want))
		}
		_ = limit
		return probeAccept("ssh", "2.0", 95)
	}
	if len(w) >= 255 {
		return ProbeResult{Verdict: ProbeReject, Protocol: "ssh", Reason: "identification exceeds 255 bytes"}
	}
	return probeNeed("ssh", "2.0", len(w), min(limit, 255))
}

const sshMaxPreLines = 8
const sshMaxPreBytes = 2048
const sshMaxPreLineBytes = 1024

// RFC 4253 4.2 permits pre-identification lines only from the server. These
// limits define a bounded observed profile, not a limit imposed by the RFC.
func sshPreLine(raw []byte) (string, error) {
	if len(raw) == 0 || raw[len(raw)-1] != '\n' || len(raw) > sshMaxPreLineBytes || bytes.HasPrefix(raw, []byte("SSH-")) {
		return "", protocolError(ErrMalformedMessage, "SSH pre-identification line is invalid")
	}
	line := raw[:len(raw)-1]
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	if !utf8.Valid(line) {
		return "", protocolError(ErrMalformedMessage, "SSH pre-identification is not UTF-8")
	}
	for _, r := range string(line) {
		if unicode.IsControl(r) && r != '\t' {
			return "", protocolError(ErrMalformedMessage, "SSH pre-identification contains control bytes")
		}
	}
	return string(line), nil
}

func sshIdentification(raw []byte) (string, string, error) {
	if len(raw) == 0 || len(raw) > 255 || raw[len(raw)-1] != '\n' {
		return "", "", protocolError(ErrMalformedMessage, "SSH identification length or termination is invalid")
	}
	line := raw[:len(raw)-1]
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	version := "2.0"
	var software []byte
	if bytes.HasPrefix(line, []byte("SSH-2.0-")) {
		software = line[8:]
	} else if bytes.HasPrefix(line, []byte("SSH-1.99-")) {
		version = "1.99"
		software = line[9:]
	} else {
		return "", "", protocolError(ErrUnsupportedVersion, "SSH identification version is unsupported")
	}
	if i := bytes.IndexByte(software, ' '); i >= 0 {
		software = software[:i]
	}
	if len(software) == 0 {
		return "", "", protocolError(ErrMalformedMessage, "SSH software version is empty")
	}
	for _, b := range software {
		if b <= 32 || b >= 127 || b == '-' {
			return "", "", protocolError(ErrMalformedMessage, "SSH software version is invalid")
		}
	}
	for _, b := range line {
		if b < 32 || b == 127 {
			return "", "", protocolError(ErrMalformedMessage, "SSH identification contains control bytes")
		}
	}
	if !utf8.Valid(line) {
		return "", "", protocolError(ErrMalformedMessage, "SSH identification is not UTF-8")
	}
	return string(line), version, nil
}

// Retain the literal wire string. The owned validation copy only establishes
// that '-' in softwareversion is the sole strict-grammar discrepancy; it never
// rewrites capture bytes or turns that discrepancy into standards compliance.
func sshObservedIdentification(raw []byte) (string, string, string, error) {
	line, version, originalErr := sshIdentification(raw)
	if originalErr == nil {
		return line, version, "", nil
	}
	start := 0
	if bytes.HasPrefix(raw, []byte("SSH-2.0-")) {
		start = 8
	} else if bytes.HasPrefix(raw, []byte("SSH-1.99-")) {
		start = 9
	} else {
		return "", "", "", originalErr
	}
	trimmed := bytes.TrimSuffix(bytes.TrimSuffix(raw, []byte{'\n'}), []byte{'\r'})
	end := len(trimmed)
	if i := bytes.IndexByte(trimmed[start:], ' '); i >= 0 {
		end = start + i
	}
	check := bytes.Clone(raw)
	changed := false
	for i := start; i < end; i++ {
		if check[i] == '-' {
			check[i], changed = '_', true
		}
	}
	if _, version, err := sshIdentification(check); changed && err == nil {
		return string(trimmed), version, "softwareversion uses '-' (RFC4253 4.2)", nil
	}
	return "", "", "", originalErr
}

func (s *binSSH) identificationWarningFields(info map[string]any) {
	var warnings []string
	for dir, warning := range s.identWarning {
		if warning != "" {
			warnings = append(warnings, fmt.Sprintf("endpoint %d: %s", dir, warning))
		}
	}
	if len(warnings) > 0 {
		info["Identification Conformance"] = "nonconformant"
		info["Identification Warning"] = strings.Join(warnings, "; ")
	}
}

// Call only with observed server direction. A line of arbitrary text, a port
// hint, or a client preamble alone never admits SSH.
func probeSSHServerPreamble(w []byte, limit int) ProbeResult {
	bound := min(limit, sshMaxPreBytes+255)
	if bound <= 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	offset, lines := 0, 0
	for offset < len(w) {
		p := w[offset:]
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			if len(w) >= bound || len(p) >= sshMaxPreLineBytes {
				return ProbeResult{Verdict: ProbeReject}
			}
			return probeNeed("ssh", "2.0", len(w), min(bound, len(w)+1))
		}
		n := i + 1
		if bytes.HasPrefix(p, []byte("SSH-")) {
			if offset+n > bound {
				return ProbeResult{Verdict: ProbeReject}
			}
			if _, _, err := sshIdentification(p[:n]); err == nil {
				return probeAccept("ssh", "server-preamble/2.0", 95)
			}
			return ProbeResult{Verdict: ProbeReject}
		}
		if _, err := sshPreLine(p[:n]); err != nil || lines >= sshMaxPreLines || offset+n > sshMaxPreBytes {
			return ProbeResult{Verdict: ProbeReject}
		}
		offset += n
		lines++
	}
	if offset >= bound {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeNeed("ssh", "2.0", len(w), min(bound, len(w)+1))
}

func (f *binFlow) frameSSH(dir int, w []byte) (int, *binSpec, error) {
	s := f.ssh
	if s == nil {
		return 0, nil, sessionContext("SSH session was not observed")
	}
	if err := f.reserveSession(512 + int64(len(s.ident[0])+len(s.ident[1]))); err != nil {
		return 0, nil, err
	}
	if s.newkeys[dir] || s.encrypted {
		if len(w) == 0 {
			return 0, nil, nil
		}
		return 0, nil, protocolError(ErrEncrypted, "SSH transport is encrypted after NEWKEYS")
	}
	if !s.banner[dir] {
		if i := bytes.IndexByte(w, '\n'); i >= 0 {
			n := i + 1
			if bytes.HasPrefix(w, []byte("SSH-")) {
				_, _, warning, err := sshObservedIdentification(w[:n])
				if err != nil {
					return 0, nil, err
				}
				if warning != "" && !s.compatIdentProof[dir] && !s.compatIdentProof[1-dir] && !(s.banner[1-dir] && s.identWarning[1-dir] == "") && probeSSH(w, min(f.a.budget.MaxFrameBytes, f.a.config.MaxMessageBytes)).Verdict != ProbeAccept {
					return 0, nil, protocolError(ErrContextRequired, "SSH nonconformant identification needs a confirmed peer or complete initial KEXINIT")
				}
				if err := f.reserveSession(512 + int64(len(s.ident[0])+len(s.ident[1])+n)); err != nil {
					return 0, nil, err
				}
				if warning != "" {
					s.compatIdentProof[dir] = true
				}
				return n, &binSpec{entry: "SSHIdentification"}, nil
			}
			if !s.clientKnown {
				return 0, nil, protocolError(ErrContextRequired, "SSH pre-identification requires observed server direction")
			}
			if dir == s.client {
				return 0, nil, protocolError(ErrMalformedMessage, "SSH client must start with its identification")
			}
			if _, err := sshPreLine(w[:n]); err != nil {
				return 0, nil, err
			}
			if s.preLines >= sshMaxPreLines || s.preBytes+n > sshMaxPreBytes {
				return 0, nil, protocolError(ErrResourceExceeded, "SSH pre-identification line/byte limit exceeded")
			}
			return n, &binSpec{entry: "SSHPreIdentification"}, nil
		}
		ident := bytes.HasPrefix([]byte("SSH-"), w) || bytes.HasPrefix(w, []byte("SSH-"))
		if ident && len(w) >= 255 {
			return 0, nil, protocolError(ErrMalformedMessage, "SSH identification exceeds 255 bytes")
		}
		if !ident {
			if !s.clientKnown {
				return 0, nil, protocolError(ErrContextRequired, "SSH pre-identification requires observed server direction")
			}
			if dir == s.client {
				return 0, nil, protocolError(ErrMalformedMessage, "SSH client must start with its identification")
			}
			if len(w) >= sshMaxPreLineBytes || s.preBytes+len(w) >= sshMaxPreBytes {
				return 0, nil, protocolError(ErrResourceExceeded, "SSH pre-identification byte limit exceeded")
			}
		}
		return 0, nil, nil
	}
	if len(w) < 4 {
		return 0, nil, nil
	}
	plen := int(binary.BigEndian.Uint32(w[:4]))
	if plen < 5 || plen > 35000 {
		return 0, nil, fmt.Errorf("ssh: invalid packet length")
	}
	n := 4 + plen
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	pad := int(w[4])
	if pad < 4 || pad > plen-1 {
		return 0, nil, fmt.Errorf("ssh: invalid padding length")
	}
	return n, f.spec("ssh", "SSHPacket"), nil
}

func (s *binSSH) consume(dir int, raw []byte) (map[string]any, error) {
	info, err := s.consumeMessage(dir, raw)
	if err == nil && info != nil {
		s.identificationWarningFields(info)
	}
	return info, err
}

func (s *binSSH) consumeMessage(dir int, raw []byte) (map[string]any, error) {
	if s.client < 0 {
		s.client = dir
	}
	if !s.banner[dir] {
		if !bytes.HasPrefix(raw, []byte("SSH-")) {
			if !s.clientKnown || dir == s.client {
				return nil, protocolError(ErrMalformedMessage, "SSH pre-identification requires observed server direction")
			}
			line, err := sshPreLine(raw)
			if err != nil {
				return nil, err
			}
			if s.preLines >= sshMaxPreLines || s.preBytes+len(raw) > sshMaxPreBytes {
				return nil, protocolError(ErrResourceExceeded, "SSH pre-identification limit exceeded")
			}
			s.preLines++
			s.preBytes += len(raw)
			return map[string]any{"Packet Name": "pre-identification line", "Text": line, "Line Number": s.preLines, "Context Level": "observed-server"}, nil
		}
		line, ver, warning, err := sshObservedIdentification(raw)
		if err != nil {
			return nil, err
		}
		if warning != "" && !s.compatIdentProof[dir] && !s.compatIdentProof[1-dir] && !(s.banner[1-dir] && s.identWarning[1-dir] == "") {
			return nil, protocolError(ErrContextRequired, "SSH nonconformant identification lacks independent admission evidence")
		}
		s.banner[dir], s.ident[dir] = true, line
		s.identWarning[dir] = warning
		return map[string]any{
			"Packet Name":    "identification",
			"Identification": line,
			"Version":        ver,
			"Context Level":  "observed",
		}, nil
	}
	if len(raw) < 6 {
		return nil, fmt.Errorf("ssh: truncated packet")
	}
	plen := int(binary.BigEndian.Uint32(raw[:4]))
	if 4+plen != len(raw) {
		return nil, fmt.Errorf("ssh: packet length disagrees with framed message")
	}
	pad := int(raw[4])
	end := len(raw) - pad
	payload := raw[5:end]
	if len(payload) == 0 {
		return nil, fmt.Errorf("ssh: empty payload")
	}
	msg := payload[0]
	profile := "transport"
	kex := s.selected["Kex Algorithm"]
	switch {
	case strings.HasPrefix(kex, "diffie-hellman-group-exchange-"):
		profile = "dh-gex"
	case strings.HasPrefix(kex, "diffie-hellman-group"):
		profile = "dh"
	case kex == "ecdh-sha2-nistp256":
		profile = "ecdh-nistp256"
	case kex == "curve25519-sha256" || kex == "curve25519-sha256@libssh.org":
		profile = "ecdh-curve25519"
	}
	ignoredGuess := false
	if msg != 20 && s.guessPending[dir] {
		if s.haveKEX[0] && s.haveKEX[1] && s.clientKnown {
			own := s.kex[dir]
			ignoredGuess = strings.Split(own.Kex, ",")[0] != kex || strings.Split(own.HostKey, ",")[0] != s.selected["Host Key Algorithm"]
			if ignoredGuess {
				profile = "transport"
			}
		}
	}
	initial, err := stream_parser.DecodeSSHInitialPacket(raw, profile)
	if err != nil {
		return nil, protocolError(ErrMalformedMessage, err.Error())
	}
	info := map[string]any{
		"Packet Name":    sshExchangeMessageName(msg, kex),
		"Message Number": msg,
		"Context Level":  "observed",
		"Identification": s.ident[dir],
	}
	if msg != 20 {
		for key, value := range s.selected {
			info[key] = value
		}
	}
	if !s.clientKnown && (msg == 20 || s.haveKEX[0] && s.haveKEX[1]) {
		info["Negotiation Context"] = "client/server role not observed"
		info["Context Level"] = "partial"
	}
	if msg != 20 && s.guessPending[dir] {
		s.guessPending[dir] = false
		if ignoredGuess {
			info["Packet Name"] = "Ignored guessed key-exchange packet"
			info["Payload Interpretation"] = "discarded after algorithm mismatch"
			return info, nil
		}
	}
	switch msg {
	case 20:
		if s.haveKEX[dir] {
			return nil, protocolError(ErrMalformedMessage, "SSH repeated KEXINIT before NEWKEYS")
		}
		lists, err := sshParseKexInit(payload[1:])
		if err != nil {
			return nil, err
		}
		s.kex[dir] = lists
		s.haveKEX[dir] = true
		info["Kex Algorithms"] = lists.Kex
		info["Host Key Algorithms"] = lists.HostKey
		// KEXINIT contains sender offers. Client-preference negotiation below
		// is a different observation and must not overwrite those wire lists.
		info["Offered Encryption C2S"] = lists.EncC2S
		info["Offered Encryption S2C"] = lists.EncS2C
		info["Offered MAC C2S"] = lists.MACC2S
		info["Offered MAC S2C"] = lists.MACS2C
		info["Offered Compression C2S"] = lists.CompC2S
		info["Offered Compression S2C"] = lists.CompS2C
		if wf, ok := initial["Wire Fields"].(map[string]any); ok {
			if p, ok := wf["Payload"].(map[string]any); ok {
				for _, key := range []string{"Languages C2S", "Languages S2C", "First Kex Packet Follows", "Reserved"} {
					info[key] = p[key]
				}
				follows, _ := p["First Kex Packet Follows"].(byte)
				s.guessPending[dir] = follows != 0
			}
		}
		if s.haveKEX[0] && s.haveKEX[1] && s.clientKnown {
			s.selected = sshNegotiate(s.kex[s.client], s.kex[1-s.client])
			for k, v := range s.selected {
				info[k] = v
			}
			info["Kex Family"] = sshKexFamily(s.selected["Kex Algorithm"])
		}
	case 21:
		s.newkeys[dir] = true
		if s.newkeys[0] && s.newkeys[1] {
			s.encrypted = true
			info["Encrypted"] = true
			info["Protocol Transition"] = "ssh->encrypted"
		}
	case 30:
		info["Kex Family"] = sshKexFamily(s.selected["Kex Algorithm"])
		if !s.haveKEX[0] || !s.haveKEX[1] {
			info["Context Level"] = "partial"
		}
	case 31, 33:
		if kex == "" || msg == 31 && profile == "dh-gex" || msg == 33 && profile != "dh-gex" {
			info["Kex Family"] = sshKexFamily(kex)
			break
		}
		host, err := sshParseHostKey(payload[1:])
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(host)
		info["Host Key Format"] = sshHostKeyFormat(host)
		info["Host Key Fingerprint"] = "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
		info["Host Key SHA256"] = fmt.Sprintf("%x", sum[:])
		info["Signature Verified"], info["Peer Identity Verified"] = false, false
		info["Kex Family"] = sshKexFamily(s.selected["Kex Algorithm"])
	}
	if msg >= 30 && msg <= 34 && profile != "transport" {
		info["Initial Packet Fields"] = initial["Wire Fields"]
		info["Signature Verified"], info["Peer Identity Verified"] = false, false
	}
	return info, nil
}

func sshParseKexInit(body []byte) (sshKexLists, error) {
	var lists sshKexLists
	if len(body) < 16 {
		return lists, fmt.Errorf("ssh: truncated KEXINIT cookie")
	}
	i := 16
	fields := []*string{&lists.Kex, &lists.HostKey, &lists.EncC2S, &lists.EncS2C, &lists.MACC2S, &lists.MACS2C, &lists.CompC2S, &lists.CompS2C}
	for _, dst := range fields {
		s, n, err := sshReadString(body, i)
		if err != nil {
			return lists, err
		}
		*dst = s
		i = n
	}
	// The two language lists, follows boolean and reserved word are mandatory,
	// even when their values do not affect algorithm choice.
	for n := 0; n < 2; n++ {
		_, next, err := sshReadString(body, i)
		if err != nil {
			return lists, err
		}
		i = next
	}
	if len(body)-i != 5 {
		return lists, fmt.Errorf("ssh: KEXINIT missing or trailing fields")
	}
	return lists, nil
}

func sshExchangeMessageName(msg byte, kex string) string {
	if msg < 30 || msg > 49 {
		return sshMsgName(msg)
	}
	if strings.HasPrefix(kex, "diffie-hellman-group-exchange-") {
		if name := map[byte]string{30: "KEX_DH_GEX_REQUEST_OLD", 31: "KEX_DH_GEX_GROUP", 32: "KEX_DH_GEX_INIT", 33: "KEX_DH_GEX_REPLY", 34: "KEX_DH_GEX_REQUEST"}[msg]; name != "" {
			return name
		}
	} else if strings.Contains(kex, "curve25519") || strings.HasPrefix(kex, "ecdh-") {
		if msg == 30 {
			return "KEX_ECDH_INIT"
		}
		if msg == 31 {
			return "KEX_ECDH_REPLY"
		}
	} else if strings.HasPrefix(kex, "diffie-hellman-group") {
		if msg == 30 {
			return "KEXDH_INIT"
		}
		if msg == 31 {
			return "KEXDH_REPLY"
		}
	}
	return fmt.Sprintf("KEY_EXCHANGE_%d (layout unconfirmed)", msg)
}

func sshParseHostKey(body []byte) ([]byte, error) {
	s, _, err := sshReadBytes(body, 0)
	if err != nil {
		return nil, err
	}
	if len(s) == 0 {
		return nil, fmt.Errorf("ssh: empty host key")
	}
	return s, nil
}

func sshHostKeyFormat(blob []byte) string {
	s, _, err := sshReadString(blob, 0)
	if err != nil {
		return ""
	}
	return s
}

func sshReadBytes(b []byte, i int) ([]byte, int, error) {
	if i+4 > len(b) {
		return nil, i, fmt.Errorf("ssh: truncated string length")
	}
	n := int(binary.BigEndian.Uint32(b[i : i+4]))
	i += 4
	if n < 0 || i+n > len(b) {
		return nil, i, fmt.Errorf("ssh: truncated string")
	}
	return b[i : i+n], i + n, nil
}

func sshReadString(b []byte, i int) (string, int, error) {
	raw, next, err := sshReadBytes(b, i)
	if err != nil {
		return "", next, err
	}
	return string(raw), next, nil
}

func sshNegotiate(client, server sshKexLists) map[string]string {
	pick := func(c, s string) string {
		have := map[string]bool{}
		for _, n := range strings.Split(s, ",") {
			if n != "" {
				have[n] = true
			}
		}
		for _, n := range strings.Split(c, ",") {
			if n != "" && have[n] {
				return n
			}
		}
		return ""
	}
	return map[string]string{
		"Kex Algorithm":      pick(sshRealKexNames(client.Kex), sshRealKexNames(server.Kex)),
		"Host Key Algorithm": pick(client.HostKey, server.HostKey),
		"Encryption C2S":     pick(client.EncC2S, server.EncC2S),
		"Encryption S2C":     pick(client.EncS2C, server.EncS2C),
		"MAC C2S":            pick(client.MACC2S, server.MACC2S),
		"MAC S2C":            pick(client.MACS2C, server.MACS2C),
		"Compression C2S":    pick(client.CompC2S, server.CompC2S),
		"Compression S2C":    pick(client.CompS2C, server.CompS2C),
	}
}

func sshRealKexNames(names string) string {
	var out []string
	for _, n := range strings.Split(names, ",") {
		if !strings.HasPrefix(n, "ext-info-") && !strings.HasPrefix(n, "kex-strict-") {
			out = append(out, n)
		}
	}
	return strings.Join(out, ",")
}

func sshKexFamily(name string) string {
	switch {
	case strings.Contains(name, "curve25519") || strings.HasPrefix(name, "ecdh-"):
		return "ECDH"
	case strings.Contains(name, "diffie-hellman"):
		return "DH"
	default:
		return ""
	}
}

func sshMsgName(msg byte) string {
	switch msg {
	case 20:
		return "KEXINIT"
	case 21:
		return "NEWKEYS"
	case 30:
		return "KEXDH_INIT"
	case 31:
		return "KEXDH_REPLY"
	case 34:
		return "KEX_DH_GEX_REQUEST"
	}
	return fmt.Sprintf("MSG_%d", msg)
}
