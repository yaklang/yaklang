package sharkcli

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"testing"
)

func TestRetainedMalformedDetailsVisible(t *testing.T) {
	history, err := pcaputil.NewProtocolInspector(4, 4096)
	require.NoError(t, err)
	// A retained malformed observation has useful raw evidence even when its
	// structured detail cannot be decoded. A genuinely absent ID stays absent.
	bad := &pcaputil.ProtocolEvent{ID: 7, Protocol: "quic", Transport: "tcp", Status: "malformed", Error: "MalformedMessage: invalid observed QUIC header", Raw: []byte{0, 0, 0}, Length: 3, Rule: "application-layer.quic", Entry: "QUIC"}
	history.OnEvent(bad)
	normal := &pcaputil.ProtocolEvent{ID: 8, Protocol: "quic", Transport: "tcp", Status: "context-required", Error: "Encrypted: keys unavailable", Raw: []byte{64}, Length: 1}
	history.OnEvent(normal)
	p := &capturedPacket{history: history, eventIDs: []uint64{7, 8, 999999}}
	es := p.protocolEvents()
	require.Len(t, es, 2)
	require.Equal(t, uint64(7), es[0].ID)
	require.Equal(t, bad.Raw, es[0].Raw)
	require.Equal(t, bad.Error, es[0].Error)
	require.Equal(t, "malformed", es[0].Status)
	require.Equal(t, uint64(8), es[1].ID)
	require.Equal(t, normal.Error, es[1].Error)
	es[0].Raw[0] = 255
	again := p.protocolEvents()
	require.Equal(t, byte(0), again[0].Raw[0])
	retained := retainedStreamEvents(history, &streamSnapshot{})
	require.Len(t, retained, 2)
	require.Equal(t, bad.Error, retained[0].Error)
}
