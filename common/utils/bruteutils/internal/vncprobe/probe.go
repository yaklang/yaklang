// Package vncprobe is a login-only RFB client (RFC 6143).
//
// It negotiates ProtocolVersion and security, then stops at SecurityResult
// (or the RFB 3.3 None shortcut). It never sends ClientInit, never reads
// ServerInit, and has no framebuffer / encoding / message loop.
package vncprobe

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	secNone     = 1
	secVNCAuth  = 2
	secTight    = 16
	secVeNCrypt = 19

	veTLSNone  uint32 = 257
	veTLSVnc   uint32 = 258
	veX509None uint32 = 260
	veX509Vnc  uint32 = 261

	secResultOK      uint32 = 0
	secResultFailed  uint32 = 1
	secResultTooMany uint32 = 2

	maxReason   = 4096
	maxSecTypes = 32
	maxTight    = 64
	versionLen  = 12

	vendorSTDV  = "STDV"
	vendorTGHT  = "TGHT"
	sigNoTunnel = "NOTUNNEL"
	sigNoAuth   = "NOAUTH__"
	sigVNCAuth  = "VNCAUTH_"
)

// MaxTimeout bounds the entire login exchange.
const MaxTimeout = 20 * time.Second

// Dialer must honor cancellation and deadlines supplied through its context.
type Dialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

type Options struct {
	Address, Password string
	// Timeout defaults to 10 seconds and is capped at MaxTimeout.
	Timeout time.Duration
	// UnauthOnly selects only None (or Tight wrapping None). Password
	// authentication is not attempted, so a mixed None+VNC-Auth server
	// is reported as unauthenticated access rather than a password hit.
	UnauthOnly bool
}

// Result is one login-only RFB exchange. Err is nil on success.
type Result struct {
	// SecurityType is the innermost type actually used (1 None, 2 VNC-Auth).
	SecurityType uint8
	// OuterType is the RFB security type selected (1, 2, 16, 19).
	OuterType uint8
	// Subtype is the VeNCrypt subtype actually selected, if any.
	Subtype     uint32
	AuthNone    bool
	Locked      bool
	TLSVerified bool
	Extra       string
	Err         error
}

func (r Result) OK() bool { return r.Err == nil }

func (r *Result) fillExtra() {
	auth := "vnc"
	if r.AuthNone {
		auth = "none"
	}
	r.Extra = fmt.Sprintf("auth=%s outer=%d", auth, r.OuterType)
	if r.Subtype != 0 {
		r.Extra += fmt.Sprintf(" subtype=%d", r.Subtype)
	}
	if r.OuterType == secVeNCrypt || r.Subtype != 0 {
		r.Extra += fmt.Sprintf(" verified=%v", r.TLSVerified)
	}
}

var (
	ErrAuthFailed        = errors.New("vnc: authentication failed")
	ErrLocked            = errors.New("vnc: too many authentication failures")
	ErrTransient         = errors.New("vnc: transient handshake failure")
	ErrProtocolMismatch  = errors.New("vnc: protocol mismatch")
	ErrNoCompatibleAuth  = errors.New("vnc: no compatible security type")
	ErrUnsupportedServer = errors.New("vnc: unsupported protocol version")
	ErrUnsupportedTLS    = errors.New("vnc: tls backend does not support this vencrypt subtype")
)

func isProtocolMismatch(err error) bool {
	return errors.Is(err, ErrProtocolMismatch) || errors.Is(err, ErrNoCompatibleAuth) ||
		errors.Is(err, ErrUnsupportedServer) || errors.Is(err, ErrUnsupportedTLS)
}

type capability struct {
	code      int32
	vendor    string
	signature string
}

func (c capability) match(code int32, vendor, sig string) bool {
	return c.code == code && c.vendor == vendor && c.signature == sig
}

type session struct {
	conn                  net.Conn
	major, minor          uint
	sawRFB                bool
	requireSecurityResult bool
	veSubtype             uint32
	tlsVerified           bool
}

// Probe returns a structured login outcome. The connection is always closed
// before return. Wrong password is ErrAuthFailed; lockout is ErrLocked;
// a non-RFB peer is ErrProtocolMismatch / ErrNoCompatibleAuth; I/O after a
// valid RFB banner is ErrTransient (or the context error).
func Probe(ctx context.Context, dialer Dialer, o Options) (r Result) {
	if ctx == nil {
		ctx = context.Background()
	}
	if dialer == nil {
		dialer = &net.Dialer{}
	}
	if o.Timeout <= 0 {
		o.Timeout = 10 * time.Second
	}
	if o.Timeout > MaxTimeout {
		o.Timeout = MaxTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	if o.Address == "" {
		r.Err = ErrProtocolMismatch
		return r
	}

	skip := map[uint8]bool{}
	var last error
	for i := 0; i < int(maxSecTypes)+1; i++ {
		if err := ctx.Err(); err != nil {
			if last == nil {
				last = err
			}
			break
		}
		conn, err := dialer.DialContext(ctx, "tcp", o.Address)
		if err != nil {
			r.Err = err
			return r
		}
		if d, ok := ctx.Deadline(); ok {
			_ = conn.SetDeadline(d)
		}
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		s := &session{conn: conn}
		res, retry, chosen := s.oneAttempt(o, skip)
		stop()
		_ = conn.Close()
		r = res
		if !retry {
			if ctx.Err() != nil && r.Err != nil && !classifiedLoginErr(r.Err) {
				r.Err = ctx.Err()
			}
			r.fillExtra()
			return r
		}
		if chosen != 0 {
			skip[chosen] = true
		}
		last = r.Err
	}
	if r.Err == nil {
		r.Err = last
	}
	if r.Err == nil {
		r.Err = ErrNoCompatibleAuth
	}
	r.fillExtra()
	return r
}

func (s *session) oneAttempt(o Options, skip map[uint8]bool) (r Result, retry bool, chosen uint8) {
	if err := s.handshakeVersion(); err != nil {
		r.Err = err
		return r, false, 0
	}
	types, err := s.readSecurityList()
	if err != nil {
		r.Err = err
		return r, false, 0
	}
	chosen, err = s.chooseSecurity(types, o.UnauthOnly, skip)
	if err != nil {
		r.Err = err
		return r, false, 0
	}
	r.OuterType = chosen
	used, err := s.doSecurity(chosen, o.Password, o.UnauthOnly)
	if err != nil {
		r.Err = err
		r.Subtype = s.veSubtype
		if skipCandidate(err) {
			nextSkip := map[uint8]bool{}
			for k, v := range skip {
				nextSkip[k] = v
			}
			nextSkip[chosen] = true
			if pickSecurity(types, o.UnauthOnly, nextSkip) != 0 {
				return r, true, chosen
			}
		}
		return r, false, chosen
	}
	r.SecurityType = used
	r.Subtype = s.veSubtype
	r.TLSVerified = s.tlsVerified
	r.AuthNone = used == secNone
	r.Err = s.securityResult(used == secNone)
	if errors.Is(r.Err, ErrLocked) {
		r.Locked = true
	}
	return r, false, chosen
}

func skipCandidate(err error) bool {
	return errors.Is(err, ErrNoCompatibleAuth) || errors.Is(err, ErrUnsupportedTLS)
}

func classifiedLoginErr(err error) bool {
	return errors.Is(err, ErrAuthFailed) || errors.Is(err, ErrLocked) ||
		errors.Is(err, ErrTransient) || isProtocolMismatch(err)
}

func (s *session) ioErr(op string, err error) error {
	if err == nil {
		return nil
	}
	kind := ErrProtocolMismatch
	if s.sawRFB {
		kind = ErrTransient
	}
	return fmt.Errorf("%w: %s: %v", kind, op, err)
}

func looksLikeRFBPrefix(b []byte) bool {
	const p = "RFB "
	if len(b) == 0 {
		return false
	}
	if len(b) >= len(p) {
		return bytes.HasPrefix(b, []byte(p))
	}
	return bytes.Equal(b, []byte(p[:len(b)]))
}

func isEOF(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

func (s *session) handshakeVersion() error {
	var buf [versionLen]byte
	n, err := io.ReadFull(s.conn, buf[:])
	if err != nil {
		got := buf[:n]
		// Timeout/cancel after an RFB-looking prefix is transient: more
		// passwords may still work. A peer that closes after a short
		// write (truncated banner) stays protocol-mismatch.
		if looksLikeRFBPrefix(got) && !isEOF(err) {
			return fmt.Errorf("%w: version: %v", ErrTransient, err)
		}
		return fmt.Errorf("%w: version: %v", ErrProtocolMismatch, err)
	}
	var major, minor uint
	parsed, err := fmt.Sscanf(string(buf[:]), "RFB %d.%d\n", &major, &minor)
	if parsed != 2 || err != nil || major < 3 {
		return fmt.Errorf("%w: %q", ErrUnsupportedServer, buf)
	}
	if minor < 3 {
		return fmt.Errorf("%w: minor %d", ErrUnsupportedServer, minor)
	}
	s.sawRFB = true
	reply := "RFB 003.003\n"
	s.major, s.minor = 3, 3
	switch {
	case minor >= 8:
		reply = "RFB 003.008\n"
		s.minor = 8
	case minor == 7:
		reply = "RFB 003.007\n"
		s.minor = 7
	}
	if _, err := s.conn.Write([]byte(reply)); err != nil {
		return s.ioErr("version reply", err)
	}
	return nil
}

func (s *session) readSecurityList() ([]byte, error) {
	if s.minor < 7 {
		var raw uint32
		if err := binary.Read(s.conn, binary.BigEndian, &raw); err != nil {
			return nil, s.ioErr("security type", err)
		}
		if raw == 0 {
			_, _ = s.readReason()
			return nil, ErrProtocolMismatch
		}
		if raw > 255 {
			return nil, ErrNoCompatibleAuth
		}
		return []byte{uint8(raw)}, nil
	}
	var n uint8
	if err := binary.Read(s.conn, binary.BigEndian, &n); err != nil {
		return nil, s.ioErr("security count", err)
	}
	if n == 0 {
		_, _ = s.readReason()
		return nil, ErrProtocolMismatch
	}
	if n > maxSecTypes {
		return nil, ErrProtocolMismatch
	}
	types := make([]byte, n)
	if _, err := io.ReadFull(s.conn, types); err != nil {
		return nil, s.ioErr("security types", err)
	}
	return types, nil
}

func (s *session) chooseSecurity(types []byte, unauth bool, skip map[uint8]bool) (uint8, error) {
	chosen := pickSecurity(types, unauth, skip)
	if chosen == 0 {
		return 0, fmt.Errorf("%w: server types %v", ErrNoCompatibleAuth, types)
	}
	if s.minor < 7 {
		if types[0] != chosen {
			return 0, fmt.Errorf("%w: server type %d", ErrNoCompatibleAuth, types[0])
		}
		return chosen, nil
	}
	if err := binary.Write(s.conn, binary.BigEndian, chosen); err != nil {
		return 0, s.ioErr("security select", err)
	}
	return chosen, nil
}

func pickSecurity(types []byte, unauth bool, skip map[uint8]bool) uint8 {
	has := func(want byte) bool {
		if skip[want] {
			return false
		}
		for _, t := range types {
			if t == want {
				return true
			}
		}
		return false
	}
	if unauth {
		switch {
		case has(secNone):
			return secNone
		case has(secVeNCrypt):
			return secVeNCrypt
		case has(secTight):
			return secTight
		default:
			return 0
		}
	}
	switch {
	case has(secVNCAuth):
		return secVNCAuth
	case has(secVeNCrypt):
		return secVeNCrypt
	case has(secTight):
		return secTight
	case has(secNone):
		return secNone
	default:
		return 0
	}
}

func (s *session) doSecurity(sec uint8, password string, unauth bool) (uint8, error) {
	switch sec {
	case secNone:
		return secNone, nil
	case secVNCAuth:
		if unauth {
			return 0, ErrNoCompatibleAuth
		}
		return secVNCAuth, s.vncAuth(password)
	case secTight:
		return s.tightAuth(password, unauth)
	case secVeNCrypt:
		return s.veNCrypt(password, unauth)
	default:
		return 0, ErrNoCompatibleAuth
	}
}

func (s *session) vncAuth(password string) error {
	challenge := make([]byte, 16)
	if _, err := io.ReadFull(s.conn, challenge); err != nil {
		return s.ioErr("challenge", err)
	}
	resp, err := vncAuthResponse(password, challenge)
	if err != nil {
		return err
	}
	if _, err := s.conn.Write(resp); err != nil {
		return s.ioErr("challenge response", err)
	}
	return nil
}

func (s *session) tightAuth(password string, unauth bool) (uint8, error) {
	nTunnels, err := s.readU32()
	if err != nil {
		return 0, s.ioErr("tight tunnels", err)
	}
	if nTunnels > maxTight {
		return 0, ErrProtocolMismatch
	}
	if nTunnels > 0 {
		tunnels := make([]capability, 0, nTunnels)
		for i := uint32(0); i < nTunnels; i++ {
			cap, err := s.readCapability()
			if err != nil {
				return 0, s.ioErr("tight tunnel cap", err)
			}
			tunnels = append(tunnels, cap)
		}
		if !hasCapability(tunnels, 0, vendorTGHT, sigNoTunnel) {
			return 0, fmt.Errorf("%w: tight: NOTUNNEL not advertised", ErrNoCompatibleAuth)
		}
		if err := s.writeU32(0); err != nil {
			return 0, s.ioErr("tight notunnel", err)
		}
	}
	nAuth, err := s.readU32()
	if err != nil {
		return 0, s.ioErr("tight auth", err)
	}
	if nAuth > maxTight {
		return 0, ErrProtocolMismatch
	}
	if nAuth == 0 {
		return secNone, nil
	}
	auths := make([]capability, 0, nAuth)
	for i := uint32(0); i < nAuth; i++ {
		cap, err := s.readCapability()
		if err != nil {
			return 0, s.ioErr("tight auth cap", err)
		}
		auths = append(auths, cap)
	}
	chosen := pickTightAuth(auths, unauth)
	if chosen == 0 {
		return 0, ErrNoCompatibleAuth
	}
	if err := s.writeU32(uint32(chosen)); err != nil {
		return 0, s.ioErr("tight auth select", err)
	}
	return s.doSecurity(chosen, password, unauth)
}

func pickTightAuth(auths []capability, unauth bool) uint8 {
	has := func(code int32, vendor, sig string) bool {
		return hasCapability(auths, code, vendor, sig)
	}
	if unauth {
		if has(secNone, vendorSTDV, sigNoAuth) {
			return secNone
		}
		return 0
	}
	if has(secVNCAuth, vendorSTDV, sigVNCAuth) {
		return secVNCAuth
	}
	if has(secNone, vendorSTDV, sigNoAuth) {
		return secNone
	}
	return 0
}

func hasCapability(caps []capability, code int32, vendor, sig string) bool {
	for _, c := range caps {
		if c.match(code, vendor, sig) {
			return true
		}
	}
	return false
}

func (s *session) readCapability() (capability, error) {
	var buf [16]byte
	if _, err := io.ReadFull(s.conn, buf[:]); err != nil {
		return capability{}, err
	}
	return capability{
		code:      int32(binary.BigEndian.Uint32(buf[0:4])),
		vendor:    string(buf[4:8]),
		signature: string(buf[8:16]),
	}, nil
}

func (s *session) securityResult(noneAuth bool) error {
	// Bare RFB None on 3.3/3.7 omits SecurityResult. VeNCrypt None
	// subtypes always send it, including on 3.7.
	if noneAuth && s.minor < 8 && !s.requireSecurityResult {
		return nil
	}
	var result uint32
	if err := binary.Read(s.conn, binary.BigEndian, &result); err != nil {
		return s.ioErr("security result", err)
	}
	if result == secResultOK {
		return nil
	}
	reason := ""
	if s.minor >= 8 {
		reason, _ = s.readReason()
	}
	if result == secResultTooMany || lockoutReason(reason) {
		return ErrLocked
	}
	return ErrAuthFailed
}

func lockoutReason(reason string) bool {
	s := strings.ToLower(reason)
	if s == "" {
		return false
	}
	// Generic "Authentication failed" / "Security failure" is ordinary
	// VNC-Auth reject, not a lockout. Only too-many / locked phrasing.
	return strings.Contains(s, "too many") ||
		strings.Contains(s, "temporarily locked") ||
		strings.Contains(s, "account locked") ||
		strings.Contains(s, "locked out")
}

func (s *session) readReason() (string, error) {
	var n uint32
	if err := binary.Read(s.conn, binary.BigEndian, &n); err != nil {
		return "", err
	}
	if n > maxReason {
		n = maxReason
	}
	buf := make([]byte, n)
	_, err := io.ReadFull(s.conn, buf)
	return string(buf), err
}

func (s *session) readU32() (uint32, error) {
	var v uint32
	err := binary.Read(s.conn, binary.BigEndian, &v)
	return v, err
}

func (s *session) writeU32(v uint32) error {
	return binary.Write(s.conn, binary.BigEndian, v)
}
