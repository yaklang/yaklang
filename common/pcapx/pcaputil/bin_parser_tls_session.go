package pcaputil

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

// TLSSecretProvider supplies authorized NSS key-log secrets. Returned bytes are
// borrowed, read-only, and selected by ClientRandom, not by line order. A
// provider must be safe for concurrent use and must not return unrelated keys.
type TLSSecretProvider interface {
	LookupTLSSecret(label string, clientRandom []byte) ([]byte, bool)
}
type TLSKeyLog struct{ secrets map[string][]byte }

func (k *TLSKeyLog) LookupTLSSecret(label string, random []byte) ([]byte, bool) {
	if k == nil {
		return nil, false
	}
	v, ok := k.secrets[label+":"+hex.EncodeToString(random)]
	return bytes.Clone(v), ok
}

// ParseTLSKeyLog parses a bounded, immutable key log. Conflicting duplicate
// entries are errors; secrets are never included in diagnostic messages.
func ParseTLSKeyLog(text string) (*TLSKeyLog, error) {
	if len(text) > 1<<20 {
		return nil, fmt.Errorf("TLS key log exceeds 1 MiB")
	}
	k := &TLSKeyLog{secrets: map[string][]byte{}}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 3 {
			return nil, fmt.Errorf("invalid TLS key log entry")
		}
		random, err := hex.DecodeString(f[1])
		if err != nil || len(random) != 32 {
			return nil, fmt.Errorf("invalid TLS client random")
		}
		secret, err := hex.DecodeString(f[2])
		if err != nil || len(secret) < 16 || len(secret) > 64 {
			return nil, fmt.Errorf("invalid TLS secret length")
		}
		key := f[0] + ":" + hex.EncodeToString(random)
		if old, ok := k.secrets[key]; ok && !bytes.Equal(old, secret) {
			return nil, fmt.Errorf("conflicting TLS key log entries")
		}
		if len(k.secrets) >= 4096 {
			return nil, fmt.Errorf("TLS key log entry budget exceeded")
		}
		k.secrets[key] = secret
	}
	return k, nil
}

// WithTLSSecrets enables authenticated TLS decryption for the documented AES
// 128 GCM profiles. Missing keys preserve encrypted events. Certificate trust
// and application authentication are not established by record authentication.
func WithTLSSecrets(provider TLSSecretProvider) CaptureOption {
	return func(c *CaptureConfig) error { c.tlsSecrets = provider; return nil }
}

type binTLS struct {
	client               int
	random, serverRandom []byte
	version, suite       uint16
	sni, alpn            string
	handshake            [2][]byte
	seq                  [2]uint64
	app                  [2]bool
	ccs                  [2]bool
	keys                 [2]cipher.AEAD
	iv                   [2][]byte
	secret               [2][]byte
	child                *binFlow
	plaintext            []byte
	content              byte
}

func newBinTLS() *binTLS         { return &binTLS{client: -1} }
func (t *binTLS) storage() int64 { return 2048 + int64(cap(t.handshake[0])+cap(t.handshake[1])) }
func tlsU24(b []byte) int        { return int(b[0])<<16 | int(b[1])<<8 | int(b[2]) }
func (f *binFlow) consumeTLS(dir int, e *ProtocolEvent) (map[string]any, error) {
	t := f.tls
	if t == nil {
		t = newBinTLS()
		f.tls = t
	}
	w := e.Raw
	typ, plain := w[0], w[5:]
	info := map[string]any{"Record Type": typ, "Record Version": binary.BigEndian.Uint16(w[1:3]), "Record Length": len(plain), "Authentication Verified": false, "Certificate Trust": "not-evaluated"}
	e.Session = info
	if t.version == 0x304 && typ == 22 && t.suite != 0 {
		return nil, protocolError(ErrMalformedMessage, "TLS 1.3 cleartext handshake after ServerHello")
	}
	encrypted := typ == 23 || t.ccs[dir] && t.version != 0x304
	limit := 16384
	if encrypted {
		limit += 2048
	}
	if encrypted && t.version == 0x304 {
		limit = 16640
	}
	if len(plain) > limit {
		return nil, protocolError(ErrMalformedMessage, "TLS record exceeds version limit")
	}

	if encrypted {
		info["Content Visibility"] = "encrypted"
		if t.client < 0 || len(t.random) != 32 || t.version == 0 {
			info["Secret Lookup Status"] = "missing-context"
			return map[string]any{"fields": info}, nil
		}
		if t.version != 0x304 && t.version != 0x303 || t.version == 0x304 && t.suite != 0x1301 || t.version == 0x303 && t.suite != 0xc02f && t.suite != 0xc02b {
			info["Secret Lookup Status"] = "unsupported-suite"
			return map[string]any{"fields": info}, nil
		}
		if t.keys[dir] == nil {
			ok, err := t.loadKey(dir, f.a.tlsSecrets)
			if err != nil {
				return nil, err
			}
			if !ok {
				info["Secret Lookup Status"] = "missing-key"
				return map[string]any{"fields": info}, nil
			}
		}
		var err error
		plain, typ, err = t.open(dir, w)
		if err != nil {
			return nil, err
		}
		info["Authentication Verified"] = true
		info["Secret Lookup Status"] = "found"
		info["Content Visibility"] = "authenticated-plaintext"
		info["Inner Content Type"] = typ
	} else {
		info["Content Visibility"] = "cleartext"
	}
	if typ == 20 {
		if !bytes.Equal(plain, []byte{1}) {
			return nil, protocolError(ErrMalformedMessage, "invalid TLS ChangeCipherSpec")
		}
		if t.version != 0x304 {
			t.ccs[dir] = true
			t.seq[dir] = 0
		}
	} else if typ == 22 {
		if len(t.handshake[dir])+len(plain) > f.a.config.MaxMessageBytes {
			return nil, protocolError(ErrResourceExceeded, "TLS handshake buffer limit")
		}
		if err := f.reserveSession(t.storage() + int64(len(plain))); err != nil {
			return nil, err
		}
		t.handshake[dir] = append(t.handshake[dir], plain...)
		var messages []map[string]any
		for len(t.handshake[dir]) >= 4 {
			b := t.handshake[dir]
			n := 4 + tlsU24(b[1:4])
			if n > f.a.config.MaxMessageBytes {
				return nil, protocolError(ErrResourceExceeded, "TLS handshake length limit")
			}
			if n > len(b) {
				break
			}
			if len(messages) >= f.a.budget.MaxCollectionElements {
				return nil, protocolError(ErrResourceExceeded, "TLS handshake count limit")
			}
			m, err := t.handshakeMessage(dir, b[:n], encrypted)
			if err != nil {
				return nil, err
			}
			messages = append(messages, m)
			t.handshake[dir] = b[n:]
		}
		if len(t.handshake[dir]) == 0 {
			t.handshake[dir] = nil
		}
		info["Handshake Messages"] = messages
		info["Handshake Incomplete"] = len(t.handshake[dir]) > 0
	} else if typ == 23 && encrypted {
		t.plaintext = bytes.Clone(plain)
		t.content = typ
	} else if typ == 21 {
		if len(plain) != 2 {
			return nil, protocolError(ErrMalformedMessage, "invalid TLS alert length")
		}
		info["Alert Level"], info["Alert Description"] = plain[0], plain[1]
	}
	info["Negotiated Version"], info["Cipher Suite"], info["SNI"], info["ALPN"] = t.version, t.suite, t.sni, t.alpn
	if err := f.reserveSession(t.storage()); err != nil {
		return nil, err
	}
	return map[string]any{"fields": info}, nil
}
func (t *binTLS) handshakeMessage(dir int, b []byte, authenticated bool) (map[string]any, error) {
	typ, body := b[0], b[4:]
	m := map[string]any{"Type": typ, "Length": len(body), "Authenticated": authenticated}
	switch typ {
	case 1, 2:
		if len(body) < 35 {
			return nil, protocolError(ErrMalformedMessage, "truncated TLS hello")
		}
		p := 35 + int(body[34])
		if body[34] > 32 || p > len(body) {
			return nil, protocolError(ErrMalformedMessage, "TLS session ID length")
		}
		if typ == 1 {
			if t.client >= 0 && (t.client != dir || !bytes.Equal(t.random, body[2:34])) {
				return nil, protocolError(ErrUnsupportedFeature, "TLS renegotiation requires a new epoch")
			}
			t.client = dir
			t.random = bytes.Clone(body[2:34])
			if len(body)-p < 2 {
				return nil, protocolError(ErrMalformedMessage, "TLS cipher vector")
			}
			n := int(binary.BigEndian.Uint16(body[p:]))
			p += 2
			if n < 2 || n%2 != 0 || n > len(body)-p {
				return nil, protocolError(ErrMalformedMessage, "TLS cipher vector length")
			}
			p += n
			if p >= len(body) {
				return nil, protocolError(ErrMalformedMessage, "TLS compression vector")
			}
			n = int(body[p])
			p++
			if n < 1 || n > len(body)-p {
				return nil, protocolError(ErrMalformedMessage, "TLS compression length")
			}
			p += n
		} else {
			if len(body)-p < 3 {
				return nil, protocolError(ErrMalformedMessage, "TLS server cipher")
			}
			t.suite = binary.BigEndian.Uint16(body[p:])
			t.version = binary.BigEndian.Uint16(body)
			t.serverRandom = bytes.Clone(body[2:34])
			p += 3
		}
		if p < len(body) {
			if err := t.extensions(body[p:], typ == 1); err != nil {
				return nil, err
			}
		}
	case 8:
		if !authenticated {
			return nil, protocolError(ErrMalformedMessage, "cleartext TLS EncryptedExtensions")
		}
		if err := t.extensions(body, false); err != nil {
			return nil, err
		}
	case 20:
		if authenticated && t.version == 0x304 {
			t.app[dir] = true
			t.seq[dir] = 0
			t.keys[dir] = nil
			t.iv[dir] = nil
			t.secret[dir] = nil
		}
	case 24:
		if !authenticated || !t.app[dir] || len(body) != 1 || body[0] > 1 {
			return nil, protocolError(ErrMalformedMessage, "invalid TLS KeyUpdate")
		}
		next, err := quicHKDFExpandLabel(t.secret[dir], "traffic upd", 32)
		if err != nil {
			return nil, err
		}
		if err = t.install13(dir, next); err != nil {
			return nil, err
		}
		t.seq[dir] = 0
	case 11:
		// Certificate bytes are evidence, not a trust assertion. Store hashes only;
		// large chains remain bounded by the handshake message budget.
		sum := sha256.Sum256(body)
		m["Certificate Message SHA256"] = hex.EncodeToString(sum[:])
		certs, err := tlsCertificates(body, t.version == 0x304)
		if err != nil {
			return nil, err
		}
		m["Certificates"] = certs
	}
	return m, nil
}
func (t *binTLS) extensions(b []byte, client bool) error {
	if len(b) < 2 || int(binary.BigEndian.Uint16(b)) != len(b)-2 {
		return protocolError(ErrMalformedMessage, "TLS extensions length")
	}
	b = b[2:]
	seen := map[uint16]bool{}
	for len(b) > 0 {
		if len(b) < 4 {
			return protocolError(ErrMalformedMessage, "TLS extension header")
		}
		typ, n := binary.BigEndian.Uint16(b), int(binary.BigEndian.Uint16(b[2:]))
		b = b[4:]
		if n > len(b) || seen[typ] {
			return protocolError(ErrMalformedMessage, "TLS duplicate or truncated extension")
		}
		seen[typ] = true
		v := b[:n]
		b = b[n:]
		switch typ {
		case 43:
			if !client {
				if len(v) != 2 {
					return protocolError(ErrMalformedMessage, "TLS selected version length")
				}
				t.version = binary.BigEndian.Uint16(v)
			}
		case 0:
			if client {
				if len(v) < 5 || int(binary.BigEndian.Uint16(v)) != len(v)-2 || v[2] != 0 || int(binary.BigEndian.Uint16(v[3:])) != len(v)-5 {
					return protocolError(ErrMalformedMessage, "TLS SNI length")
				}
				t.sni = string(v[5:])
			}
		case 16:
			if len(v) < 3 || int(binary.BigEndian.Uint16(v)) != len(v)-2 {
				return protocolError(ErrMalformedMessage, "TLS ALPN vector")
			}
			v = v[2:]
			var protocols []string
			for len(v) > 0 {
				n := int(v[0])
				v = v[1:]
				if n == 0 || n > len(v) {
					return protocolError(ErrMalformedMessage, "TLS ALPN length")
				}
				protocols = append(protocols, string(v[:n]))
				v = v[n:]
			}
			if !client {
				if len(protocols) != 1 {
					return protocolError(ErrMalformedMessage, "TLS selected ALPN count")
				}
				t.alpn = protocols[0]
			}
		}
	}
	return nil
}
func (t *binTLS) loadKey(dir int, p TLSSecretProvider) (bool, error) {
	if p == nil {
		return false, nil
	}
	if t.version == 0x304 {
		label := "SERVER_HANDSHAKE_TRAFFIC_SECRET"
		if dir == t.client {
			label = "CLIENT_HANDSHAKE_TRAFFIC_SECRET"
		}
		if t.app[dir] {
			label = "SERVER_TRAFFIC_SECRET_0"
			if dir == t.client {
				label = "CLIENT_TRAFFIC_SECRET_0"
			}
		}
		s, ok := p.LookupTLSSecret(label, t.random)
		if !ok {
			return false, nil
		}
		if len(s) != 32 {
			return false, protocolError(ErrContextRequired, "TLS AES128 traffic secret length")
		}
		return true, t.install13(dir, s)
	}
	master, ok := p.LookupTLSSecret("CLIENT_RANDOM", t.random)
	if !ok {
		return false, nil
	}
	if len(master) != 48 || len(t.serverRandom) != 32 {
		return false, protocolError(ErrContextRequired, "TLS 1.2 master secret or random missing")
	}
	seed := append([]byte("key expansion"), t.serverRandom...)
	seed = append(seed, t.random...)
	block := tlsPRF(master, seed, 40)
	i := 0
	if dir != t.client {
		i = 1
	}
	c, err := aes.NewCipher(block[i*16 : (i+1)*16])
	if err != nil {
		return false, err
	}
	t.keys[dir], err = cipher.NewGCM(c)
	t.iv[dir] = bytes.Clone(block[32+i*4 : 36+i*4])
	return err == nil, err
}
func tlsPRF(secret, seed []byte, n int) []byte {
	mac := func(data []byte) []byte { h := hmac.New(sha256.New, secret); h.Write(data); return h.Sum(nil) }
	a := mac(seed)
	out := make([]byte, 0, n+32)
	for len(out) < n {
		out = append(out, mac(append(bytes.Clone(a), seed...))...)
		a = mac(a)
	}
	return out[:n]
}
func (t *binTLS) install13(dir int, secret []byte) error {
	key, err := quicHKDFExpandLabel(secret, "key", 16)
	if err != nil {
		return err
	}
	iv, err := quicHKDFExpandLabel(secret, "iv", 12)
	if err != nil {
		return err
	}
	c, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	a, err := cipher.NewGCM(c)
	if err != nil {
		return err
	}
	t.keys[dir], t.iv[dir], t.secret[dir] = a, iv, bytes.Clone(secret)
	return nil
}
func (t *binTLS) open(dir int, w []byte) ([]byte, byte, error) {
	if t.seq[dir] == ^uint64(0) {
		return nil, 0, protocolError(ErrResourceExceeded, "TLS record sequence exhausted")
	}
	nonce := bytes.Clone(t.iv[dir])
	aad := w[:5]
	payload := w[5:]
	typ := w[0]
	if t.version == 0x304 {
		if len(nonce) != 12 || w[0] != 23 || binary.BigEndian.Uint16(w[1:3]) != 0x303 {
			return nil, 0, protocolError(ErrMalformedMessage, "TLS 1.3 protected record header")
		}
		for i := 0; i < 8; i++ {
			nonce[11-i] ^= byte(t.seq[dir] >> uint(i*8))
		}
	} else {
		if len(payload) < 24 {
			return nil, 0, protocolError(ErrMalformedMessage, "TLS GCM record length")
		}
		nonce = append(nonce, payload[:8]...)
		payload = payload[8:]
		aad = make([]byte, 13)
		binary.BigEndian.PutUint64(aad, t.seq[dir])
		copy(aad[8:11], w[:3])
		binary.BigEndian.PutUint16(aad[11:], uint16(len(payload)-16))
	}
	plain, err := t.keys[dir].Open(nil, nonce, payload, aad)
	if err != nil {
		return nil, 0, protocolError(ErrAuthenticationFailed, "TLS record authentication failed")
	}
	if t.version == 0x304 {
		i := len(plain) - 1
		for i >= 0 && plain[i] == 0 {
			i--
		}
		if i < 0 || plain[i] < 21 || plain[i] > 23 {
			return nil, 0, protocolError(ErrMalformedMessage, "TLS inner content type")
		}
		typ = plain[i]
		plain = plain[:i]
	}
	t.seq[dir]++
	return plain, typ, nil
}
func (f *binFlow) deliverTLS(dir int, parent *ProtocolEvent) {
	t := f.tls
	if t == nil || t.plaintext == nil {
		return
	}
	plain := t.plaintext
	t.plaintext = nil
	if t.child == nil {
		t.child = &binFlow{a: f.a, id: f.id, endpoints: f.endpoints, ports: f.ports, domain: f.domain, byteSource: "decrypted", captureTCP: true}
		// STARTTLS carries observed application state into authenticated plaintext.
		// The new child owns that state; guessing from a port would lose the
		// greeting capabilities and sequence number for MySQL.
		if f.mysql != nil && f.mysql.phase == "tls" {
			t.child.protocol = "mysql"
			t.child.mysql = f.mysql
			f.mysql = nil
			t.child.mysql.phase = "handshake"
		}
		if f.pg != nil {
			t.child.protocol = "postgresql"
			t.child.pg = f.pg
			f.pg = nil
		}

	}
	t.child.parentID = parent.ID
	d := &t.child.directions[dir]
	refs := parent.SourceBytes.PacketRefs
	if len(refs) == 0 {
		refs = []PacketReference{{}}
	}
	for _, ref := range refs {
		if int64(f.a.config.MaxBufferedBytes)-f.a.buffered.Load() >= int64(f.a.config.MaxMessageBytes)+80 && len(d.contributions) < f.a.budget.MaxCollectionElements && f.a.reserveEvidence(80) {
			start := d.offset + uint64(len(d.buffer))
			d.contributions = append(d.contributions, contribution{start: start, end: start + uint64(len(plain)), parent: parent.ID, ref: ref})
		} else {
			d.evidenceLimited = true
			break
		}
	}
	t.child.feed(dir, plain, parent.Timestamp)
}

func tlsCertificates(b []byte, tls13 bool) ([]map[string]any, error) {
	bad := func() error { return protocolError(ErrMalformedMessage, "TLS certificate vector") }
	if tls13 {
		if len(b) < 1 || int(b[0]) > len(b)-1 {
			return nil, bad()
		}
		b = b[1+int(b[0]):]
	}
	if len(b) < 3 || tlsU24(b) != len(b)-3 {
		return nil, bad()
	}
	b = b[3:]
	var out []map[string]any
	for len(b) > 0 {
		if len(out) >= 64 {
			return nil, protocolError(ErrResourceExceeded, "TLS certificate count")
		}
		if len(b) < 3 {
			return nil, bad()
		}
		n := tlsU24(b)
		b = b[3:]
		if n == 0 || n > len(b) {
			return nil, bad()
		}
		der := b[:n]
		b = b[n:]
		sum := sha256.Sum256(der)
		row := map[string]any{"SHA256": hex.EncodeToString(sum[:]), "DER Length": n, "Trust": "not-evaluated"}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			row["Parse Error"] = "invalid X.509"
		} else {
			row["Subject"] = cert.Subject.String()
			row["Issuer"] = cert.Issuer.String()
			row["DNS Names"] = cert.DNSNames
			row["Not Before"] = cert.NotBefore
			row["Not After"] = cert.NotAfter
			row["Serial"] = cert.SerialNumber.String()
		}
		out = append(out, row)
		if tls13 {
			if len(b) < 2 {
				return nil, bad()
			}
			n = int(binary.BigEndian.Uint16(b))
			b = b[2:]
			if n > len(b) {
				return nil, bad()
			}
			b = b[n:]
		}
	}
	return out, nil
}
