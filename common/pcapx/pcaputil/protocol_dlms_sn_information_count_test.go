package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func snInformationCountLoad(t *testing.T, name, pin string, count int) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-sn-information-count-source", "DLMS_SN_INFORMATION_COUNT_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var g struct{ Cases []hdlcOpCase }
	var m struct {
		Cases []struct {
			Steps []map[string]any
			UDP   string `json:"sha256"`
			TCP   string `json:"tcp_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &g))
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Len(t, g.Cases, count)
	require.Len(t, m.Cases, count)
	out := make([]hdlcSNReadCase, count)
	for i, c := range g.Cases {
		// The independent gold stores an actual test-close string; the old
		// checker accepts a reason placeholder and expands it before comparing.
		for j := range c.Close.Events {
			e := &c.Close.Events[j]
			if e.Profile == "dlms-hdlc-sn-information-segmented" {
				require.Equal(t, "test-close: "+e.SummaryUDP, e.SummaryTCP)
				e.SummaryTCP = "<reason>: " + e.SummaryUDP
			}
		}
		for j := range m.Cases[i].Steps {
			if message, ok := m.Cases[i].Steps[j]["error_message"]; ok {
				m.Cases[i].Steps[j]["query_error_message"] = message
			}
		}
		out[i] = hdlcSNReadCase{c, m.Cases[i].Steps, m.Cases[i].UDP, m.Cases[i].TCP}
	}
	return out
}
func TestDLMSSNInformationCountExistingAPI(t *testing.T) {
	snInformationCountNative(t, snInformationCountLoad(t, "baseline-controls.json", "ee6ead096bacc7a45f8174a6a657f5810417bb034d2bcc16cd2b286508697b42", 3), 41000, 4059)
}
func TestDLMSSNInformationCountNative(t *testing.T) {
	snInformationCountNative(t, snInformationCountLoad(t, "controls.json", "fea77c50d4857cc2156ce5b26df78630cb37e5ccd724e53c803694622ce3ffd8", 17), 41000, 4059)
}
func snInformationCountNative(t *testing.T, cases []hdlcSNReadCase, src, dst uint16) {
	for _, tc := range cases {
		for _, tr := range []string{"udp", "tcp"} {
			for _, d := range []bool{false, true} {
				for _, chunk := range []int{0, 1, 7} {
					if tr == "udp" && chunk != 0 {
						continue
					}
					t.Run(fmt.Sprintf("%s/%s/d%v/chunk%d", tc.Name, tr, d, chunk), func(t *testing.T) {
						c := tc.hdlcOpCase
						c.Steps = append([]hdlcOpStep(nil), c.Steps...)
						ids := make([]uint64, len(c.Steps))
						b := DefaultParserBudget()
						if c.Budget.Collection > 0 {
							b.MaxCollectionElements = c.Budget.Collection
						}
						if c.Budget.Depth > 0 {
							b.MaxRecursionDepth = c.Budget.Depth
						}
						s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(tr), WithSessionPorts(src, dst))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = d
						var es, saved []*ProtocolEvent
						var flow uint64
						eventIDs := make(map[uint64]bool)
						for i, st := range c.Steps {
							wire := wrapperWire(t, st.Wire)
							require.Equal(t, st.SHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
							var stepEvents []*ProtocolEvent
							var final *ProtocolError
							fatal := tr == "tcp" && st.TCPOverride != nil
							for at := 0; at < len(wire); {
								end := len(wire)
								if chunk > 0 {
									end = min(end, at+chunk)
								}
								in := bytes.Clone(wire[at:end])
								r := s.Feed(st.Direction, time.Unix(st.At, 0), in)
								clear(in)
								require.Equal(t, end-at, r.Consumed)
								if fatal {
									require.Empty(t, r.Events)
									require.False(t, r.NeedMore)
									rocTypedError(t, "FatalSessionError", r.Err)
									require.Equal(t, st.TCPOverride.Message, r.Err.Message)
								} else if len(r.Events) == 0 {
									require.Equal(t, "tcp", tr)
									require.True(t, r.NeedMore)
									rocTypedError(t, "NeedMore", r.Err)
								} else {
									require.False(t, r.NeedMore)
									final = r.Err
									stepEvents = append(stepEvents, r.Events...)
								}
								at = end
							}
							if fatal {
								require.Zero(t, s.Stats().BufferedBytes)
								continue
							}
							require.Len(t, stepEvents, 1)
							if st.Error == "" {
								require.Nil(t, final)
							} else {
								rocTypedError(t, st.Error, final)
								require.Equal(t, tc.Metadata[i]["error_message"], final.Message)
							}
							e := stepEvents[0]
							require.Equal(t, tc.Metadata[i]["public_error"], e.Error)
							ids[i] = e.ID
							st = hdlcSNBlockBindStep(t, st, ids)
							c.Steps[i] = st
							require.NotZero(t, e.ID)
							require.False(t, eventIDs[e.ID], "event ID must be unique before public poisoning")
							eventIDs[e.ID] = true
							wantFlow := tc.Metadata[i]["native_udp_flow_nonzero"].(bool)
							if tr == "tcp" {
								wantFlow = true
							}
							if wantFlow {
								require.NotZero(t, e.FlowID)
								if flow == 0 {
									flow = e.FlowID
								}
								require.Equal(t, flow, e.FlowID)
							} else {
								require.Zero(t, e.FlowID)
							}
							require.Equal(t, CaptureDomain{}, e.Domain)
							require.Equal(t, tr, e.Transport)
							ends := []string{fmt.Sprintf("192.0.2.1:%d", src), fmt.Sprintf("192.0.2.2:%d", dst)}
							require.Equal(t, ends[st.Direction], e.Source)
							require.Equal(t, ends[1-st.Direction], e.Destination)
							if st.Error == "" {
								f, err := e.GetFields()
								require.NoError(t, err)
								hdlcOpEqual(t, st.Fields, f)
								hdlcSNReadTypes(t, f)
								hdlcSNReadTypes(t, e.Session)
								hdlcSNReadQueries(t, e, st)
								poisonWrapperStructured(f)
							} else {
								snReadQueryError(t, e, st.Error)
								snMultiOwnedDiagnosis(t, e, st.Error, tc.Metadata[i]["error_message"].(string))
								final.Kind, final.Message = ErrNeedMore, "caller-owned FeedResult error"
								require.Equal(t, tc.Metadata[i]["public_error"], e.Error)
								snMultiOwnedDiagnosis(t, e, st.Error, tc.Metadata[i]["error_message"].(string))
							}
							es = append(es, actionSnapshot(e))
							saved = append(saved, e)
							actionPoison(e)
						}
						hdlcOpCheck(t, c, es, tr, false, d)
						closed := s.Close("test-close")
						for _, e := range closed {
							require.NotZero(t, e.ID)
							require.False(t, eventIDs[e.ID], "Close ID must be distinct from every Feed event")
							eventIDs[e.ID] = true
						}
						hdlcGetCloseCheck(t, c, closed, tr, flow, false, "test-close")
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
						require.EqualValues(t, len(c.Close.Events), s.Stats().Incomplete)
						// Re-query poisoned public events after all later frames and Close. They
						// must keep the independently expected private fields/errors and owners.
						index := 0
						for i, st := range c.Steps {
							if tr == "tcp" && st.TCPOverride != nil {
								continue
							}
							hdlcSNReadQueries(t, saved[index], st)
							if st.Error != "" {
								snMultiOwnedDiagnosis(t, saved[index], st.Error, tc.Metadata[i]["error_message"].(string))
							}
							index++
						}
					})
				}
			}
		}
	}
}

func TestDLMSSNInformationCountMatrix(t *testing.T) {
	cases := snInformationCountLoad(t, "controls.json", "fea77c50d4857cc2156ce5b26df78630cb37e5ccd724e53c803694622ce3ffd8", 17)
	hdlcSNReadMatrixChecked(t, cases, "dlms-sn-information-count-captures", []string{"tcp", "udp"}, func(t *testing.T, e *ProtocolEvent, st hdlcOpStep, meta map[string]any) {
		require.Equal(t, meta["public_error"], e.Error)
		if st.Error != "" {
			snMultiOwnedDiagnosis(t, e, st.Error, meta["error_message"].(string))
		}
		actionPoison(e)
		hdlcSNReadQueries(t, e, st)
		if st.Error != "" {
			snMultiOwnedDiagnosis(t, e, st.Error, meta["error_message"].(string))
		}
	})
}

func TestDLMSSNInformationCountWidthNative(t *testing.T) {
	snInformationCountNative(t, snInformationCountLoad(t, "width-controls.json", "f6507db73394e9a0a08b74c786d1187cb254bae1ed49bd2e8558526e0690972c", 8), 41000, 4059)
}
func TestDLMSSNInformationCountWidthMatrix(t *testing.T) {
	cases := snInformationCountLoad(t, "width-controls.json", "f6507db73394e9a0a08b74c786d1187cb254bae1ed49bd2e8558526e0690972c", 8)
	hdlcSNReadMatrixChecked(t, cases, "dlms-sn-information-count-captures", []string{"tcp", "udp"}, func(t *testing.T, e *ProtocolEvent, st hdlcOpStep, meta map[string]any) {
		require.Equal(t, meta["public_error"], e.Error)
		if st.Error != "" {
			snMultiOwnedDiagnosis(t, e, st.Error, meta["error_message"].(string))
		}
		actionPoison(e)
		hdlcSNReadQueries(t, e, st)
		if st.Error != "" {
			snMultiOwnedDiagnosis(t, e, st.Error, meta["error_message"].(string))
		}
	})
}
