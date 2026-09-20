package vncprobe

import (
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// veNCrypt is RFB security type 19, VeNCrypt 0.2:
// version exchange, U32 subtype list, U8 accept, then TLS, then nested
// None (X509None) or VNC-Auth (X509Vnc). Anonymous TLSNone/TLSVnc are
// not implemented by crypto/tls.
func (s *session) veNCrypt(password string, unauth bool) (uint8, error) {
	var ver [2]byte
	if _, err := io.ReadFull(s.conn, ver[:]); err != nil {
		return 0, s.ioErr("vencrypt version", err)
	}
	if ver[0] != 0 || ver[1] < 2 {
		return 0, fmt.Errorf("%w: vencrypt %d.%d", ErrUnsupportedServer, ver[0], ver[1])
	}
	if _, err := s.conn.Write([]byte{0, 2}); err != nil {
		return 0, s.ioErr("vencrypt version reply", err)
	}
	var ack uint8
	if err := binary.Read(s.conn, binary.BigEndian, &ack); err != nil {
		return 0, s.ioErr("vencrypt version ack", err)
	}
	if ack != 0 {
		return 0, fmt.Errorf("%w: vencrypt version rejected", ErrNoCompatibleAuth)
	}
	var n uint8
	if err := binary.Read(s.conn, binary.BigEndian, &n); err != nil {
		return 0, s.ioErr("vencrypt count", err)
	}
	if n == 0 || n > maxSecTypes {
		return 0, ErrNoCompatibleAuth
	}
	types := make([]uint32, n)
	for i := range types {
		if err := binary.Read(s.conn, binary.BigEndian, &types[i]); err != nil {
			return 0, s.ioErr("vencrypt types", err)
		}
	}
	chosen, err := pickVeNCrypt(types, unauth)
	if err != nil {
		return 0, err
	}
	s.veSubtype = chosen
	if err := s.writeU32(chosen); err != nil {
		return 0, s.ioErr("vencrypt select", err)
	}
	var subAck uint8
	if err := binary.Read(s.conn, binary.BigEndian, &subAck); err != nil {
		return 0, s.ioErr("vencrypt ack", err)
	}
	if subAck != 1 {
		return 0, ErrNoCompatibleAuth
	}
	if err := s.wrapTLS(); err != nil {
		return 0, err
	}
	switch chosen {
	case veX509None:
		s.requireSecurityResult = true
		return secNone, nil
	case veX509Vnc:
		if unauth {
			return 0, ErrNoCompatibleAuth
		}
		return secVNCAuth, s.vncAuth(password)
	default:
		return 0, ErrNoCompatibleAuth
	}
}

func pickVeNCrypt(types []uint32, unauth bool) (uint32, error) {
	has := func(want uint32) bool {
		for _, t := range types {
			if t == want {
				return true
			}
		}
		return false
	}
	if unauth {
		if has(veX509None) {
			return veX509None, nil
		}
	} else {
		if has(veX509Vnc) {
			return veX509Vnc, nil
		}
		if has(veX509None) {
			return veX509None, nil
		}
	}
	if has(veTLSNone) || has(veTLSVnc) {
		return 0, fmt.Errorf("%w: anonymous TLS (subtypes %v)", ErrUnsupportedTLS, types)
	}
	return 0, fmt.Errorf("%w: vencrypt subtypes %v", ErrNoCompatibleAuth, types)
}

func (s *session) wrapTLS() error {
	cfg := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS10,
	}
	c := tls.Client(s.conn, cfg)
	if err := c.Handshake(); err != nil {
		return classifyTLS(err)
	}
	s.conn = c
	s.tlsVerified = false
	return nil
}

func classifyTLS(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "cipher suite") || strings.Contains(msg, "no certificates"):
		return fmt.Errorf("%w: %w", ErrUnsupportedTLS, err)
	default:
		return fmt.Errorf("%w: %w", ErrTransient, err)
	}
}
