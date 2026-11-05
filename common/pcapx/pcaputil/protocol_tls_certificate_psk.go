package pcaputil

import (
	"bytes"
	"encoding/binary"
)

// RFC 8773 sections 4 and 5.1: these are observable wire declarations, not
// evidence of an external PSK, a verified binder or a trusted certificate.
// The same allocation-free check runs before collection admission and before
// a hello can change the connection's private authentication mode.
type tlsCertificatePSKHelloExtensions struct {
	present, early              bool
	psk, share, modes, versions []byte
}

func (v tlsCertificatePSKHelloExtensions) check(client bool) error {
	if !v.present {
		return nil
	}
	bad := func() error {
		return protocolError(ErrMalformedMessage, "TLS certificate-with-external-PSK hello companions")
	}
	if v.psk == nil || v.share == nil || v.early {
		return bad()
	}
	if client {
		if err := tlsPSKClientRequirements(v.versions, v.modes); err != nil {
			return err
		}
		if bytes.IndexByte(v.modes[1:], 1) < 0 {
			return bad()
		}
		if len(v.share) < 2 || int(binary.BigEndian.Uint16(v.share)) != len(v.share)-2 {
			return bad()
		}
		// An empty client key_share vector legitimately asks for HRR. Reject
		// malformed/duplicate entries without allocating a group collection.
		var groups [1024]uint64
		for rest := v.share[2:]; len(rest) > 0; {
			if len(rest) < 4 {
				return bad()
			}
			group, n := binary.BigEndian.Uint16(rest), int(binary.BigEndian.Uint16(rest[2:]))
			rest = rest[4:]
			bit := uint64(1) << (group % 64)
			if n == 0 || n > len(rest) || groups[group/64]&bit != 0 {
				return bad()
			}
			groups[group/64] |= bit
			rest = rest[n:]
		}
		return nil
	}
	if !bytes.Equal(v.versions, []byte{3, 4}) || len(v.share) < 5 || int(binary.BigEndian.Uint16(v.share[2:])) != len(v.share)-4 {
		return bad()
	}
	return nil
}
