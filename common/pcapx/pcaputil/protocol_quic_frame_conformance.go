package pcaputil

import (
	"crypto/sha256"
	"encoding/hex"
)

func quicRetainUndeliveredPayload(info map[string]any, plain []byte) {
	info["Rejected Packet Payload"] = append([]byte{}, plain...)
	sum := sha256.Sum256(plain)
	info["Rejected Packet Payload SHA256"] = hex.EncodeToString(sum[:])
	info["Payload Delivery"] = "not delivered to protocol state; packet AEAD verified"
}

// These are observed wire-role and packet-type constraints, not complete TLS,
// transport-parameter, stream-state or endpoint conformance certification.
func quicValidateNativePacketFrames(h quicHdr, dir int, frames []map[string]any) error {
	if len(frames) == 0 {
		return protocolError(ErrMalformedMessage, "QUIC protected packet contains no frames")
	}
	if h.typeName == "0-RTT" && dir != 0 {
		return protocolError(ErrMalformedMessage, "QUIC 0-RTT packet cannot be sent by the observed server")
	}
	for _, frame := range frames {
		name, _ := frame["Frame Type"].(string)
		if h.typeName == "0-RTT" {
			switch name {
			case "ACK", "CRYPTO", "NEW_TOKEN", "PATH_RESPONSE", "HANDSHAKE_DONE", "RETIRE_CONNECTION_ID":
				return protocolError(ErrMalformedMessage, "QUIC %s frame forbidden in 0-RTT", name)
			}
		}
		if (name == "NEW_TOKEN" || name == "HANDSHAKE_DONE") && dir == 0 {
			return protocolError(ErrMalformedMessage, "QUIC %s frame cannot be sent by the observed client", name)
		}
		if name == "NEW_TOKEN" {
			if token, _ := frame["Token"].([]byte); len(token) == 0 {
				return protocolError(ErrMalformedMessage, "QUIC NEW_TOKEN frame has an empty token")
			}
		}
	}
	return nil
}

// Remember authenticated protocol violations in the existing bounded PN window.
// Refused frames cannot later become clean duplicates or advance stream/TLS/CID
// state. Resource and unsupported-profile declines remain eligible for reanalysis.
func (q *binQUIC) rejectAuthenticatedQUICPacket(dir, space int, pn uint64, info map[string]any, plain []byte, err error) (map[string]any, error) {
	if q.native {
		quicRetainUndeliveredPayload(info, plain)
		if _, typed := classifySessionError(err); typed != nil && typed.Kind == ErrMalformedMessage {
			state := q.space(dir, space)
			if quicSeen(state, pn) {
				info["Duplicate"] = true
				if refused, ok := state.rejected[pn]; ok {
					return info, &refused
				}
				return info, err
			}
			q.observePN(state, pn, info)
			if state.rejected == nil {
				state.rejected = map[uint64]ProtocolError{}
			}
			state.rejected[pn] = *typed
		}
	}
	return info, err
}
