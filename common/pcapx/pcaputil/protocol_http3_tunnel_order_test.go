package pcaputil

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHTTP3TunnelClientBeforeResponseDoesNotProvePostSuccessOrder(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	request := h3TunnelRequest()
	_, _, err := h3BodyTestFeed(q, 0, 0, 0, request, false)
	require.NoError(t, err)
	trailer := h3LiteralFields([2]string{"x-trailer", "y"})
	_, _, err = h3BodyTestFeed(q, 0, 0, uint64(len(request)), trailer, false)
	require.NoError(t, err)
	_, info, err := h3BodyTestFeed(q, 1, 0, 0, h3ResponseHeaders(), false)
	require.NoError(t, err)
	// Directional stream offsets order client frames, not the client's frame
	// relative to the successful server response. Do not fabricate causality
	// from a missing response or reversed cross-direction capture order.
	require.Nil(t, q.h3Fault)
	fact := h3TunnelFacts(t, info)[0]
	require.Equal(t, "order-unverified", fact["Observation Verdict"])
	require.NotContains(t, fact, "Error Code")
	_, _, err = h3BodyTestFeed(q, 0, 0, uint64(len(request)+len(trailer)), h3Data([]byte("later tunnel bytes")), false)
	require.NoError(t, err, "unknown earlier order must not quarantine later DATA")
}

func TestHTTP3TunnelUnverifiedPartialClientFrameCanFinishWithoutRetroactiveFault(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	request, trailer := h3TunnelRequest(), h3LiteralFields([2]string{"x-trailer", "y"})
	_, _, err := h3BodyTestFeed(q, 0, 0, 0, request, false)
	require.NoError(t, err)
	_, _, err = h3BodyTestFeed(q, 0, 0, uint64(len(request)), trailer[:1], false)
	require.NoError(t, err)
	_, info, err := h3BodyTestFeed(q, 1, 0, 0, h3ResponseHeaders(), false)
	require.NoError(t, err)
	require.Equal(t, "order-unverified", h3TunnelFacts(t, info)[0]["Observation Verdict"])
	_, info, err = h3BodyTestFeed(q, 0, 0, uint64(len(request)+1), trailer[1:], false)
	require.NoError(t, err)
	require.NotContains(t, info, "HTTP3 Tunnel Observations")
	require.Nil(t, q.h3Fault)
	_, _, err = h3BodyTestFeed(q, 0, 0, uint64(len(request)+len(trailer)), h3Data([]byte("tunnel")), false)
	require.NoError(t, err)
	_, _, err = h3BodyTestFeed(q, 0, 0, uint64(len(request)+len(trailer)+len(h3Data([]byte("tunnel")))), []byte{1}, false)
	require.True(t, isH3ConnectionObservationError(err), "a new forbidden type under known success context is a separate observation")
}

func TestHTTP3TunnelGapDoesNotIdentifyTypeUntilReassemblyCompletesIt(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	_, _, err := h3BodyTestFeed(q, 0, 0, 0, h3TunnelRequest(), false)
	require.NoError(t, err)
	response := h3ResponseHeaders()
	_, _, err = h3BodyTestFeed(q, 1, 0, 0, response, false)
	require.NoError(t, err)
	_, info, err := h3BodyTestFeed(q, 1, 0, uint64(len(response)+1), []byte{1}, false)
	require.NoError(t, err)
	require.NotContains(t, info, "HTTP3 Connection Error")
	_, info, err = h3BodyTestFeed(q, 1, 0, uint64(len(response)), []byte{0x40}, false)
	require.True(t, isH3ConnectionObservationError(err))
	fact := h3TunnelFacts(t, info)[0]
	require.Equal(t, []byte{0x40, 1}, fact["Frame Header Bytes"])
	source := fact["Byte Source"].(map[string]any)
	require.Equal(t, true, source["Byte Coverage Complete"])
	require.Equal(t, true, source["Packet Coverage Complete"])
	require.ElementsMatch(t, []uint64{3, 4}, source["Parent PDUs"])
}

func TestHTTP3TunnelLateBudgetContextDoesNotDiscardCurrentDATA(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	request, trailer := h3TunnelRequest(), h3LiteralFields([2]string{"x-trailer", "y"})
	_, _, err := h3BodyTestFeed(q, 0, 0, 0, request, false)
	require.NoError(t, err)
	_, _, err = h3BodyTestFeed(q, 0, 0, uint64(len(request)), trailer, false)
	require.NoError(t, err)
	q.eventID++
	q.eventRefs = []PacketReference{{Number: q.eventID}}
	fr := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": uint64(0), "Stream Data": h3ResponseHeaders(), "FIN": false}
	err = q.applyFrames(1, quicSpaceApplication, []map[string]any{fr}, map[string]any{}, 1)
	require.Error(t, err, "the established frame budget contract is retained")
	_, info, err := h3BodyTestFeed(q, 0, 0, uint64(len(request)+len(trailer)), h3Data([]byte("later tunnel bytes")), false)
	require.NoError(t, err)
	require.Nil(t, q.h3Fault)
	require.Equal(t, "order-unverified", h3TunnelFacts(t, info)[0]["Observation Verdict"])
	frames := info["HTTP3 Frames"].([]map[string]any)
	require.Equal(t, []byte("later tunnel bytes"), frames[0]["Payload"])
}
