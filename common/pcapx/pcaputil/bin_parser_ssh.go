package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
)

// binSSH is the M0 session state for RFC 4253 first handshake. After both
// NEWKEYS messages the transport is encrypted; ciphertext is not interpreted
// as login, channel, or shell. Port 22 is never consulted.
type binSSH struct {
	client    int
	banner    [2]bool
	ident     [2]string
	kex       [2]sshKexLists
	haveKEX   [2]bool
	newkeys   [2]bool
	selected  map[string]string
	encrypted bool
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
		line := bytes.TrimRight(w[:i], "\r")
		if !bytes.HasPrefix(line, []byte("SSH-2.0-")) && !bytes.HasPrefix(line, []byte("SSH-1.99-")) {
			return ProbeResult{Verdict: ProbeReject, Protocol: "ssh", Reason: "unsupported identification"}
		}
		_ = limit
		return probeAccept("ssh", "2.0", 95)
	}
	if len(w) >= 255 {
		return ProbeResult{Verdict: ProbeReject, Protocol: "ssh", Reason: "identification exceeds 255 bytes"}
	}
	return probeNeed("ssh", "2.0", len(w), min(limit, 255))
}

func (f *binFlow) frameSSH(dir int, w []byte) (int, *binSpec, error) {
	s := f.ssh
	if s == nil {
		return 0, nil, sessionContext("SSH session was not observed")
	}
	if err := f.reserveSession(512); err != nil {
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
			if n > 255 {
				return 0, nil, fmt.Errorf("ssh: identification exceeds 255 bytes")
			}
			line := bytes.TrimRight(w[:i], "\r")
			if !bytes.HasPrefix(line, []byte("SSH-")) {
				return 0, nil, fmt.Errorf("ssh: identification must start with SSH-")
			}
			return n, f.spec("ssh", "SSH"), nil
		}
		if len(w) >= 255 {
			return 0, nil, fmt.Errorf("ssh: identification exceeds 255 bytes")
		}
		if len(w) > 0 && !bytes.HasPrefix([]byte("SSH-"), w) && !bytes.HasPrefix(w, []byte("SSH-")) {
			return 0, nil, fmt.Errorf("ssh: identification must start with SSH-")
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
	if s.client < 0 {
		s.client = dir
	}
	if !s.banner[dir] {
		i := bytes.IndexByte(raw, '\n')
		if i < 0 {
			return nil, fmt.Errorf("ssh: truncated identification")
		}
		line := string(bytes.TrimRight(raw[:i], "\r"))
		s.banner[dir] = true
		s.ident[dir] = line
		ver := "2.0"
		if strings.HasPrefix(line, "SSH-1.99-") {
			ver = "1.99"
		}
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
	info := map[string]any{
		"Packet Name":    sshMsgName(msg),
		"Message Number": msg,
		"Context Level":  "observed",
		"Identification": s.ident[dir],
	}
	switch msg {
	case 20:
		lists, err := sshParseKexInit(payload[1:])
		if err != nil {
			return nil, err
		}
		s.kex[dir] = lists
		s.haveKEX[dir] = true
		info["Kex Algorithms"] = lists.Kex
		info["Host Key Algorithms"] = lists.HostKey
		info["Encryption C2S"] = lists.EncC2S
		info["Encryption S2C"] = lists.EncS2C
		info["MAC C2S"] = lists.MACC2S
		info["MAC S2C"] = lists.MACS2C
		info["Compression C2S"] = lists.CompC2S
		if s.haveKEX[0] && s.haveKEX[1] {
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
	case 31:
		host, err := sshParseHostKey(payload[1:])
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(host)
		info["Host Key Format"] = sshHostKeyFormat(host)
		info["Host Key Fingerprint"] = "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
		info["Host Key SHA256"] = fmt.Sprintf("%x", sum[:])
		info["Kex Family"] = sshKexFamily(s.selected["Kex Algorithm"])
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
	return lists, nil
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
		"Kex Algorithm":      pick(client.Kex, server.Kex),
		"Host Key Algorithm": pick(client.HostKey, server.HostKey),
		"Encryption C2S":     pick(client.EncC2S, server.EncC2S),
		"Encryption S2C":     pick(client.EncS2C, server.EncS2C),
		"MAC C2S":            pick(client.MACC2S, server.MACC2S),
		"MAC S2C":            pick(client.MACS2C, server.MACS2C),
		"Compression C2S":    pick(client.CompC2S, server.CompC2S),
		"Compression S2C":    pick(client.CompS2C, server.CompS2C),
	}
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
