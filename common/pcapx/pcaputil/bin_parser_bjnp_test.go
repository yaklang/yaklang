package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBJNPDatagramAdmission(t *testing.T) {
	w := make([]byte, 16)
	copy(w, "BJNP")
	w[4], w[5] = 1, 1
	binary.BigEndian.PutUint32(w[6:10], 1)
	fields, err := decodeBJNPMessage(w, 0)
	require.NoError(t, err)
	require.Equal(t, "Discover", fields["Packet Name"])
	require.Equal(t, "request", fields["Role"])
	for _, mutated := range [][]byte{
		w[:15],
		append(bytes.Clone(w), 0),
		func() []byte { x := bytes.Clone(w); x[0] = 'X'; return x }(),
		func() []byte { x := bytes.Clone(w); x[4] = 0x02; return x }(),
		func() []byte { x := bytes.Clone(w); x[5] = 0xff; return x }(),
		func() []byte { x := bytes.Clone(w); x[9] = 0; return x }(),
		func() []byte { x := bytes.Clone(w); x[11] = 1; return x }(),
		func() []byte { x := bytes.Clone(w); x[15] = 1; return x }(),
	} {
		require.Falsef(t, validBJNPMessage(mutated), "%x", mutated)
	}
}

func TestWinlab5013BJNPEightDatagrams(t *testing.T) {
	events := replayWinlab5013Protocols(t, "06-bjnp.pcapng")
	require.Len(t, events, 8)
	want := []string{"Discover", "Discover", "Get Printer Identity", "Get Printer Identity", "Print Job Details", "Print Job Details", "Get Printer Status", "Get Printer Status"}
	for i, event := range events {
		require.Equal(t, "bjnp", event.Protocol, "datagram %d", i)
		require.Equal(t, "decoded", event.Status, "datagram %d: %s", i, event.Error)
		require.Equal(t, want[i], event.Fields["Packet Name"])
	}
	require.Equal(t, []byte("LAB-PRINTER"), events[1].Fields["Payload"])
	require.Equal(t, []byte("SN:5013"), events[3].Fields["Payload"])
	require.Equal(t, []byte("job=7"), events[4].Fields["Payload"])
	require.Equal(t, []byte("accepted"), events[5].Fields["Payload"])
	require.Equal(t, []byte("idle"), events[7].Fields["Payload"])
}
