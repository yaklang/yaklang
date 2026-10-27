package sharkcli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// This opt-in audit compares the actual page store with a separate native
// callback replay. Its external source must already have a wire/oracle proof;
// this test proves page projection, not an independent TCP implementation.
// No capture, key, raw preview or generated audit belongs in the repository.
func TestNativePreviewExternalCaptureAudit(t *testing.T) {
	path := os.Getenv("SHARK_PREVIEW_AUDIT_CAPTURE")
	if path == "" {
		t.Skip("set SHARK_PREVIEW_AUDIT_CAPTURE and SHARK_PREVIEW_AUDIT_OUTPUT for a bounded external TCP audit")
	}
	output := os.Getenv("SHARK_PREVIEW_AUDIT_OUTPUT")
	require.NotEmpty(t, output)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.LessOrEqual(t, info.Size(), int64(32<<20), "this complete-byte audit has an explicit 32 MiB input scope")
	wire, err := os.ReadFile(path)
	require.NoError(t, err)
	type flowEvidence struct {
		Endpoints [2]string
		Domain    pcaputil.CaptureDomain
		Packets   uint64
		First     time.Time
		Last      time.Time
		Pending   bool
		Data      [2][]byte
	}
	flows := map[uint64]*flowEvidence{}
	owners := map[uint64]uint64{}
	retained := 0
	err = pcaputil.ReplayPcap(bytes.NewReader(wire), pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete: true, Stream: true}),
		pcaputil.WithOnProtocolMessage(func(*pcaputil.ProtocolEvent) {}),
		pcaputil.WithOnTrafficFlowCreated(func(f *pcaputil.TrafficFlow) {
			require.NotContains(t, flows, f.Index)
			require.Less(t, len(flows), 64, "bounded audit flow scope")
			flows[f.Index] = &flowEvidence{Endpoints: [2]string{f.ClientConn.LocalAddr().String(), f.ServerConn.LocalAddr().String()}, Domain: f.CaptureDomain()}
		}), pcaputil.WithOnTrafficFlowPacket(func(f *pcaputil.TrafficFlow, _ *pcaputil.TrafficConnection, refs []pcaputil.PacketReference, ts time.Time) {
			v := flows[f.Index]
			v.Packets++
			if v.Packets == 1 {
				v.First, v.Last = ts, ts
			} else {
				if ts.Before(v.First) {
					v.First = ts
				}
				if ts.After(v.Last) {
					v.Last = ts
				}
			}
			for _, ref := range refs {
				require.NotContains(t, owners, ref.Number, "this direct-TCP audit expects one ownership callback per physical frame")
				owners[ref.Number] = f.Index
			}
		}), pcaputil.WithOnTrafficFlowOnDataFrameArrived(func(f *pcaputil.TrafficFlow, c *pcaputil.TrafficConnection, frame *pcaputil.TrafficFrame) {
			v, direction := flows[f.Index], 0
			if c != f.ClientConn {
				direction = 1
			}
			retained += len(frame.Payload)
			require.LessOrEqual(t, retained, 16<<20, "complete-byte audit memory scope")
			require.LessOrEqual(t, len(v.Data[0])+len(v.Data[1])+len(frame.Payload), 1<<20, "the page must fit this entire audited flow")
			v.Data[direction] = append(v.Data[direction], frame.Payload...)
		}), pcaputil.WithOnTrafficFlowClosed(func(_ pcaputil.TrafficFlowCloseReason, f *pcaputil.TrafficFlow) {
			flows[f.Index].Pending = f.HasPendingGaps()
		}))
	require.NoError(t, err)
	require.NotEmpty(t, flows)
	src, err := openOffline(path, "", true)
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, captureConfig{maxStreams: 64, streamBytes: 1 << 20, ignoreEmbeddedSecrets: true}, false)
	defer s.Stop()
	bindings := map[uint64]uint64{}
	var deferred []uint64
	physical, associated := 0, 0
	for p := range s.packets {
		physical++
		if p.tcpFlowID == 0 {
			if owners[p.number] != 0 {
				require.NotNil(t, p.transportView)
				require.NotZero(t, p.transportView.TCPFlags&2, "only a held opening SYN may await later association in this direct-TCP profile")
				deferred = append(deferred, p.number)
			}
			continue
		}
		associated++
		require.Equal(t, owners[p.number], p.tcpFlowID, "physical packet %d retains native generation", p.number)
		require.NotZero(t, p.streamID, "native packet %d has a usable page preview", p.number)
		if old := bindings[p.tcpFlowID]; old != 0 {
			require.Equal(t, old, p.streamID, "one page record per native generation")
		}
		bindings[p.tcpFlowID] = p.streamID
	}
	require.NoError(t, s.result())
	require.Len(t, bindings, len(flows))
	require.Empty(t, s.streams.active, "no secondary TCP assembler is used")
	type previewProof struct {
		Flow      uint64                 `json:"native_flow"`
		Preview   uint64                 `json:"preview"`
		Endpoints [2]string              `json:"endpoints"`
		Domain    pcaputil.CaptureDomain `json:"domain"`
		Packets   uint64                 `json:"packets"`
		Bytes     [2]int                 `json:"bytes"`
		SHA256    [2]string              `json:"ordered_byte_SHA256"`
		Stack     []string               `json:"protocol_stack"`
		Pending   bool                   `json:"incomplete"`
		First     time.Time              `json:"first"`
		Last      time.Time              `json:"last"`
	}
	var proofs []previewProof
	for flow := uint64(1); len(proofs) < len(flows); flow++ {
		want := flows[flow]
		if want == nil {
			require.Less(t, flow, uint64(1024), "bounded native ID scope")
			continue
		}
		v := s.streams.snapshot(bindings[flow])
		require.NotNil(t, v)
		require.Equal(t, flow, v.nativeFlowID)
		require.Equal(t, want.Endpoints, v.endpoints)
		require.Equal(t, want.Domain, v.domain)
		require.Equal(t, want.Packets, v.packets)
		require.True(t, want.First.Equal(v.first))
		require.True(t, want.Last.Equal(v.last))
		require.Equal(t, want.Pending, v.retentionNotice != "")
		require.Zero(t, v.omitted, "a complete-byte proof must not ignore evicted bytes")
		proof := previewProof{Flow: flow, Preview: v.id, Endpoints: v.endpoints, Domain: v.domain, Packets: v.packets, Stack: v.protocolStack, Pending: want.Pending, First: v.first, Last: v.last}
		for direction := range want.Data {
			require.True(t, bytes.Equal(want.Data[direction], []byte(directionData(v, direction))), "all ordered bytes of flow %d direction %d", flow, direction)
			require.EqualValues(t, len(want.Data[direction]), v.bytes[direction])
			require.Equal(t, want.Data[direction][:min(len(want.Data[direction]), streamPrefixLimit)], v.prefix[direction])
			proof.Bytes[direction] = len(want.Data[direction])
			digest := sha256.Sum256(want.Data[direction])
			proof.SHA256[direction] = hex.EncodeToString(digest[:])
		}
		for _, event := range v.events {
			require.Equal(t, flow, event.TCPFlowID)
		}
		proofs = append(proofs, proof)
	}
	digest := sha256.Sum256(wire)
	file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	require.NoError(t, err)
	require.NoError(t, json.NewEncoder(file).Encode(map[string]any{"capture_SHA256": hex.EncodeToString(digest[:]), "physical_rows": physical, "associated_rows": associated, "deferred_opening_rows": deferred, "flows": proofs, "complete_native_to_page_byte_identity": true}))
	require.NoError(t, file.Close())
}
