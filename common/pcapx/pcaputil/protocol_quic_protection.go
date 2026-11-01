package pcaputil

import (
	"bytes"
	"crypto/sha256"
)

// Prepare packet protection without changing a live generation. The ownership
// probe and frame-delivery path must agree on Retry keys, token and CID context.
func (q *binQUIC) protectionCandidate(dir int, h quicHdr) (binQUIC, bool, error) {
	candidate := *q
	if h.long {
		if q.version != 0 && h.version != q.version {
			return candidate, false, protocolError(ErrContextRequired, "QUIC version changed from 0x%08x to 0x%08x; compatible negotiation is not selected", q.version, h.version)
		}
		candidate.version = h.version
	} else {
		candidate.version = q.effectiveVersion()
	}
	candidate.keys = cloneQUICKeyring(q.keys)
	oldInitial := false
	if q.native && q.retrySeen && h.typeName == "Initial" && dir == 1 && !bytes.Equal(candidate.initialDCID, q.retrySCID) {
		candidate.initialDCID = bytes.Clone(q.retrySCID)
		if candidate.keys != nil {
			candidate.keys.initial = [2]*quicTrafficKeys{}
		}
	}
	if q.native && q.retrySeen && h.typeName == "Initial" && dir == 0 {
		if !bytes.Equal(h.scid, q.initialSCID) {
			return candidate, false, protocolError(ErrMalformedMessage, "QUIC client Initial changed source CID after Retry")
		}
		switch {
		case bytes.Equal(h.dcid, q.retrySCID):
			if len(h.token) != q.retryTokenLen || sha256.Sum256(h.token) != q.retryTokenHash {
				return candidate, false, protocolError(ErrContextRequired, "QUIC Initial does not carry the observed Retry token")
			}
			if !bytes.Equal(candidate.initialDCID, h.dcid) {
				candidate.initialDCID = bytes.Clone(h.dcid)
				if candidate.keys != nil {
					candidate.keys.initial = [2]*quicTrafficKeys{}
				}
			}
		case bytes.Equal(h.dcid, q.originalDCID):
			oldInitial = true
			candidate.initialDCID = bytes.Clone(q.originalDCID)
			if candidate.keys != nil {
				candidate.keys.initial = [2]*quicTrafficKeys{}
			}
		case q.spaces[1][quicSpaceInitial].init:
			owner, observed := q.cids[string(h.dcid)]
			observedServerCID := observed && owner == 1 || len(h.dcid) == 0 && q.zeroCID[1]
			if !observedServerCID || len(h.token) != q.retryTokenLen || sha256.Sum256(h.token) != q.retryTokenHash {
				return candidate, false, protocolError(ErrContextRequired, "QUIC Initial does not match the post-Retry server CID/token")
			}
		default:
			return candidate, false, protocolError(ErrContextRequired, "QUIC Initial DCID differs from observed Retry context")
		}
	}
	if h.space == quicSpaceInitial && dir == 0 && len(candidate.initialDCID) == 0 {
		candidate.initialDCID = bytes.Clone(h.dcid)
	}
	return candidate, oldInitial, nil
}
