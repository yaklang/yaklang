package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQUICV2InitialSecretsRFC9369(t *testing.T) {
	keys, err := quicInitialKeysVersion(rfcHex("8394c8f03e515708"), quicVersion2)
	require.NoError(t, err)
	for d, expected := range [][4]string{
		{"14ec9d6eb9fd7af83bf5a668bc17a7e283766aade7ecd0891f70f9ff7f4bf47b", "8b1a0bc121284290a29e0971b5cd045d", "91f73e2351d8fa91660e909f", "45b95e15235d6f45a6b19cbcb0294ba9"},
		{"0263db1782731bf4588e7e4d93b7463907cb8cd8200b5da55a8bd488eafc37c1", "82db637861d55e1d011f19ea71d5d2a7", "dd13c276499c0249d3310652", "edf6d05c83121201b436e16877593c3a"},
	} {
		require.Equal(t, rfcHex(expected[0]), keys[d].secret)
		require.Equal(t, rfcHex(expected[1]), keys[d].key)
		require.Equal(t, rfcHex(expected[2]), keys[d].iv)
		require.Equal(t, rfcHex(expected[3]), keys[d].hp)
		require.Equal(t, quicVersion2, keys[d].version)
	}
	_, err = quicInitialKeysVersion([]byte("cid"), 2)
	require.ErrorContains(t, err, "UnsupportedVersion", "the v2 wire value is not integer2")
}

func TestQUICV2RFC9369NativeInitialIngressAndLiteralFields(t *testing.T) {
	es, stats := discoveryReplay(t, sessionDatagramPCAP(t, []sessionStep{{0, rfc9369ClientInitial()}, {1, rfc9369ServerInitial()}}, 14443), 1, false, false)
	require.Len(t, es, 2)
	require.NotZero(t, es[0].FlowID)
	require.Equal(t, es[0].FlowID, es[1].FlowID)
	for d, e := range es {
		require.Equal(t, "quic-v2-native", e.Profile)
		require.Equal(t, "decoded", e.Status)
		require.Equal(t, true, e.Session["Authentication Verified"])
		require.Equal(t, false, e.Session["Peer Identity Verified"])
		require.Equal(t, quicVersion2, e.Session["Version"])
		require.EqualValues(t, 2-d, e.Session["Packet Number"])
		fields, err := e.GetFields()
		require.NoError(t, err)
		require.Equal(t, quicVersion2, fields["Version"])
		v, ok := e.ownedQUICHeaderField("Version")
		require.True(t, ok)
		require.EqualValues(t, quicVersion2, v)
		v, ok = e.ownedQUICHeaderField("Long Packet Type")
		require.True(t, ok)
		require.EqualValues(t, 1, v, "raw v2 Initial type, not the v1 type0")
		aliases := e.DisplayFieldValues()
		require.Equal(t, []any{uint64(1)}, aliases.FieldMap()["quic.long.packet_type_v2"])
		require.Empty(t, aliases.FieldMap()["quic.long.packet_type"], "Wireshark uses separate v1/v2 type fields")
		for _, f := range aliases.Values {
			if f.Name == "quic.long.packet_type_v2" {
				require.Equal(t, &DisplayFieldRange{Offset: 0, Length: 1}, f.Range)
			}
		}
	}
	require.Zero(t, stats.BufferedBytes)
}

func TestQUICV2RetryRFC9369AndVersionIsolation(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 1 << 20}
	_, err := q.consume(0, rfc9369ClientInitial(), 4096)
	require.NoError(t, err)
	retry := rfc9369Retry()
	require.Equal(t, retry[len(retry)-16:], quicRetryTagVersion(q.originalDCID, retry[:len(retry)-16], quicVersion2))
	bad := bytes.Clone(retry)
	copy(bad[len(bad)-16:], quicRetryTag(q.originalDCID, bad[:len(bad)-16]))
	_, err = q.consume(1, bad, 4096)
	require.ErrorContains(t, err, "integrity tag failed")
	require.False(t, q.retrySeen)
	info, err := q.consume(1, retry, 4096)
	require.NoError(t, err)
	require.Equal(t, true, info["Retry Integrity Verified"])
	require.Equal(t, false, info["Authentication Verified"])
	require.NotContains(t, info, "Packet Number Space")
	before := bytes.Clone(q.initialDCID)
	_, err = q.consume(0, rfc9001ClientInitial(), 4096)
	require.ErrorContains(t, err, "compatible negotiation is not selected")
	require.Equal(t, quicVersion2, q.version)
	require.Equal(t, before, q.initialDCID)
	wrongVersion := bytes.Clone(rfc9369ClientInitial())
	binary.BigEndian.PutUint32(wrongVersion[1:5], 2)
	_, err = quicParseHeader(wrongVersion, true)
	require.ErrorContains(t, err, "UnsupportedVersion")
}

func TestQUICV2ChaChaAndKeyUpdateRFC9369(t *testing.T) {
	secret := rfcHex("9ac312a7f877468ebe69422748ad00a15443f18203a07d6060f688f30f21632b")
	k, err := quicDeriveTrafficVersion(secret, quicAEADChaCha20Poly1305, quicVersion2)
	require.NoError(t, err)
	require.Equal(t, rfcHex("3bfcddd72bcf02541d7fa0dd1f5f9eeea817e09a6963a0e6c7df0f9a1bab90f2"), k.key)
	require.Equal(t, rfcHex("a6b5bc6ab7dafce30ffff5dd"), k.iv)
	require.Equal(t, rfcHex("d659760d2ba434a226fd37b35c69e2da8211d10c4f12538787d65645d5d1b8e2"), k.hp)
	next, err := quicNextPhase(k)
	require.NoError(t, err)
	require.Equal(t, rfcHex("c69374c49e3d2a9466fa689e49d476db5d0dfbc87d32ceeaa6343fd0ae4c7d88"), next.secret)
	require.Equal(t, k.hp, next.hp)
	q := &binQUIC{native: true, version: quicVersion2, keys: &quicKeyring{version: quicVersion2}, zeroCID: [2]bool{true, true}}
	q.keys.app[1] = [2]*quicTrafficKeys{k, next}
	q.spaces[1][quicSpaceApplication] = quicPNSpace{largest: 654360563, init: true}
	info, err := q.consume(1, rfcHex("5558b1c60ae7b6b932bc27d786f4bc2bb20f2162ba"), 4096)
	require.NoError(t, err)
	require.EqualValues(t, 654360564, info["Packet Number"])
	require.Equal(t, true, info["Authentication Verified"])
	old, err := quicDeriveTraffic(secret, quicAEADChaCha20Poly1305)
	require.NoError(t, err)
	_, err = q.consume(1, epochQUICPacket(t, old, nil, 654360565, 0, 4), 4096)
	require.ErrorContains(t, err, "authentication failed", "identical TLS secret cannot share v1/v2 packet keys")
	require.EqualValues(t, 654360564, q.spaces[1][quicSpaceApplication].largest)
	info, err = q.consume(1, epochQUICPacket(t, next, nil, 654360566, 1, 4), 4096)
	require.NoError(t, err)
	require.Equal(t, 1, info["Key Phase"])
	require.Equal(t, 1, q.keyPhase[1])
}

func FuzzQUICVersionAndKeyPhaseIsolation(f *testing.F) {
	f.Add(uint64(769), uint8(0), uint8(1), uint8(0))
	f.Add(uint64(19), uint8(2), uint8(0), uint8(20))
	f.Fuzz(func(t *testing.T, number uint64, algorithm, phase, cidLength uint8) {
		alg := []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305}[algorithm%3]
		n := 32
		if alg == quicAEADAES256GCM {
			n = 48
		}
		secret, cid := bytes.Repeat([]byte{7}, n), bytes.Repeat([]byte{0x8a}, int(cidLength%21))
		pn, p := number%(1<<30)+1, int(phase&1)
		for _, version := range []uint32{quicVersion1, quicVersion2} {
			other := quicVersion1
			if version == quicVersion1 {
				other = quicVersion2
			}
			key, err := quicDeriveTrafficVersion(secret, alg, version)
			require.NoError(t, err)
			next, err := quicNextPhase(key)
			require.NoError(t, err)
			foreign, err := quicDeriveTrafficVersion(secret, alg, other)
			require.NoError(t, err)
			if p == 1 {
				foreign, err = quicNextPhase(foreign)
				require.NoError(t, err)
			}
			q := &binQUIC{native: true, version: version, keys: &quicKeyring{version: version}, zeroCID: [2]bool{len(cid) == 0, len(cid) == 0}, cids: map[string]int{}}
			if len(cid) > 0 {
				q.cids[string(cid)] = 1
			}
			q.keys.app[0] = [2]*quicTrafficKeys{key, next}
			q.spaces[0][quicSpaceApplication].largest = pn - 1
			_, err = q.consume(0, epochQUICPacket(t, foreign, cid, pn, p, 2), 64)
			require.Error(t, err, "same TLS secret cannot borrow another version's packet keys")
			require.False(t, q.spaces[0][quicSpaceApplication].init)
			info, err := q.consume(0, epochQUICPacket(t, q.keys.app[0][p], cid, pn, p, 2), 64)
			require.NoError(t, err)
			require.Equal(t, true, info["Authentication Verified"])
			require.EqualValues(t, pn, info["Packet Number"])
			require.Equal(t, p, q.keyPhase[0])
		}
	})
}
