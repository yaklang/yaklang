package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
)

// These controls exercise the post-authentication STREAM stage directly; they
// are algorithm fixtures, not captured network packets or AEAD proofs.
func quicSourceFrame(t *testing.T, q *binQUIC, pdu, off uint64, data string) map[string]any {
	t.Helper()
	q.eventID = pdu
	q.eventRefs = []PacketReference{{Number: pdu}}
	fr := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": off, "Stream Data": []byte(data), "FIN": false}
	require.NoError(t, q.applyFrames(0, quicSpaceApplication, []map[string]any{fr}, map[string]any{}, 1024))
	return fr
}

func TestQUICSourceWitnessBudgetsOwnershipAndIsolation(t *testing.T) {
	var s quicStreamSources
	refs := []PacketReference{{Number: 2, Domain: CaptureDomain{Interface: 7, Encapsulation: "/vlan:9"}}}
	s.remember(20, 4, 8, refs, 1)
	refs[0].Number = 99
	s.remember(10, 0, 4, []PacketReference{{Number: 1}}, 1)
	m := s.describe(8, 1, 0, 8)
	require.Equal(t, "limited", m["Coverage"])
	require.Equal(t, false, m["Byte Coverage Complete"])
	m = s.describe(8, 1, 4, 8)
	require.Equal(t, "complete", m["Coverage"])
	r := m["Packet References"].([]map[string]any)
	require.Equal(t, uint64(2), r[0]["Packet"])
	require.Equal(t, 7, r[0]["Interface"])
	r[0]["Packet"] = uint64(100)
	require.Equal(t, uint64(2), s.describe(8, 1, 4, 8)["Packet References"].([]map[string]any)[0]["Packet"])
	s.prune(8)
	require.Zero(t, s.memory())
	s.remember(30, 8, 12, []PacketReference{{Number: 3}}, 1)
	require.Equal(t, "complete", s.describe(8, 1, 8, 12)["Coverage"], "future frame can have complete coverage after an older bounded proof was pruned")
	var unknown quicStreamSources
	unknown.remember(0, 0, 4, nil, 10)
	require.Equal(t, "limited", unknown.describe(0, 0, 0, 4)["Coverage"])
	refs = make([]PacketReference, 65)
	for i := range refs {
		refs[i].Number = uint64(i + 1)
	}
	unknown.remember(1, 0, 4, refs, 10)
	m = unknown.describe(0, 0, 0, 4)
	require.Equal(t, true, m["Byte Coverage Complete"])
	require.Equal(t, false, m["Packet Coverage Complete"])
	q := &binQUIC{native: true, byteLimit: 65536}
	quicSourceFrame(t, q, 1, 0, "abcd")
	q.eventID = 2
	fr := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": uint64(2), "Stream Data": []byte("XXef"), "FIN": false}
	require.Error(t, q.applyFrames(0, quicSpaceApplication, []map[string]any{fr}, map[string]any{}, 1024))
	require.Equal(t, []uint64{1}, q.streams[0].sources[0].describe(0, 0, 0, 6)["Parent PDUs"], "conflicting bytes never become source witnesses")
	require.Equal(t, "limited", q.streams[0].sources[1].describe(0, 1, 0, 4)["Coverage"], "opposite direction cannot borrow witnesses")
}

func TestHTTP3FrameSourcesCoverOriginalHeaderAndFragmentedPayload(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	headers := h3RequestHeaders()
	quicSourceFrame(t, q, 1, 0, string(headers))
	// A legal nonminimal two-byte Type plus two-byte Length must retain its
	// literal header, rather than being re-encoded as minimum-length varints.
	frame := append([]byte{0x40, 0, 0x40, 8}, []byte("abcdefgh")...)
	base := uint64(len(headers))
	quicSourceFrame(t, q, 2, base+8, string(frame[8:]))
	fr := quicSourceFrame(t, q, 3, base, string(frame[:8]))
	app := fr["HTTP3 Frames"].([]map[string]any)[0]
	require.Equal(t, []byte{0x40, 0, 0x40, 8}, app["Frame Header Bytes"])
	require.Equal(t, uint64(0), app["Frame Type Value"])
	require.Equal(t, uint64(8), app["Payload Length"])
	require.Equal(t, base, app["Stream Offset"])
	require.Equal(t, uint64(0), app["Stream ID"])
	require.Equal(t, []byte("abcdefgh"), app["Payload"])
	full := app["Byte Source"].(map[string]any)
	payload := app["Payload Byte Source"].(map[string]any)
	require.Equal(t, "complete", full["Coverage"])
	require.Equal(t, []uint64{2, 3}, full["Parent PDUs"])
	require.Equal(t, base+4, payload["Offset"])
	require.Equal(t, uint64(8), payload["Length"])
	require.Equal(t, []uint64{2, 3}, payload["Parent PDUs"])
	require.Empty(t, q.streams[0].sources[0].ranges, "completed application bytes do not keep accumulating witnesses")
}
func TestQUICStreamByteSourceKeepsOriginalOffsetAcrossOverlap(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536}
	quicSourceFrame(t, q, 1, 0, "abcd")
	fr := quicSourceFrame(t, q, 2, 2, "cdef")
	require.Equal(t, uint64(4), fr["Contiguous Offset"])
	require.Equal(t, 2, fr["Delivered Bytes"])
	source := fr["Byte Source"].(map[string]any)
	require.Equal(t, uint64(2), source["Offset"], "raw Stream Data begins at its literal STREAM offset, not the newly contiguous suffix")
}
func TestQUICStreamDeliverySourceIncludesEarlierOutOfOrderPDU(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536}
	quicSourceFrame(t, q, 1, 4, "efgh")
	fr := quicSourceFrame(t, q, 2, 0, "abcd")
	require.Equal(t, 8, fr["Delivered Bytes"])
	source, ok := fr["Delivered Byte Source"].(map[string]any)
	require.True(t, ok, "delivery combines two observed PDUs, rather than attributing buffered bytes only to the gap filler")
	require.Equal(t, []uint64{1, 2}, source["Parent PDUs"])
	require.Equal(t, uint64(0), source["Offset"])
	require.Equal(t, uint64(8), source["Length"])
}
