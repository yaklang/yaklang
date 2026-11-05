package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTP3LateRequestAddsImmutableResponseAssociation(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	response, err := qpackRecoveryStream(t, q, 11, 1, 0, 0, h3ResponseHeaders())
	require.NoError(t, err)
	before, err := json.Marshal(response)
	require.NoError(t, err)
	request, err := qpackRecoveryStream(t, q, 12, 0, 0, 0, h3RequestHeaders())
	require.NoError(t, err)
	after, err := json.Marshal(response)
	require.NoError(t, err)
	require.Equal(t, before, after, "already published response fields must remain immutable")
	links, ok := request["HTTP3 Late Associations"].([]map[string]any)
	require.True(t, ok, "a later association must be explicit")
	require.Len(t, links, 1)
	require.Equal(t, uint64(11), links[0]["Response PDU ID"])
	require.Equal(t, uint64(12), links[0]["Request PDU ID"])
	require.Equal(t, uint64(0), links[0]["Stream ID"])
}

func TestHTTP3LateAssociationDoesNotRetainProofForAnInOrderResponse(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	_, err := qpackRecoveryStream(t, q, 11, 0, 0, 0, h3RequestHeaders())
	require.NoError(t, err)
	info, err := qpackRecoveryStream(t, q, 12, 1, 0, 0, h3ResponseHeaders())
	require.NoError(t, err)
	require.NotContains(t, info, "HTTP3 Late Associations")
	require.Nil(t, q.streams[0].h3.response)
}

func TestHTTP3LateAssociationKeepsPrivateOriginAndPublishesOnce(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	response, err := qpackRecoveryStream(t, q, 11, 1, 0, 0, h3ResponseHeaders())
	require.NoError(t, err)
	header := response["HTTP3 Frames"].([]map[string]any)[0]
	source := header["Byte Source"].(map[string]any)
	source["Parent PDUs"].([]uint64)[0] = 999
	source["Packet References"].([]map[string]any)[0]["Packet"] = uint64(999)
	requestWire := h3RequestHeaders()
	request, err := qpackRecoveryStream(t, q, 12, 0, 0, 0, requestWire)
	require.NoError(t, err)
	link := request["HTTP3 Late Associations"].([]map[string]any)[0]
	require.Equal(t, []uint64{11}, link["Response Header Origin PDUs"])
	require.Equal(t, []map[string]any{{"Packet": uint64(11), "Section": uint32(0), "Interface": 7, "Encapsulation": "/vlan:91"}}, link["Response Header Packet References"])
	require.Equal(t, true, link["Response Header References Complete"])
	require.Empty(t, q.streams[0].h3.response.refs)
	require.Empty(t, q.streams[0].h3.response.origins)
	link["Response PDU ID"] = uint64(777)
	duplicate, err := qpackRecoveryStream(t, q, 13, 0, 0, 0, requestWire)
	require.NoError(t, err)
	require.NotContains(t, duplicate, "HTTP3 Late Associations")
}

func TestHTTP3LateAssociationUsesFinalResponseAndSameStream(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	informational := h3LiteralFields([2]string{":status", "103"})
	_, err := qpackRecoveryStream(t, q, 10, 1, 0, 0, informational)
	require.NoError(t, err)
	require.Nil(t, q.streams[0].h3.response)
	_, err = qpackRecoveryStream(t, q, 11, 1, 0, uint64(len(informational)), h3ResponseHeaders())
	require.NoError(t, err)
	other, err := qpackRecoveryStream(t, q, 12, 0, 4, 0, h3RequestHeaders())
	require.NoError(t, err)
	require.NotContains(t, other, "HTTP3 Late Associations")
	request, err := qpackRecoveryStream(t, q, 13, 0, 0, 0, h3RequestHeaders())
	require.NoError(t, err)
	link := request["HTTP3 Late Associations"].([]map[string]any)[0]
	require.Equal(t, uint64(11), link["Response PDU ID"])
	require.Equal(t, uint64(0), link["Stream ID"])
}

func TestHTTP3LateAssociationKeepsQPACKObservationAndWireOriginsSeparate(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	_, err := qpackRecoveryStream(t, q, 10, 0, 2, 0, h3ControlSETTINGS(1, 64, 7, 1))
	require.NoError(t, err)
	blocked, err := qpackRecoveryStream(t, q, 11, 1, 0, 0, h3Frame(1, []byte{2, 0, 0xd9, 0x80}))
	require.NoError(t, err)
	require.Equal(t, true, blocked["QPACK Blocked"])
	resumed, err := qpackRecoveryStream(t, q, 13, 1, 7, 0, []byte{2, 0x3f, 0x21, 0x41, 'x', 1, 'y'})
	require.NoError(t, err)
	require.Len(t, resumed["QPACK Resumed Streams"].([]map[string]any), 1)
	request, err := qpackRecoveryStream(t, q, 14, 0, 0, 0, h3RequestHeaders())
	require.NoError(t, err)
	link := request["HTTP3 Late Associations"].([]map[string]any)[0]
	require.Equal(t, uint64(13), link["Response PDU ID"], "PDU that actually observed the decompressed response header")
	require.Equal(t, []uint64{11}, link["Response Header Origin PDUs"], "compressed header bytes belong to the older response observation")
	require.Equal(t, []map[string]any{{"Packet": uint64(11), "Section": uint32(0), "Interface": 7, "Encapsulation": "/vlan:91"}}, link["Response Header Packet References"])
}

func TestHTTP3LateAssociationDoesNotReviveClosedRetiredOrResetStreams(t *testing.T) {
	for _, state := range []string{"closed", "retired", "reset"} {
		t.Run(state, func(t *testing.T) {
			q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
			_, err := qpackRecoveryStream(t, q, 11, 1, 0, 0, h3ResponseHeaders())
			require.NoError(t, err)
			switch state {
			case "closed":
				q.closed = true
			case "retired":
				q.retired = true
			case "reset":
				q.streams[0].resetDirection[0] = true
			}
			info, _ := qpackRecoveryStream(t, q, 12, 0, 0, 0, h3RequestHeaders())
			require.NotContains(t, info, "HTTP3 Late Associations")
		})
	}
}

func TestHTTP3LateAssociationReferenceBudgetIsExplicit(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3", eventID: 11}
	for i := 0; i < 128; i++ {
		q.eventRefs = append(q.eventRefs, PacketReference{Number: uint64(i + 1)})
	}
	fr := map[string]any{"Frame Type": "STREAM", "Stream ID": uint64(0), "Offset": uint64(0), "Stream Data": h3ResponseHeaders(), "FIN": false}
	info := map[string]any{"Frames": []map[string]any{fr}}
	require.NoError(t, q.applyFrames(1, quicSpaceApplication, []map[string]any{fr}, info, 128))
	require.Len(t, q.streams[0].h3.response.refs, 64)
	request, err := qpackRecoveryStream(t, q, 12, 0, 0, 0, h3RequestHeaders())
	require.NoError(t, err)
	link := request["HTTP3 Late Associations"].([]map[string]any)[0]
	require.Equal(t, false, link["Response Header References Complete"])
	require.Len(t, link["Response Header Packet References"].([]map[string]any), 64)
}

func TestHTTP3LateAssociationNativeProtectedPacketsAndSealedQueries(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, cipher := range []string{quicAEADAES128GCM, quicAEADAES256GCM, quicAEADChaCha20Poly1305} {
			t.Run(fmt.Sprintf("v%x/%s", version, cipher), func(t *testing.T) {
				a := earlyRecoveryParser(t)
				q, key := quicConformanceContext(t, version, cipher)
				q.alpn, q.byteLimit = "h3", 65536
				domain := CaptureDomain{Interface: 7, Encapsulation: "/vlan:91"}
				f := earlyRecoveryOwner(a, q, domain)
				base := earlyRecoveryBase(f, domain, 11)
				base.Source, base.Destination = base.Destination, base.Source
				first, ok := a.decodeQUICDatagram(base, quicConformancePacket(t, key, "1-RTT", 1, 0, quicStreamFrame(0, 0, false, h3ResponseHeaders())))
				require.True(t, ok)
				require.Len(t, first, 1)
				require.Equal(t, "decoded", first[0].Status)
				a.emit(first[0])
				history, err := NewProtocolInspector(8, 1<<20)
				require.NoError(t, err)
				history.OnEvent(first[0])
				oldView, err := history.Details(first[0].ID)
				require.NoError(t, err)
				responseRef := first[0].SourceBytes.PacketRefs[0]
				revision := history.PacketQueryRevision(responseRef, nil, false)
				frozen, err := first[0].Decode()
				require.NoError(t, err)
				responseID := first[0].ID
				first[0].Raw = bytes.Repeat([]byte{0}, len(first[0].Raw))
				first[0].SourceBytes.PacketRefs[0].Number = 999
				second, ok := a.decodeQUICDatagram(earlyRecoveryBase(f, domain, 12), quicConformancePacket(t, key, "1-RTT", 2, 0, quicStreamFrame(0, 0, false, h3RequestHeaders())))
				require.True(t, ok)
				require.Len(t, second, 1)
				require.Equal(t, "decoded", second[0].Status)
				a.emit(second[0])
				history.OnEvent(second[0])
				require.NotEqual(t, revision, history.PacketQueryRevision(responseRef, nil, false), "old packet caches must see the later association")
				view, err := history.Details(responseID)
				require.NoError(t, err)
				require.NotContains(t, oldView.Session, "HTTP3 Resolved Associations")
				require.Zero(t, view.ResponseTo)
				require.Equal(t, []any{second[0].ID}, view.DisplayFieldValues().FieldMap()["pdu.resolved_response_to"])
				require.Equal(t, responseRef, view.SourceBytes.PacketRefs[0], "context is not a byte contributor")
				view.Session["HTTP3 Resolved Associations"].([]map[string]any)[0]["Request PDU ID"] = uint64(999)
				require.Equal(t, []any{second[0].ID}, view.DisplayFieldValues().FieldMap()["pdu.resolved_response_to"])
				metadata := history.Rows("", 0)[0]
				metadata.Protocol = "doq"
				require.Equal(t, []any{"http3"}, metadata.DisplayFieldValues().FieldMap()["protocol"])
				link := second[0].Session["HTTP3 Late Associations"].([]map[string]any)[0]
				require.Equal(t, responseID, link["Response PDU ID"])
				require.Equal(t, []uint64{responseID}, link["Response Header Origin PDUs"])
				require.Zero(t, first[0].ResponseTo, "the original response remains immutable")
				require.Zero(t, second[0].ResponseTo, "a request must not impersonate a response")
				snapshot := second[0].DisplayFieldValues()
				require.Equal(t, []any{responseID}, snapshot.FieldMap()["http3.late_association.response_pdu"])
				for _, value := range snapshot.Values {
					if value.Name == "http3.late_association.response_pdu" {
						require.Nil(t, value.Range)
						require.Equal(t, "context", value.Origin)
					}
				}
				link["Response PDU ID"] = uint64(777)
				second[0].Protocol = "doq"
				second[0].Session["HTTP3"] = false
				require.Equal(t, []any{responseID}, second[0].DisplayFieldValues().FieldMap()["http3.late_association.response_pdu"])
				decoded, err := first[0].Decode()
				require.NoError(t, err)
				require.Equal(t, frozen, decoded)
			})
		}
	}
}

func TestHTTP3LateHeaderCompletionKeepsRequestTransactionAnchor(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	_, err := qpackRecoveryStream(t, q, 11, 1, 0, 0, h3ResponseHeaders())
	require.NoError(t, err)
	wire := h3RequestHeaders()
	partial, err := qpackRecoveryStream(t, q, 12, 0, 0, 0, wire[:3])
	require.NoError(t, err)
	require.NotContains(t, partial, "HTTP3 Late Associations")
	complete, err := qpackRecoveryStream(t, q, 13, 0, 0, 3, wire[3:])
	require.NoError(t, err)
	links, ok := complete["HTTP3 Late Associations"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, links, 1)
	require.Equal(t, uint64(11), links[0]["Response PDU ID"])
	require.Equal(t, uint64(12), links[0]["Request PDU ID"])
	require.Equal(t, uint64(13), links[0]["Request Headers PDU ID"])
}

func TestHTTP3RejectedRequestHeaderCannotClaimLateResponse(t *testing.T) {
	q := &binQUIC{native: true, byteLimit: 65536, alpn: "h3"}
	_, err := qpackRecoveryStream(t, q, 11, 1, 0, 0, h3ResponseHeaders())
	require.NoError(t, err)
	bad, err := qpackRecoveryStream(t, q, 12, 0, 0, 0, h3LiteralFields([2]string{":status", "200"}))
	require.True(t, isH3FieldStreamError(err))
	require.NotContains(t, bad, "HTTP3 Late Associations")
	require.Zero(t, q.streams[0].requestPDU)
}
