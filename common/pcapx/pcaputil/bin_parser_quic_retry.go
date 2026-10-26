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
	key := []byte{0xbe, 0x0c, 0x69, 0x0b, 0x9f, 0x66, 0x57, 0x5a, 0x1d, 0x76, 0x6b, 0x54, 0xe3, 0x68, 0xc8, 0x4e}
	nonce := []byte{0x46, 0x15, 0x99, 0xd3, 0x5d, 0x63, 0x2b, 0xf2, 0x23, 0x98, 0x25, 0xbb}
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)
	pseudo := make([]byte, 1+len(originalDCID)+len(packetWithoutTag))
	pseudo[0] = byte(len(originalDCID))
	copy(pseudo[1:], originalDCID)
	copy(pseudo[1+len(originalDCID):], packetWithoutTag)
	return aead.Seal(nil, nonce, nil, pseudo)
}

func (q *binQUIC) consumeRetry(dir int, raw []byte, h quicHdr, info map[string]any, max int) (map[string]any, error) {
	if len(q.originalDCID) == 0 {
		return info, protocolError(ErrUnsupportedFeature, "QUIC Retry requires original-DCID integrity context")
	}
	info["Peer Identity Verified"] = false
	if dir != 1 || !bytes.Equal(h.dcid, q.initialSCID) || len(h.scid) == 0 || bytes.Equal(h.scid, q.originalDCID) || len(raw)-h.payloadOff <= 16 {
		return info, protocolError(ErrMalformedMessage, "QUIC Retry endpoint/CID/token does not match the observed Initial")
	}
	if q.retrySeen || q.spaces[1][quicSpaceInitial].init {
		return info, protocolError(ErrContextRequired, "QUIC Retry is duplicate or follows a server Initial")
	}
	if !bytes.Equal(quicRetryTag(q.originalDCID, raw[:len(raw)-16]), raw[len(raw)-16:]) {
		return info, protocolError(ErrAuthenticationFailed, "QUIC Retry integrity tag failed")
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
