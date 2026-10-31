package pcaputil

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func nf5NativeWire(seq uint32) []byte {
	w := make([]byte, 72)
	w[1], w[3] = 5, 1
	binary.BigEndian.PutUint32(w[16:], seq)
	binary.BigEndian.PutUint16(w[22:], 0x8123)
	copy(w[24:], []byte{192, 0, 2, 9})
	copy(w[28:], []byte{198, 51, 100, 7})
	w[24+19], w[24+37], w[24+38], w[24+44], w[24+45] = 17, 0x12, 6, 24, 32
	return w
}

func TestNetFlowV5IndependentExportsDoNotInheritAcrossLossReuseOrReordering(t *testing.T) {
	a := nf9NativeParser(t)
	for i, seq := range []uint32{100, 800, 2, 0xffffffff, 0} {
		w := nf5NativeWire(seq)
		w[27] = byte(i + 1)
		e := &ProtocolEvent{Source: "192.0.2.1:53000", Destination: "198.51.100.1:2055", Transport: "udp", FlowID: 55, ResponseTo: 66, TransactionID: 77, Direction: 1}
		require.True(t, a.decodeControlDatagram(e, w, 53000, 2055, ""))
		a.emit(e)
		require.Equal(t, "netflow-v5", e.Protocol)
		require.Equal(t, "decoded", e.Status)
		require.Zero(t, e.FlowID)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.Direction)
		f := e.DisplayFields()
		require.Equal(t, []any{uint64(seq)}, f["cflow.sequence"])
		require.Equal(t, []any{uint64(2)}, f["cflow.samplingmode"])
		require.Equal(t, []any{uint64(0x123)}, f["cflow.samplerate"])
		require.Equal(t, []any{uint64(17)}, f["cflow.packets"])
		require.NotContains(t, f, "cflow.source_id")
		require.NotContains(t, f, "cflow.template_id")
		require.Equal(t, "192.0.2.1:53000", e.Source)
		e.Session = map[string]any{"Sequence Number": uint64(777)}
		require.Equal(t, []any{uint64(seq)}, e.DisplayFields()["cflow.sequence"])
		d, err := e.Decode()
		require.NoError(t, err)
		require.Equal(t, uint64(seq), d["fields"].(map[string]any)["Sequence Number"])
	}
}

func TestNetFlowV5HintsAndExplicitSelectorNeverBypassWireGrammar(t *testing.T) {
	a := nf9NativeParser(t)
	w := nf5NativeWire(1)
	for _, b := range [][]byte{w[:4], w[:71], append(append([]byte(nil), w...), 0)} {
		e := &ProtocolEvent{Transport: "udp"}
		require.False(t, a.decodeControlDatagram(e, b, 53000, 2055, ""))
		e = &ProtocolEvent{Transport: "udp"}
		require.True(t, a.decodeControlDatagram(e, b, 53000, 2055, "netflow-v5"))
		require.NotEqual(t, "decoded", e.Status)
	}
	e := &ProtocolEvent{Transport: "udp"}
	require.False(t, a.decodeControlDatagram(e, w, 40000, 9990, ""))
	require.True(t, a.decodeControlDatagram(e, w, 40000, 9990, "netflow-v5"))
	require.Equal(t, "decoded", e.Status)
	require.Equal(t, "explicit-decode-as", e.Admission)
	require.False(t, a.decodeNetFlowV5Datagram(&ProtocolEvent{}, w, 40000, 2055, "netflow-v9"))
	c := NewDefaultConfig()
	require.NoError(t, WithProtocolDecodeAs("udp", 9990, "netflow-v5")(c))
}

func TestNetFlowV5BadTimestampKeepsCompleteLiteralEvidence(t *testing.T) {
	a := nf9NativeParser(t)
	w := nf5NativeWire(1)
	binary.BigEndian.PutUint32(w[12:], 1000000000)
	e := &ProtocolEvent{Transport: "udp"}
	require.True(t, a.decodeControlDatagram(e, w, 53000, 2055, ""))
	a.emit(e)
	require.Equal(t, "netflow-v5", e.Protocol)
	require.Equal(t, "malformed", e.Status)
	require.Equal(t, false, e.Session["Export Timestamp Valid"])
	require.Equal(t, true, e.Session["Record Count Verified"])
	require.Contains(t, e.Error, "nanoseconds")
	require.Equal(t, "malformed", e.Completeness)
	require.Equal(t, []any{uint64(1000000000)}, e.DisplayFields()["cflow.unix_nsecs"])
	require.Equal(t, []any{"192.0.2.9"}, e.DisplayFields()["cflow.srcaddr"])
	require.True(t, e.DisplayFieldValues().Incomplete)
}
