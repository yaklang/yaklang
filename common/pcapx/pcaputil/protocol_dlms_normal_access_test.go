package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestDLMSHDLCNormalAccessExistingAPI(t *testing.T) {
	for _, transport := range []string{"tcp", "udp"} {
		for _, selected := range []bool{false, true} {
			t.Run(transport+"/"+map[bool]string{false: "adjacent-unselected", true: "selected-structure"}[selected], func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				defer s.Close("access")
				wire := "7ea0190321107fdae6e600c001c1000100002a0000ff020012807e"
				if selected {
					wire = "7ea0220321109856e6e600c001c1000100002a0000ff020102020212000112000200d07e"
				}
				q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, wire))
				require.Len(t, q.Events, 1)
				f, err := q.Events[0].GetFields()
				require.NoError(t, err, "existing native API must observe one complete selected descriptor")
				require.Equal(t, selected, f["Selective Access"])
				require.Equal(t, "Get Request Normal", f["Frame Kind"])
				require.EqualValues(t, 1, f["Class ID"])
				require.Equal(t, "0.0.42.0.0.255", f["Logical Name"])
				require.EqualValues(t, 2, f["Attribute ID"])
				if selected {
					require.EqualValues(t, 1, f["Access Selection Raw"])
					require.EqualValues(t, 2, f["Access Selector"])
					require.Equal(t, false, f["Selector Semantics Verified"])
					var want map[string]any
					require.NoError(t, json.Unmarshal([]byte(`{"type":2,"raw_hex":"0202120001120002","length":2,"length_encoding_hex":"02","elements":[{"type":18,"raw_hex":"120001","value":1},{"type":18,"raw_hex":"120002","value":2}]}`), &want))
					rocEqualFields(t, want, f["Access Parameters"].(map[string]any))
				} else {
					require.NotContains(t, f, "Access Parameters")
				}
				require.Equal(t, "observed-request", q.Events[0].Session["Association"])
				require.Zero(t, q.Events[0].TransactionID)
				require.Zero(t, q.Events[0].ResponseTo)
				r := s.Feed(1, time.Unix(2, 0), wrapperWire(t, "7ea013210330d381e6e700c401c1000403afda957e"))
				require.Len(t, r.Events, 1)
				_, err = r.Events[0].GetFields()
				require.NoError(t, err)
				require.Equal(t, q.Events[0].ID, r.Events[0].ResponseTo)
				require.Zero(t, r.Events[0].TransactionID)
				require.Equal(t, "observed-response", r.Events[0].Session["Association"])
				require.Empty(t, s.Close("access"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}

func dlmsNormalAccessControls(t *testing.T) []dlmsListControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("dlms-hdlc-normal-access/controls.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []dlmsListControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, "owned-dlms-hdlc-normal-access/v1", m.Schema)
	require.Len(t, m.Cases, 38)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range m.Cases {
		answer, err := trafficfixture.ReadFile("dlms-hdlc-normal-access/" + c.Answer)
		require.NoError(t, err)
		require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
		var a dlmsListControl
		require.NoError(t, json.Unmarshal(answer, &a))
		require.Equal(t, c.Events, a.Events)
		bound := 0
		for _, batch := range all {
			for _, input := range batch.Cases {
				if input.ID != "dlms-hdlc-normal-access/"+c.ID {
					continue
				}
				require.Equal(t, c.SHA256, input.Input.SHA256)
				require.Equal(t, c.InputAlias, input.Input.OriginalPath)
				require.Equal(t, c.Packets, input.Facts.PacketCount)
				for _, expectation := range input.Expectations {
					var bind struct {
						File string `json:"answer_file"`
						SHA  string `json:"answer_sha256"`
					}
					require.NoError(t, json.Unmarshal(expectation.PayloadConstraints, &bind))
					if bind.File == c.Answer {
						require.Equal(t, c.AnswerSHA, bind.SHA)
						bound++
					}
				}
			}
		}
		require.Equal(t, 1, bound, c.ID)
	}
	return m.Cases
}

func TestDLMSHDLCNormalAccessSealedMatrix(t *testing.T) {
	dlmsSealedMatrix(t, dlmsNormalAccessControls(t))
}
func TestDLMSHDLCNormalAccessOwnershipAndChunks(t *testing.T) {
	dlmsOwnershipAndChunks(t, dlmsNormalAccessControls(t))
}

func TestDLMSHDLCNormalAccessProjectionLimits(t *testing.T) {
	for _, c := range dlmsNormalAccessControls(t) {
		if c.ID != "structure-udp" && c.ID != "nested-compact-udp" {
			continue
		}
		w := wrapperWire(t, c.Events[0].Raw)
		require.Equal(t, ProbeAccept, probeDLMS(w, 2049).Verdict)
		_, err := decodeDLMSBudget(w, 0, 64)
		rocTypedError(t, "ResourceExceeded", err)
		for _, deferred := range []bool{false, true} {
			// One byte below the required projection must refuse before expansion.
			// A preceding normal request makes pending-state retirement observable.
			old := wrapperWire(t, "7ea0190321107fdae6e600c001c1000100002a0000ff020012807e")
			need := int(512 + 2*int64(len(old)) + dlmsProjection(w))
			for _, delta := range []int{-1, 0} {
				b := DefaultParserBudget()
				b.MaxFrameBytes, b.MaxMessageBytes, b.MaxBufferedBytes = 2049, 2049, need+delta
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				q := s.Feed(0, time.Unix(1, 0), old)
				require.Len(t, q.Events, 1)
				_, err = q.Events[0].GetFields()
				require.NoError(t, err)
				out := s.Feed(0, time.Unix(2, 0), w)
				require.Len(t, out.Events, 1)
				f, err := out.Events[0].GetFields()
				want := "ResourceExceeded"
				if delta == 0 {
					want = "ContextRequired"
				}
				rocTypedError(t, want, err)
				require.Nil(t, f)
				require.Nil(t, out.Events[0].Session)
				require.Zero(t, out.Events[0].ResponseTo)
				require.Zero(t, out.Events[0].TransactionID)
				late := s.Feed(1, time.Unix(3, 0), wrapperWire(t, c.Events[1].Raw))
				require.Len(t, late.Events, 1)
				_, err = late.Events[0].GetFields()
				rocTypedError(t, "ContextRequired", err)
				require.Nil(t, late.Events[0].Session)
				require.Zero(t, late.Events[0].ResponseTo)
				require.Zero(t, late.Events[0].TransactionID)
				require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
				require.Empty(t, s.Close("projection"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			}
		}
		if c.ID != "structure-udp" {
			continue
		}
		for _, limits := range []struct{ nodes, depth int }{{2, 64}, {3, 64}, {4096, 5}, {4096, 6}} {
			b := DefaultParserBudget()
			b.MaxCollectionElements, b.MaxRecursionDepth = limits.nodes, limits.depth
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			q := s.Feed(0, time.Unix(1, 0), w)
			require.Len(t, q.Events, 1)
			_, err = q.Events[0].GetFields()
			if limits.nodes == 2 || limits.depth == 5 {
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, q.Events[0].Session)
				require.Zero(t, q.Events[0].TransactionID)
				require.Zero(t, q.Events[0].ResponseTo)
			} else {
				require.NoError(t, err)
				r := s.Feed(1, time.Unix(2, 0), wrapperWire(t, c.Events[1].Raw))
				require.Len(t, r.Events, 1)
				dlmsListAssert(t, c, append(q.Events, r.Events...))
			}
			require.Empty(t, s.Close("limits"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
	}
}
