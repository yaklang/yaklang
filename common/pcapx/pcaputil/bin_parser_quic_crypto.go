package pcaputil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/chacha20"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

// QUIC v1 Initial salt from RFC 9001 §5.2.
var quicInitialSalt = []byte{
	0x38, 0x76, 0x2c, 0xf7, 0xf5, 0x59, 0x34, 0xb3, 0x4d, 0x17,
	0x9a, 0xe6, 0xa4, 0xc8, 0x0c, 0xad, 0xcc, 0xbb, 0x7f, 0x0a,
}

// RFC9369 section3.3.1, distinct from the version1 Initial salt.
var quicInitialSaltV2 = []byte{
	0x0d, 0xed, 0xe3, 0xde, 0xf7, 0x00, 0xa6, 0xdb, 0x81, 0x93,
	0x81, 0xbe, 0x6e, 0x26, 0x9d, 0xcb, 0xf9, 0xbd, 0x2e, 0xd9,
}

const (
	quicAEADAES128GCM        = "aes-128-gcm"
	quicAEADAES256GCM        = "aes-256-gcm"
	quicAEADChaCha20Poly1305 = "chacha20-poly1305"
)

// QUICKeyMaterial is the caller-provided key source for Handshake/0-RTT/1-RTT.
// Initial secrets are derived from the client's first DCID and do not need this.
type QUICKeyMaterial struct {
	KeyLog  string
	Secrets map[string][]byte
	AEAD    string
	Version uint32 // wire version; zero preserves the existing v1 key-source API
}

type quicTrafficKeys struct {
	key, iv, hp, secret []byte
	aead                string
	version             uint32
}

type quicKeyring struct {
	version   uint32
	aead      string
	initial   [2]*quicTrafficKeys
	handshake [2]*quicTrafficKeys
	early     [2]*quicTrafficKeys
	app       [2][2]*quicTrafficKeys // [direction][key phase]
}

func quicLoadKeys(km QUICKeyMaterial) (*quicKeyring, error) {
	version := quicDefaultVersion(km.Version)
	if !quicSupportedVersion(version) {
		return nil, protocolError(ErrUnsupportedVersion, "QUIC key material version 0x%08x", version)
	}
	aead := km.AEAD
	if aead == "" {
		aead = quicAEADAES128GCM
	}
	if aead != quicAEADAES128GCM && aead != quicAEADAES256GCM && aead != quicAEADChaCha20Poly1305 {
		return nil, fmt.Errorf("quic: unsupported AEAD %q", aead)
	}
	r := &quicKeyring{aead: aead, version: version}
	put := func(label string, secret []byte) error {
		tk, err := quicDeriveTrafficVersion(secret, aead, version)
		if err != nil {
			return err
		}
		switch label {
		case "CLIENT_HANDSHAKE_TRAFFIC_SECRET", "QUIC_CLIENT_HANDSHAKE_TRAFFIC_SECRET":
			r.handshake[0] = tk
		case "SERVER_HANDSHAKE_TRAFFIC_SECRET", "QUIC_SERVER_HANDSHAKE_TRAFFIC_SECRET":
			r.handshake[1] = tk
		case "CLIENT_EARLY_TRAFFIC_SECRET", "QUIC_CLIENT_EARLY_TRAFFIC_SECRET":
			r.early[0], r.early[1] = tk, tk
		case "CLIENT_TRAFFIC_SECRET_0", "QUIC_CLIENT_TRAFFIC_SECRET_0":
			r.app[0][0] = tk
			r.app[0][1], err = quicNextPhase(tk)
		case "SERVER_TRAFFIC_SECRET_0", "QUIC_SERVER_TRAFFIC_SECRET_0":
			r.app[1][0] = tk
			r.app[1][1], err = quicNextPhase(tk)
		default:
			return nil
		}
		return err
	}
	for k, v := range km.Secrets {
		if err := put(k, v); err != nil {
			return nil, err
		}
	}
	for _, line := range strings.Split(km.KeyLog, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		sec, err := hex.DecodeString(f[2])
		if err != nil {
			return nil, fmt.Errorf("quic: key log secret: %w", err)
		}
		if err := put(f[0], sec); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func quicDeriveTraffic(secret []byte, aead string) (*quicTrafficKeys, error) {
	return quicDeriveTrafficVersion(secret, aead, quicVersion1)
}

func quicDeriveTrafficVersion(secret []byte, aead string, version uint32) (*quicTrafficKeys, error) {
	version = quicDefaultVersion(version)
	if !quicSupportedVersion(version) {
		return nil, protocolError(ErrUnsupportedVersion, "QUIC traffic version 0x%08x", version)
	}
	label := "quic "
	if version == quicVersion2 {
		label = "quicv2 "
	}
	keyLen, hpLen := 16, 16
	if aead == quicAEADChaCha20Poly1305 || aead == quicAEADAES256GCM {
		keyLen, hpLen = 32, 32
	}
	key, err := quicHKDFExpandLabel(secret, label+"key", keyLen)
	if err != nil {
		return nil, err
	}
	iv, err := quicHKDFExpandLabel(secret, label+"iv", 12)
	if err != nil {
		return nil, err
	}
	hp, err := quicHKDFExpandLabel(secret, label+"hp", hpLen)
	if err != nil {
		return nil, err
	}
	return &quicTrafficKeys{key: key, iv: iv, hp: hp, secret: append([]byte(nil), secret...), aead: aead, version: version}, nil
}

func quicNextPhase(tk *quicTrafficKeys) (*quicTrafficKeys, error) {
	if tk == nil {
		return nil, nil
	}
	label := "quic ku"
	if tk.version == quicVersion2 {
		label = "quicv2 ku"
	}
	next, err := quicHKDFExpandLabel(tk.secret, label, len(tk.secret))
	if err != nil {
		return nil, err
	}
	out, err := quicDeriveTrafficVersion(next, tk.aead, quicDefaultVersion(tk.version))
	if err == nil {
		out.hp = append([]byte(nil), tk.hp...)
	} // Header protection keys never update.
	return out, err
}

func quicInitialKeys(dcid []byte) ([2]*quicTrafficKeys, error) {
	return quicInitialKeysVersion(dcid, quicVersion1)
}

func quicInitialKeysVersion(dcid []byte, version uint32) ([2]*quicTrafficKeys, error) {
	var out [2]*quicTrafficKeys
	version = quicDefaultVersion(version)
	if !quicSupportedVersion(version) {
		return out, protocolError(ErrUnsupportedVersion, "QUIC Initial version 0x%08x", version)
	}
	salt := quicInitialSalt
	if version == quicVersion2 {
		salt = quicInitialSaltV2
	}
	prk := hkdf.Extract(sha256.New, dcid, salt)
	client, err := quicHKDFExpandLabel(prk, "client in", 32)
	if err != nil {
		return out, err
	}
	server, err := quicHKDFExpandLabel(prk, "server in", 32)
	if err != nil {
		return out, err
	}
	out[0], err = quicDeriveTrafficVersion(client, quicAEADAES128GCM, version)
	if err != nil {
		return out, err
	}
	out[1], err = quicDeriveTrafficVersion(server, quicAEADAES128GCM, version)
	return out, err
}

func quicHKDFExpandLabel(secret []byte, label string, length int) ([]byte, error) {
	full := "tls13 " + label
	info := make([]byte, 0, 3+len(full)+1)
	info = binary.BigEndian.AppendUint16(info, uint16(length))
	info = append(info, byte(len(full)))
	info = append(info, full...)
	info = append(info, 0)
	out := make([]byte, length)
	hashFn := sha256.New
	if len(secret) == 48 {
		hashFn = sha512.New384
	}
	n, err := hkdf.Expand(hashFn, secret, info).Read(out)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n != length {
		return nil, fmt.Errorf("quic: HKDF-Expand-Label short read")
	}
	return out, nil
}

func (q *binQUIC) ensureInitialKeys(dcid []byte) error {
	if q.keys == nil {
		q.keys = &quicKeyring{aead: quicAEADAES128GCM, version: q.effectiveVersion()}
	}
	if q.keys.initial[0] != nil {
		return nil
	}
	if len(q.initialDCID) == 0 {
		q.initialDCID = append([]byte(nil), dcid...)
	}
	if len(q.initialDCID) == 0 {
		return fmt.Errorf("quic: no Initial DCID")
	}
	keys, err := quicInitialKeysVersion(q.initialDCID, q.effectiveVersion())
	if err != nil {
		return err
	}
	q.keys.initial = keys
	return nil
}

func (q *binQUIC) keysFor(dir int, h quicHdr) (*quicTrafficKeys, string) {
	if dir != 0 && dir != 1 {
		dir = 0
	}
	if q.keys == nil {
		q.keys = &quicKeyring{aead: quicAEADAES128GCM}
	}
	switch {
	case h.space == quicSpaceInitial:
		if err := q.ensureInitialKeys(h.dcid); err != nil {
			return nil, "initial-secret-failed"
		}
		return q.keys.initial[dir], ""
	case h.typeName == "Handshake":
		if q.keys.handshake[dir] == nil {
			return nil, "handshake-keys-missing"
		}
		return q.keys.handshake[dir], ""
	case h.typeName == "0-RTT":
		if q.keys.early[dir] == nil {
			return nil, "early-keys-missing"
		}
		return q.keys.early[dir], ""
	default:
		// 1-RTT: HP is phase-independent; AEAD phase is chosen after header unprotect.
		if q.keys.app[dir][0] == nil && q.keys.app[dir][1] == nil {
			return nil, "application-keys-missing"
		}
		if q.keys.app[dir][0] != nil {
			return q.keys.app[dir][0], ""
		}
		return q.keys.app[dir][1], ""
	}
}

func quicUnprotect(raw []byte, h quicHdr, tk *quicTrafficKeys, phaseKeys [2]*quicTrafficKeys, largest uint64) (plain []byte, pn uint64, pnLen int, first byte, phase int, err error) {
	if tk == nil || len(raw) < h.payloadOff+20 {
		return nil, 0, 0, 0, 0, fmt.Errorf("quic: truncated protected packet")
	}
	sampleOff := h.payloadOff + 4
	if sampleOff+16 > len(raw) || sampleOff+16 > h.size {
		return nil, 0, 0, 0, 0, fmt.Errorf("quic: header protection sample truncated")
	}
	mask, err := quicHeaderMask(tk, raw[sampleOff:sampleOff+16])
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	pkt := append([]byte(nil), raw[:h.size]...)
	if h.long {
		pkt[0] ^= mask[0] & 0x0f
	} else {
		pkt[0] ^= mask[0] & 0x1f
	}
	first = pkt[0]
	pnLen = int(first&0x03) + 1
	if h.payloadOff+pnLen > h.size {
		return nil, 0, 0, 0, 0, fmt.Errorf("quic: packet number truncated")
	}
	for i := 0; i < pnLen; i++ {
		pkt[h.payloadOff+i] ^= mask[1+i]
	}
	truncated := quicTruncatedPN(pkt[h.payloadOff : h.payloadOff+pnLen])
	pn = quicDecodePN(largest, truncated, pnLen)
	phase = 0
	aeadKey := tk
	if !h.long {
		phase = int(first>>2) & 1
		if phaseKeys[phase] != nil {
			aeadKey = phaseKeys[phase]
		}
	}
	nonce := quicNonce(aeadKey.iv, pn)
	aad := pkt[:h.payloadOff+pnLen]
	ct := pkt[h.payloadOff+pnLen : h.size]
	plain, err = quicOpen(aeadKey, nonce, ct, aad)
	if err != nil {
		// A passive capture may miss more than half a truncated PN window.
		// Try four neighboring windows on either side, committing nothing
		// until the AEAD tag proves the reconstructed full packet number.
		window := uint64(1) << uint(8*pnLen)
		for n := uint64(1); n <= 4; n++ {
			for _, forward := range []bool{true, false} {
				candidate := pn + n*window
				if !forward {
					if pn < n*window {
						continue
					}
					candidate = pn - n*window
				}
				if candidate >= 1<<62 {
					continue
				}
				if p, e := quicOpen(aeadKey, quicNonce(aeadKey.iv, candidate), ct, aad); e == nil {
					return p, candidate, pnLen, first, phase, nil
				}
			}
		}
		return nil, 0, 0, 0, 0, err
	}
	return plain, pn, pnLen, first, phase, nil
}

func quicHeaderMask(tk *quicTrafficKeys, sample []byte) ([]byte, error) {
	if tk.aead == quicAEADChaCha20Poly1305 {
		nonce := sample[4:16]
		c, err := chacha20.NewUnauthenticatedCipher(tk.hp, nonce)
		if err != nil {
			return nil, err
		}
		c.SetCounter(binary.LittleEndian.Uint32(sample[:4]))
		mask := make([]byte, 5)
		c.XORKeyStream(mask, mask)
		return mask, nil
	}
	block, err := aes.NewCipher(tk.hp)
	if err != nil {
		return nil, err
	}
	out := make([]byte, block.BlockSize())
	block.Encrypt(out, sample)
	return out[:5], nil
}

func quicNonce(iv []byte, pn uint64) []byte {
	n := make([]byte, len(iv))
	copy(n, iv)
	var pnb [8]byte
	binary.BigEndian.PutUint64(pnb[:], pn)
	for i := 0; i < 8 && i < len(n); i++ {
		n[len(n)-1-i] ^= pnb[7-i]
	}
	return n
}

func quicOpen(tk *quicTrafficKeys, nonce, ct, aad []byte) ([]byte, error) {
	var (
		aead cipher.AEAD
		err  error
	)
	if tk.aead == quicAEADChaCha20Poly1305 {
		aead, err = chacha20poly1305.New(tk.key)
	} else {
		block, berr := aes.NewCipher(tk.key)
		if berr != nil {
			return nil, berr
		}
		aead, err = cipher.NewGCM(block)
	}
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ct, aad)
}

// ApplyQUICKeys installs Handshake/0-RTT/1-RTT secrets on a ProtocolSession.
// Initial packet keys are derived from the client's first DCID and do not
// require this. Missing keys keep Handshake/1-RTT payloads Encrypted.
func ApplyQUICKeys(s ProtocolSession, km QUICKeyMaterial) error {
	cs, ok := s.(*captureSession)
	if !ok {
		return fmt.Errorf("quic: unsupported session type")
	}
	ring, err := quicLoadKeys(km)
	if err != nil {
		return err
	}
	cs.f.a.quicKeys = ring
	if cs.f.quic != nil {
		cs.f.quic.keys = ring
	}
	return nil
}

// Traffic keys are immutable. Copy the per-connection slots so deriving Initial
// keys or installing a phase never mutates another connection's keyring.
func cloneQUICKeyring(r *quicKeyring) *quicKeyring {
	if r == nil {
		return nil
	}
	copy := *r
	return &copy
}
