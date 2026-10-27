package sharkcli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestStreamingJSONPublishesCaptureCloseDiagnosticsWithoutInventingPackets(t *testing.T) {
	request := "GET /unfinished HTTP/1.1\r\nHost: test\r\n\r\n"
	input := fixture(t, "unfinished-http.pcap",
		nativeOpeningPacket(t, 1, 99, 0, "", false, true, false),
		nativeOpeningPacket(t, 2, 100, 0, "GE", false, false, false),
		nativeOpeningPacket(t, 3, 104, 0, request[4:], false, false, false),
	)
	var output bytes.Buffer
	require.NoError(t, invoke(&output, "-r", input, "--json"))
	physical, publications := 0, 0
	decoder := json.NewDecoder(&output)
	for {
		var row map[string]any
		if err := decoder.Decode(&row); err == io.EOF {
			break
		} else {
			require.NoError(t, err)
		}
		if row["record_type"] != "protocol_publication" {
			physical++
			require.EqualValues(t, physical, row["number"])
			continue
		}
		publications++
		require.Equal(t, "capture_close", row["publication_phase"])
		for _, name := range []string{"number", "time", "length", "captured_length", "capture_domain", "source", "destination", "protocol", "pdus"} {
			require.NotContains(t, row, name, "an analysis publication is not a physical packet")
		}
		entries := row["published_pdus"].([]any)
		require.Len(t, entries, 1, "each bounded publication owns its separate field budget")
		pdu := entries[0].(map[string]any)
		require.Equal(t, "incomplete", pdu["status"])
		require.NotZero(t, pdu["id"])
		require.NotZero(t, pdu["tcp_flow_id"])
		require.NotEmpty(t, pdu["summary"])
	}
	require.Equal(t, 3, physical)
	require.NotZero(t, publications, "EOF gaps are already native diagnostics and must survive the JSON projection")
}

func TestClosePublicationMatchesNativeFieldsAndSourcePastOnePacketBudget(t *testing.T) {
	var packets [][]byte
	for i := 0; i < 140; i++ {
		for _, part := range []struct {
			sequence uint32
			text     string
			syn      bool
		}{{99, "", true}, {100, "GE", false}} {
			wire := nativeOpeningPacket(t, 1, part.sequence, 0, part.text, false, part.syn, false)
			decoded := gopacket.NewPacket(wire, layers.LinkTypeEthernet, gopacket.Default)
			ip := decoded.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
			tcp := decoded.Layer(layers.LayerTypeTCP).(*layers.TCP)
			tcp.SrcPort += layers.TCPPort(i)
			require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
			buffer := gopacket.NewSerializeBuffer()
			require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, decoded.Layer(layers.LayerTypeEthernet).(*layers.Ethernet), ip, tcp, gopacket.Payload([]byte(part.text))))
			packets = append(packets, buffer.Bytes())
		}
	}
	input := fixture(t, "many-unfinished.pcap", packets...)
	closing := false
	expected := map[uint64]map[string]any{}
	analyzer, err := pcaputil.NewPacketAnalyzer(pcaputil.WithCaptureReplayClock(), pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete: true, Stream: true}), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) {
		if closing {
			p := &capturedPacket{publicationPhase: "capture_close", observedFieldBudget: observedPacketFieldBytes - 1024}
			p.recordObservedPDU(e)
			require.Len(t, p.observedPDUs, 1)
			expected[e.ID] = p.observedPDUs[0]
		}
	}))
	require.NoError(t, err)
	src, err := openOffline(input, "")
	require.NoError(t, err)
	for {
		data, ci, err := src.reader.ReadPacketData()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		require.NoError(t, analyzer.Feed(data, ci, src.link))
	}
	closing = true
	require.NoError(t, analyzer.Close())
	src.Close()
	require.Greater(t, len(expected), 128, "the regression must exceed a single physical row's PDU budget")
	var output bytes.Buffer
	require.NoError(t, invoke(&output, "-r", input, "--json"))
	physical, seen := 0, map[uint64]bool{}
	decoder := json.NewDecoder(&output)
	for decoder.More() {
		var row map[string]json.RawMessage
		require.NoError(t, decoder.Decode(&row))
		if len(row["record_type"]) == 0 {
			physical++
			continue
		}
		var pdus []struct {
			ID uint64 `json:"id"`
		}
		require.NoError(t, json.Unmarshal(row["published_pdus"], &pdus))
		require.Len(t, pdus, 1)
		id := pdus[0].ID
		require.False(t, seen[id])
		seen[id] = true
		require.Contains(t, expected, id)
		encoded, err := json.Marshal([]map[string]any{expected[id]})
		require.NoError(t, err)
		require.JSONEq(t, string(encoded), string(row["published_pdus"]), "all native typed fields and original contributor metadata survive")
	}
	require.Equal(t, len(packets), physical)
	require.Len(t, seen, len(expected))

	// A consumer that stops draining must still be able to cancel a producer
	// blocked on the bounded final-publication queue.
	src, err = openOffline(input, "")
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, captureConfig{freezePacketFields: true, exportProtocolPublications: true}, false)
	for i := 0; i < len(packets); i++ {
		require.NotNil(t, <-s.packets)
	}
	stopped := make(chan struct{})
	go func() { s.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("capture-close backpressure prevented cancellation")
	}
}

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

func TestClosePublicationFilteringUsesItsOwnFieldsAndReportsBudgetRefusal(t *testing.T) {
	makePublication := func(budget int) *capturedPacket {
		p := &capturedPacket{publicationPhase: "capture_close", fieldsAtObservation: true, observedFieldBudget: budget}
		p.recordObservedPDU(&pcaputil.ProtocolEvent{ID: 42, Protocol: "bgp", Status: "incomplete", Session: map[string]any{"Type": uint64(4)}, SourceBytes: pcaputil.ByteSource{Kind: "reassembled", PacketRefs: []pcaputil.PacketReference{{Number: 88}}}})
		return p
	}
	for _, expression := range []string{"bgp.type == 4", "bgp.type == 1"} {
		filter, err := pcaputil.CompileDisplayFilter(expression)
		require.NoError(t, err)
		output, err := observedJSON(t, makePublication(observedPacketFieldBytes), filter)
		require.NoError(t, err)
		require.Equal(t, expression == "bgp.type == 4", len(output) > 0)
	}
	limited := makePublication(1)
	output, err := observedJSON(t, limited, nil)
	require.NoError(t, err)
	require.Contains(t, string(output), `"status":"limited"`)
	filter, err := pcaputil.CompileDisplayFilter("not bgp.type")
	require.NoError(t, err)
	_, err = observedJSON(t, limited, filter)
	require.ErrorContains(t, err, "capture_close protocol publication")
	s := &captureSession{packets: make(chan *capturedPacket, 1), done: make(chan struct{})}
	s.packets <- makePublication(observedPacketFieldBytes)
	close(s.packets)
	close(s.done)
	var plain bytes.Buffer
	require.NoError(t, runPlain(context.Background(), s, &plain, false))
	require.Empty(t, plain.String(), "plain packet counts and summaries cannot include analysis-only records")
}
