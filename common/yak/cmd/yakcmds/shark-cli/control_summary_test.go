package sharkcli

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestMailRTSPAcceptedSummaryUsesSpecificObservedFields(t *testing.T) {
	for _, tc := range []struct {
		protocol string
		session  map[string]any
		want     string
	}{
		{"imap", map[string]any{"Profile": "imap-sasl-observed", "SASL Stage": "response", "Mechanism": "PLAIN", "Tag": "a"}, "IMAP SASL PLAIN · response · tag=a"},
		{"imap", map[string]any{"Profile": "imap-idle-observed", "IDLE Stage": "done", "Tag": "a"}, "IMAP IDLE · done · tag=a"},
		{"imap", map[string]any{"Profile": "imap-namespace-observed", "Namespace Stage": "command", "Tag": "n"}, "IMAP NAMESPACE · command · tag=n"},
		{"rtsp", map[string]any{"Packet Name": "FLUSH", "CSeq": uint32(7), "URI": "rtsp://example.test/a", "RTP Info": []any{map[string]any{"seq": uint64(0), "rtptime": uint64(4294967295)}}}, "RTSP FLUSH CSeq=7 · rtsp://example.test/a · seq=0 · rtptime=4294967295"},
		{"rtsp", map[string]any{"Packet Name": "Response", "CSeq": uint32(7), "Status": 200, "In Reply To": "FLUSH"}, "RTSP 200 CSeq=7 · reply to FLUSH"},
	} {
		e := &pcaputil.ProtocolEvent{Protocol: tc.protocol, Status: "decoded", Session: tc.session}
		require.Equal(t, tc.want, applicationEventSummary(e))
		e.Status, e.Error = "context-required", "missing peer bytes"
		_, ok := mailRTSPEventSummary(e)
		require.False(t, ok, "a failed message cannot get an accepted summary")
	}
}

func TestLateReassemblyNoticeSurvivesNarrowInfoAndRedraw(t *testing.T) {
	history, err := pcaputil.NewProtocolInspector(8, 1<<20)
	require.NoError(t, err)
	first, last := pcaputil.PacketReference{Number: 1}, pcaputil.PacketReference{Number: 2}
	p := tcpPacket(t, 1, 100, "* NAMES", true, false, false, false)
	p.reference, p.history = first, history
	s, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("test")
	ts := time.Unix(1, 0)
	s.Feed(1, ts, []byte("* OK IMAP4rev1 ready\r\n"))
	s.Feed(0, ts, []byte("n NAMESPACE\r\n"))
	r := s.Feed(1, ts, []byte("* NAMESPACE ((\"\" \"/\")) NIL NIL\r\n"))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	e := r.Events[0]
	e.SourceBytes.PacketRefs = []pcaputil.PacketReference{first, last}
	e.SourceBytes.CompletedIn = &last
	history.OnEvent(e)
	entry := packetEntry{packet: p}
	row := entry.row()
	badge := fmt.Sprintf("Reassembled PDU #%d · complete in frame #2", e.ID)
	require.True(t, strings.HasPrefix(row.Info, badge))
	require.Contains(t, clipText(row.Info, 70), "complete in frame #2")
	require.Equal(t, row, entry.row(), "stable redraw does not duplicate the reassembly notice")
	final := tcpPacket(t, 2, 107, "PACE", true, false, false, false)
	final.reference, final.history = last, history
	finalEntry := packetEntry{packet: final}
	require.NotContains(t, finalEntry.row().Info, "complete in frame #2", "the actual completion frame needs no contributor warning")
}
