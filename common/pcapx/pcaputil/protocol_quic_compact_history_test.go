package pcaputil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func compactQUICHistoryEvent(t *testing.T, size int, deferred bool) *ProtocolEvent {
	t.Helper()
	wire := bytes.Repeat([]byte{0xaa}, size)
	wire[0] = 0x40
	event := &ProtocolEvent{ID: 1, Protocol: "quic", Transport: "udp", nativeQUICObservation: true, nativeQUICWire: true}
	analyzer := &binParser{config: BinParserConfig{Deferred: deferred}}
	analyzer.finishProtocolDatagram(event, wire, nil, nil)
	require.Empty(t, event.Error)
	require.Empty(t, event.semanticFields, "accepted ciphertext must not retain a boxed scalar per octet")
	require.Equal(t, wire, event.nativeQUICWireBytes)
	return event
}

func TestNativeQUICAcceptedWireHasLinearRetainedCostAndLegacyQueryShape(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		event := compactQUICHistoryEvent(t, 1<<20, deferred)
		cost := event.RetainedBytes()
		require.GreaterOrEqual(t, cost, 2*len(event.Raw), "both public and private owned bytes must be charged")
		require.Less(t, cost, 3*len(event.Raw)+4096, "the wire tree must not exhaust a64MiB inspector with a1MiB envelope")
		fields, err := event.GetFields()
		require.NoError(t, err)
		require.Equal(t, byte(0x40), fields["First Byte"])
		payload, ok := fields["Protected Payload"].([]any)
		require.True(t, ok, "GetFields preserves the established boxed byte leaf API")
		require.Len(t, payload, len(event.Raw)-1)
		for i, value := range payload {
			if b, ok := value.(byte); !ok || b != 0xaa {
				t.Fatalf("protected byte%d changed value or scalar type: %T", i, value)
			}
		}
		event.Raw[0], event.Raw[1] = 0xff, 0
		event.Fields = map[string]any{"First Byte": byte(0)}
		event.Structured = map[string]any{"fields": event.Fields}
		event.Session = map[string]any{"First Byte": byte(1)}
		payload[0] = byte(0)
		again, err := event.GetFields()
		require.NoError(t, err)
		require.Equal(t, byte(0x40), again["First Byte"])
		require.Equal(t, byte(0xaa), again["Protected Payload"].([]any)[0], "queries and public caches cannot rewrite accepted wire")
	}
}

func TestNativeQUICCompactHistoryOwnsItsBytesAndKeepsMetadataSmall(t *testing.T) {
	event := compactQUICHistoryEvent(t, 4096, true)
	inspector, err := NewBinParserInspector(8, 64<<20)
	require.NoError(t, err)
	inspector.OnEvent(event)
	event.Raw[1], event.nativeQUICWireBytes[1] = 0, 0
	detail, err := inspector.Details(1)
	require.NoError(t, err)
	fields, err := detail.GetFields()
	require.NoError(t, err)
	require.Equal(t, byte(0xaa), fields["Protected Payload"].([]any)[0])
	detail.nativeQUICWireBytes[1] = 0
	fields["Protected Payload"].([]any)[0] = byte(0)
	again, err := inspector.Details(1)
	require.NoError(t, err)
	fields, err = again.GetFields()
	require.NoError(t, err)
	require.Equal(t, byte(0xaa), fields["Protected Payload"].([]any)[0])
	rows := inspector.Rows("", 0)
	rowsAfter, _, _ := inspector.RowsAfter(0, 8, "", 0)
	for _, row := range append(rows, rowsAfter...) {
		require.Nil(t, row.nativeQUICWireBytes)
		require.Nil(t, row.Raw)
		require.Nil(t, row.semanticFields)
	}
	// Exercise the generic TCP metadata view without claiming QUIC-over-TCP
	// admission. Inspector callers can supply their own transport metadata.
	tcpMetadata := *compactQUICHistoryEvent(t, 64, true)
	tcpMetadata.ID, tcpMetadata.TCPFlowID, tcpMetadata.Transport = 2, 7, "tcp"
	inspector.OnEvent(&tcpMetadata)
	tcpRows := inspector.TCPFlowRows(CaptureDomain{}, 7)
	require.Len(t, tcpRows, 1)
	require.Nil(t, tcpRows[0].nativeQUICWireBytes)
	require.Nil(t, tcpRows[0].Raw)
}
