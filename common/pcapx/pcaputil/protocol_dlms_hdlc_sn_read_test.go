package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"strings"
	"testing"
	"time"
)

type hdlcSNReadCase struct {
	hdlcOpCase
	Metadata         []map[string]any
	UDPHash, TCPHash string
}

func hdlcSNReadControls(t *testing.T) []hdlcSNReadCase {
	return hdlcSNReadCases(t, "controls-safe-owner.json", "3bab1436ded0f609435ca0a0c6337f15274a6a04ee4f0c71949b5eb61a2dc704", 15)
}
func hdlcSNReadTruncationControls(t *testing.T) []hdlcSNReadCase {
	return hdlcSNReadCases(t, "sn-truncation-supplement.json", "47fa3b11723db494417d531b8ee275fac317de79e9db161b8999e6f5cc9b4ed5", 2)
}
func hdlcSNReadCases(t *testing.T, name, pin string, count int) []hdlcSNReadCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-read", "DLMS_HDLC_SN_READ_TEST_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	var meta struct {
		Cases []struct {
			Steps []map[string]any
			UDP   string `json:"sha256"`
			TCP   string `json:"tcp_sha256"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &d))
	require.NoError(t, json.Unmarshal(raw, &meta))
	require.Len(t, d.Cases, count)
	out := make([]hdlcSNReadCase, len(d.Cases))
	for i, c := range d.Cases {
		for j := range c.Steps {
			for _, f := range []map[string]any{c.Steps[j].Fields, c.Steps[j].Session} {
				if f == nil {
					continue
				}
				if text, ok := f["LLC"].(string); ok {
					b, err := base64.StdEncoding.DecodeString(text)
					require.NoError(t, err)
					require.Len(t, b, 3)
					f["LLC"] = map[string]any{"hexbytes": fmt.Sprintf("%x", b)}
				}
			}
		}
		out[i] = hdlcSNReadCase{c, meta.Cases[i].Steps, meta.Cases[i].UDP, meta.Cases[i].TCP}
	}
	if name == "controls-safe-owner.json" && pin == "3bab1436ded0f609435ca0a0c6337f15274a6a04ee4f0c71949b5eb61a2dc704" {
		return hdlcSNWriteUpgradeHistoricalRead(t, raw, out)
	}
	return out
}
func hdlcSNReadTypes(t *testing.T, f map[string]any) {
	t.Helper()
	hdlcOpTypes(t, f)
	if raw, exists := f["HDLC Segmentation"]; exists {
		meta, ok := raw.(map[string]any)
		require.True(t, ok, "segmentation must retain its public map type")
		require.IsType(t, int(0), meta["observed_frames"])
		require.IsType(t, false, meta["more"])
		require.IsType(t, false, meta["complete"])
		require.IsType(t, "", meta["information_hex"])
		if value, exists := meta["assembled_information_hex"]; exists {
			require.IsType(t, "", value)
		}
	}
	if sn, ok := f["SN Read"].(map[string]any); ok {
		snReadTypes(t, sn)
		for _, k := range []string{"wrapper_version", "source_wport", "destination_wport", "wrapper_header_hex", "invoke_id", "invoke_id_and_priority", "service_choice"} {
			require.NotContains(t, sn, k)
		}
		for _, k := range []string{"Invoke ID", "Invoke ID and Priority", "High Priority", "Confirmed Service", "Outstanding"} {
			require.NotContains(t, f, k)
		}
	}
	if value, exists := f["SN Write"]; exists {
		sn, ok := value.(map[string]any)
		require.True(t, ok, "SN Write must retain its public map type")
		snWriteTypes(t, sn)
		for _, k := range []string{"wrapper_version", "source_wport", "destination_wport", "wrapper_header_hex", "invoke_id", "invoke_id_and_priority", "service_choice"} {
			require.NotContains(t, sn, k)
		}
		for _, k := range []string{"Invoke ID", "Invoke ID and Priority", "High Priority", "Confirmed Service", "Outstanding"} {
			require.NotContains(t, f, k)
		}
	}
	if list, ok := f["Get List"].(map[string]any); ok {
		snReadTypes(t, list)
	}
	if v, ok := f["Class ID"]; ok {
		require.IsType(t, uint16(0), v)
		require.IsType(t, int8(0), f["Attribute ID"])
	}
	if v, ok := f["Data Type"]; ok {
		require.IsType(t, byte(0), v)
		if v == byte(9) {
			require.IsType(t, []byte{}, f["Data Value"])
		}
		if v == byte(17) {
			require.IsType(t, byte(0), f["Data Value"])
		}
	}
}

// Preserve the public Go value types in every owned query projection too,
// not just their JSON-equivalent values. Mutation cannot alter later answers.
func hdlcSNReadQueries(t *testing.T, e *ProtocolEvent, st hdlcOpStep) {
	t.Helper()
	hdlcOpQueries(t, e, st.Fields, st.Error)
	if st.Error != "" {
		return
	}
	for repeat := 0; repeat < 2; repeat++ {
		f, err := e.GetFields()
		require.NoError(t, err)
		hdlcSNReadTypes(t, f)
		d, err := e.Decode()
		require.NoError(t, err)
		for _, k := range []string{"fields", "session"} {
			v, ok := d[k].(map[string]any)
			require.True(t, ok)
			hdlcSNReadTypes(t, v)
		}
		poisonWrapperStructured(f)
		poisonWrapperStructured(d)
	}
}
func hdlcSNReadNative(t *testing.T, cases []hdlcSNReadCase, src, dst uint16) {
	for _, tc := range cases {
		for _, tr := range []string{"udp", "tcp"} {
			for _, d := range []bool{false, true} {
				for _, chunk := range []int{0, 1, 7} {
					if tr == "udp" && chunk != 0 {
						continue
					}
					t.Run(fmt.Sprintf("%s/%s/d%v/chunk%d", tc.Name, tr, d, chunk), func(t *testing.T) {
						c := tc.hdlcOpCase
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
							}
							e := stepEvents[0]
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
						for _, st := range c.Steps {
							if tr == "tcp" && st.TCPOverride != nil {
								continue
							}
							hdlcSNReadQueries(t, saved[index], st)
							index++
						}
					})
				}
			}
		}
	}
}
func TestDLMSHDLCSNReadNative(t *testing.T) { hdlcSNReadNative(t, hdlcSNReadControls(t), 41000, 4059) }
func TestDLMSHDLCSNReadOffPort(t *testing.T) {
	c := hdlcSNReadControls(t)
	hdlcSNReadNative(t, []hdlcSNReadCase{c[0], c[9]}, 45000, 45001)
}
func TestDLMSHDLCSNReadTruncationNative(t *testing.T) {
	hdlcSNReadNative(t, hdlcSNReadTruncationControls(t), 41000, 4059)
}
func TestDLMSHDLCSNReadTruncationMatrix(t *testing.T) {
	hdlcSNReadMatrix(t, hdlcSNReadTruncationControls(t), "dlms-hdlc-sn-read")
}
func TestDLMSHDLCSNReadMatrix(t *testing.T) {
	hdlcSNReadMatrix(t, hdlcSNReadControls(t), "dlms-hdlc-sn-read")
}
func hdlcSNReadMatrix(t *testing.T, cases []hdlcSNReadCase, batch string) {
	env := "DLMS_HDLC_SN_READ_TEST_ROOT"
	if batch == "dlms-sn-information-segments" || batch == "dlms-sn-information-segments-hard" || batch == "dlms-sn-information-segments-lookalike" {
		env = "DLMS_SN_SEGMENTS_TEST_ROOT"
	}
	if batch == "dlms-hdlc-sn-write" || batch == "dlms-hdlc-sn-write-resources" {
		env = "DLMS_HDLC_SN_WRITE_TEST_ROOT"
	}
	for _, tc := range cases {
		for _, tr := range []string{"udp", "tcp"} {
			t.Run(tc.Name+"/"+tr, func(t *testing.T) {
				c := tc.hdlcOpCase
				c.Steps = append([]hdlcOpStep(nil), c.Steps...)
				for i := range c.Steps {
					if tr == "udp" {
						c.Steps[i].Direction = int(tc.Metadata[i]["capture_udp_direction"].(float64))
					}
				}
				name := c.Name + "." + tr + ".pcap"
				var reuse map[string]string
				require.NoError(t, json.Unmarshal(finsReadBatch(t, batch, env, "reuse-inputs.json"), &reuse))
				var raw []byte
				if alias, ok := reuse[name]; ok {
					var err error
					const prefix = "common/pcapx/pcaputil/"
					require.True(t, strings.HasPrefix(alias, prefix))
					if (batch == "dlms-sn-information-segments" || batch == "dlms-sn-information-segments-hard" || batch == "dlms-sn-information-segments-lookalike") && strings.HasPrefix(alias, prefix+batch+"/") {
						name := strings.TrimPrefix(alias, prefix+batch+"/")
						require.NotContains(t, name, "/")
						raw = finsReadBatch(t, batch, env, name)
					} else {
						raw, err = trafficfixture.ReadFile(strings.TrimPrefix(alias, prefix))
						require.NoError(t, err)
					}
				} else {
					raw = finsReadBatch(t, batch, env, name)
				}
				expected := tc.UDPHash
				if tr == "tcp" {
					expected = tc.TCPHash
				}
				require.NotEmpty(t, expected)
				require.Equal(t, expected, fmt.Sprintf("%x", sha256.Sum256(raw)))
				discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
					es, stats := hdlcOpReplay(t, c, raw, w, d, o, tr)
					count := len(c.Steps)
					if tr == "tcp" {
						for _, st := range c.Steps {
							if st.TCPOverride != nil {
								count--
							}
						}
					}
					require.Len(t, es, count+len(c.Close.Events))
					eventIDs := make(map[uint64]bool)
					for _, e := range es {
						require.NotZero(t, e.ID)
						require.False(t, eventIDs[e.ID], "capture Feed and Close IDs must all be distinct")
						eventIDs[e.ID] = true
					}
					// The actual physical endpoints and packet references are independent
					// of a cold UDP observation direction. Check them explicitly below.

					var flow uint64
					idx := 0
					for i, st := range c.Steps {
						if tr == "tcp" && st.TCPOverride != nil {
							continue
						}
						e := es[idx]
						require.Equal(t, tc.Metadata[i]["source"], e.Source)
						require.Equal(t, tc.Metadata[i]["destination"], e.Destination)
						require.Equal(t, CaptureDomain{}, e.Domain)
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						refKey := "PacketRef_udp"
						if tr == "tcp" {
							refKey = "PacketRef_tcp"
						}
						require.EqualValues(t, tc.Metadata[i][refKey], e.SourceBytes.PacketRefs[0].Number)
						require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
						idx++
						require.NotZero(t, e.ID)
						wantFlow := tc.Metadata[i]["captured_udp_flow_nonzero"].(bool)
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
						if st.Error == "" {
							f, err := e.GetFields()
							require.NoError(t, err)
							hdlcSNReadTypes(t, f)
							hdlcSNReadTypes(t, e.Session)
							hdlcSNReadQueries(t, e, st)
						}
					}
					hdlcOpCheck(t, c, es[:count], tr, false, d)
					hdlcGetCloseCheck(t, c, es[count:], tr, flow, false, "ctx-canceled")
					require.EqualValues(t, count, stats.Messages)
					require.EqualValues(t, len(c.Close.Events), stats.Incomplete)
				})
			})
		}
	}
}
