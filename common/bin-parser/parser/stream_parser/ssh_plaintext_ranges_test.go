package stream_parser

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func sshPlaintextRangeMap(spans map[string][2]uint64) map[string]any {
	out := make(map[string]any, len(spans))
	for name, span := range spans {
		out[name] = []any{span[0], span[1]}
	}
	return out
}

func TestSSHPlaintextRangesRepeatedOffersAndEmptyLanguages(t *testing.T) {
	payload := append([]byte{20}, bytes.Repeat([]byte{0x58}, 16)...)
	names := []string{"Kex Algorithms", "Host Key Algorithms", "Encryption C2S", "Encryption S2C", "MAC C2S", "MAC S2C", "Compression C2S", "Compression S2C"}
	for range names {
		payload = append(payload, sshPlaintextTestString([]byte("x"))...)
	}
	payload = append(payload, sshPlaintextTestString(nil)...)
	payload = append(payload, sshPlaintextTestString(nil)...)
	payload = append(payload, 255, 1, 2, 3, 4)
	wire := sshPlaintextTestPacket(payload)
	require.Len(t, wire, 80)

	info, err := DecodeSSHInitialPacket(wire, "transport")
	require.NoError(t, err)
	fields := info["Wire Fields"].(map[string]any)
	p := fields["Payload"].(map[string]any)
	expected := map[string]any{
		"Message Number": byte(20), "Cookie": bytes.Repeat([]byte{0x58}, 16),
		"Languages C2S Length": uint32(0), "Languages C2S": "",
		"Languages S2C Length": uint32(0), "Languages S2C": "",
		"First Kex Packet Follows": byte(255), "Reserved": uint32(0x01020304),
	}
	spans := map[string][2]uint64{
		"Message Number": {5, 6}, "Cookie": {6, 22},
		"Languages C2S Length": {62, 66}, "Languages C2S": {66, 66},
		"Languages S2C Length": {66, 70}, "Languages S2C": {70, 70},
		"First Kex Packet Follows": {70, 71}, "Reserved": {71, 75},
	}
	for i, name := range names {
		start := uint64(22 + 5*i)
		expected[name+" Length"], expected[name] = uint32(1), "x"
		spans[name+" Length"], spans[name] = [2]uint64{start, start + 4}, [2]uint64{start + 4, start + 5}
	}
	expected["Wire Field Ranges"] = sshPlaintextRangeMap(spans)
	require.Equal(t, expected, p, "wire values and each repeated occurrence keep their own exact positions")
	require.Equal(t, map[string]any{
		"Packet Length": uint32(76), "Padding Length": byte(5),
		"Payload": expected, "Padding": make([]byte, 5),
		"Wire Field Ranges": sshPlaintextRangeMap(map[string][2]uint64{
			"Packet Length": {0, 4}, "Padding Length": {4, 5}, "Padding": {75, 80},
		}),
	}, fields)
	ranges := p["Wire Field Ranges"].(map[string]any)
	require.NotEqual(t, ranges["Compression C2S"], ranges["Compression S2C"], "equal lists do not share a wire occurrence")
	require.Equal(t, []any{uint64(66), uint64(66)}, ranges["Languages C2S"], "empty strings do not consume the next length field")
	require.NotContains(t, fields["Wire Field Ranges"], "Payload", "a composite is not an extra literal leaf")
}

func sshPlaintextRangeCurveReply() []byte {
	s := sshPlaintextTestString
	host := append(s([]byte("ssh-ed25519")), s(make([]byte, 32))...)
	signature := append(s([]byte("ssh-ed25519")), s(make([]byte, 64))...)
	return sshPlaintextTestPacket([]byte{31}, s(host), s(make([]byte, 32)), s(signature))
}

func TestSSHPlaintextRangesNestedIdenticalBytesKeepOwners(t *testing.T) {
	wire := sshPlaintextRangeCurveReply()
	require.Len(t, wire, 192)
	info, err := DecodeSSHInitialPacket(wire, "ecdh-curve25519")
	require.NoError(t, err)
	fields := info["Wire Fields"].(map[string]any)
	p := fields["Payload"].(map[string]any)
	host, signature := p["Host Key"].(map[string]any), p["Signature"].(map[string]any)
	require.Equal(t, "ssh-ed25519", host["Host Key Format"])
	require.Equal(t, host["Host Key Format"], signature["Signature Format"])
	require.Equal(t, make([]byte, 32), host["Ed25519 Public Key"])
	require.Equal(t, host["Ed25519 Public Key"], p["Server Curve25519 Public Key"])
	require.Equal(t, make([]byte, 64), signature["Ed25519 Signature Bytes"])
	require.Equal(t, sshPlaintextRangeMap(map[string][2]uint64{
		"Packet Length": {0, 4}, "Padding Length": {4, 5}, "Padding": {184, 192},
	}), fields["Wire Field Ranges"])
	require.Equal(t, sshPlaintextRangeMap(map[string][2]uint64{
		"Message Number": {5, 6}, "Host Key Length": {6, 10},
		"Server Curve25519 Public Key Length": {61, 65}, "Server Curve25519 Public Key": {65, 97},
		"Signature Length": {97, 101},
	}), p["Wire Field Ranges"])
	require.Equal(t, sshPlaintextRangeMap(map[string][2]uint64{
		"Host Key Format Length": {10, 14}, "Host Key Format": {14, 25},
		"Ed25519 Public Key Length": {25, 29}, "Ed25519 Public Key": {29, 61},
	}), host["Wire Field Ranges"])
	require.Equal(t, sshPlaintextRangeMap(map[string][2]uint64{
		"Signature Format Length": {101, 105}, "Signature Format": {105, 116},
		"Ed25519 Signature Bytes Length": {116, 120}, "Ed25519 Signature Bytes": {120, 184},
	}), signature["Wire Field Ranges"])
	require.NotContains(t, p["Wire Field Ranges"], "Host Key Format")
	require.NotContains(t, p["Wire Field Ranges"], "Signature Format")

	// Returned field bytes and range maps are owned independently of the input
	// and of a subsequent decode. Repeated values are not aliases of each other.
	host["Ed25519 Public Key"].([]byte)[0] = 1
	require.Equal(t, byte(0), p["Server Curve25519 Public Key"].([]byte)[0])
	host["Wire Field Ranges"].(map[string]any)["Ed25519 Public Key"].([]any)[0] = uint64(0)
	again, err := DecodeSSHInitialPacket(wire, "ecdh-curve25519")
	require.NoError(t, err)
	a := again["Wire Fields"].(map[string]any)["Payload"].(map[string]any)["Host Key"].(map[string]any)
	require.Equal(t, []any{uint64(29), uint64(61)}, a["Wire Field Ranges"].(map[string]any)["Ed25519 Public Key"])
	wire[29] = 2
	require.Equal(t, byte(0), a["Ed25519 Public Key"].([]byte)[0], "input mutation cannot rewrite an already decoded leaf")
}

func TestSSHPlaintextRangesOpaqueSelectedAndUnselectedBodies(t *testing.T) {
	s := sshPlaintextTestString
	format := []byte("same@example.test")
	inner := append(s(format), []byte{0x5a, 0x5a, 0x5a}...)
	wire := sshPlaintextTestPacket([]byte{31}, s(inner), s([]byte{2}), s(inner))
	require.Len(t, wire, 72)
	info, err := DecodeSSHInitialPacket(wire, "dh")
	require.NoError(t, err)
	p := info["Wire Fields"].(map[string]any)["Payload"].(map[string]any)
	host, signature := p["Host Key"].(map[string]any), p["Signature"].(map[string]any)
	require.Equal(t, sshPlaintextRangeMap(map[string][2]uint64{
		"Host Key Format Length": {10, 14}, "Host Key Format": {14, 31}, "Opaque Host Key Data": {31, 34},
	}), host["Wire Field Ranges"])
	require.Equal(t, sshPlaintextRangeMap(map[string][2]uint64{
		"Signature Format Length": {43, 47}, "Signature Format": {47, 64}, "Opaque Signature Data": {64, 67},
	}), signature["Wire Field Ranges"])
	require.Equal(t, []byte{0x5a, 0x5a, 0x5a}, host["Opaque Host Key Data"])
	require.Equal(t, host["Opaque Host Key Data"], signature["Opaque Signature Data"])
	require.Equal(t, false, info["Host Key Fields Decoded"])
	require.Equal(t, false, info["Signature Fields Decoded"])

	// Selecting only the common transport envelope must not promote any inner
	// string, even though these bytes also form a valid selected DH reply.
	generic, err := DecodeSSHInitialPacket(wire, "transport")
	require.NoError(t, err)
	g := generic["Wire Fields"].(map[string]any)["Payload"].(map[string]any)
	require.Equal(t, sshPlaintextRangeMap(map[string][2]uint64{
		"Message Number": {5, 6}, "Opaque Message Data": {6, 67},
	}), g["Wire Field Ranges"])
	require.Equal(t, wire[6:67], g["Opaque Message Data"])
	require.NotContains(t, g, "Host Key")
	require.NotContains(t, g, "Signature")

	for _, code := range []byte{21, 99} {
		info, err := DecodeSSHInitialPacket(sshPlaintextTestPacket([]byte{code}), "transport")
		require.NoError(t, err)
		p := info["Wire Fields"].(map[string]any)["Payload"].(map[string]any)
		expected := map[string][2]uint64{"Message Number": {5, 6}}
		if code == 99 {
			expected["Opaque Message Data"] = [2]uint64{6, 6}
			require.Empty(t, p["Opaque Message Data"])
		}
		require.Equal(t, sshPlaintextRangeMap(expected), p["Wire Field Ranges"])
	}
}

func TestSSHPlaintextRangesSelectedFailurePublishesNoSuccessfulPrefix(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]byte)
	}{
		{"host-exceeds-payload", func(w []byte) { binary.BigEndian.PutUint32(w[6:10], uint32(len(w))) }},
		{"inner-key-borrows-sibling", func(w []byte) { binary.BigEndian.PutUint32(w[25:29], 33) }},
		{"short-host-parent", func(w []byte) { binary.BigEndian.PutUint32(w[6:10], 50) }},
		{"signature-borrows-padding", func(w []byte) { binary.BigEndian.PutUint32(w[97:101], 84) }},
		{"short-fixed-signature", func(w []byte) { binary.BigEndian.PutUint32(w[116:120], 63) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := sshPlaintextRangeCurveReply()
			tc.mutate(wire)
			info, err := DecodeSSHInitialPacket(wire, "ecdh-curve25519")
			require.Error(t, err)
			require.Nil(t, info, "failed selected layouts must not expose a partial successful field/range tree")
			// These remain complete generic packets: the caller's unselected
			// layout owns only opaque bytes, not the refused inner primitives.
			generic, err := DecodeSSHInitialPacket(wire, "transport")
			require.NoError(t, err)
			p := generic["Wire Fields"].(map[string]any)["Payload"].(map[string]any)
			require.Equal(t, sshPlaintextRangeMap(map[string][2]uint64{
				"Message Number": {5, 6}, "Opaque Message Data": {6, 184},
			}), p["Wire Field Ranges"])
			require.Equal(t, wire[6:184], p["Opaque Message Data"])
		})
	}
	info, err := DecodeSSHInitialPacket(sshPlaintextRangeCurveReply()[:191], "ecdh-curve25519")
	require.Error(t, err)
	require.Nil(t, info)
}
