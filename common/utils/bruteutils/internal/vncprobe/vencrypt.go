package vncprobe

import (
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
)

// veNCrypt is RFB security type 19, VeNCrypt 0.2:
// version exchange, U32 subtype list, U8 accept, then TLS, then nested
// None (TLSNone/X509None) or VNC-Auth (TLSVnc/X509Vnc).
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
	chosen := pickVeNCrypt(types, unauth)
	if chosen == 0 {
		return 0, fmt.Errorf("%w: vencrypt subtypes %v", ErrNoCompatibleAuth, types)
	}
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
	case veTLSNone, veX509None:
		return secNone, nil
	case veTLSVnc, veX509Vnc:
		if unauth {
			return 0, ErrNoCompatibleAuth
		}
		return secVNCAuth, s.vncAuth(password)
	default:
		return 0, ErrNoCompatibleAuth
	}
}

func pickVeNCrypt(types []uint32, unauth bool) uint32 {
	has := func(want uint32) bool {
		for _, t := range types {
			if t == want {
				return true
			}
		}
		return false
	}
	if unauth {
		switch {
		case has(veTLSNone):
			return veTLSNone
		case has(veX509None):
			return veX509None
		default:
			return 0
		}
	}
	switch {
	case has(veTLSVnc):
		return veTLSVnc
	case has(veX509Vnc):
		return veX509Vnc
	case has(veTLSNone):
		return veTLSNone
	case has(veX509None):
		return veX509None
	default:
		return 0
	}
}

func (s *session) wrapTLS() error {
	cfg := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS10,
	}
	c := tls.Client(s.conn, cfg)
	if err := c.Handshake(); err != nil {
		return fmt.Errorf("%w: tls: %v", ErrTransient, err)
	}
	s.conn = c
	return nil
}
