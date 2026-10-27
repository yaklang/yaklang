package sharkcli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestPacketRowRefreshesAfterLateSSHAdmission(t *testing.T) {
	history, err := pcaputil.NewProtocolInspector(8, 1<<20)
	require.NoError(t, err)
	ref := pcaputil.PacketReference{Number: 1}
	banner := "SSH-2.0-Cisco-1.25\n"
	p := tcpPacket(t, 1, 100, banner, true, false, false, false)
	p.reference, p.history = ref, history
	p.application, p.evidence = "ssh", "validated protocol events"
	entry := packetEntry{packet: p}
	before := entry.row()
	require.Contains(t, before.Info, "no complete decoded message")
	s, err := pcaputil.NewProtocolSessionWithOptions(pcaputil.DefaultParserBudget(), pcaputil.WithSessionTransport("tcp"), pcaputil.WithSessionClientDirection(0))
	require.NoError(t, err)
	defer s.Close("late presentation test")
	s.Feed(0, time.Unix(1, 0), []byte("SSH-2.0-OpenSSH_Review\r\n"))
	r := s.Feed(1, time.Unix(1, 0), []byte(banner))
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	e.SourceBytes.PacketRefs = []pcaputil.PacketReference{ref}
	e.SourceBytes.CompletedIn = &ref
	history.OnEvent(e)
	after := entry.row()
	require.Equal(t, "SSH", after.Protocol)
	require.Contains(t, after.Info, "SSH-2.0-Cisco-1.25")
	require.Contains(t, after.Info, "softwareversion")
	require.NotContains(t, after.Info, "no complete decoded message")
	require.Equal(t, before.Number, after.Number)
	require.Equal(t, before.Length, after.Length)
	require.Equal(t, before.Source, after.Source)
	require.Equal(t, before.Destination, after.Destination)
	require.Equal(t, after, entry.row(), "stable redraw must not duplicate context")
	other := tcpPacket(t, 1, 100, banner, true, false, false, false)
	other.reference = ref
	other.reference.Domain.Interface = 1
	other.history, other.application, other.evidence = history, "ssh", "validated protocol events"
	neighbour := packetEntry{packet: other}
	require.Contains(t, neighbour.row().Info, "no complete decoded message", "another capture domain cannot acquire this late PDU")
}
