// Package vncprobe is a login-only RFB client (RFC 6143).
//
// It negotiates ProtocolVersion and security, then stops at SecurityResult
// (or the RFB 3.3 None shortcut). It never sends ClientInit, never reads
// ServerInit, and has no framebuffer / encoding / message loop.
package vncprobe

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	secNone    = 1
	secVNCAuth = 2
	secTight   = 16

	maxReason   = 4096
	maxSecTypes = 32
	maxTight    = 64
	versionLen  = 12
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
}

var (
	ErrAuthFailed        = errors.New("vnc: authentication failed")
	ErrProtocolMismatch  = errors.New("vnc: protocol mismatch")
	ErrNoCompatibleAuth  = errors.New("vnc: no compatible security type")
	ErrUnsupportedServer = errors.New("vnc: unsupported protocol version")
)

func isProtocolMismatch(err error) bool {
	return errors.Is(err, ErrProtocolMismatch) || errors.Is(err, ErrNoCompatibleAuth) || errors.Is(err, ErrUnsupportedServer)
}

type session struct {
	conn         net.Conn
	major, minor uint
}

// Probe returns nil only after the server accepts the password (or None auth).
// The connection is always closed before return. A failed password is
// ErrAuthFailed; a non-RFB peer is ErrProtocolMismatch / ErrNoCompatibleAuth.
func Probe(ctx context.Context, dialer Dialer, o Options) (err error) {
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
		return ErrProtocolMismatch
	}

	conn, err := dialer.DialContext(ctx, "tcp", o.Address)
	if err != nil {
		return err
	}
	defer conn.Close()
	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	defer func() {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}()

	s := &session{conn: conn}
	if err := s.handshakeVersion(); err != nil {
		return err
	}
	sec, err := s.negotiateSecurity()
	if err != nil {
		return err
	}
	used, err := s.doSecurity(sec, o.Password)
	if err != nil {
		return err
	}
	return s.securityResult(used == secNone)
}

func (s *session) handshakeVersion() error {
	var buf [versionLen]byte
	if _, err := io.ReadFull(s.conn, buf[:]); err != nil {
		return fmt.Errorf("%w: version: %v", ErrProtocolMismatch, err)
	}
	var major, minor uint
	n, err := fmt.Sscanf(string(buf[:]), "RFB %d.%d\n", &major, &minor)
	if n != 2 || err != nil || major < 3 {
		return fmt.Errorf("%w: %q", ErrUnsupportedServer, buf)
	}
	if minor < 3 {
		return fmt.Errorf("%w: minor %d", ErrUnsupportedServer, minor)
	}
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
		return err
	}
	return nil
}

func (s *session) negotiateSecurity() (uint8, error) {
	if s.minor < 7 {
		var raw uint32
		if err := binary.Read(s.conn, binary.BigEndian, &raw); err != nil {
			return 0, fmt.Errorf("%w: security type: %v", ErrProtocolMismatch, err)
		}
		if raw == 0 {
			_ = s.readReason()
			return 0, ErrProtocolMismatch
		}
		if raw > 255 {
			return 0, ErrNoCompatibleAuth
		}
		return uint8(raw), nil
	}

	var n uint8
	if err := binary.Read(s.conn, binary.BigEndian, &n); err != nil {
		return 0, fmt.Errorf("%w: security count: %v", ErrProtocolMismatch, err)
	}
	if n == 0 {
		_ = s.readReason()
		return 0, ErrProtocolMismatch
	}
	if n > maxSecTypes {
		return 0, ErrProtocolMismatch
	}
	types := make([]byte, n)
	if _, err := io.ReadFull(s.conn, types); err != nil {
		return 0, fmt.Errorf("%w: security types: %v", ErrProtocolMismatch, err)
	}
	chosen := pickSecurity(types)
	if chosen == 0 {
		return 0, fmt.Errorf("%w: server types %v", ErrNoCompatibleAuth, types)
	}
	if err := binary.Write(s.conn, binary.BigEndian, chosen); err != nil {
		return 0, err
	}
	return chosen, nil
}

func pickSecurity(types []byte) uint8 {
	has := func(want byte) bool {
		for _, t := range types {
			if t == want {
				return true
			}
		}
		return false
	}
	switch {
	case has(secVNCAuth):
		return secVNCAuth
	case has(secTight):
		return secTight
	case has(secNone):
		return secNone
	default:
		return 0
	}
}

func (s *session) doSecurity(sec uint8, password string) (uint8, error) {
	switch sec {
	case secNone:
		return secNone, nil
	case secVNCAuth:
		return secVNCAuth, s.vncAuth(password)
	case secTight:
		return s.tightAuth(password)
	default:
		return 0, ErrNoCompatibleAuth
	}
}

func (s *session) vncAuth(password string) error {
	challenge := make([]byte, 16)
	if _, err := io.ReadFull(s.conn, challenge); err != nil {
		return fmt.Errorf("%w: challenge: %v", ErrProtocolMismatch, err)
	}
	resp, err := vncAuthResponse(password, challenge)
	if err != nil {
		return err
	}
	_, err = s.conn.Write(resp)
	return err
}

func (s *session) tightAuth(password string) (uint8, error) {
	nTunnels, err := s.readU32()
	if err != nil {
		return 0, fmt.Errorf("%w: tight tunnels: %v", ErrProtocolMismatch, err)
	}
	if nTunnels > maxTight {
		return 0, ErrProtocolMismatch
	}
	if nTunnels > 0 {
		if err := s.skip(int(nTunnels) * 16); err != nil {
			return 0, err
		}
		if err := s.writeU32(0); err != nil { // NOTUNNEL
			return 0, err
		}
	}
	nAuth, err := s.readU32()
	if err != nil {
		return 0, fmt.Errorf("%w: tight auth: %v", ErrProtocolMismatch, err)
	}
	if nAuth > maxTight {
		return 0, ErrProtocolMismatch
	}
	if nAuth == 0 {
		return secNone, nil // Tight default: None
	}
	codes := make([]uint32, 0, nAuth)
	for i := uint32(0); i < nAuth; i++ {
		code, err := s.readU32()
		if err != nil {
			return 0, err
		}
		if err := s.skip(12); err != nil {
			return 0, err
		}
		codes = append(codes, code)
	}
	chosen := pickTightAuth(codes)
	if chosen == 0 {
		return 0, ErrNoCompatibleAuth
	}
	if err := s.writeU32(chosen); err != nil {
		return 0, err
	}
	return s.doSecurity(uint8(chosen), password)
}

func pickTightAuth(codes []uint32) uint32 {
	has := func(want uint32) bool {
		for _, c := range codes {
			if c == want {
				return true
			}
		}
		return false
	}
	switch {
	case has(secVNCAuth):
		return secVNCAuth
	case has(secNone):
		return secNone
	default:
		return 0
	}
}

func (s *session) securityResult(noneAuth bool) error {
	// RFB < 3.8 omits SecurityResult after None.
	if noneAuth && s.minor < 8 {
		return nil
	}
	var result uint32
	if err := binary.Read(s.conn, binary.BigEndian, &result); err != nil {
		return fmt.Errorf("%w: security result: %v", ErrProtocolMismatch, err)
	}
	if result == 0 {
		return nil
	}
	if s.minor >= 8 {
		_ = s.readReason()
	}
	return ErrAuthFailed
}

func (s *session) readReason() error {
	var n uint32
	if err := binary.Read(s.conn, binary.BigEndian, &n); err != nil {
		return err
	}
	if n > maxReason {
		n = maxReason
	}
	buf := make([]byte, n)
	_, err := io.ReadFull(s.conn, buf)
	return err
}

func (s *session) readU32() (uint32, error) {
	var v uint32
	err := binary.Read(s.conn, binary.BigEndian, &v)
	return v, err
}

func (s *session) writeU32(v uint32) error {
	return binary.Write(s.conn, binary.BigEndian, v)
}

func (s *session) skip(n int) error {
	if n <= 0 {
		return nil
	}
	_, err := io.CopyN(io.Discard, s.conn, int64(n))
	return err
}
