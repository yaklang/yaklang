package pcaputil

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/chacha20poly1305"
)

func quicConformancePacket(t *testing.T, key *quicTrafficKeys, kind string, pn uint64, reserved byte, plain []byte) []byte {
	t.Helper()
	header := []byte{0x43 | reserved}
	if kind != "1-RTT" {
		typ := map[string]byte{"Initial": 0, "0-RTT": 1, "Handshake": 2}[kind]
		if key.version == quicVersion2 {
			typ = (typ + 1) & 3
		}
		header = binary.BigEndian.AppendUint32([]byte{0xc3 | typ<<4 | reserved}, key.version)
		header = append(header, 8)
		header = append(header, []byte("seedcid8")...)
		header = append(header, 0)
		if kind == "Initial" {
			header = append(header, 0)
		}
		header = append(header, quicPutVarint(uint64(4+len(plain)+16))...)
	}
	pnAt := len(header)
	header = binary.BigEndian.AppendUint32(header, uint32(pn))
	var aead cipher.AEAD
	var err error
	if key.aead == quicAEADChaCha20Poly1305 {
		aead, err = chacha20poly1305.New(key.key)
	} else {
		block, e := aes.NewCipher(key.key)
		require.NoError(t, e)
		aead, err = cipher.NewGCM(block)
	}
	require.NoError(t, err)
	wire := append(bytes.Clone(header), aead.Seal(nil, quicNonce(key.iv, pn), plain, header)...)
	mask, err := quicHeaderMask(key, wire[pnAt+4:pnAt+20])
	require.NoError(t, err)
	bits := byte(31)
	if kind != "1-RTT" {
		bits = 15
	}
	wire[0] ^= mask[0] & bits
	for i := 0; i < 4; i++ {
		wire[pnAt+i] ^= mask[1+i]
	}
	return wire
}

func quicConformanceContext(t *testing.T, version uint32, algorithm string) (*binQUIC, *quicTrafficKeys) {
	t.Helper()
	n := 32
	if algorithm == quicAEADAES256GCM {
		n = 48
	}
	key, err := quicDeriveTrafficVersion(bytes.Repeat([]byte{7}, n), algorithm, version)
	require.NoError(t, err)
	initial, err := quicInitialKeysVersion([]byte("seedcid8"), version)
	require.NoError(t, err)
	q := &binQUIC{native: true, version: version, state: "application", initialDCID: []byte("seedcid8"), zeroCID: [2]bool{true, true}, keys: &quicKeyring{version: version, initial: initial}}
	for d := 0; d < 2; d++ {
		q.keys.app[d][0], q.keys.handshake[d], q.keys.early[d] = key, key, key
		next, err := quicNextPhase(key)
		require.NoError(t, err)
		q.keys.app[d][1] = next
	}
	return q, key
}

func FuzzQUICNativeFrameRoleAndKeyPhaseIsolation(f *testing.F) {
	f.Add([]byte{0xff, 0, 0xc3}, uint8(0), uint32(1), true)
	f.Add([]byte("token"), uint8(11), uint32(9), false)
	f.Fuzz(func(t *testing.T, token []byte, flags uint8, sequence uint32, v2 bool) {
		if len(token) > 128 {
			return
		}
		version := quicVersion1
		if v2 {
			version = quicVersion2
		}
		algorithm := []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305}[(flags>>4)%3]
		q, key := quicConformanceContext(t, version, algorithm)
		direction, bad := 0, true
		plain := []byte{0x1e}
		switch flags % 5 {
		case 1:
			if len(token) == 0 {
				token = []byte{0xff}
			}
			plain = append(append([]byte{7}, quicPutVarint(uint64(len(token)))...), token...)
		case 2:
			direction, plain = 1, []byte{7, 0}
		case 3:
			direction, bad = 1, false
			if len(token) == 0 {
				token = []byte{0xff}
			}
			plain = append(append([]byte{7}, quicPutVarint(uint64(len(token)))...), token...)
		case 4:
			plain, bad = []byte{1}, false
		}
		phase := int(flags >> 3 & 1)
		if phase != 0 {
			key = q.keys.app[direction][1]
		}
		pn := uint64(sequence%(1<<24)) + 1
		wire := quicConformancePacket(t, key, "1-RTT", pn, byte(phase<<2), plain)
		info, err := q.consume(direction, wire, 128)
		require.Equal(t, true, info["Authentication Verified"])
		if bad {
			require.Error(t, err)
			_, typed := classifySessionError(err)
			require.Equal(t, ErrMalformedMessage, typed.Kind)
			require.Equal(t, 0, q.keyPhase[direction], "refused frames cannot commit a candidate key update")
			require.Empty(t, q.streams)
			require.NotContains(t, info, "Frames")
			_, err = q.consume(direction, wire, 128)
			require.Error(t, err)
			require.Len(t, q.spaces[direction][quicSpaceApplication].seen, 1)
			good := quicConformancePacket(t, key, "1-RTT", pn+1, byte(phase<<2), []byte{1})
			_, err = q.consume(direction, good, 128)
			require.NoError(t, err)
		} else {
			require.NoError(t, err, "allowed server NEW_TOKEN and PING controls remain accepted")
		}
		require.Equal(t, phase, q.keyPhase[direction])
	})
}

func TestQUICNativeProtectedFrameRoleAndLevelConformance(t *testing.T) {
	cases := []struct {
		name, kind string
		direction  int
		reserved   byte
		plain      []byte
		want       ProtocolErrorKind
	}{
		{"client-handshake-done", "1-RTT", 0, 0, []byte{0x1e}, ErrMalformedMessage},
		{"client-new-token", "1-RTT", 0, 0, []byte{7, 1, 'x'}, ErrMalformedMessage},
		{"server-empty-new-token", "1-RTT", 1, 0, []byte{7, 0}, ErrMalformedMessage},
		{"early-ack", "0-RTT", 0, 0, quicAckFrame(0), ErrMalformedMessage},
		{"early-crypto", "0-RTT", 0, 0, []byte{6, 0, 0}, ErrMalformedMessage},
		{"early-new-token", "0-RTT", 0, 0, []byte{7, 1, 'x'}, ErrMalformedMessage},
		{"early-handshake-done", "0-RTT", 0, 0, []byte{0x1e}, ErrMalformedMessage},
		{"early-path-response", "0-RTT", 0, 0, append([]byte{0x1b}, make([]byte, 8)...), ErrMalformedMessage},
		{"early-retire-cid", "0-RTT", 0, 0, []byte{0x19, 0}, ErrMalformedMessage},
		{"server-early", "0-RTT", 1, 0, []byte{1}, ErrMalformedMessage},
		{"Initial-stream", "Initial", 0, 0, []byte{0x0a, 0, 0}, ErrMalformedMessage},
		{"Handshake-stream", "Handshake", 1, 0, []byte{0x0a, 0, 0}, ErrMalformedMessage},
		{"empty-packet", "1-RTT", 0, 0, nil, ErrMalformedMessage},
		{"short-reserved-bits", "1-RTT", 0, 8, []byte{1}, ErrMalformedMessage},
		{"nonminimal-ping", "1-RTT", 0, 0, []byte{0x40, 1}, ErrMalformedMessage},
		{"unselected-extension", "1-RTT", 0, 0, []byte{0x1f}, ErrUnsupportedFeature},
		{"wide-unselected-extension", "1-RTT", 0, 0, []byte{0x40, 0x40}, ErrUnsupportedFeature},
	}
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, algorithm := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			for _, tc := range cases {
				t.Run(fmt.Sprintf("v%x/%s/%s", version, algorithm, tc.name), func(t *testing.T) {
					q, key := quicConformanceContext(t, version, algorithm)
					if tc.kind == "Initial" {
						key = q.keys.initial[tc.direction]
					}
					wire := quicConformancePacket(t, key, tc.kind, 1, tc.reserved, tc.plain)
					info, err := q.consume(tc.direction, wire, 128)
					require.Error(t, err)
					_, typed := classifySessionError(err)
					require.NotNil(t, typed)
					require.Equal(t, tc.want, typed.Kind)
					require.Equal(t, true, info["Authentication Verified"], "packet AEAD success and protocol validity are distinct")
					require.Equal(t, false, info["Peer Identity Verified"])
					require.NotContains(t, info, "Frames", "invalid packet cannot leak delivered application frames")
					require.True(t, bytes.Equal(tc.plain, info["Rejected Packet Payload"].([]byte)), "refused authenticated octets remain inspectable without delivery")
					require.Contains(t, info["Payload Delivery"], "not delivered")
					require.Empty(t, q.streams)
					require.Empty(t, q.cryptoData[0][0].data)
					require.Equal(t, "application", q.state)
					if tc.want == ErrMalformedMessage {
						for i := 0; i < 140; i++ {
							_, repeated := q.consume(tc.direction, wire, 128)
							require.Error(t, repeated)
						}
						h, err := quicParseHeader(wire, true)
						require.NoError(t, err)
						require.Len(t, q.space(tc.direction, h.space).seen, 1, "duplicates cannot evict the bounded distinct-PN history")
					}
					good := quicConformancePacket(t, key, tc.kind, 2, 0, []byte{1})
					if tc.kind == "0-RTT" && tc.direction == 1 {
						good = quicConformancePacket(t, key, "1-RTT", 2, 0, []byte{1})
					}
					_, err = q.consume(tc.direction, good, 128)
					require.NoError(t, err, "a later valid packet in the observed generation remains analyzable")
				})
			}
		}
	}
}

func TestQUICFrameResourceLimitsRemainLimitedAndPaddingCounts(t *testing.T) {
	_, err := quicParseFrames([]byte{1, 0}, 1)
	_, typed := classifySessionError(err)
	require.NotNil(t, typed)
	require.Equal(t, ErrResourceExceeded, typed.Kind)
	frames, err := quicParseFrames(bytes.Repeat([]byte{0}, 100), 1)
	require.NoError(t, err)
	require.Len(t, frames, 1, "a contiguous padding run is one compact observation")
	q, key := quicConformanceContext(t, quicVersion2, quicAEADAES128GCM)
	wire := quicConformancePacket(t, key, "1-RTT", 1, 0, []byte{1, 0})
	info, err := q.consume(0, wire, 1)
	status, typed := classifySessionError(err)
	require.Equal(t, "limited", status)
	require.Equal(t, ErrResourceExceeded, typed.Kind)
	require.Equal(t, true, info["Authentication Verified"])
	require.False(t, q.spaces[0][quicSpaceApplication].init, "resource refusal does not consume delivery or duplicate state")
	_, err = q.consume(0, wire, 2)
	require.NoError(t, err, "more analysis budget allows the same bytes to be examined")
	oversized := append([]byte{0xd0, 0x6b, 0x33, 0x43, 0xcf, 0, 0}, quicPutVarint(20481)...)
	_, err = quicParseHeader(oversized, true)
	status, typed = classifySessionError(err)
	require.Equal(t, "limited", status)
	require.Equal(t, ErrResourceExceeded, typed.Kind)
}
