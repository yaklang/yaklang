package pcaputil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func retryIdentityPacket(originalDCID, dcid, scid, token []byte) []byte {
	wire := append([]byte{0xf0, 0, 0, 0, 1, byte(len(dcid))}, dcid...)
	wire = append(wire, byte(len(scid)))
	wire = append(wire, scid...)
	wire = append(wire, token...)
	return append(wire, quicRetryTag(originalDCID, wire)...)
}

func TestQUICRetryCannotReuseOriginalDCIDOrChangeClientSCID(t *testing.T) {
	seed, clientCID, retryCID, token := []byte("initial!"), []byte("client!!"), []byte("retry-id"), []byte("token")
	q := &binQUIC{native: true, byteLimit: 1 << 20}
	_, err := q.consume(0, retryTestProtectedInitial(t, seed, seed, clientCID, nil, 0, 0), 64)
	require.NoError(t, err)
	// A mathematically valid public Retry tag cannot override RFC9000's
	// prohibition on reusing the original destination CID as the Retry SCID.
	_, err = q.consume(1, retryIdentityPacket(seed, clientCID, seed, token), 64)
	require.ErrorContains(t, err, "endpoint/CID/token")
	require.False(t, q.retrySeen)
	require.Empty(t, q.retrySCID)
	_, err = q.consume(1, retryIdentityPacket(seed, clientCID, retryCID, token), 64)
	require.NoError(t, err)
	_, err = q.consume(0, retryTestProtectedInitial(t, retryCID, retryCID, []byte("changed!"), token, 1, 0), 64)
	require.ErrorContains(t, err, "changed source CID")
	require.NotContains(t, q.cids, "changed!")
	require.Equal(t, seed, q.initialDCID, "rejected clear-header change cannot commit keys")
	info, err := q.consume(0, retryTestProtectedInitial(t, retryCID, retryCID, clientCID, token, 1, 0), 64)
	require.NoError(t, err)
	require.Equal(t, true, info["Authentication Verified"])
	require.Equal(t, retryCID, q.initialDCID)
}

func TestQUICRetryConflictingCryptoCannotReplaceTLSOrBecomeCleanDuplicate(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 1 << 20}
	_, err := q.consume(0, rfc9001ClientInitial(), 4096)
	require.NoError(t, err)
	original, firstCID := bytes.Clone(q.cryptoData[0][quicSpaceInitial].data), bytes.Clone(q.originalDCID)
	random := bytes.Clone(q.tls.random)
	retryCID, token := []byte("retry-id"), []byte("token")
	_, err = q.consume(1, retryIdentityPacket(firstCID, q.initialSCID, retryCID, token), 4096)
	require.NoError(t, err)
	changed := bytes.Clone(original)
	changed[6] ^= 1 // First ClientHello random byte; the same CRYPTO offset differs.
	crypto := func(data []byte) []byte {
		plain := append([]byte{6, 0}, quicPutVarint(uint64(len(data)))...)
		return append(plain, data...)
	}
	bad := retryTestInitialFrames(t, retryCID, retryCID, q.initialSCID, token, 3, 0, crypto(changed))
	info, err := q.consume(0, bad, 4096)
	require.ErrorContains(t, err, "conflicting retransmission")
	require.Equal(t, true, info["Authentication Verified"], "protected bytes do not make a conflicting TLS transcript conformant")
	require.Equal(t, random, q.tls.random)
	require.Equal(t, original, q.cryptoData[0][quicSpaceInitial].data)
	firstError := err.(*ProtocolError)
	firstError.Message = "caller changed returned error"
	for n := 0; n < 2; n++ {
		info, err = q.consume(0, bad, 4096)
		require.ErrorContains(t, err, "conflicting retransmission", "known refused packet cannot become a successful duplicate")
		require.Equal(t, true, info["Duplicate"])
		err.(*ProtocolError).Message = "caller changed duplicate error"
	}
	good := retryTestInitialFrames(t, retryCID, retryCID, q.initialSCID, token, 4, 0, crypto(original))
	_, err = q.consume(0, good, 4096)
	require.NoError(t, err, "an identical retransmission still repairs the capture")
	_, err = q.consume(0, bad, 4096)
	require.ErrorContains(t, err, "conflicting retransmission", "later valid packets do not erase the earlier refusal")
	for pn := byte(5); pn < 145; pn++ {
		_, err = q.consume(0, retryTestInitialFrames(t, retryCID, retryCID, q.initialSCID, token, pn, 0, crypto(changed)), 4096)
		require.Error(t, err)
	}
	space := q.space(0, quicSpaceInitial)
	require.Len(t, space.seen, 128)
	require.LessOrEqual(t, len(space.rejected), 128)
	require.NotContains(t, space.rejected, uint64(3), "rejection cache follows the bounded observed-PN window")
	require.Equal(t, random, q.tls.random)
}

func FuzzQUICRetryCryptoRefusalSurvivesDuplicate(f *testing.F) {
	f.Add(uint16(6), uint8(1), uint8(3))
	f.Add(uint16(240), uint8(255), uint8(90))
	f.Fuzz(func(t *testing.T, offset uint16, flip, packetNumber uint8) {
		q := &binQUIC{native: true, byteLimit: 1 << 20}
		_, err := q.consume(0, rfc9001ClientInitial(), 4096)
		require.NoError(t, err)
		original := bytes.Clone(q.cryptoData[0][quicSpaceInitial].data)
		random := bytes.Clone(q.tls.random)
		retryCID, token := []byte("retry-id"), []byte("token")
		_, err = q.consume(1, retryIdentityPacket(q.originalDCID, q.initialSCID, retryCID, token), 4096)
		require.NoError(t, err)
		changed := bytes.Clone(original)
		if flip == 0 {
			flip = 1
		}
		changed[int(offset)%len(changed)] ^= flip
		plain := append([]byte{6, 0}, quicPutVarint(uint64(len(changed)))...)
		plain = append(plain, changed...)
		if packetNumber < 3 {
			packetNumber += 3
		}
		wire := retryTestInitialFrames(t, retryCID, retryCID, q.initialSCID, token, packetNumber, 0, plain)
		_, err = q.consume(0, wire, 4096)
		require.ErrorContains(t, err, "conflicting retransmission")
		info, err := q.consume(0, wire, 4096)
		require.ErrorContains(t, err, "conflicting retransmission")
		require.Equal(t, true, info["Duplicate"])
		require.Equal(t, true, info["Authentication Verified"])
		require.Equal(t, original, q.cryptoData[0][quicSpaceInitial].data)
		require.Equal(t, random, q.tls.random)
	})
}

func TestQUICRetryRejectedNativePacketDuplicateIsNotSuccess(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 1 << 20}
	first := rfc9001ClientInitial()
	_, err := q.consume(0, first, 4096)
	require.NoError(t, err)
	retryCID, token := []byte("retry-id"), []byte("token")
	retry := retryIdentityPacket(q.originalDCID, q.initialSCID, retryCID, token)
	changed := bytes.Clone(q.cryptoData[0][quicSpaceInitial].data)
	changed[6] ^= 1
	plain := append([]byte{6, 0}, quicPutVarint(uint64(len(changed)))...)
	plain = append(plain, changed...)
	bad := retryTestInitialFrames(t, retryCID, retryCID, q.initialSCID, token, 3, 0, plain)
	es, stats := discoveryReplay(t, sessionDatagramPCAP(t, []sessionStep{{0, first}, {1, retry}, {0, bad}, {0, bad}}, 14443), 1, false, false)
	require.Len(t, es, 4)
	require.Equal(t, es[0].FlowID, es[3].FlowID)
	for _, e := range es[2:] {
		require.Equal(t, "malformed", e.Status)
		require.Equal(t, string(ErrMalformedMessage), e.ExpertCode)
		require.Contains(t, e.Error, "conflicting retransmission")
		require.Equal(t, true, e.Session["Authentication Verified"], "packet protection is separate from CRYPTO consistency")
		require.Equal(t, "authenticated-wire-and-observed-CID", e.Admission)
	}
	require.Equal(t, true, es[3].Session["Duplicate"])
	require.Zero(t, stats.BufferedBytes)
}

func TestQUICUnnumberedHeadersDoNotInventPacketNumberSpace(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 1 << 20}
	_, err := q.consume(0, rfc9001ClientInitial(), 4096)
	require.NoError(t, err)
	versionNegotiation := rfcHex("c0000000000008f067a5502a4262b50000000100000002")
	info, err := q.consume(1, versionNegotiation, 4096)
	require.NoError(t, err)
	require.Equal(t, "Version Negotiation", info["Packet Name"])
	for _, key := range []string{"Packet Number Space", "Packet Number", "Packet Number Length"} {
		require.NotContains(t, info, key)
	}
	info, err = q.consume(1, retryIdentityPacket(q.originalDCID, q.initialSCID, []byte("retry-id"), []byte("token")), 4096)
	require.NoError(t, err)
	require.Equal(t, true, info["Retry Integrity Verified"])
	for _, key := range []string{"Packet Number Space", "Packet Number", "Packet Number Length"} {
		require.NotContains(t, info, key)
	}
	require.Equal(t, false, info["Authentication Verified"])
}
