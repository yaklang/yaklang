package sharkcli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func observedJSON(t *testing.T, p *capturedPacket, filter *pcaputil.DisplayFilter) ([]byte, error) {
	t.Helper()
	s := &captureSession{packets: make(chan *capturedPacket, 1), done: make(chan struct{})}
	s.packets <- p
	close(s.packets)
	close(s.done)
	var out bytes.Buffer
	err := runPlainDisplay(context.Background(), s, &out, true, filter)
	return out.Bytes(), err
}

func TestStreamingPacketFieldsDoNotDependOnLaterHistoryOrConsumerSpeed(t *testing.T) {
	history, err := pcaputil.NewProtocolInspector(1, 1<<20)
	require.NoError(t, err)
	ref := pcaputil.PacketReference{Number: 1}
	frame := udpPacket(t, 9000, []byte("test"))
	p := &capturedPacket{number: 1, data: frame, ci: gopacket.CaptureInfo{Length: len(frame), CaptureLength: len(frame)}, link: layers.LinkTypeEthernet, history: history, reference: ref, physicalRecord: true, fieldsAtObservation: true, observedFieldBudget: observedPacketFieldBytes - 1024}
	// An empty packet observation is stable even when unrelated future PDUs
	// begin evicting history. It must not acquire a spurious lost-PDU marker.
	before, err := observedJSON(t, p, nil)
	require.NoError(t, err)
	for id := uint64(2); id < 6; id++ {
		history.OnEvent(&pcaputil.ProtocolEvent{ID: id, Protocol: "bgp", Status: "decoded", Session: map[string]any{"Type": uint64(4)}})
	}
	require.Positive(t, history.Evicted())
	after, err := observedJSON(t, p, nil)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
	// Real observed scalar values and physical references also remain owned.
	e := &pcaputil.ProtocolEvent{ID: 11, Protocol: "bgp", Status: "decoded", Completeness: "message", Session: map[string]any{"Type": uint64(4)}, SourceBytes: pcaputil.ByteSource{Kind: "captured", PacketRefs: []pcaputil.PacketReference{ref}}}
	p.recordObservedPDU(e)
	before, err = observedJSON(t, p, nil)
	require.NoError(t, err)
	e.Session["Type"] = uint64(1)
	e.SourceBytes.PacketRefs[0].Number = 999
	e.Completeness = "limited"
	after, err = observedJSON(t, p, nil)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
	filter, err := pcaputil.CompileDisplayFilter("bgp.type == 4")
	require.NoError(t, err)
	filtered, err := observedJSON(t, p, filter)
	require.NoError(t, err)
	require.Equal(t, string(before), string(filtered))
	var row map[string]any
	require.NoError(t, json.Unmarshal(after, &row))
	require.Equal(t, "observed_at_packet", row["pdu_scope"])
}

func TestStreamingPacketFieldBudgetIsExplicitAndCannotProveAbsence(t *testing.T) {
	frame := udpPacket(t, 9000, []byte("test"))
	p := &capturedPacket{number: 1, data: frame, link: layers.LinkTypeEthernet, fieldsAtObservation: true, observedFieldBudget: 1}
	e := &pcaputil.ProtocolEvent{ID: 11, Protocol: "bgp", Status: "decoded", Completeness: "message", Session: map[string]any{"Type": uint64(4)}}
	for i := 0; i < 1000; i++ {
		p.recordObservedPDU(e)
	}
	require.Len(t, p.observedPDUs, 1)
	require.Len(t, p.observedFields, 1)
	require.Equal(t, "limited", p.observedFields[0].Completeness)
	filter, err := pcaputil.CompileDisplayFilter("not bgp.type")
	require.NoError(t, err)
	_, err = observedJSON(t, p, filter)
	require.ErrorContains(t, err, "display filter unavailable")
}
