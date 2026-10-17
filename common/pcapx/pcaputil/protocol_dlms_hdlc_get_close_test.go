package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func hdlcGetCloseCases(t *testing.T) []hdlcOpCase {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-get-close", "DLMS_HDLC_GET_CLOSE_TEST_ROOT", "api-controls.json")
	require.Equal(t, "e056252536c0a17982b791789a2095095a2684f8724e6ed3ce2cfe3cb07e4c82", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &x))
	require.Len(t, x.Cases, 6)
	return x.Cases
}

func hdlcGetCloseCheck(t *testing.T, c hdlcOpCase, es []*ProtocolEvent, tr string, flow uint64, reverse bool, reason string) {
	hdlcGetCloseCheckEndpoints(t, c, es, tr, flow, reverse, reason, []string{"192.0.2.1:41000", "192.0.2.2:4059"})
}

func hdlcGetCloseCheckEndpoints(t *testing.T, c hdlcOpCase, es []*ProtocolEvent, tr string, flow uint64, reverse bool, reason string, ends []string) {
	t.Helper()
	require.Len(t, es, len(c.Close.Events))
	require.Len(t, ends, 2)
	for i, e := range es {
		w := c.Close.Events[i]
		snBlock := w.Profile == "dlms-hdlc-sn-read-block" || w.Profile == "dlms-hdlc-sn-read-upload" || w.Profile == "dlms-hdlc-sn-write-upload"
		snGBTInformation := w.Profile == "dlms-hdlc-sn-gbt-information-segmented"
		dir := w.Direction
		if reverse {
			dir = 1 - dir
		}
		require.NotZero(t, e.ID)
		require.Equal(t, flow, e.FlowID)
		require.NotZero(t, e.FlowID)
		require.Equal(t, "dlms", e.Protocol)
		require.Equal(t, w.Profile, e.Profile)
		require.Equal(t, "incomplete", e.Status)
		require.Equal(t, "incomplete", e.Completeness)
		require.Equal(t, tr, e.Transport)
		require.Equal(t, dir, e.Direction)
		require.Equal(t, ends[dir], e.Source)
		require.Equal(t, ends[1-dir], e.Destination)
		require.Equal(t, CaptureDomain{}, e.Domain)
		require.Nil(t, e.Raw)
		require.Zero(t, e.Length)
		require.Zero(t, e.TransactionID)
		require.Zero(t, e.ResponseTo)
		at, off, summary := w.TimeUDP, w.OffsetUDP, w.SummaryUDP
		if tr == "tcp" {
			at, off, summary = w.TimeTCP, w.OffsetTCP, reason+": DLMS HDLC exchange ended with an unmatched request"
		}
		if w.Profile == "dlms-hdlc-sn-information-segmented" && tr == "tcp" {
			require.Equal(t, "<reason>: "+w.SummaryUDP, w.SummaryTCP)
			summary = strings.Replace(w.SummaryTCP, "<reason>", reason, 1)
		}
		if (snBlock || snGBTInformation) && tr == "tcp" {
			require.Equal(t, "test-close: "+w.SummaryUDP, w.SummaryTCP)
			summary = strings.Replace(w.SummaryTCP, "test-close:", reason+":", 1)
		}
		require.Equal(t, time.Unix(at, 0), e.Timestamp)
		require.Equal(t, off, e.Offset)
		require.Equal(t, summary, e.Summary)
		expectedError := summary
		if snBlock {
			expectedError = "NeedMore: " + summary
		}
		require.Equal(t, expectedError, e.Error)
		if snBlock {
			require.Len(t, w.Session, 6)
			require.Len(t, e.Session, 6)
			lengthKey := "assembled_data_length"
			if w.Profile != "dlms-hdlc-sn-read-block" {
				lengthKey = "assembled_body_length"
			}
			for _, key := range []string{"Buffered SN Groups", "observed_block_count", lengthKey} {
				require.IsType(t, int(0), e.Session[key])
			}
			for _, key := range []string{"source_address", "destination_address"} {
				require.IsType(t, uint32(0), e.Session[key])
			}
			require.IsType(t, uint16(0), e.Session["block_number"])
			require.Equal(t, 1, e.Session["Buffered SN Groups"])
			hdlcOpEqual(t, w.Session, e.Session)
		} else if w.Profile == "dlms-hdlc-sn-information-segmented" || snGBTInformation {
			require.Len(t, w.Session, 3)
			require.Len(t, e.Session, 3)
			for _, key := range []string{"Outstanding", "ObservedFrames", "InformationBytes"} {
				require.Contains(t, w.Session, key)
				require.IsType(t, int(0), e.Session[key])
			}
			require.Equal(t, 1, e.Session["Outstanding"])
			require.Positive(t, e.Session["ObservedFrames"])
			require.Positive(t, e.Session["InformationBytes"])
			hdlcOpEqual(t, w.Session, e.Session)
		} else {
			require.Equal(t, map[string]any{"Outstanding": 1}, e.Session)
		}
		for n := 0; n < 3; n++ {
			f, err := e.GetFields()
			require.Nil(t, f)
			rocTypedError(t, "NeedMore", err)
			pe := err.(*ProtocolError)
			require.Equal(t, summary, pe.Message)
			pe.Kind = ErrMalformedMessage
			pe.Message = "poison"
			d, de := e.Decode()
			require.Nil(t, d)
			rocTypedError(t, "NeedMore", de)
			p := de.(*ProtocolError)
			require.Equal(t, summary, p.Message)
			p.Kind = ErrResourceExceeded
			p.Message = "poison"
			actionPoison(e)
		}
	}
}

// Only established public session APIs and old helper symbols are used, so
// compilation succeeds on the pre-fix implementation. The source-bound gold
// requires the missing UDP diagnostic and typed TCP diagnostic, not new APIs.
func TestDLMSHDLCGetCloseExistingAPI(t *testing.T) { hdlcGetCloseNative(t, false) }
func TestDLMSHDLCGetCloseNative(t *testing.T)      { hdlcGetCloseNative(t, true) }
func hdlcGetCloseNative(t *testing.T, reversed bool) {
	for _, original := range hdlcGetCloseCases(t) {
		for _, rev := range []bool{false, true} {
			if rev && !reversed {
				continue
			}
			for _, tr := range []string{"tcp", "udp"} {
				for _, deferred := range []bool{false, true} {
					for _, chunk := range []int{0, 1, 7} {
						if tr == "udp" && chunk != 0 {
							continue
						}
						t.Run(fmt.Sprintf("%s/%s/d%v/chunk%d/rev%v", original.Name, tr, deferred, chunk, rev), func(t *testing.T) {
							c := original
							c.Steps = append([]hdlcOpStep(nil), original.Steps...)
							s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(tr), WithSessionPorts(41000, 4059))
							require.NoError(t, err)
							defer s.Close("cleanup")
							s.(*captureSession).f.a.config.Deferred = deferred
							var es, saved []*ProtocolEvent
							var flow uint64
							for i, st := range c.Steps {
								if rev {
									st.Direction = 1 - st.Direction
									c.Steps[i].Direction = st.Direction
								}
								wire := wrapperWire(t, st.Wire)
								require.Equal(t, st.SHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
								before := len(es)
								var lastErr *ProtocolError
								for at := 0; at < len(wire); {
									end := len(wire)
									if chunk > 0 {
										end = min(end, at+chunk)
									}
									in := bytes.Clone(wire[at:end])
									r := s.Feed(st.Direction, time.Unix(st.At, 0), in)
									clear(in)
									require.Equal(t, end-at, r.Consumed)
									if len(r.Events) == 0 {
										require.True(t, r.NeedMore)
										rocTypedError(t, "NeedMore", r.Err)
									} else {
										require.False(t, r.NeedMore)
										lastErr = r.Err
									}
									for _, e := range r.Events {
										es = append(es, actionSnapshot(e))
										saved = append(saved, e)
										if flow == 0 {
											flow = e.FlowID
										}
										require.Equal(t, flow, e.FlowID)
										require.NotZero(t, e.FlowID)
										if st.Error == "" {
											f, err := e.GetFields()
											require.NoError(t, err)
											hdlcOpEqual(t, st.Fields, f)
											hdlcOpTypes(t, f)
											poisonWrapperStructured(f)
										}
										actionPoison(e)
									}
									at = end
								}
								require.Equal(t, before+1, len(es))
								if st.Error == "" {
									require.Nil(t, lastErr)
								} else {
									rocTypedError(t, st.Error, lastErr)
								}
							}
							hdlcOpCheck(t, c, es, tr, false, deferred)
							closed := s.Close("test-close")
							hdlcGetCloseCheck(t, c, closed, tr, flow, rev, "test-close")
							require.Empty(t, s.Close("again"))
							require.Zero(t, s.Stats().BufferedBytes)
							for i, e := range saved {
								hdlcOpQueries(t, e, c.Steps[i].Fields, c.Steps[i].Error)
							}
						})
					}
				}
			}
		}
	}
}

// The two foreign-flow captures keep their original inputs and all original
// event answers. This sidecar adds only the pending GET Close observation.
func hdlcGetCloseHistoricalItems(t *testing.T) []wrapperCloseHistory {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-get-close", "DLMS_HDLC_GET_CLOSE_TEST_ROOT", "historical-GET-close-upgrades.json")
	require.Equal(t, "50150dc27d1c2c051375ba33e192846bfab62a4b3ef364f0aa5808b262aa3c5e", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var doc struct{ Items []wrapperCloseHistory }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Items, 2)
	return doc.Items
}
func hdlcGetCloseHistorical(t *testing.T, es []*ProtocolEvent, name, alias string) {
	t.Helper()
	var w *wrapperCloseHistory
	for _, v := range hdlcGetCloseHistoricalItems(t) {
		if v.Name == name {
			q := v
			w = &q
			break
		}
	}
	require.NotNil(t, w)
	require.Equal(t, w.InputAlias, alias)
	require.Equal(t, "udp", w.Transport)
	require.Len(t, es, 1)
	e := es[0]
	require.NotZero(t, e.ID)
	require.NotZero(t, e.FlowID)
	require.Zero(t, e.TransactionID)
	require.Zero(t, e.ResponseTo)
	require.Equal(t, "dlms", e.Protocol)
	require.Equal(t, w.Profile, e.Profile)
	require.Equal(t, "udp", e.Transport)
	require.Equal(t, "incomplete", e.Status)
	require.Equal(t, "incomplete", e.Completeness)
	require.Equal(t, w.Direction, e.Direction)
	require.Equal(t, CaptureDomain{Interface: w.Interface}, e.Domain)
	require.Equal(t, w.Source, e.Source)
	require.Equal(t, w.Destination, e.Destination)
	require.Equal(t, w.CaptureTime, w.NativeTime)
	require.Equal(t, w.CaptureOffset, w.NativeOffset)
	require.Equal(t, w.CaptureTime, e.Timestamp.UnixNano())
	require.Equal(t, w.CaptureOffset, e.Offset)
	require.Nil(t, e.Raw)
	require.Zero(t, e.Length)
	require.Empty(t, e.SourceBytes.PacketRefs)
	require.Equal(t, map[string]any{"Outstanding": 1}, e.Session)
	require.IsType(t, int(0), e.Session["Outstanding"])
	summary := "DLMS HDLC exchange ended with an unmatched request"
	require.Equal(t, summary, e.Summary)
	require.Equal(t, summary, e.Error)
	wrapperCloseNeedMore(t, e, summary)
	actionPoison(e)
	wrapperCloseNeedMore(t, e, summary)
}
func TestDLMSHDLCGetCloseHistoricalBinding(t *testing.T) {
	for _, w := range hdlcGetCloseHistoricalItems(t) {
		t.Run(w.Name, func(t *testing.T) {
			raw := wrapperStructuredInput(t, "common/pcapx/pcaputil/"+w.MemberAlias)
			require.Equal(t, w.MemberSHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
			var d struct {
				Cases []struct {
					ID, Capture, SHA256 string
					Events              []udpContextEvent
				}
			}
			require.NoError(t, json.Unmarshal(raw, &d))
			require.GreaterOrEqual(t, w.Index, 0)
			require.Less(t, w.Index, len(d.Cases))
			c := d.Cases[w.Index]
			require.Equal(t, w.Name, c.ID)
			require.Equal(t, w.InputAlias, "common/pcapx/pcaputil/udp-context/"+c.Capture)
			require.Equal(t, w.CaptureSHA, c.SHA256)
			capture := wrapperStructuredInput(t, w.InputAlias)
			require.Equal(t, w.CaptureSHA, fmt.Sprintf("%x", sha256.Sum256(capture)))
			require.Len(t, c.Events, 3)
			foreign := c.Events[1]
			require.Equal(t, "dlms", foreign.Protocol)
			require.Equal(t, int(w.NativeTime/1000000000), int(foreign.Timestamp))
			require.Equal(t, w.Interface, foreign.Interface)
			require.Equal(t, 0, w.Direction)
			require.Equal(t, 0, foreign.Direction)
			require.Equal(t, "dlms-hdlc-get-normal", w.Profile)
		})
	}
}
