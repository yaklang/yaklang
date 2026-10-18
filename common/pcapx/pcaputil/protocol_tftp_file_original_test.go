package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"io"
	"testing"
)

func TestTFTPOriginalObservedFile(t *testing.T) {
	alias := "../../bin-parser/testdata/protocol-corpus/captures/ndpi/ndpi-tftp.pcap"
	raw, err := trafficfixture.ReadFile(alias)
	require.NoError(t, err)
	require.Equal(t, "f64092f3a21e7be0d185c4208e97fcf7af23729a7b1efdd4d592444e315efae3", fmt.Sprintf("%x", sha256.Sum256(raw)))
	discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
		var files, events []*ProtocolEvent
		var stats ProtocolStats
		a, err := NewPacketAnalyzer(WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithProtocolTFTPFileAssembly(69, 65536), WithOnProtocolMessage(func(e *ProtocolEvent) {
			snap := *e
			snap.Raw = bytes.Clone(e.Raw)
			snap.Session = cloneSession(e.Session)
			snap.Fields = cloneSession(e.Fields)
			snap.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
			events = append(events, &snap)
			if _, ok := snap.Session["File Bytes"]; ok {
				files = append(files, &snap)
			}
			if observe && e.nativeTFTPObservation {
				tlsDTLSPoison(e, "redis")
			}
			if e.nativeTFTPObservation && snap.Error == "" {
				out, x := e.Decode()
				require.NoError(t, x)
				rocEqualFields(t, snap.Session, out["session"].(map[string]any))
			}
		}), WithOnProtocolStats(func(s ProtocolStats) { stats = s }))
		require.NoError(t, err)
		r, err := NewCaptureReader(bytes.NewReader(raw))
		require.NoError(t, err)
		for {
			b, ci, x := r.ReadPacketData()
			if x == io.EOF {
				break
			}
			require.NoError(t, x)
			require.NoError(t, a.Feed(b, ci, r.LinkType()))
			clear(b)
		}
		require.NoError(t, a.Close())
		require.NoError(t, a.Close())
		require.Len(t, files, 1)
		e := files[0]
		body := e.Session["File Bytes"].([]byte)
		require.Len(t, body, 24599)
		require.Equal(t, "39c9534e5fa6fecd3ac083ffd6256c2cc9a58f9f1058cb2e472d1782040231f9", fmt.Sprintf("%x", sha256.Sum256(body)))
		require.Equal(t, 24599, e.Session["File Byte Count"])
		require.Equal(t, true, e.Session["Transfer Complete"])
		out, err := e.Decode()
		require.NoError(t, err)
		rocEqualFields(t, e.Session, out["session"].(map[string]any))
		require.Zero(t, stats.BufferedBytes)
		require.Zero(t, stats.CallbackPanics)
		tftpFileSources(t, events)
		// This asserts the independently exported complete file and source links,
		// not a whole-capture semantic oracle for every historical packet.
	})
}
