package pcaputil

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func h3TunnelRequest() []byte {
	return h3LiteralFields([2]string{":method", "CONNECT"}, [2]string{":authority", "example.com:443"})
}

func h3TunnelFacts(t *testing.T, info map[string]any) []map[string]any {
	t.Helper()
	facts, ok := info["HTTP3 Tunnel Observations"].([]map[string]any)
	require.True(t, ok, "the original forbidden frame and later context need an explicit observation")
	require.NotEmpty(t, facts)
	return facts
}

func TestHTTP3TunnelKnownTypesRejectedBeforePayloadOrQPACK(t *testing.T) {
	for _, direction := range []int{0, 1} {
		for _, typ := range []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 13} {
			t.Run(fmt.Sprintf("direction%d/type%d", direction, typ), func(t *testing.T) {
				q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
				_, _, err := h3BodyTestFeed(q, 0, 0, 0, h3TunnelRequest(), false)
				require.NoError(t, err)
				response := h3ResponseHeaders()
				_, _, err = h3BodyTestFeed(q, 1, 0, 0, response, false)
				require.NoError(t, err)
				off := len(response)
				if direction == 0 {
					off = len(h3TunnelRequest())
				}
				// The authenticated, contiguous type varint alone proves a
				// forbidden known frame; an attacker need not finish its payload.
				fr, info, err := h3BodyTestFeed(q, direction, 0, uint64(off), quicPutVarint(typ), false)
				require.Error(t, err)
				require.True(t, isH3ConnectionObservationError(err))
				require.False(t, isH3FieldStreamError(err))
				fault := info["HTTP3 Connection Error"].(map[string]any)
				require.Equal(t, "H3_FRAME_UNEXPECTED", fault["Error Name"])
				require.Equal(t, uint64(0x105), fault["Error Code"])
				require.Equal(t, "connection", fault["Error Scope"])
				fact := h3TunnelFacts(t, info)[0]
				require.Equal(t, typ, fact["Frame Type Value"])
				require.Equal(t, uint64(direction), fact["Direction"])
				require.Equal(t, uint64(off), fact["Frame Offset"])
				require.Equal(t, q.eventID, fact["Frame PDU ID"])
				require.Equal(t, q.eventID, fact["Observed In PDU ID"])
				require.Equal(t, quicPutVarint(typ), fact["Frame Header Bytes"])
				require.NotNil(t, fr["HTTP3 Frame Error"])
				require.NotContains(t, fr, "QPACK Block")
				_, _, err = h3BodyTestFeed(q, direction, 0, uint64(off+1), []byte{0xff}, false)
				require.True(t, isH3ConnectionObservationError(err), "later suffix cannot make the stream conformant")
				require.Equal(t, "H3_FRAME_UNEXPECTED", q.h3Fault.snapshot()["Error Name"])
			})
		}
	}
}

func TestHTTP3TunnelLateContextKeepsValidCurrentMessage(t *testing.T) {
	for _, context := range []string{"method", "successful-response"} {
		for _, partial := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/partial%v", context, partial), func(t *testing.T) {
				q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
				trailer := h3LiteralFields([2]string{"x-trailer", "y"})
				if partial {
					trailer = []byte{1}
				}
				if context == "successful-response" {
					_, _, err := h3BodyTestFeed(q, 0, 0, 0, h3TunnelRequest(), false)
					require.NoError(t, err)
				} else {
					_, _, err := h3BodyTestFeed(q, 1, 0, 0, h3ResponseHeaders(), false)
					require.NoError(t, err)
				}
				dir, off := 1, len(h3ResponseHeaders())
				if context == "successful-response" {
					dir, off = 0, len(h3TunnelRequest())
				}
				_, prior, err := h3BodyTestFeed(q, dir, 0, uint64(off), trailer, false)
				require.NoError(t, err)
				pdu := q.eventID
				frozen, err := json.Marshal(prior)
				require.NoError(t, err)
				var info map[string]any
				if context == "method" {
					_, info, err = h3BodyTestFeed(q, 0, 0, 0, h3TunnelRequest(), false)
				} else {
					_, info, err = h3BodyTestFeed(q, 1, 0, 0, h3ResponseHeaders(), false)
				}
				require.NoError(t, err, "the later request or successful response is valid")
				fact := h3TunnelFacts(t, info)[0]
				require.Equal(t, pdu, fact["Frame PDU ID"])
				require.Equal(t, q.eventID, fact["Observed In PDU ID"])
				require.Equal(t, uint64(dir), fact["Direction"])
				if context == "method" {
					require.Equal(t, uint64(0x105), fact["Error Code"])
				} else {
					require.NotContains(t, fact, "Error Code")
					require.Equal(t, "order-unverified", fact["Observation Verdict"])
					require.Nil(t, q.h3Fault)
				}
				if context == "method" {
					require.Equal(t, "violation-observed", h3LateBodyResult(t, info)["Observation Verdict"])
				}
				after, err := json.Marshal(prior)
				require.NoError(t, err)
				require.Equal(t, frozen, after)
			})
		}
	}
}

func TestHTTP3TunnelOrdinaryTrailersAndUnknownExtensionsRemainUsable(t *testing.T) {
	for _, method := range []string{"GET", "CONNECT"} {
		for _, status := range []string{"200", "407"} {
			t.Run(method+"/"+status, func(t *testing.T) {
				q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
				request := h3BodyRequest(method, "-")
				if method == "CONNECT" {
					request = h3TunnelRequest()
				}
				_, _, err := h3BodyTestFeed(q, 0, 0, 0, request, false)
				require.NoError(t, err)
				response := h3LiteralFields([2]string{":status", status})
				response = append(response, h3Frame(0x21, []byte("unknown extension"))...)
				response = append(response, h3Data([]byte("tunnel or ordinary content"))...)
				if method != "CONNECT" || status != "200" {
					response = append(response, h3LiteralFields([2]string{"x-trailer", "y"})...)
				}
				_, info, err := h3BodyTestFeed(q, 1, 0, 0, response, true)
				require.NoError(t, err)
				require.NotContains(t, info, "HTTP3 Connection Error")
				require.Nil(t, q.h3Fault)
			})
		}
	}
}
