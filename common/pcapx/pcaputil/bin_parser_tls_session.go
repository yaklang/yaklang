package pcaputil

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"hash"
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
	// Legacy NSS RSA entries identify an encrypted-premaster prefix, not a
	// ClientRandom. Validate their bounded envelope and duplicate consistency,
	// but never offer a premaster through the ClientRandom secret provider.
	legacyRSA := map[string][32]byte{}
	for _, line := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' }) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 3 {
			return nil, fmt.Errorf("invalid TLS key log entry")
		}
		if f[0] == "RSA" {
			prefix, prefixErr := hex.DecodeString(f[1])
			premaster, secretErr := hex.DecodeString(f[2])
			if prefixErr != nil || secretErr != nil || len(prefix) != 8 || len(premaster) != 48 {
				return nil, fmt.Errorf("invalid legacy RSA key log entry")
			}
			key, value := hex.EncodeToString(prefix), sha256.Sum256(premaster)
			if old, ok := legacyRSA[key]; ok {
				if old != value {
					return nil, fmt.Errorf("conflicting TLS key log entries")
				}
				continue
			}
			if len(k.secrets)+len(legacyRSA) >= 4096 {
				return nil, fmt.Errorf("TLS key log entry budget exceeded")
			}
			legacyRSA[key] = value
			continue
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
		if _, exists := k.secrets[key]; !exists && len(k.secrets)+len(legacyRSA) >= 4096 {
			return nil, fmt.Errorf("TLS key log entry budget exceeded")
		}
		k.secrets[key] = secret
	}
	return k, nil
}

// WithTLSSecrets enables authenticated TLS decryption for the documented AEAD
// profiles. Missing keys preserve encrypted events. Certificate trust
// and application authentication are not established by record authentication.
func WithTLSSecrets(provider TLSSecretProvider) CaptureOption {
	return func(c *CaptureConfig) error { c.tlsSecrets = provider; return nil }
}

type binTLS struct {
	protectedRecordObserved                [2]bool
	serverHelloDirectionKnown              bool
	serverHelloDirection                   int8
	earlyDataOfferKnown                    bool
	earlyDataOffered                       bool
	earlyDataSelectionKnown                bool
	earlyDataPSKSelected                   bool
	earlyDataIdentity                      uint16
	early                                  tlsEarlyRecordState
	recordGap                              [2]bool
	handshakeGap                           [2]bool
	recoveryFailures                       [2]uint8
	extraStorage                           int64
	client                                 int
	random, serverRandom                   []byte
	version, suite                         uint16
	sni, alpn                              string
	handshake                              [2][]byte
	handshakeAuthenticated                 [2]bool
	alert                                  [2]byte
	alertPending                           [2]bool
	alertAuthenticated                     [2]bool // retained alert octet, not a public DTO claim
	closeNotify                            [2]bool // authenticated sender closure, isolated to this generation/direction
	closeNotifyPending                     [2]bool // delivered only after the owning record callback
	fatalAlert                             [2]bool // authenticated abort cannot later become a clean application EOF
	seq                                    [2]uint64
	app                                    [2]bool
	ccs                                    [2]bool
	keys                                   [2]cipher.AEAD
	iv                                     [2][]byte
	secret                                 [2][]byte
	child                                  *binFlow
	plaintext                              []byte
	content                                byte
	firstHello, retryCookie                []byte
	alpnOffer                              []byte // immutable admitted ClientHello vector, retained after final ServerHello
	alpnOfferKnown                         bool
	retrySeen, retryClientSeen, finalHello bool
	retrySuite, retryGroup                 uint16
	resumption                             tlsResumptionState
	requestContexts                        map[string]struct{}
	phaOfferKnown, phaOffered              bool // outlive the owned retry ClientHello body
	phaRequests                            map[string]tlsPHARequest
	phaResponse                            tlsPHAResponse
	phaInitialFinished, phaGap             [2]bool
	phaPeerPrefixComplete                  bool // current record only; no connection-wide ACK inference
	phaEarlyResponses                      map[string]tlsPHAResponse
	serverHandshakeStage                   uint8 // accepted initial TLS 1.3 server observations only
	clientHandshakeStage                   uint8 // accepted initial TLS 1.3 client observations only
	clientCertificateEmpty                 bool  // private fact from an admitted initial client Certificate
	initialPSKAuthentication               uint8 // complete ServerHello observation; never a public session hint
	keyExchangeOffers                      tls12KeyExchangeOffers
	clientKeyExchangeObserved              bool // one admitted TLS 1.2 CKE in this epoch
	certificateStatus                      tlsCertificateStatusState
	precontext                             [2][]tlsPrecontextRecord
	precontextCharged                      [2]int64
}

func newBinTLS() *binTLS { return &binTLS{client: -1} }
func (t *binTLS) storage() int64 {
	return 2048 + t.extraStorage + t.early.storage() + t.resumption.storage() + t.requestContextStorage() + t.precontextStorage() + int64(cap(t.handshake[0])+cap(t.handshake[1])+cap(t.firstHello)+cap(t.retryCookie)+cap(t.alpnOffer)+cap(t.keyExchangeOffers.groups)+cap(t.keyExchangeOffers.signatures))
}
func tlsU24(b []byte) int { return int(b[0])<<16 | int(b[1])<<8 | int(b[2]) }
func (f *binFlow) consumeTLS(dir int, e *ProtocolEvent) (map[string]any, error) {
	t := f.tls
	if t == nil {
		t = newBinTLS()
		if f.ftpData != nil {
			t.extraStorage = f.ftpData.charge
		}
		f.tls = t
	}
	w := e.Raw
	t.phaPeerPrefixComplete = f.phaPeerPrefixComplete(dir, e)
	t.plaintext = nil
	t.content = 0
	t.early.record = false
	typ, plain := w[0], w[5:]
	// Preserve already observed negotiation on encrypted records too. Missing
	// keys and unsupported suites return early; neither erases the wire-proven
	// version, SNI or selected ALPN from the connection.
	info := map[string]any{"Record Type": typ, "Record Version": binary.BigEndian.Uint16(w[1:3]), "Record Length": len(plain), "Authentication Verified": false, "Certificate Trust": "not-evaluated"}
	if s := f.wcfTLS; s != nil {
		info["Observed WCF Carrier"] = s.contextFields()
	}
	if t.version != 0 && t.suite != 0 {
		info["Negotiated Version"], info["Cipher Suite"] = t.version, t.suite
	}
	if t.sni != "" {
		info["SNI"] = t.sni
	}
	if t.alpn != "" {
		info["ALPN"] = t.alpn
	}
	e.Session = info
	defer func() {
		t.resumption.project(info)
		// A refused record still has exact header/authentication observations.
		// Freeze those separately from editable public maps. sessionError is
		// checked before this cache by Decode/GetFields, so it grants no
		// successful decode or application admission to a rejected message.
		e.semanticFields = cloneSession(info)
	}()
	if typ == 21 && t.tls13SenderNeedsRecordProtection(dir) {
		info["Content Visibility"] = "cleartext"
		return nil, protocolError(ErrMalformedMessage, "TLS 1.3 cleartext alert after sender protection started")
	}
	if t.version == 0x304 && typ == 22 && t.finalHello {
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
		if held, err := f.retainTLSEarlyRecord(dir, e, info); held {
			return map[string]any{"fields": info}, err
		}
		if err := t.checkCiphertextEnvelope(w); err != nil {
			f.noteTLSEnvelopeFailure(dir, info)
			return nil, err
		}
		if t.client < 0 || len(t.random) != 32 || t.version == 0 || t.retrySeen && !t.finalHello {
			t.notePHAGap(dir)
			info["Secret Lookup Status"] = "missing-context"
			f.retainPrecontextFinished(dir, e, info)
			return map[string]any{"fields": info}, nil
		}
		if !tlsSupportedSuite(t.version, t.suite) {
			t.notePHAGap(dir)
			info["Secret Lookup Status"] = "unsupported-suite"
			f.discardPrecontextFinished(dir)
			return map[string]any{"fields": info}, nil
		}
		var err error
		var early bool
		plain, typ, early, err = f.openTLSEarlyRecord(dir, w, info)
		if !early && err == nil && t.keys[dir] == nil {
			ok, err := t.loadKey(dir, f.a.tlsSecrets)
			if err != nil {
				return nil, err
			}
			if !ok {
				t.notePHAGap(dir)
				info["Secret Lookup Status"] = "missing-key"
				// A complete Hello context with an unavailable secret is not
				// precontext recovery. Only continue a queue already started
				// before that context; do not reserve new identities or label
				// continuously captured encrypted traffic as rejected records.
				if len(t.precontext[dir]) != 0 {
					f.retainPrecontextFinished(dir, e, info)
				}
				return map[string]any{"fields": info}, nil
			}
		}
		if !early && err == nil {
			f.recoverPrecontextFinished(dir, info)
		}
		if !early && err == nil && t.recoveryFailures[dir] >= 4 {
			t.notePHAGap(dir)
			info["Secret Lookup Status"] = "recovery-exhausted"
			info["Recovery State"] = "bounded record recovery suspended; encrypted envelope retained"
			return map[string]any{"fields": info}, nil
		}
		if !early && err == nil {
			plain, typ, err = f.openTLSRecord(dir, w, info)
		}
		if err != nil {
			if tlsAuthenticationError(err) && t.earlyEligible(dir) && !f.usableTLSEarlyKey() {
				// An unavailable early secret cannot turn a valid early-key
				// record into a bad-tag assertion made with a handshake key.
				// Keep handshake sequence zero for an independently verifiable
				// later Finished; the missing application prefix stays a gap.
				info["Secret Lookup Status"] = "missing-early-key"
				info["Early Data Decision"] = "accepted by observed server; early secret unavailable"
				if !t.early.accepted {
					info["Early Data Decision"] = "rejected by observed server; early secret unavailable"
				}
				f.markTLSInnerGap(dir)
				return map[string]any{"fields": info}, nil
			}
			if tlsAuthenticationError(err) {
				if t.earlyEligible(dir) {
					// A failed early-key trial cannot consume handshake record
					// zero. The next exact boundary may authenticate either epoch.
					f.noteTLSRecordGap(dir, info, false)
					t.early.gap = true
					if t.early.seq != ^uint64(0) {
						t.early.seq++
					}
				} else {
					f.noteTLSRecordFailure(dir, info)
				}
			}
			return nil, err
		}
		info["Authentication Verified"] = true
		if t.version == 0x304 {
			t.protectedRecordObserved[dir] = true
		}
		if info["Missing Earlier Plaintext"] == true {
			f.markTLSInnerGap(dir)
		}
		info["Secret Lookup Status"] = "found"
		info["Content Visibility"] = "authenticated-plaintext"
		info["Inner Content Type"] = typ
		if early {
			if err := t.admitEarlyRecord(typ, plain, info); err != nil {
				return nil, err
			}
			if typ == 23 && !t.early.accepted {
				info["Authenticated Plaintext"] = bytes.Clone(plain)
				info["Application Decoding"] = "early data rejected by observed server; plaintext not delivered"
				return map[string]any{"fields": info}, nil
			}
		} else if t.earlyEligible(dir) {
			// A handshake-key record can follow a lost EndOfEarlyData, or
			// a rejection which never sends EndOfEarlyData. It does not
			// reconstruct missing early application bytes.
			t.early.ended = true
			if t.early.accepted {
				info["Early Data End"] = "not observed; later key epoch authenticated"
				f.markTLSInnerGap(dir)
			}
		}
	} else {
		info["Content Visibility"] = "cleartext"
	}
	if encrypted && (t.closeNotify[dir] || t.fatalAlert[0] || t.fatalAlert[1]) {
		// RFC 5246 7.2.1 / RFC 8446 6.1: later records do not reopen the
		// sender's application stream. Authenticate them for the record view,
		// but never deliver their bytes or mutate application/handshake state.
		info["TLS Closure"] = "authenticated close_notify already observed; later data ignored"
		if t.fatalAlert[0] || t.fatalAlert[1] {
			info["TLS Closure"] = "authenticated fatal alert already observed; later data ignored"
		}
		return map[string]any{"fields": info}, nil
	}
	if encrypted && typ != 22 && typ != 21 && t.phaClientApplication(dir) && t.phaResponse.stage != tlsPHAResponseNone {
		return nil, protocolError(ErrMalformedMessage, "TLS post-handshake response interleaved")
	}
	// Authentication consumed this old-epoch record, but application bytes
	// cannot interrupt a continuously observed TLS1.3 handshake fragment.
	if t.version == 0x304 && encrypted && typ == 23 && len(t.handshake[dir]) != 0 {
		return nil, protocolError(ErrMalformedMessage, "TLS 1.3 application data interleaves a fragmented handshake")
	}
	if typ == 20 {
		if !bytes.Equal(plain, []byte{1}) {
			return nil, protocolError(ErrMalformedMessage, "invalid TLS ChangeCipherSpec")
		}
		if t.version == 0x304 && t.app[dir] {
			return nil, protocolError(ErrMalformedMessage, "TLS 1.3 ChangeCipherSpec after sender Finished/application epoch")
		}
		// The pending byte belongs to the previous cipher epoch. Without
		// the missing old-epoch byte there is no safe concatenation with a
		// subsequently decrypted alert. Do not invent a normative wire error.
		if t.alertPending[dir] && t.version != 0x304 {
			info["Alert Buffered Bytes"] = 1
			return nil, protocolError(ErrContextRequired, "TLS alert fragment crosses a cipher-state transition")
		}
		if t.version != 0x304 {
			t.ccs[dir] = true
			t.seq[dir] = 0
		}
	} else if typ == 22 {
		if t.handshakeGap[dir] {
			info["Handshake Parsing"] = "unavailable after missing protected handshake bytes"
			return map[string]any{"fields": info}, nil
		}
		if len(t.handshake[dir])+len(plain) > f.a.config.MaxMessageBytes {
			return nil, protocolError(ErrResourceExceeded, "TLS handshake buffer limit")
		}
		if err := f.reserveSession(t.storage() + int64(len(plain))); err != nil {
			return nil, err
		}
		if len(plain) > 0 {
			if len(t.handshake[dir]) == 0 {
				t.handshakeAuthenticated[dir] = encrypted
			} else {
				t.handshakeAuthenticated[dir] = t.handshakeAuthenticated[dir] && encrypted
			}
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
			// A KeyUpdate must finish its record before installing the next
			// epoch. Earlier old-key fragments and preceding messages are valid;
			// complete or partial trailing handshake bytes are not. Inner padding
			// has already been removed by record decryption.
			if t.version == 0x304 && b[0] == 24 && n != len(b) {
				return nil, protocolError(ErrMalformedMessage, "TLS KeyUpdate does not end at the record boundary")
			}
			if t.version == 0x304 && b[0] == 5 && n != len(b) {
				return nil, protocolError(ErrMalformedMessage, "TLS EndOfEarlyData does not end at the record boundary")
			}
			if t.version == 0x304 && b[0] == 20 && n != len(b) {
				return nil, protocolError(ErrMalformedMessage, "TLS Finished does not end at the record boundary")
			}
			if len(messages) >= f.a.budget.MaxCollectionElements {
				return nil, protocolError(ErrResourceExceeded, "TLS handshake count limit")
			}
			// Hello snapshots and retry cookies survive this record. Reserve before copying.
			if b[0] == 1 || b[0] == 2 {
				if err := f.reserveSession(t.storage() + int64(n)); err != nil {
					return nil, err
				}
			}
			trial := *t
			m, err := trial.handshakeMessage(dir, b[:n], t.handshakeAuthenticated[dir], f.a.budget.MaxCollectionElements)
			if err != nil {
				return nil, err
			}
			// Request history is immutable copy-on-write. Refusing its retained
			// capacity cannot publish the request or consume its context. Record
			// authentication and sequence consumption have already happened.
			if trial.storage() > t.storage() {
				if err := f.reserveSession(trial.storage()); err != nil {
					return nil, err
				}
			}
			*t = trial
			messages = append(messages, m)
			t.handshake[dir] = b[n:]
			// Any remainder was contributed by this record; earlier records
			// ended inside the message just consumed.
			t.handshakeAuthenticated[dir] = encrypted
		}
		if len(t.handshake[dir]) == 0 {
			t.handshake[dir] = nil
		}
		info["Handshake Messages"] = messages
		info["Handshake Incomplete"] = len(t.handshake[dir]) > 0
		if t.early.decisionKnown && t.early.decisionPDU == 0 && t.earlyDataOffered {
			if e.ID == 0 {
				e.ID = f.a.ids.Add(1)
			}
			t.early.decisionPDU = e.ID
		}
	} else if typ == 23 && encrypted {
		if f.plaintextGap[dir] {
			info["Authenticated Plaintext"] = bytes.Clone(plain)
			info["Application Decoding"] = "unavailable across prior plaintext gap"
		}
		t.plaintext = bytes.Clone(plain)
		t.content = typ
	} else if typ == 21 {
		closed, err := t.observeAlert(dir, plain, info, f.a.budget.MaxCollectionElements, encrypted)
		if err != nil {
			return nil, err
		}
		if closed {
			t.closeNotify[dir] = true
			info["Authenticated Close Notify"] = true
		}
	}
	info["Negotiated Version"], info["Cipher Suite"], info["SNI"], info["ALPN"] = t.version, t.suite, t.sni, t.alpn
	if err := f.reserveSession(t.storage()); err != nil {
		return nil, err
	}
	if info["Authenticated Close Notify"] == true {
		t.closeNotifyPending[dir] = true
	}
	return map[string]any{"fields": info}, nil
}
func tlsSupportedSuite(version, suite uint16) bool {
	if version == 0x304 {
		return suite == 0x1301 || suite == 0x1302 || suite == 0x1303
	}
	if version != 0x303 {
		return false
	}
	switch suite {
	case 0xc02f, 0xc02b, 0xc030, 0xc02c, 0x009c, 0x009d, 0x009e, 0x009f, 0xcca8, 0xcca9, 0xccaa:
		return true
	}
	return false
}
func (t *binTLS) prfHash() func() hash.Hash {
	if t.suite == 0xc030 || t.suite == 0xc02c || t.suite == 0x009d || t.suite == 0x009f {
		return sha512.New384
	}
	return sha256.New
}

func (t *binTLS) handshakeMessage(dir int, b []byte, authenticated bool, limits ...int) (map[string]any, error) {
	if dir < 0 || dir > 1 || len(b) < 4 || tlsU24(b[1:4]) != len(b)-4 {
		return nil, protocolError(ErrMalformedMessage, "TLS handshake framing")
	}
	if err := t.checkHandshakeVersion(b[0], authenticated); err != nil {
		return nil, err
	}
	// Hello/extension validation and Finished/KeyUpdate transitions commit
	// together. A rejected message cannot retain half-applied peer state.
	// Retained slices and metadata are immutable or replaced, never edited.
	if err := t.checkInitialPSKAuthentication(dir, b[0], authenticated); err != nil {
		return nil, err
	}
	stage, err := t.observedServerHandshakeStage(dir, b[0], authenticated)
	if err != nil {
		return nil, err
	}
	clientStage, err := t.observedClientHandshakeStage(dir, b[0], authenticated)
	if err != nil {
		return nil, err
	}
	trial := *t
	m, err := trial.applyHandshakeMessage(dir, b, authenticated, limits...)
	if err == nil {
		if trial.version >= 0x301 && trial.version <= 0x303 {
			trial.certificateStatus.lastKnown[dir] = true
			trial.certificateStatus.last[dir] = b[0]
		}
		if stage != 0 {
			trial.serverHandshakeStage = stage
		}
		if clientStage != 0 {
			trial.clientHandshakeStage = clientStage
		}
		*t = trial
	}
	return m, err
}

func (t *binTLS) applyHandshakeMessage(dir int, b []byte, authenticated bool, limits ...int) (map[string]any, error) {
	typ, body := b[0], b[4:]
	m := map[string]any{"Type": typ, "Length": len(body), "Authenticated": authenticated}
	if !authenticated && t.version == 0x304 && (t.finalHello || t.retrySeen && typ != 1 && typ != 2) {
		return nil, protocolError(ErrMalformedMessage, "TLS 1.3 unexpected cleartext handshake")
	}
	if err := t.checkPHAHandshakeOrder(dir, typ); err != nil {
		return nil, err
	}
	switch typ {
	case 1, 2:
		h, err := tlsHello(body, typ == 1, limits...)
		if err != nil {
			return nil, err
		}
		if err = tlsHelloExtensionStructure(h, typ == 1); err != nil {
			return nil, err
		}
		if typ == 1 && t.client >= 0 && (!t.retrySeen || t.retryClientSeen) || typ == 2 && t.finalHello {
			return nil, protocolError(ErrUnsupportedFeature, "TLS repeated hello or renegotiation requires a new epoch")
		}
		if err = t.observeResumptionHello(dir, typ, body, h, m, limits...); err != nil {
			return nil, err
		}
		retry, err := t.observeHello(dir, typ, body, limits...)
		if err != nil {
			return nil, err
		}
		if retry {
			m["Hello Retry Request"] = true
		}
		if err := t.observeCertificateStatusHello(h, typ == 1, limits...); err != nil {
			return nil, err
		}
		if typ == 1 && t.retryClientSeen {
			m["Retry ClientHello"] = true
		}
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
			if err := t.extensions(body[p:], typ == 1, limits...); err != nil {
				return nil, err
			}
		}
	case 4:
		ticket, err := t.newSessionTicket(dir, body, authenticated, limits...)
		if err != nil {
			return nil, err
		}
		m["New Session Ticket"] = ticket
	case 5:
		if err := t.admitEndOfEarlyData(dir, b, authenticated); err != nil {
			return nil, err
		}
		t.early.ended = true
		m["End Of Early Data"] = "observed accepted early epoch ended; handshake traffic sequence starts independently"
		m["Encoded End Of Early Data"] = bytes.Clone(b)
	case 8:
		if !authenticated {
			return nil, protocolError(ErrMalformedMessage, "cleartext TLS EncryptedExtensions")
		}
		if err := t.admitEncryptedExtensions(dir, body, limits...); err != nil {
			return nil, err
		}
		maxElements := 4096
		if len(limits) > 0 {
			maxElements = limits[0]
		}
		view, err := DecodeTLSEncryptedExtensions(b, t.version, maxElements)
		if err != nil {
			return nil, err
		}
		if err := t.extensions(body, false, limits...); err != nil {
			return nil, err
		}
		m["Encrypted Extensions"], m["Encrypted Extensions Version"] = view.Fields, t.version
		m["Encoded Encrypted Extensions"] = bytes.Clone(b)
		// Commit only after the complete EE vector and its context pass.
		t.early.decisionKnown = true
		t.early.accepted = false
		for rest := body[2:]; len(rest) > 0; {
			kind, n := binary.BigEndian.Uint16(rest), int(binary.BigEndian.Uint16(rest[2:]))
			t.early.accepted = t.early.accepted || kind == 42
			rest = rest[4+n:]
		}
	case 12:
		view, status, err := t.admitServerKeyExchange(dir, b, limits...)
		if err != nil {
			return nil, err
		}
		m["Server Key Exchange Status"] = status
		if view != nil {
			m["Server Key Exchange"], m["Encoded Server Key Exchange"] = view.Fields, bytes.Clone(b)
			m["Server Key Exchange Version"], m["Server Key Exchange Cipher Suite"] = t.version, t.suite
		}
	case 16:
		view, status, err := t.admitClientKeyExchange(dir, b, authenticated)
		if err != nil {
			return nil, err
		}
		m["Client Key Exchange Status"] = status
		if view != nil {
			m["Client Key Exchange"], m["Encoded Client Key Exchange"] = view.Fields, bytes.Clone(b)
			m["Client Key Exchange Version"], m["Client Key Exchange Cipher Suite"] = t.version, t.suite
			m["Client Key Exchange Profile"] = view.Profile
		}
	case 15:
		if t.version < 0x301 || t.version > 0x304 {
			m["Certificate Verify Status"] = "negotiated version unavailable or unsupported; body uninterpreted"
			break
		}
		if t.version == 0x304 && !authenticated {
			return nil, protocolError(ErrMalformedMessage, "cleartext TLS 1.3 CertificateVerify")
		}
		if t.client >= 0 && dir != t.client && (t.version < 0x304 || t.app[dir]) {
			return nil, protocolError(ErrMalformedMessage, "server sent client-only TLS CertificateVerify")
		}
		verify, err := DecodeTLSCertificateVerify(b, t.version)
		if err != nil {
			return nil, err
		}
		if algorithm, ok := verify.Fields["Signature Algorithm"].(uint16); ok && tlsGREASE16(algorithm) {
			return nil, protocolError(ErrMalformedMessage, fmt.Sprintf("TLS CertificateVerify selects reserved GREASE signature algorithm 0x%04x", algorithm))
		}
		phase := "handshake"
		if t.version == 0x304 && t.app[dir] {
			phase = "application"
		}
		m["Certificate Verify"], m["Certificate Verify Version"] = verify.Fields, t.version
		m["Certificate Verify Phase"], m["Encoded Certificate Verify"] = phase, bytes.Clone(b)
		m["Certificate Verify Status"] = "body vector decoded; signature not evaluated; certificate trust and handshake context not evaluated"
		association, err := t.admitPHACertificateVerify(dir)
		if err != nil {
			return nil, err
		}
		if association != nil {
			m["Post Handshake Response"] = association
			if association["Context Association"] == "matched observed CertificateRequest" {
				m["Certificate Verify Status"] = "body vector decoded; observed response order matched; signature, transcript and trust not evaluated"
			}
		}
	case 20:
		if t.version != 0x303 && t.version != 0x304 {
			m["Finished Status"] = "negotiated version unavailable or unsupported; body uninterpreted"
			break
		}
		if !authenticated {
			return nil, protocolError(ErrMalformedMessage, "unauthenticated TLS Finished")
		}
		verifyData, err := DecodeTLSFinished(b, t.version, t.suite)
		if err != nil {
			return nil, err
		}
		phase := "handshake"
		if t.version == 0x304 && t.app[dir] {
			// Post-handshake client authentication uses application traffic
			// keys, including its Finished. Only the initial Finished changes
			// from handshake to application keys. Do not reset this sequence.
			if t.client >= 0 && dir != t.client {
				return nil, protocolError(ErrMalformedMessage, "server sent application-key TLS Finished")
			}
			phase = "application"
		}
		m["Finished Phase"], m["Finished Version"], m["Finished Cipher Suite"] = phase, t.version, t.suite
		m["Finished Verify Data"], m["Encoded Finished"] = verifyData, bytes.Clone(b)
		m["Finished Verification Status"] = "AEAD authenticated; transcript MAC not evaluated"
		if phase == "application" {
			m["Finished Context Status"] = "application-key handshake; PHA exchange context not validated"
			association, err := t.admitPHAFinished(dir)
			if err != nil {
				return nil, err
			}
			if association != nil {
				m["Post Handshake Response"] = association
				if association["Context Association"] == "matched observed CertificateRequest" {
					m["Finished Context Status"] = "observed response context and order matched; transcript MAC not evaluated"
				}
			}
		}
		if t.version == 0x304 && !t.app[dir] {
			t.phaInitialFinished[dir] = true
			t.app[dir] = true
			t.seq[dir] = 0
			t.keys[dir] = nil
			t.iv[dir] = nil
			t.secret[dir] = nil
		}
	case 22:
		view, status, err := t.admitCertificateStatus(dir, b, authenticated)
		if err != nil {
			return nil, err
		}
		m["Certificate Status Validation"] = status
		if view != nil {
			m["Certificate Status"], m["Encoded Certificate Status"] = view.Fields, bytes.Clone(b)
			m["Certificate Status Version"] = t.version
		}
	case 24:
		if t.version != 0x304 || !authenticated || !t.app[dir] || len(body) != 1 || body[0] > 1 {
			return nil, protocolError(ErrMalformedMessage, "invalid TLS KeyUpdate")
		}
		next, err := quicHKDFExpandLabel(t.secret[dir], "traffic upd", len(t.secret[dir]))
		if err != nil {
			return nil, err
		}
		if err = t.install13(dir, next); err != nil {
			return nil, err
		}
		t.seq[dir] = 0
		m["Key Update Request"] = body[0]
		m["Key Update Request Name"] = []string{"update_not_requested", "update_requested"}[body[0]]
		// These bytes belong to authenticated handshake plaintext, not the
		// outer record ciphertext. Keep them owned and free of wire ranges.
		m["Encoded Key Update"] = bytes.Clone(b)
	case 11:
		// Certificate bytes are evidence, not a trust assertion. The existing
		// legacy summaries remain compatible; TLS 1.3 also retains the complete
		// owned vector for its request context, entry and extension ranges.
		sum := sha256.Sum256(body)
		m["Certificate Message SHA256"] = hex.EncodeToString(sum[:])
		if t.version == 0x304 {
			limit := 4096
			if len(limits) > 0 {
				limit = limits[0]
			}
			view, err := DecodeTLSCertificate(b, t.version, limit)
			if err != nil {
				return nil, err
			}
			if err := tlsCertificateGREASE(view); err != nil {
				return nil, err
			}
			// The grammar API is role-neutral: clients may send an empty
			// certificate list, including a post-handshake request context.
			// Enforce these server-only rules only after the connection has
			// established which direction sent ClientHello (RFC 8446 4.4.2).
			if t.client >= 0 && dir != t.client {
				// Post-handshake authentication is a client response. An
				// authenticated application key does not permit the server
				// to send another Certificate (RFC 8446 4.6.2).
				if t.app[dir] {
					return nil, protocolError(ErrMalformedMessage, "server TLS Certificate in application epoch")
				}
				if view.Fields["Context Length"].(byte) != 0 {
					return nil, protocolError(ErrMalformedMessage, "server TLS Certificate has nonempty request context")
				}
				if len(view.Certificates) == 0 {
					return nil, protocolError(ErrMalformedMessage, "server TLS Certificate has empty certificate list")
				}
			} else if t.client >= 0 && !t.app[dir] && view.Fields["Context Length"].(byte) != 0 {
				return nil, protocolError(ErrMalformedMessage, "initial client TLS Certificate context must be empty")
			}
			m["Certificates"], m["Certificate Body"] = view.Certificates, view.Fields
			m["Certificate Version"], m["Encoded Certificate"] = t.version, bytes.Clone(b)
			m["Certificate Body Status"] = "body vectors decoded; request context, certificate type and trust not evaluated"
			association, err := t.admitPHACertificate(dir, view.Fields["Context"].([]byte), len(view.Certificates) == 0, authenticated, limit)
			if err != nil {
				return nil, err
			}
			if association != nil {
				m["Post Handshake Response"] = association
				if association["Context Association"] == "matched observed CertificateRequest" {
					m["Certificate Body Status"] = "body vectors decoded; observed request context matched; certificate type and trust not evaluated"
				}
			}
			if authenticated && t.client >= 0 && dir == t.client && !t.app[dir] {
				t.clientCertificateEmpty = len(view.Certificates) == 0
			}
		} else {
			certs, err := tlsCertificates(body, false, limits...)
			if err != nil {
				return nil, err
			}
			m["Certificates"] = certs
		}
	case 13:
		if t.version == 0x304 && !authenticated {
			return nil, protocolError(ErrMalformedMessage, "cleartext TLS 1.3 CertificateRequest")
		}
		if t.client >= 0 && dir == t.client {
			return nil, protocolError(ErrMalformedMessage, "client sent TLS CertificateRequest")
		}
		if t.version == 0 {
			// The record's legacy version is not a negotiated version. Preserve
			// the observed envelope without selecting a speculative body grammar.
			m["Certificate Request Status"] = "negotiated version unavailable; body uninterpreted"
			break
		}
		limit := 4096
		if len(limits) > 0 {
			limit = limits[0]
		}
		request, err := DecodeTLSCertificateRequest(b, t.version, limit)
		if err != nil {
			return nil, err
		}
		// Initial authentication uses the empty request context. Application
		// key recovery is authenticated before this dispatch, so a missing
		// Finished cannot turn a post-handshake context into an initial one.
		if t.version == 0x304 && t.client >= 0 && !t.app[dir] && request.Fields["Context Length"].(byte) != 0 {
			return nil, protocolError(ErrMalformedMessage, "initial TLS CertificateRequest context must be empty")
		}
		admission, err := t.admitCertificateRequest(dir, request, limit)
		if err != nil {
			return nil, err
		}
		if admission != nil {
			m["Certificate Request Admission"] = admission
		}
		m["Certificate Request"], m["Certificate Request Version"] = request.Fields, t.version
		m["Encoded Certificate Request"] = bytes.Clone(b)
		m["Certificate Request Status"] = "body vectors decoded; handshake conformance not established"
	default:
		if t.phaClientApplication(dir) {
			// An unsupported message can replace an authentication message,
			// for example CompressedCertificate (RFC 8879). Its context is
			// opaque here; do not infer a missing Certificate from a later CV.
			t.notePHAGap(dir)
			m["PHA Response Context"] = "unavailable after uninterpreted post-handshake message"
		}
	}
	return m, nil
}
func (t *binTLS) extensions(b []byte, client bool, limits ...int) error {
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
				name, err := tlsSNIName(v)
				if err != nil {
					return err
				}
				t.sni = string(name)
			}
		case 16:
			selected, err := tlsALPNName(v, !client, limits...)
			if err != nil {
				return err
			}
			if !client {
				t.alpn = string(selected)
			}
		case 49:
			if !client || len(v) != 0 {
				return protocolError(ErrMalformedMessage, "TLS post_handshake_auth must be an empty ClientHello extension")
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
		size := 32
		if t.suite == 0x1302 {
			size = 48
		}
		if len(s) != size {
			return false, protocolError(ErrContextRequired, "TLS traffic secret length disagrees with cipher suite")
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
	keyLen, ivLen := 16, 4
	if t.suite == 0xc030 || t.suite == 0xc02c || t.suite == 0x009d || t.suite == 0x009f {
		keyLen = 32
	}
	if t.suite == 0xcca8 || t.suite == 0xcca9 || t.suite == 0xccaa {
		keyLen, ivLen = 32, 12
	}
	block := tlsPRFHash(master, seed, 2*(keyLen+ivLen), t.prfHash())
	i := 0
	if dir != t.client {
		i = 1
	}
	var err error
	if ivLen == 12 {
		t.keys[dir], err = chacha20poly1305.New(block[i*keyLen : (i+1)*keyLen])
	} else {
		c, e := aes.NewCipher(block[i*keyLen : (i+1)*keyLen])
		if e != nil {
			return false, e
		}
		t.keys[dir], err = cipher.NewGCM(c)
	}
	t.iv[dir] = bytes.Clone(block[2*keyLen+i*ivLen : 2*keyLen+(i+1)*ivLen])
	return err == nil, err
}
func tlsPRF(secret, seed []byte, n int) []byte {
	return tlsPRFHash(secret, seed, n, sha256.New)
}
func tlsPRFHash(secret, seed []byte, n int, hashFn func() hash.Hash) []byte {
	mac := func(data []byte) []byte { h := hmac.New(hashFn, secret); h.Write(data); return h.Sum(nil) }
	a := mac(seed)
	out := make([]byte, 0, n+32)
	for len(out) < n {
		out = append(out, mac(append(bytes.Clone(a), seed...))...)
		a = mac(a)
	}
	return out[:n]
}
func (t *binTLS) install13(dir int, secret []byte) error {
	keyLen := 16
	if t.suite == 0x1302 || t.suite == 0x1303 {
		keyLen = 32
	}
	key, err := quicHKDFExpandLabel(secret, "key", keyLen)
	if err != nil {
		return err
	}
	iv, err := quicHKDFExpandLabel(secret, "iv", 12)
	if err != nil {
		return err
	}
	var a cipher.AEAD
	if t.suite == 0x1303 {
		a, err = chacha20poly1305.New(key)
	} else {
		c, e := aes.NewCipher(key)
		if e != nil {
			return e
		}
		a, err = cipher.NewGCM(c)
	}
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
	// Direct and recovery callers must retain the same negotiated size gate.
	if err := t.checkCiphertextEnvelope(w); err != nil {
		return nil, 0, err
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
		explicit := 8
		if t.suite == 0xcca8 || t.suite == 0xcca9 || t.suite == 0xccaa {
			explicit = 0
		}
		if len(payload) < explicit+16 {
			return nil, 0, protocolError(ErrMalformedMessage, "TLS GCM record length")
		}
		if explicit == 8 {
			nonce = append(nonce, payload[:8]...)
		} else {
			for i := 0; i < 8; i++ {
				nonce[11-i] ^= byte(t.seq[dir] >> uint(i*8))
			}
		}
		payload = payload[explicit:]
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
	if t == nil {
		return
	}
	if t.closeNotifyPending[dir] {
		t.closeNotifyPending[dir] = false
		if t.child != nil && !f.plaintextGap[dir] && !t.child.plaintextGap[dir] && !t.child.directions[dir].stopped {
			t.child.finishHTTPCloseDelimited(dir, &tlsHTTPBoundary{pdu: parent.ID, direction: dir, version: t.version})
		}
	}
	if t.plaintext == nil {
		return
	}
	plain := t.plaintext
	t.plaintext = nil
	if t.child == nil {
		t.child = &binFlow{a: f.a, id: f.id, endpoints: f.endpoints, ports: f.ports, domain: f.domain, transportFlow: f.transportFlow, byteSource: "decrypted", captureTCP: true, tcpClientKnown: f.tcpClientKnown, tcpClientDir: f.tcpClientDir}
		t.child.plaintextGap = f.plaintextGap
		for d, gap := range f.plaintextGap {
			if gap {
				t.child.directions[d].stopped, t.child.directions[d].tlsPlaintextGap = true, true
			}
		}
		if f.ftpData != nil {
			t.child.protocol, t.child.ftpData = "ftp-data", f.ftpData
		}
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
		// STARTTLS resets application negotiation. Preserve the observed inner
		// protocol, but do not carry pre-TLS authentication/capability state.
		switch {
		case f.smtp != nil && f.smtp.encrypted:
			t.child.protocol, t.child.smtp = "smtp", &binSMTP{client: f.smtp.client, maxPending: f.a.budget.MaxCollectionElements}
			f.smtp = nil
		case f.imap != nil && f.imap.encrypted:
			t.child.protocol, t.child.imap = "imap", &binIMAP{pending: map[string]string{}, maxPending: f.a.budget.MaxCollectionElements}
			f.imap = nil
		case f.pop3 != nil && f.pop3.encrypted:
			t.child.protocol, t.child.pop3 = "pop3", &binPOP3{}
			f.pop3 = nil
		case f.ftp != nil && f.ftp.encrypted:
			t.child.protocol, t.child.ftp = "ftp", &binFTP{}
			f.ftp = nil
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

func tlsCertificates(b []byte, tls13 bool, limits ...int) ([]map[string]any, error) {
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
	max := 64
	if len(limits) > 0 {
		max = min(max, limits[0])
	}
	// Admission precedes hashing, X.509 parsing and result allocation. Preserve
	// the existing hard64 ceiling and format/error behavior for callers without
	// an explicit configured collection limit. Validate the same complete vector
	// before the configured limit, preserving malformed-vector error priority.
	// This bounds certificate entries,
	// not a newly invented count of fixed ASN.1 structure fields.
	count, extensions := 0, 0
	for scan := b; len(scan) > 0; {
		if count >= 64 {
			return nil, protocolError(ErrResourceExceeded, "TLS certificate count")
		}
		if len(scan) < 3 {
			return nil, bad()
		}
		n := tlsU24(scan)
		scan = scan[3:]
		if n == 0 || n > len(scan) {
			return nil, bad()
		}
		scan = scan[n:]
		if tls13 {
			if len(scan) < 2 {
				return nil, bad()
			}
			n = int(binary.BigEndian.Uint16(scan))
			scan = scan[2:]
			if n > len(scan) {
				return nil, bad()
			}
			// RFC 8446 4.4.2 uses complete Extension entries, not an opaque
			// byte string. Check each certificate's block independently before
			// hashing/parsing any certificate or admitting its collection.
			elements, err := tlsCertificateEntryExtensions(scan[:n])
			if err != nil {
				return nil, err
			}
			extensions += elements
			scan = scan[n:]
		}
		count++
	}

	if count > max {
		return nil, protocolError(ErrResourceExceeded, "TLS certificate count")
	}
	if len(limits) > 0 && count+extensions > limits[0] {
		return nil, protocolError(ErrResourceExceeded, "TLS certificate extension collection limit")
	}
	var out []map[string]any
	for len(b) > 0 {
		if len(out) >= max {
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

// The fixed bitset bounds duplicate tracking without allocating an attacker-
// sized map. Reusing a type in another CertificateEntry is allowed. Extension
// payloads remain uninterpreted; this does not verify offers, OCSP, SCT or trust.
func tlsCertificateEntryExtensions(b []byte) (int, error) {
	var seen [1024]uint64
	count := 0
	for len(b) > 0 {
		if len(b) < 4 {
			return 0, protocolError(ErrMalformedMessage, "TLS certificate extension header")
		}
		typ, n := binary.BigEndian.Uint16(b), int(binary.BigEndian.Uint16(b[2:]))
		b = b[4:]
		bit := uint64(1) << (typ % 64)
		if n > len(b) || seen[typ/64]&bit != 0 {
			return 0, protocolError(ErrMalformedMessage, "TLS certificate extension length or duplicate type")
		}
		seen[typ/64] |= bit
		count++
		b = b[n:]
	}
	return count, nil
}
