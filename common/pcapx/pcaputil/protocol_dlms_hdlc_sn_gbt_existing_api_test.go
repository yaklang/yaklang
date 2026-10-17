package pcaputil

// Authored outside the repository. Existing public APIs and existing helpers
// only; this file is deliberately NOT executed by the independent agent.
import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func hdlcSNGBTIndependentCases(t *testing.T, baseline bool) []hdlcOpCase {
	t.Helper()
	name, pin, count := "controls.json", "536e7fa6a451f57906c5fd60b6fc5f3cc62eb9043cfebc16d9b49fe62985007f", 6
	if baseline {
		name, pin, count = "baseline-controls.json", "e04930ac23575a2721aaed1b37fb731ebc9bb33e325eb457894bbdaa8a6b0b97", 3
	}
	raw := finsReadBatch(t, "dlms-hdlc-sn-gbt-source", "DLMS_HDLC_SN_GBT_TEST_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, count)
	return d.Cases
}
func hdlcSNGBTBind(st hdlcOpStep, ids []uint64) hdlcOpStep {
	if st.Fields != nil {
		st.Fields = wrapperCloseRemap(st.Fields, ids).(map[string]any)
	}
	if st.Session != nil {
		st.Session = wrapperCloseRemap(st.Session, ids).(map[string]any)
	}
	return st
}
func hdlcSNGBTTypes(t *testing.T, f map[string]any) {
	t.Helper()
	hdlcOpTypes(t, f)
	if raw, ok := f["GBT"]; ok {
		q, yes := raw.(map[string]any)
		require.True(t, yes, "GBT retains public map type")
		snWriteTypes(t, q)
		for _, k := range []string{"command", "block_control", "window_size"} {
			require.IsType(t, byte(0), q[k])
		}
		for _, k := range []string{"block_number", "block_number_ack"} {
			require.IsType(t, uint16(0), q[k])
		}
		require.IsType(t, uint64(0), q["block_data_length"])
		for _, k := range []string{"payload_length", "assembled_data_length"} {
			if z, ok := q[k]; ok {
				require.IsType(t, int(0), z)
			}
		}
		for _, k := range []string{"transfer_start_event", "acknowledged_block_event", "duplicate_of_event"} {
			if z, ok := q[k]; ok {
				require.IsType(t, uint64(0), z)
			}
		}
		require.IsType(t, false, q["last_block"])
		require.IsType(t, false, q["transfer_complete"])
		for _, k := range []string{"apdu_hex", "block_data_hex", "length_encoding_hex"} {
			require.IsType(t, "", q[k])
		}
		for _, k := range []string{"wrapper_version", "source_wport", "destination_wport", "wrapper_header_hex", "invoke_id"} {
			require.NotContains(t, q, k)
		}
		for _, k := range []string{"streaming", "window_semantics_verified", "negotiated_conformance_verified"} {
			require.IsType(t, false, q[k])
		}
		if raw, ok := q["assembled_sn_apdu"]; ok {
			sn, yes := raw.(map[string]any)
			require.True(t, yes)
			require.IsType(t, "", q["assembled_sn_apdu_hex"])
			for _, k := range []string{"wrapper_version", "source_wport", "destination_wport", "wrapper_header_hex", "invoke_id"} {
				require.NotContains(t, sn, k)
			}
		}
	}
}
func hdlcSNGBTErrorOwnership(t *testing.T, e *ProtocolEvent, kind string) {
	t.Helper()
	fields, err := e.GetFields()
	require.Nil(t, fields)
	rocTypedError(t, kind, err)
	var pe *ProtocolError
	require.ErrorAs(t, err, &pe)
	message := pe.Message
	if kind == "ContextRequired" {
		require.Equal(t, "DLMS HDLC SN GBT cannot consume an observed LN owner or information group", message)
	}
	if kind == "MalformedMessage" {
		require.Equal(t, "DLMS HDLC SN GBT: SN LLC direction differs from command", message)
	}
	for round := 0; round < 2; round++ {
		pe.Kind, pe.Message = ErrNeedMore, "caller mutation"
		decoded, next := e.Decode()
		require.Nil(t, decoded)
		rocTypedError(t, kind, next)
		require.ErrorAs(t, next, &pe)
		require.Equal(t, message, pe.Message)
		pe.Kind, pe.Message = ErrNeedMore, "caller mutation"
		fields, next = e.GetFields()
		require.Nil(t, fields)
		rocTypedError(t, kind, next)
		require.ErrorAs(t, next, &pe)
		require.Equal(t, message, pe.Message)
	}
}
func hdlcSNGBTPublicAPINative(t *testing.T, cases []hdlcOpCase) {
	for _, original := range cases {
		for _, tr := range []string{"tcp", "udp"} {
			for _, deferred := range []bool{false, true} {
				chunks := []int{0, 1, 7}
				if tr == "udp" {
					chunks = []int{0}
				}
				for _, chunk := range chunks {
					t.Run(fmt.Sprintf("%s/%s/deferred%t/chunk%d", original.Name, tr, deferred, chunk), func(t *testing.T) {
						// Each configuration starts from a fresh expected case, preserving gold.
						raw, err := json.Marshal(original)
						require.NoError(t, err)
						var c hdlcOpCase
						require.NoError(t, json.Unmarshal(raw, &c))
						budget := DefaultParserBudget()
						if c.Budget.Collection > 0 {
							budget.MaxCollectionElements = c.Budget.Collection
						}
						if c.Budget.Depth > 0 {
							budget.MaxRecursionDepth = c.Budget.Depth
						}
						s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport(tr), WithSessionPorts(41000, 4059))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = deferred
						ids := make([]uint64, len(c.Steps))
						saved := make([]*ProtocolEvent, len(c.Steps))
						var snapshots []*ProtocolEvent
						var flow uint64
						for i, st := range c.Steps {
							wire := wrapperWire(t, st.Wire)
							require.Equal(t, st.SHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
							var events []*ProtocolEvent
							var last *ProtocolError
							size := chunk
							if size == 0 {
								size = len(wire)
							}
							for at := 0; at < len(wire); at += size {
								end := min(at+size, len(wire))
								input := append([]byte(nil), wire[at:end]...)
								result := s.Feed(st.Direction, time.Unix(st.At, 0), input)
								clear(input)
								require.Equal(t, end-at, result.Consumed)
								if len(result.Events) == 0 {
									require.Equal(t, "tcp", tr)
									require.True(t, result.NeedMore)
									rocTypedError(t, "NeedMore", result.Err)
								} else {
									require.False(t, result.NeedMore)
									last = result.Err
									events = append(events, result.Events...)
								}
							}
							require.Len(t, events, 1)
							if st.Error == "" {
								require.Nil(t, last)
							} else {
								rocTypedError(t, st.Error, last)
							}
							e := events[0]
							if st.Error == "ContextRequired" {
								require.Equal(t, "DLMS HDLC SN GBT cannot consume an observed LN owner or information group", last.Message)
							}
							require.NotZero(t, e.ID)
							for _, id := range ids {
								if id != 0 {
									require.NotEqual(t, id, e.ID)
								}
							}
							ids[i] = e.ID
							if flow == 0 {
								flow = e.FlowID
							}
							require.NotZero(t, flow)
							require.Equal(t, flow, e.FlowID)
							require.Equal(t, tr, e.Transport)
							require.Equal(t, CaptureDomain{}, e.Domain)
							ends := []string{"192.0.2.1:41000", "192.0.2.2:4059"}
							require.Equal(t, ends[st.Direction], e.Source)
							require.Equal(t, ends[1-st.Direction], e.Destination)
							if st.Error == "ContextRequired" {
								require.Equal(t, "DLMS HDLC SN GBT cannot consume an observed LN owner or information group", last.Message)
							}
							if st.Error == "MalformedMessage" {
								require.Equal(t, "DLMS HDLC SN GBT: SN LLC direction differs from command", last.Message)
							}
							bound := hdlcSNGBTBind(st, ids)
							c.Steps[i] = bound
							if st.Error != "" {
								last.Kind, last.Message = ErrNeedMore, "caller mutation"
								hdlcSNGBTErrorOwnership(t, e, st.Error)
							}
							hdlcOpQueries(t, e, bound.Fields, bound.Error)
							if st.Error == "" {
								f, err := e.GetFields()
								require.NoError(t, err)
								hdlcSNGBTTypes(t, f)
								hdlcSNGBTTypes(t, e.Session)
								d, err := e.Decode()
								require.NoError(t, err)
								for _, key := range []string{"fields", "session"} {
									v, ok := d[key].(map[string]any)
									require.True(t, ok)
									hdlcSNGBTTypes(t, v)
								}
							}
							snapshots = append(snapshots, actionSnapshot(e))
							saved[i] = e
							actionPoison(e)
							clear(wire)
							hdlcOpQueries(t, e, bound.Fields, bound.Error)
						}
						hdlcOpCheck(t, c, snapshots, tr, false, deferred)
						closed := s.Close("test-close")
						for ci, ce := range closed {
							require.Equal(t, flow, ce.FlowID)
							for _, fid := range ids {
								require.NotEqual(t, fid, ce.ID)
							}
							for j := 0; j < ci; j++ {
								require.NotEqual(t, closed[j].ID, ce.ID)
							}
						}
						hdlcSNGBTCloseCheck(t, c, closed, tr, "test-close")
						require.Empty(t, s.Close("repeat"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.EqualValues(t, len(c.Close.Events), s.Stats().Incomplete)
						for i, e := range saved {
							hdlcOpQueries(t, e, c.Steps[i].Fields, c.Steps[i].Error)
						}
					})
				}
			}
		}
	}
}
func TestDLMSHDLCSNGBTExistingAPI(t *testing.T) {
	hdlcSNGBTPublicAPINative(t, hdlcSNGBTIndependentCases(t, true))
}
func TestDLMSHDLCSNGBTIndependentNative(t *testing.T) {
	hdlcSNGBTPublicAPINative(t, hdlcSNGBTIndependentCases(t, false))
}

func hdlcSNGBTCloseCheck(t *testing.T, c hdlcOpCase, events []*ProtocolEvent, tr, reason string) {
	t.Helper()
	require.Len(t, events, len(c.Close.Events))
	for i, e := range events {
		w := c.Close.Events[i]
		require.Equal(t, "dlms", e.Protocol)
		require.Equal(t, tr, e.Transport)
		require.Equal(t, CaptureDomain{}, e.Domain)
		require.Equal(t, w.Profile, e.Profile)
		require.Equal(t, w.Status, e.Status)
		require.Equal(t, w.Completeness, e.Completeness)
		require.Equal(t, w.Direction, e.Direction)
		require.NotZero(t, e.ID)
		require.NotZero(t, e.FlowID)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
		require.Nil(t, e.Raw)
		require.Zero(t, e.Length)
		summary, at, offset := w.SummaryUDP, w.TimeUDP, w.OffsetUDP
		if tr == "tcp" {
			summary, at, offset = strings.Replace(w.SummaryTCP, "<reason>", reason, 1), w.TimeTCP, w.OffsetTCP
		}
		require.Equal(t, summary, e.Summary)
		require.Equal(t, (&ProtocolError{Kind: ErrNeedMore, Message: summary}).Error(), e.Error)
		require.Equal(t, at, e.Timestamp.Unix())
		require.Equal(t, offset, e.Offset)
		hdlcOpEqual(t, w.Session, e.Session)
		require.Len(t, e.Session, 6)
		for _, k := range []string{"Outstanding", "observed_block_count", "assembled_data_length"} {
			require.IsType(t, int(0), e.Session[k])
		}
		for _, k := range []string{"source_address", "destination_address"} {
			require.IsType(t, uint32(0), e.Session[k])
		}
		require.IsType(t, uint16(0), e.Session["block_number"])
		ends := []string{"192.0.2.1:41000", "192.0.2.2:4059"}
		require.Equal(t, ends[w.Direction], e.Source)
		require.Equal(t, ends[1-w.Direction], e.Destination)
		for n := 0; n < 2; n++ {
			fields, err := e.GetFields()
			require.Nil(t, fields)
			rocTypedError(t, "NeedMore", err)
			var pe *ProtocolError
			require.ErrorAs(t, err, &pe)
			require.Equal(t, summary, pe.Message)
			pe.Kind, pe.Message = ErrMalformedMessage, "caller mutation"
			decoded, err := e.Decode()
			require.Nil(t, decoded)
			rocTypedError(t, "NeedMore", err)
			require.ErrorAs(t, err, &pe)
			require.Equal(t, summary, pe.Message)
			pe.Kind, pe.Message = ErrMalformedMessage, "caller mutation"
			actionPoison(e)
		}
	}
}
func hdlcSNGBTSupplement(t *testing.T, name, pin string, count int) []hdlcOpCase {
	raw := finsReadBatch(t, "dlms-hdlc-sn-gbt-source", "DLMS_HDLC_SN_GBT_TEST_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, count)
	return d.Cases
}
func TestDLMSHDLCSNGBTCloseDepthNative(t *testing.T) {
	cases := hdlcSNGBTSupplement(t, "close-depth-supplement-v2.json", "a1aaed04e65713d5231eaba3d4b5c5cb9961572603c249a51e4c545b6a74b88b", 3)
	cases = append(cases, hdlcSNGBTSupplement(t, "response-close-supplement-v2.json", "b8713f0175f19114923db28fc1299111ae24fa94360976c3c2d602a4864bddc7", 1)...)
	hdlcSNGBTPublicAPINative(t, cases)
}
func TestDLMSHDLCSNGBTLLCDirectionNative(t *testing.T) {
	hdlcSNGBTPublicAPINative(t, hdlcSNGBTSupplement(t, "llc-command-supplement.json", "3cc11228956cecbd47480a3beb46cd3286441fdd00ab9409bb7b0499ca01fbd4", 1))
}

func TestDLMSHDLCSNGBTChangedFullframeNative(t *testing.T) {
	raw := finsReadBatch(t, "dlms-hdlc-sn-gbt-source", "DLMS_HDLC_SN_GBT_TEST_ROOT", "CHANGED-FULLFRAME-STATIC-WITNESS.json")
	require.Equal(t, "5fe826b97d023a92324b962ff942444553ad564676e8c83dac81d0ba708cf10b", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Case hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Case.Steps, 6)
	normal := hdlcSNGBTIndependentCases(t, true)[2]
	hdlcSNGBTPublicAPINative(t, []hdlcOpCase{normal, d.Case})
}

func TestDLMSHDLCSNGBTRefusalHintNoAlloc(t *testing.T) {
	c := hdlcSNGBTIndependentCases(t, true)[0]
	for _, i := range []int{0, 2} {
		wire := wrapperWire(t, c.Steps[i].Wire)
		require.True(t, dlmsUDPRefusalHint(wire))
		allocs := testing.AllocsPerRun(100, func() {
			if !dlmsUDPRefusalHint(wire) {
				panic("fixed refusal hint changed")
			}
		})
		require.Zero(t, allocs, "resource refusal only inspects bounded header/count without copying APDU")
	}
}

func TestDLMSHDLCSNGBTUInt32ContextNative(t *testing.T) {
	hdlcSNGBTPublicAPINative(t, hdlcSNGBTSupplement(t, "uint32-context-supplement.json", "5f526d3f4e9715822d3821740b46a1f75e2d3911b23dd5918c059e902dd0d88f", 3))
}

func TestDLMSHDLCSNGBTChangedFrameACKNative(t *testing.T) {
	cases := hdlcSNGBTSupplement(t, "changed-fullframe-ack-supplement.json", "5571287760f394a6e537ec535158f751f1ccb860284f16f86f5ac2466934bdc8", 1)
	normal := hdlcSNGBTIndependentCases(t, true)[2]
	hdlcSNGBTPublicAPINative(t, append([]hdlcOpCase{normal}, cases...))
}
