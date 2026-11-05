package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func h3LateBodyResult(t *testing.T, info map[string]any) map[string]any {
	t.Helper()
	links, ok := info["HTTP3 Late Associations"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, links, 1)
	result, ok := links[0]["Response Message Revalidation"].(map[string]any)
	require.True(t, ok, "later method knowledge must revalidate the earlier response")
	return result
}

func TestHTTP3LateBodyNativeProtectedPacketsOwnQueriesAndCurrentView(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, cipher := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			t.Run(fmt.Sprintf("v%x/%s", version, cipher), func(t *testing.T) {
				a := earlyRecoveryParser(t)
				q, key := quicConformanceContext(t, version, cipher)
				q.alpn, q.byteLimit = "h3", 65536
				domain := CaptureDomain{Section: 2, Interface: 7, Encapsulation: "/vlan:91"}
				f := earlyRecoveryOwner(a, q, domain)
				base := earlyRecoveryBase(f, domain, 11)
				base.Source, base.Destination = base.Destination, base.Source
				response := append(h3LiteralFields([2]string{":status", "200"}, [2]string{"content-length", "1"}), h3Data([]byte("x"))...)
				first, ok := a.decodeQUICDatagram(base, quicConformancePacket(t, key, "1-RTT", 1, 0, quicStreamFrame(0, 0, true, response)))
				require.True(t, ok)
				require.Len(t, first, 1)
				require.Equal(t, "decoded", first[0].Status)
				a.emit(first[0])
				history, err := NewProtocolInspector(8, 1<<20)
				require.NoError(t, err)
				history.OnEvent(first[0])
				frozen, err := first[0].Decode()
				require.NoError(t, err)
				first[0].Raw = bytes.Repeat([]byte{0}, len(first[0].Raw))
				first[0].SourceBytes.PacketRefs[0].Number = 999
				second, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, domain, 12), quicConformancePacket(t, key, "1-RTT", 2, 0, quicStreamFrame(0, 0, true, h3BodyRequest("HEAD", "-"))))
				require.True(t, ok)
				require.Len(t, second, 1)
				require.Equal(t, "decoded", second[0].Status)
				a.emit(second[0])
				history.OnEvent(second[0])
				m := h3LateBodyResult(t, second[0].Session)
				require.Equal(t, "violation-observed", m["Observation Verdict"])
				require.Equal(t, true, m["Body Coverage Complete"])
				require.Equal(t, uint64(11), m["Observed DATA Byte Source"].(map[string]any)["Packet References"].([]map[string]any)[0]["Packet"])
				view, err := history.Details(first[0].ID)
				require.NoError(t, err)
				require.Zero(t, view.ResponseTo)
				fields := view.DisplayFieldValues().FieldMap()
				require.Equal(t, []any{"HEAD"}, fields["http3.late_message.request_method"])
				require.Equal(t, []any{uint64(1)}, fields["http3.late_message.observed_data_bytes"])
				require.Equal(t, []any{"violation-observed"}, fields["http3.late_message.verdict"])
				require.Equal(t, []any{first[0].ID}, fields["http3.late_message.fin_pdu"])
				require.Equal(t, []any{second[0].ID}, fields["http3.late_message.observed_in_pdu"])
				for _, value := range view.DisplayFieldValues().Values {
					if strings.HasPrefix(value.Name, "http3.late_message.") {
						require.Nil(t, value.Range)
						require.Equal(t, "context", value.Origin)
					}
				}
				m["Observed DATA Bytes"] = uint64(999)
				m["Observation Verdict"] = "edited"
				second[0].Protocol = "doq"
				require.Equal(t, []any{uint64(1)}, second[0].DisplayFieldValues().FieldMap()["http3.late_message.observed_data_bytes"])
				view.Session["HTTP3 Resolved Associations"].([]map[string]any)[0]["Response Message Revalidation"].(map[string]any)["Observation Verdict"] = "edited"
				require.Equal(t, []any{"violation-observed"}, view.DisplayFieldValues().FieldMap()["http3.late_message.verdict"])
				decoded, err := first[0].Decode()
				require.NoError(t, err)
				require.Equal(t, frozen, decoded)
				require.Empty(t, first[0].DisplayFieldValues().FieldMap()["http3.late_message.verdict"])
			})
		}
	}
}

func TestHTTP3LateBodyViolationIsStreamLocalAndPreservesValidRequest(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	wire := append(h3LiteralFields([2]string{":status", "200"}, [2]string{"content-length", "1"}), h3Data([]byte("x"))...)
	_, _, err := h3BodyTestFeed(q, 1, 0, 0, wire, false)
	require.NoError(t, err)
	_, _, err = h3BodyTestFeed(q, 0, 0, 0, h3BodyRequest("HEAD", "-"), true)
	require.NoError(t, err)
	_, err = qpackRecoveryStream(t, q, 3, 0, 4, 0, h3RequestHeaders())
	require.NoError(t, err)
	bad := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": uint64(len(wire)), "Stream Data": h3Data([]byte("y")), "FIN": false}
	other := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(4), "Offset": uint64(0), "Stream Data": h3ResponseHeaders(), "FIN": false}
	q.eventID = 4
	q.eventRefs = []PacketReference{{Number: 4}}
	err = q.applyFrames(1, quicSpaceApplication, []map[string]any{bad, other}, map[string]any{}, 1024)
	require.True(t, isH3FieldStreamError(err))
	require.NotEmpty(t, other["HTTP3 Frames"])
	require.Equal(t, uint64(3), other["Request PDU ID"])
	require.Empty(t, q.streams[0].h3.fieldFailure[0])
	require.Empty(t, q.streams[4].h3.fieldFailure[0])
	require.Empty(t, q.streams[4].h3.fieldFailure[1])
	require.Equal(t, "h3", q.alpn)
}

func TestHTTP3LateBodyDropsUnneededFINAfterRequestBecomesKnown(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	_, _, err := h3BodyTestFeed(q, 1, 0, 100, nil, true)
	require.NoError(t, err)
	require.NotNil(t, q.streams[0].responseFIN)
	_, _, err = h3BodyTestFeed(q, 0, 0, 0, h3BodyRequest("HEAD", "-"), true)
	require.NoError(t, err)
	require.Nil(t, q.streams[0].responseFIN)
	q2 := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	_, _, err = h3BodyTestFeed(q2, 0, 0, 0, h3BodyRequest("HEAD", "-"), true)
	require.NoError(t, err)
	_, _, err = h3BodyTestFeed(q2, 1, 0, 0, h3LiteralFields([2]string{":status", "200"}, [2]string{"content-length", "100"}), true)
	require.NoError(t, err)
	require.Nil(t, q2.streams[0].responseFIN)
	require.Nil(t, q2.streams[0].h3.response)
}

func TestHTTP3LateMethodRevalidatesPriorBodyWithoutRewritingWireObservations(t *testing.T) {
	for _, tt := range []struct {
		name, method, body, check string
		bad                       bool
	}{
		{"HEAD-content-forbidden", "HEAD", "x", "malformed", true},
		{"HEAD-empty-representation-length-allowed", "HEAD", "", "exempt-no-content", false},
		{"POST-empty-too-short", "POST", "", "malformed", true},
		{"POST-matched", "POST", "x", "matched", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
			wire := append(h3LiteralFields([2]string{":status", "200"}, [2]string{"content-length", "1"}), h3Data([]byte(tt.body))...)
			_, response, err := h3BodyTestFeed(q, 1, 0, 0, wire, true)
			require.NoError(t, err)
			frozen, err := json.Marshal(response)
			require.NoError(t, err)
			_, request, err := h3BodyTestFeed(q, 0, 0, 0, h3BodyRequest(tt.method, "-"), true)
			require.NoError(t, err, "valid late request must not be blamed for an earlier response")
			m := h3LateBodyResult(t, request)
			require.Equal(t, tt.check, m["Length Check"])
			require.Equal(t, uint64(len(tt.body)), m["Observed DATA Bytes"])
			require.Equal(t, uint64(1), m["Header PDU ID"])
			require.Equal(t, uint64(2), m["Observed In PDU ID"])
			require.Equal(t, uint64(1), m["Direction"])
			require.Equal(t, true, m["Body Coverage Complete"])
			if tt.bad {
				require.Equal(t, "violation-observed", m["Observation Verdict"])
				require.Equal(t, ErrMalformedMessage, q.streams[0].h3.fieldFailure[1])
			} else {
				require.Equal(t, "no-violation-observed", m["Observation Verdict"])
				require.Empty(t, q.streams[0].h3.fieldFailure[1])
			}
			require.Empty(t, q.streams[0].h3.fieldFailure[0])
			require.Equal(t, uint64(2), q.streams[0].requestPDU)
			after, err := json.Marshal(response)
			require.NoError(t, err)
			require.Equal(t, frozen, after)
		})
	}
}

func TestHTTP3LateCONNECTRechecksResponseLengthWithoutMiscountingTunnelBytes(t *testing.T) {
	for _, tt := range []struct {
		name, status, length string
		bad                  bool
	}{
		{"successful-forbidden-length", "200", "4", true},
		{"successful-tunnel-no-length", "200", "", false},
		{"unsuccessful-ordinary-content", "403", "4", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
			fields := [][2]string{{":status", tt.status}}
			if tt.length != "" {
				fields = append(fields, [2]string{"content-length", tt.length})
			}
			wire := append(h3LiteralFields(fields...), h3Data([]byte("data"))...)
			_, _, err := h3BodyTestFeed(q, 1, 0, 0, wire, true)
			require.NoError(t, err)
			_, request, err := h3BodyTestFeed(q, 0, 0, 0, h3LiteralFields([2]string{":method", "CONNECT"}, [2]string{":authority", "example.com:443"}), true)
			require.NoError(t, err)
			m := h3LateBodyResult(t, request)
			if tt.bad {
				require.Equal(t, "violation-observed", m["Observation Verdict"])
				require.Contains(t, m["Error"], "Content-Length forbidden")
			} else {
				require.Equal(t, "no-violation-observed", m["Observation Verdict"])
				if tt.status == "200" {
					require.Contains(t, m["Length Check"], "CONNECT-stream")
				} else {
					require.Equal(t, "matched", m["Length Check"])
				}
			}
		})
	}
}

func TestHTTP3LateBodyRetainsDataFINAndContextOriginsSeparately(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	header := h3LiteralFields([2]string{":status", "200"}, [2]string{"content-length", "2"})
	_, _, err := h3BodyTestFeed(q, 1, 0, 0, header, false)
	require.NoError(t, err)
	data := h3Data([]byte("ab"))
	_, old, err := h3BodyTestFeed(q, 1, 0, uint64(len(header)), data, false)
	require.NoError(t, err)
	old["HTTP3 Frames"].([]map[string]any)[0]["Payload Byte Source"].(map[string]any)["Parent PDUs"].([]uint64)[0] = 999
	_, _, err = h3BodyTestFeed(q, 1, 0, uint64(len(header)+len(data)), nil, true)
	require.NoError(t, err)
	_, info, err := h3BodyTestFeed(q, 0, 0, 0, h3BodyRequest("HEAD", "-"), true)
	require.NoError(t, err)
	m := h3LateBodyResult(t, info)
	body := m["Observed DATA Byte Source"].(map[string]any)
	require.Equal(t, []uint64{2}, body["Parent PDUs"])
	require.Equal(t, uint64(2), body["Observed Payload Bytes"])
	require.Equal(t, []map[string]any{{"Packet": uint64(2), "Section": uint32(0), "Interface": 0, "Encapsulation": ""}}, body["Packet References"])
	fin := m["Observed FIN Source"].(map[string]any)
	require.Equal(t, uint64(3), fin["PDU ID"])
	require.Equal(t, uint64(4), m["Observed In PDU ID"])
	require.Equal(t, true, m["Body Coverage Complete"])
}

func TestHTTP3LateBodyKeepsOriginalOutOfOrderFINWitness(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	wire := append(h3LiteralFields([2]string{":status", "200"}, [2]string{"content-length", "1"}), h3Data([]byte("x"))...)
	_, _, err := h3BodyTestFeed(q, 1, 0, uint64(len(wire)), nil, true)
	require.NoError(t, err)
	_, _, err = h3BodyTestFeed(q, 1, 0, 0, wire, false)
	require.NoError(t, err)
	_, info, err := h3BodyTestFeed(q, 0, 0, 0, h3BodyRequest("HEAD", "-"), true)
	require.NoError(t, err)
	m := h3LateBodyResult(t, info)
	require.Equal(t, uint64(1), m["Observed FIN Source"].(map[string]any)["PDU ID"])
	require.Equal(t, []uint64{2}, m["Observed DATA Byte Source"].(map[string]any)["Parent PDUs"])
	require.Equal(t, true, m["Body Coverage Complete"])
}

func TestHTTP3LateBodyNoFINOrMissingSuffixCannotClaimCompleteCoverage(t *testing.T) {
	for _, gap := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-FIN-positive-content", true: "gap-before-FIN"}[gap], func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
			header := h3LiteralFields([2]string{":status", "200"}, [2]string{"content-length", "1"})
			_, _, err := h3BodyTestFeed(q, 1, 0, 0, header, false)
			require.NoError(t, err)
			if gap {
				_, _, err = h3BodyTestFeed(q, 1, 0, uint64(len(header)+3), nil, true)
			} else {
				_, _, err = h3BodyTestFeed(q, 1, 0, uint64(len(header)), h3Data([]byte("x")), false)
			}
			require.NoError(t, err)
			_, info, err := h3BodyTestFeed(q, 0, 0, 0, h3BodyRequest("HEAD", "-"), true)
			require.NoError(t, err)
			m := h3LateBodyResult(t, info)
			require.Equal(t, false, m["Body Coverage Complete"])
			if gap {
				require.Equal(t, "no-violation-observed", m["Observation Verdict"])
			} else {
				require.Equal(t, "violation-observed", m["Observation Verdict"])
			}
			require.NotEqual(t, "valid", m["Observation Verdict"])
		})
	}
}

func TestHTTP3LateBodyCounterProvenanceLimitDoesNotLoseConcreteViolation(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	header := h3LiteralFields([2]string{":status", "200"}, [2]string{"content-length", "65"})
	_, _, err := h3BodyTestFeed(q, 1, 0, 0, header, false)
	require.NoError(t, err)
	data := h3Data([]byte("x"))
	off := uint64(len(header))
	for i := 0; i < 65; i++ {
		_, _, err = h3BodyTestFeed(q, 1, 0, off, data, i == 64)
		require.NoError(t, err)
		off += uint64(len(data))
	}
	_, info, err := h3BodyTestFeed(q, 0, 0, 0, h3BodyRequest("HEAD", "-"), true)
	require.NoError(t, err)
	m := h3LateBodyResult(t, info)
	body := m["Observed DATA Byte Source"].(map[string]any)
	require.Equal(t, uint64(65), m["Observed DATA Bytes"])
	require.Len(t, body["Parent PDUs"].([]uint64), 64)
	require.Len(t, body["Packet References"].([]map[string]any), 64)
	require.Equal(t, false, body["References Complete"])
	require.Equal(t, false, m["Body Coverage Complete"])
	require.Equal(t, "violation-observed", m["Observation Verdict"])
	require.LessOrEqual(t, q.initialMemory(), int64(q.byteLimit))
	require.False(t, strings.Contains(m["Scope"].(string), "peer delivery verified"))
}

func TestHTTP3LateBodyQPACKCompletionNeverBorrowsEncoderBytesOrFIN(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	_, err := qpackRecoveryStream(t, q, 10, 0, 2, 0, h3ControlSETTINGS(1, 64, 7, 1))
	require.NoError(t, err)
	wire := append(h3Frame(1, []byte{2, 0, 0xd9, 0x80}), h3Data([]byte("x"))...)
	_, blocked, err := h3BodyTestFeed(q, 1, 0, 0, wire, true)
	require.NoError(t, err)
	require.Equal(t, true, blocked["QPACK Blocked"])
	_, err = qpackRecoveryStream(t, q, 13, 1, 7, 0, []byte{2, 0x3f, 0x21, 0x41, 'x', 1, 'y'})
	require.NoError(t, err)
	_, info, err := h3BodyTestFeed(q, 0, 0, 0, h3BodyRequest("HEAD", "-"), true)
	require.NoError(t, err)
	m := h3LateBodyResult(t, info)
	require.Equal(t, uint64(13), m["Header PDU ID"], "decoded header observation is separate from original compressed bytes")
	require.Equal(t, []uint64{11}, m["Observed DATA Byte Source"].(map[string]any)["Parent PDUs"])
	require.Equal(t, uint64(11), m["Observed FIN Source"].(map[string]any)["PDU ID"])
	require.Equal(t, uint64(14), m["Observed In PDU ID"])
	require.Equal(t, true, m["Body Coverage Complete"])
	require.Equal(t, "violation-observed", m["Observation Verdict"])
}

func TestHTTP3LateBodyResetReleasesPendingWitnessWithoutRewritingPublishedFields(t *testing.T) {
	for _, direction := range []int{0, 1} {
		t.Run(fmt.Sprintf("direction%d", direction), func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
			wire := append(h3ResponseHeaders(), h3Data([]byte("x"))...)
			_, response, err := h3BodyTestFeed(q, 1, 0, 0, wire, true)
			require.NoError(t, err)
			frozen, err := json.Marshal(response)
			require.NoError(t, err)
			require.NotNil(t, q.streams[0].h3.response)
			require.NotNil(t, q.streams[0].responseFIN)
			q.eventID = 2
			q.eventRefs = []PacketReference{{Number: 2}}
			request := h3BodyRequest("HEAD", "-")
			final := len(request)
			if direction == 1 {
				final = len(wire)
			}
			beforeMemory := q.initialMemory()
			reset := map[string]any{"Frame Type": "RESET_STREAM", "Stream ID": uint64(0), "Final Size": uint64(final), "Application Error": uint64(0x10c)}
			require.NoError(t, q.applyFrames(direction, quicSpaceApplication, []map[string]any{reset}, map[string]any{}, 1024))
			require.Nil(t, q.streams[0].h3.response)
			require.Nil(t, q.streams[0].responseFIN)
			require.Less(t, q.initialMemory(), beforeMemory)
			// Captured retransmissions stay within the declared QUIC final size.
			_, info, err := h3BodyTestFeed(q, 0, 0, 0, request, true)
			require.NoError(t, err)
			require.NotContains(t, info, "HTTP3 Late Associations")
			after, err := json.Marshal(response)
			require.NoError(t, err)
			require.Equal(t, frozen, after)
		})
	}
}
