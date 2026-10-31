package pcaputil

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
)

// RFC 9001 section 5.8. These public constants check Retry integrity against
// an observed original DCID; they do not authenticate the server's identity.
func quicRetryTag(originalDCID, packetWithoutTag []byte) []byte {
	return quicRetryTagVersion(originalDCID, packetWithoutTag, quicVersion1)
}

func quicRetryTagVersion(originalDCID, packetWithoutTag []byte, version uint32) []byte {
	key := []byte{0xbe, 0x0c, 0x69, 0x0b, 0x9f, 0x66, 0x57, 0x5a, 0x1d, 0x76, 0x6b, 0x54, 0xe3, 0x68, 0xc8, 0x4e}
	nonce := []byte{0x46, 0x15, 0x99, 0xd3, 0x5d, 0x63, 0x2b, 0xf2, 0x23, 0x98, 0x25, 0xbb}
	if version == quicVersion2 {
		key = []byte{0x8f, 0xb4, 0xb0, 0x1b, 0x56, 0xac, 0x48, 0xe2, 0x60, 0xfb, 0xcb, 0xce, 0xad, 0x7c, 0xcc, 0x92}
		nonce = []byte{0xd8, 0x69, 0x69, 0xbc, 0x2d, 0x7c, 0x6d, 0x99, 0x90, 0xef, 0xb0, 0x4a}
	} else if version != quicVersion1 {
		return nil
	}
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)
	pseudo := make([]byte, 1+len(originalDCID)+len(packetWithoutTag))
	pseudo[0] = byte(len(originalDCID))
	copy(pseudo[1:], originalDCID)
	copy(pseudo[1+len(originalDCID):], packetWithoutTag)
	return aead.Seal(nil, nonce, nil, pseudo)
}

func (q *binQUIC) consumeRetry(dir int, raw []byte, h quicHdr, info map[string]any, max int) (map[string]any, error) {
	info["Peer Identity Verified"] = false
	if err := q.verifyRetryContext(dir, raw, h); err != nil {
		return info, err
	}
	if _, present := q.cids[string(h.scid)]; !present && len(q.cids) >= max {
		return info, protocolError(ErrResourceExceeded, "QUIC Retry CID budget")
	}
	q.retrySeen, q.retrySCID = true, bytes.Clone(h.scid)
	token := raw[h.payloadOff : len(raw)-16]
	q.retryTokenHash, q.retryTokenLen = sha256.Sum256(token), len(token)
	q.cids[string(h.scid)] = 1
	info["Retry Integrity Verified"] = true
	info["Connection State"] = "retry-integrity-verified"
	info["Token Bytes"] = len(token)
	return info, nil
}

// This public integrity check binds a Retry to an observed original CID. It is
// neither packet protection nor peer authentication and changes no live state.
func (q *binQUIC) verifyRetryContext(dir int, raw []byte, h quicHdr) error {
	if len(q.originalDCID) == 0 {
		return protocolError(ErrUnsupportedFeature, "QUIC Retry requires original-DCID integrity context")
	}
	if q.version != 0 && h.version != q.version {
		return protocolError(ErrContextRequired, "QUIC Retry version differs from the observed Initial")
	}
	if dir != 1 || !bytes.Equal(h.dcid, q.initialSCID) || len(h.scid) == 0 || bytes.Equal(h.scid, q.originalDCID) || len(raw)-h.payloadOff <= 16 {
		return protocolError(ErrMalformedMessage, "QUIC Retry endpoint/CID/token does not match the observed Initial")
	}
	if q.retrySeen || q.spaces[1][quicSpaceInitial].init {
		return protocolError(ErrContextRequired, "QUIC Retry is duplicate or follows a server Initial")
	}
	if !bytes.Equal(quicRetryTagVersion(q.originalDCID, raw[:len(raw)-16], h.version), raw[len(raw)-16:]) {
		return protocolError(ErrAuthenticationFailed, "QUIC Retry integrity tag failed")
	}
	return nil
}

func (q *binQUIC) matchesExistingWireContext(dir int, wire []byte) bool {
	if q == nil || q.retired {
		return false
	}
	h, err := q.wireHeader(wire, dir)
	if err != nil || h.vn {
		return false
	}
	if h.retry {
		return q.verifyRetryContext(dir, wire, h) == nil
	}
	return q.authenticatesExistingWire(dir, wire)
}
