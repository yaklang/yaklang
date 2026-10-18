package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func dlmsACSEControls(t *testing.T) []dlmsListControl {
	t.Helper()
	if root := os.Getenv("DLMS_HDLC_ACSE_TEST_ROOT"); root != "" {
		b, e := os.ReadFile(filepath.Join(root, "controls.json"))
		require.NoError(t, e)
		var doc struct {
			Schema string
			Cases  []dlmsListControl
		}
		require.NoError(t, json.Unmarshal(b, &doc))
		require.Equal(t, "owned-dlms-hdlc-acse/v1", doc.Schema)
		require.Len(t, doc.Cases, 44)
		for _, c := range doc.Cases {
			a, e := os.ReadFile(filepath.Join(root, c.Answer))
			require.NoError(t, e)
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(a)))
			var bound dlmsListControl
			require.NoError(t, json.Unmarshal(a, &bound))
			require.Equal(t, c.Events, bound.Events)
		}
		return doc.Cases
	}
	return dlmsNamedBlockControls(t, "dlms-hdlc-acse", "owned-dlms-hdlc-acse/v1", 44)
}

func TestDLMSHDLCACSESealedMatrix(t *testing.T)    { dlmsSealedMatrix(t, dlmsACSEControls(t)) }
func TestDLMSHDLCACSESealedOwnership(t *testing.T) { dlmsOwnershipAndChunks(t, dlmsACSEControls(t)) }

func TestDLMSHDLCACSEBudgetIdleAndClose(t *testing.T) {
	q := wrapperWire(t, "7ea02b032110fbafe6e600601da109060760857405080101be10040e01000000065f1f0400000010040081e17e")
	r := wrapperWire(t, "7ea0372103306c7ce6e7006129a109060760857405080101a203020100a305a103020100be10040e0800065f1f04000000100400000786a17e")
	for _, transport := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("close/%s/%t", transport, deferred), func(t *testing.T) {
				s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
				require.NoError(t, e)
				s.(*captureSession).f.a.config.Deferred = deferred
				out := s.Feed(0, time.Unix(1, 0), bytes.Clone(q))
				require.Len(t, out.Events, 1)
				_, e = out.Events[0].GetFields()
				require.NoError(t, e)
				closed := s.Close("unfinished")
				require.Len(t, closed, 1)
				require.Equal(t, "dlms", closed[0].Protocol)
				require.Equal(t, "dlms-hdlc-acse", closed[0].Profile)
				require.Equal(t, transport, closed[0].Transport)
				require.Equal(t, out.Events[0].FlowID, closed[0].FlowID)
				require.Equal(t, out.Events[0].Source, closed[0].Source)
				require.Equal(t, out.Events[0].Destination, closed[0].Destination)
				require.Equal(t, "incomplete", closed[0].Completeness)
				require.Zero(t, closed[0].ResponseTo)
				require.Zero(t, closed[0].TransactionID)
				f, err := closed[0].GetFields()
				rocTypedError(t, "NeedMore", err)
				require.Nil(t, f)
				rocEqualFields(t, map[string]any{"Outstanding": float64(1)}, closed[0].Session)
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
			for _, depth := range []int{3, 4} {
				t.Run(fmt.Sprintf("depth/%s/%t/%d", transport, deferred, depth), func(t *testing.T) {
					b := DefaultParserBudget()
					b.MaxRecursionDepth = depth
					s, e := NewProtocolSessionWithOptions(b, WithSessionTransport(transport), WithSessionPorts(40000, 4059))
					require.NoError(t, e)
					s.(*captureSession).f.a.config.Deferred = deferred
					out := s.Feed(0, time.Unix(1, 0), bytes.Clone(q))
					require.Len(t, out.Events, 1)
					f, e := out.Events[0].GetFields()
					if depth == 3 {
						rocTypedError(t, "ResourceExceeded", e)
						require.Nil(t, f)
						require.Nil(t, out.Events[0].Session)
						require.Zero(t, out.Events[0].ResponseTo)
					} else {
						require.NoError(t, e)
						reply := s.Feed(1, time.Unix(2, 0), bytes.Clone(r))
						require.Len(t, reply.Events, 1)
						_, e = reply.Events[0].GetFields()
						require.NoError(t, e)
						require.Equal(t, out.Events[0].ID, reply.Events[0].ResponseTo)
					}
					require.Empty(t, s.Close("depth"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
	for _, delta := range []int{-1, 0} {
		t.Run(fmt.Sprintf("projection/%d", delta), func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxMessageBytes, b.MaxFrameBytes = 2049, 2049
			b.MaxBufferedBytes = int(512+128+2*len(q)+int(dlmsProjection(r))) + delta
			s, e := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, e)
			defer s.Close("cleanup")
			first := s.Feed(0, time.Unix(1, 0), bytes.Clone(q))
			require.Len(t, first.Events, 1)
			_, e = first.Events[0].GetFields()
			require.NoError(t, e)
			out := s.Feed(1, time.Unix(2, 0), bytes.Clone(r))
			require.Len(t, out.Events, 1)
			f, e := out.Events[0].GetFields()
			if delta < 0 {
				rocTypedError(t, "ResourceExceeded", e)
				require.Nil(t, f)
				require.Nil(t, out.Events[0].Session)
				require.Zero(t, out.Events[0].ResponseTo)
				s.(*captureSession).f.a.budget.MaxBufferedBytes = 1 << 20
				late := s.Feed(1, time.Unix(3, 0), bytes.Clone(r))
				require.Len(t, late.Events, 1)
				_, e = late.Events[0].GetFields()
				rocTypedError(t, "ContextRequired", e)
				require.Zero(t, late.Events[0].ResponseTo)
			} else {
				require.NoError(t, e)
				require.Equal(t, first.Events[0].ID, out.Events[0].ResponseTo)
			}
			require.Empty(t, s.Close("projection"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	t.Run("idle-pending", func(t *testing.T) {
		s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
		require.NoError(t, e)
		q0 := s.Feed(0, time.Unix(1, 0), bytes.Clone(q))
		require.Len(t, q0.Events, 1)
		late := s.Feed(1, time.Unix(32, 0), bytes.Clone(r))
		require.Len(t, late.Events, 1)
		f, e := late.Events[0].GetFields()
		rocTypedError(t, "ContextRequired", e)
		require.Nil(t, f)
		require.Nil(t, late.Events[0].Session)
		require.Zero(t, late.Events[0].ResponseTo)
		require.Empty(t, s.Close("idle"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}

func TestDLMSHDLCACSEProbeIntegrity(t *testing.T) {
	q := wrapperWire(t, "7ea02b032110fbafe6e600601da109060760857405080101be10040e01000000065f1f0400000010040081e17e")
	r := wrapperWire(t, "7ea0372103306c7ce6e7006129a109060760857405080101a203020100a305a103020100be10040e0800065f1f04000000100400000786a17e")
	for _, w := range [][]byte{q, r} {
		require.True(t, dlmsACSEWireEvidence(w))
		p := probeDLMS(w, 2049)
		require.Equal(t, ProbeAccept, p.Verdict)
		require.Equal(t, "hdlc-acse", p.Version)
		for _, at := range []int{1, 6, len(w) - 2} {
			bad := bytes.Clone(w)
			bad[at] ^= 1
			require.False(t, dlmsACSEWireEvidence(bad))
			if at == 1 {
				require.Equal(t, ProbeNeedMore, probeDLMS(bad, 2049).Verdict)
			} else {
				require.Equal(t, ProbeReject, probeDLMS(bad, 2049).Verdict)
			}
		}
	}
}

func TestDLMSHDLCACSEOrphanCannotOwnLaterRequest(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprintf("explicit-%t", explicit), func(t *testing.T) { dlmsACSEOrphanCase(t, explicit) })
	}
}
func dlmsACSEOrphanCase(t *testing.T, explicit bool) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, err)
	defer s.Close("test")
	if explicit {
		s.(*captureSession).f.a.datagramDecodeAs = map[uint16]string{4059: "dlms"}
	}
	q := wrapperWire(t, "7ea02b032110fbafe6e600601da109060760857405080101be10040e01000000065f1f0400000010040081e17e")
	r := wrapperWire(t, "7ea0372103306c7ce6e7006129a109060760857405080101a203020100a305a103020100be10040e0800065f1f04000000100400000786a17e")
	orphan := s.Feed(1, time.Unix(1, 0), bytes.Clone(r))
	require.Len(t, orphan.Events, 1)
	f, e := orphan.Events[0].GetFields()
	rocTypedError(t, "ContextRequired", e)
	require.Nil(t, f)
	require.Zero(t, orphan.Events[0].ResponseTo)
	fresh := s.Feed(0, time.Unix(2, 0), bytes.Clone(q))
	require.Len(t, fresh.Events, 1)
	late := s.Feed(1, time.Unix(3, 0), bytes.Clone(r))
	require.Len(t, late.Events, 1)
	require.Zero(t, late.Events[0].ResponseTo, "previous unobserved request's AARE cannot acquire the new request ID")
	require.Empty(t, s.Close("orphan"))
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestDLMSHDLCACSEExistingAPIBaseline(t *testing.T) {
	t.Run("adjacent-get", func(t *testing.T) {
		s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(39200, 4059))
		require.NoError(t, e)
		q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "7ea0190321107fdae6e600c001c1000f0000280000ff020091537e"))
		require.Len(t, q.Events, 1)
		r := s.Feed(1, time.Unix(2, 0), wrapperWire(t, "7ea012210330689de6e700c401c100112a43c37e"))
		require.Len(t, r.Events, 1)
		fields, e := r.Events[0].GetFields()
		require.NoError(t, e)
		require.EqualValues(t, 42, fields["Data Value"])
		require.Equal(t, q.Events[0].ID, r.Events[0].ResponseTo)
		require.Empty(t, s.Close("adjacent"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
	t.Run("reference-acse", func(t *testing.T) {
		s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(39200, 4059))
		require.NoError(t, e)
		defer s.Close("baseline")
		q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, "7ea02b032110fbafe6e600601da109060760857405080101be10040e01000000065f1f0400000010040081e17e"))
		require.Nil(t, q.Err)
		require.Len(t, q.Events, 1, "CRC-valid fixed-reference AARQ must be observed by the selected HDLC profile")
		fields, e := q.Events[0].GetFields()
		require.NoError(t, e)
		require.Equal(t, "AARQ", fields["ACSE"].(map[string]any)["kind"])
		id := q.Events[0].ID
		fields["ACSE"].(map[string]any)["conformance_bits"] = uint32(0)
		q.Events[0].Session["ACSE"].(map[string]any)["max_pdu_size"] = uint16(64)
		q.Events[0].ID = 999999
		clear(q.Events[0].Raw)
		r := s.Feed(1, time.Unix(2, 0), wrapperWire(t, "7ea0372103306c7ce6e7006129a109060760857405080101a203020100a305a103020100be10040e0800065f1f04000000100400000786a17e"))
		require.Len(t, r.Events, 1)
		require.Nil(t, r.Err)
		require.Equal(t, id, r.Events[0].ResponseTo)
		require.Zero(t, r.Events[0].TransactionID)
		f, e := r.Events[0].GetFields()
		require.NoError(t, e)
		a := f["ACSE"].(map[string]any)
		require.Equal(t, uint32(524288), a["proposed_conformance_bits"])
		require.Equal(t, uint16(1024), a["proposed_max_pdu_size"])
		require.Empty(t, s.Close("complete"))
		require.Empty(t, s.Close("again"))
		require.Zero(t, s.Stats().BufferedBytes)
	})
}
