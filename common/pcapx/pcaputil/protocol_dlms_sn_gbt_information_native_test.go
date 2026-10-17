package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

type hdlcSNGBTInformationMeta struct {
	Name  string
	Steps []struct {
		Message string `json:"error_message"`
		Public  string `json:"public_error"`
	}
	Close struct {
		Events []struct {
			TCP string `json:"public_error_tcp"`
			UDP string `json:"public_error_udp"`
		}
	}
}

func hdlcSNGBTInformationLoad(t *testing.T, name, pin string, count int) ([]hdlcOpCase, map[string]hdlcSNGBTInformationMeta) {
	t.Helper()
	raw := finsReadBatch(t, "dlms-hdlc-sn-gbt-information-source", "DLMS_HDLC_SN_GBT_INFORMATION_ROOT", name)
	require.Equal(t, pin, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var d struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, count)
	var q struct{ Cases []hdlcSNGBTInformationMeta }
	require.NoError(t, json.Unmarshal(raw, &q))
	require.Len(t, q.Cases, len(d.Cases))
	m := map[string]hdlcSNGBTInformationMeta{}
	for i, c := range d.Cases {
		require.Equal(t, c.Name, q.Cases[i].Name)
		require.Len(t, q.Cases[i].Steps, len(c.Steps))
		require.Len(t, q.Cases[i].Close.Events, len(c.Close.Events))
		m[c.Name] = q.Cases[i]
	}
	return d.Cases, m
}

func TestDLMSHDLCSNGBTInformationNative(t *testing.T) {
	c, m := hdlcSNGBTInformationLoad(t, "full-controls.json", "c2a45190a86c00e4a12404182e04d3efb10d77e424752be94c3f2d61415a60b9", 16)
	hdlcSNGBTInformationNative(t, c, m)
}
func TestDLMSHDLCSNGBTInformationIdentityNative(t *testing.T) {
	c, m := hdlcSNGBTInformationLoad(t, "same-session-identity-supplement.json", "65f886c45dfe91d74782dd08246abf8ff1a50619ef4ef4fcb08500c005a2ac30", 1)
	hdlcSNGBTInformationNative(t, c, m)
}
func hdlcSNGBTInformationTypes(t *testing.T, f map[string]any) {
	t.Helper()
	hdlcSNServiceGBTTypes(t, f)
	if v, ok := f["HDLC Segmentation"]; ok {
		q, ok := v.(map[string]any)
		require.True(t, ok)
		require.IsType(t, false, q["more"])
		require.IsType(t, false, q["complete"])
		require.IsType(t, int(0), q["observed_frames"])
		require.IsType(t, "", q["information_hex"])
		if q["complete"] == true {
			require.IsType(t, "", q["assembled_information_hex"])
		}
	}
}
func hdlcSNGBTInformationErrorOwnership(t *testing.T, e *ProtocolEvent, kind string) {
	t.Helper()
	fields, err := e.GetFields()
	require.Nil(t, fields)
	rocTypedError(t, kind, err)
	var pe *ProtocolError
	require.ErrorAs(t, err, &pe)
	message := pe.Message
	require.NotEmpty(t, message)

	for i := 0; i < 2; i++ {
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
func hdlcSNGBTInformationClose(t *testing.T, c hdlcOpCase, meta hdlcSNGBTInformationMeta, events []*ProtocolEvent, tr, reason string) {
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
			// The independent native gold fixes the caller reason to test-close.
			// CaptureAnalyzer.Close supplies ctx-canceled; only that caller
			// prefix varies, never the protocol diagnostic or its state.
			require.True(t, strings.HasPrefix(w.SummaryTCP, "test-close: "))
			summary, at, offset = reason+strings.TrimPrefix(w.SummaryTCP, "test-close"), w.TimeTCP, w.OffsetTCP
		}
		require.Equal(t, summary, e.Summary)
		public := meta.Close.Events[i].UDP
		if tr == "tcp" {
			require.True(t, strings.Contains(meta.Close.Events[i].TCP, "test-close: "))
			public = strings.Replace(meta.Close.Events[i].TCP, "test-close:", reason+":", 1)
		}
		require.Equal(t, public, e.Error)
		require.Equal(t, at, e.Timestamp.Unix())
		require.Equal(t, offset, e.Offset)
		hdlcOpEqual(t, w.Session, e.Session)
		if len(w.Session) == 3 {
			require.Len(t, e.Session, 3)
			for _, k := range []string{"Outstanding", "ObservedFrames", "InformationBytes"} {
				require.IsType(t, int(0), e.Session[k])
			}
		} else {
			require.Len(t, e.Session, 6)
			counter, length := "Outstanding", "assembled_data_length"
			if _, ok := w.Session["Buffered SN Groups"]; ok {
				counter = "Buffered SN Groups"
			}
			if _, ok := w.Session["assembled_body_length"]; ok {
				length = "assembled_body_length"
			}
			for _, k := range []string{counter, "observed_block_count", length} {
				require.IsType(t, int(0), e.Session[k])
			}
			for _, k := range []string{"source_address", "destination_address"} {
				require.IsType(t, uint32(0), e.Session[k])
			}
			require.IsType(t, uint16(0), e.Session["block_number"])
		}
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

func hdlcSNGBTInformationNative(t *testing.T, cases []hdlcOpCase, meta map[string]hdlcSNGBTInformationMeta) {
	typeCheck := hdlcSNGBTInformationTypes
	errorCheck := hdlcSNGBTInformationErrorOwnership
	closeCheck := func(t *testing.T, c hdlcOpCase, es []*ProtocolEvent, tr, reason string) {
		hdlcSNGBTInformationClose(t, c, meta[c.Name], es, tr, reason)
	}
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
							if st.Error != "" {
								require.Equal(t, meta[c.Name].Steps[i].Message, last.Message)
								require.Equal(t, meta[c.Name].Steps[i].Public, e.Error)
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

							// Exact message policy belongs to the supplied errorCheck; the
							// published default checker retains its original LLC assertion.
							bound := hdlcSNGBTBind(st, ids)
							c.Steps[i] = bound
							if st.Error != "" {
								// Bind the separately owned FeedResult message to the independently
								// classified event before poisoning either returned error object.
								_, queryErr := e.GetFields()
								var ownedError *ProtocolError
								require.ErrorAs(t, queryErr, &ownedError)
								require.Equal(t, ownedError.Message, last.Message)
								last.Kind, last.Message = ErrNeedMore, "caller mutation"
								errorCheck(t, e, st.Error)
							}
							hdlcOpQueries(t, e, bound.Fields, bound.Error)
							if st.Error == "" {
								f, err := e.GetFields()
								require.NoError(t, err)
								typeCheck(t, f)
								typeCheck(t, e.Session)
								d, err := e.Decode()
								require.NoError(t, err)
								for _, key := range []string{"fields", "session"} {
									v, ok := d[key].(map[string]any)
									require.True(t, ok)
									typeCheck(t, v)
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
						closeCheck(t, c, closed, tr, "test-close")
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

func TestDLMSHDLCSNGBTInformationCountNative(t *testing.T) {
	c, m := hdlcSNGBTInformationLoad(t, "count-kind-controls.json", "fbd5379e9981d0c905f5acf5794b5860e409f0a82844827ac41d0346822c2e0c", 6)
	hdlcSNGBTInformationNative(t, c, m)
}
func TestDLMSHDLCSNGBTInformationReadModesNative(t *testing.T) {
	c, m := hdlcSNGBTInformationLoad(t, "read-mode-supplement.json", "79c322a25dfa2556564d92fa0dac71f2ef7b6d78d5a5d683a3811942328cdb97", 4)
	hdlcSNGBTInformationNative(t, c, m)
}

func TestDLMSHDLCSNGBTInformationAbsolutePoolNative(t *testing.T) {
	c, m := hdlcSNGBTInformationLoad(t, "information-hard-pool-supplement.json", "0e0b7c3fc5901bd8b1f11bd07d19789259e20a12a8a872bd78999b1fb9c1de2c", 2)
	hdlcSNGBTInformationNative(t, c, m)
}
