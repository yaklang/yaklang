package sharkcli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestStreamingJSONKeepsPDUsReleasedFromAnEarlierTCPPacket(t *testing.T) {
	first := "GET /first HTTP/1.1\r\nHost: test\r\n\r\n"
	missing := "GET /missing HTTP/1.1\r\nHost: test\r\n\r\n"
	cached := "GET /cached HTTP/1.1\r\nHost: test\r\n\r\n"
	src, err := openOffline(fixture(t, "late-http.pcap",
		tcpPacket(t, 1, 100, "", false, true, false, false).data,
		tcpPacket(t, 2, 101, first, false, false, false, false).data,
		tcpPacket(t, 3, 101+uint32(len(first)+len(missing)), cached, false, false, false, false).data,
		tcpPacket(t, 4, 101+uint32(len(first)), missing, false, false, false, false).data,
	), "")
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, captureConfig{freezePacketFields: true}, false)
	var output bytes.Buffer
	require.NoError(t, runPlain(context.Background(), s, &output, true))
	decoder := json.NewDecoder(&output)
	var rows []map[string]any
	for decoder.More() {
		var row map[string]any
		require.NoError(t, decoder.Decode(&row))
		rows = append(rows, row)
	}
	require.Len(t, rows, 4, "one JSON row per original physical packet")
	ids := map[float64]bool{}
	var published []any
	for _, row := range rows {
		for _, key := range []string{"pdus", "published_pdus"} {
			entries, _ := row[key].([]any)
			for _, entry := range entries {
				pdu := entry.(map[string]any)
				if pdu["protocol"] == "http" && pdu["status"] == "decoded" {
					ids[pdu["id"].(float64)] = true
				}
			}
		}
		if row["number"] == float64(4) {
			published, _ = row["published_pdus"].([]any)
		}
	}
	require.Len(t, ids, 3, "a cached complete request must remain available after its contributor row was exported")
	require.Len(t, published, 1)
	source := published[0].(map[string]any)["byte_source"].(map[string]any)
	refs := source["PacketRefs"].([]any)
	require.Len(t, refs, 1)
	require.Equal(t, float64(3), refs[0].(map[string]any)["Number"], "publication on frame 4 cannot rewrite original byte ownership")
	require.NotEmpty(t, rows[3]["published_pdu_scope"])
}

func TestPublishedPDUsKeepOwnedValuesWithoutChangingPacketFilter(t *testing.T) {
	frame := udpPacket(t, 9000, []byte("physical packet"))
	p := &capturedPacket{number: 1, data: frame, link: layers.LinkTypeEthernet, fieldsAtObservation: true, observedFieldBudget: observedPacketFieldBytes - 1024}
	e := &pcaputil.ProtocolEvent{ID: 42, Protocol: "bgp", Status: "decoded", Completeness: "message", Session: map[string]any{"Type": uint64(4)}, SourceBytes: pcaputil.ByteSource{Kind: "reassembled", PacketRefs: []pcaputil.PacketReference{{Number: 88}}}}
	p.recordPublishedPDU(e)
	require.Empty(t, p.observedPDUs)
	require.Empty(t, p.observedFields)
	before, err := observedJSON(t, p, nil)
	require.NoError(t, err)
	e.Session["Type"] = uint64(1)
	e.SourceBytes.PacketRefs[0].Number = 99
	after, err := observedJSON(t, p, nil)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after), "published metadata owns its original values and contributor identity")
	filter, err := pcaputil.CompileDisplayFilter("bgp.type == 4")
	require.NoError(t, err)
	filtered, err := observedJSON(t, p, filter)
	require.NoError(t, err)
	require.Empty(t, filtered, "another packet's published PDU cannot relabel or match this physical packet")
}

func TestPublishedPDUsShareTheBoundedObservationBudget(t *testing.T) {
	p := &capturedPacket{observedFieldBudget: 1}
	e := &pcaputil.ProtocolEvent{ID: 42, Protocol: "bgp", Status: "decoded", Session: map[string]any{"Type": uint64(4)}}
	p.recordPublishedPDU(e)
	require.True(t, p.observedFieldsLimited)
	require.Empty(t, p.observedPDUs)
	require.Len(t, p.publishedPDUs, 1)
	require.Equal(t, "limited", p.publishedPDUs[0]["status"])
	filter, err := pcaputil.CompileDisplayFilter("not bgp.type")
	require.NoError(t, err)
	require.False(t, filter.EvaluateSnapshots(p.observedFields).Complete, "a shared budget refusal cannot prove missing fields absent")
	for i := 0; i < 1000; i++ {
		p.recordPublishedPDU(e)
		p.recordObservedPDU(e)
	}
	require.Len(t, p.publishedPDUs, 1)
	require.Empty(t, p.observedPDUs)
}
