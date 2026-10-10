package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type wrapperControl struct {
	Name, Transport, File, SHA256 string
	Port                          uint16
	Steps                         []struct {
		Dir int
		Hex string
	}
	CompleteFrames []string                       `json:"complete_frames_hex"`
	Fields         []map[string]any               `json:"expected_fields"`
	Error          *struct{ Kind, Detail string } `json:"expected_validation_error"`
	Session        map[string]any                 `json:"expected_session"`
	Evidence       []struct {
		Hex  string
		Refs []int `json:"packet_refs"`
	} `json:"message_evidence"`
	Packets int
}

func wrapperControls(t *testing.T) []wrapperControl {
	t.Helper()
	raw, err := trafficfixture.ReadFile("dlms-wrapper/controls.json")
	require.NoError(t, err)
	var doc struct{ Cases []wrapperControl }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Len(t, doc.Cases, 39)
	var rows struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(raw, &rows))
	inventories, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for i, c := range doc.Cases {
		a, err := trafficfixture.ReadFile("dlms-wrapper/answers/" + c.Name + ".json")
		require.NoError(t, err)
		require.JSONEq(t, string(rows.Cases[i]), string(a))
		bound := 0
		for _, inventory := range inventories {
			for _, v := range inventory.Cases {
				if v.Input.OriginalPath != "common/pcapx/pcaputil/dlms-wrapper/"+c.File {
					continue
				}
				require.Equal(t, c.SHA256, v.Input.SHA256)
				for _, e := range v.Expectations {
					var b struct {
						Answer  string `json:"answer_file"`
						SHA     string `json:"answer_sha256"`
						Packets int    `json:"packet_count"`
					}
					require.NoError(t, json.Unmarshal(e.PayloadConstraints, &b))
					if b.Answer != "answers/"+c.Name+".json" {
						continue
					}
					bound++
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(a)), b.SHA)
					require.Equal(t, c.Packets, b.Packets)
				}
			}
		}
		require.Equal(t, 1, bound, c.Name)
		if c.Name == "wrapper-get-next-block" {
			doc.Cases[i].Fields = wrapperBlocksHistoricalFields(t, c)
			doc.Cases[i].Error = nil
		}
		if c.Name == "wrapper-get-selective-access" {
			// Preserve the hash-bound original refusal; the same input now has
			// an independent supported answer in the additive selected-access batch.
			doc.Cases[i].Fields = wrapperNormalAccessHistoricalFields(t, c)
			doc.Cases[i].Error = nil
			doc.Cases[i].Session = map[string]any{"outstanding_observed_requests": float64(1)}
		}
	}
	return doc.Cases
}
func wrapperWire(t *testing.T, s string) []byte {
	t.Helper()
	w, err := hex.DecodeString(s)
	require.NoError(t, err)
	return w
}
func wrapperFind(t *testing.T, name string) wrapperControl {
	t.Helper()
	for _, c := range wrapperControls(t) {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("missing Wrapper control %s", name)
	return wrapperControl{}
}
func TestDLMSWrapperSealedCompleteByteOracle(t *testing.T) {
	for _, c := range wrapperControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			var fields []map[string]any
			var failure error
			for _, hex := range c.CompleteFrames {
				m, err := decodeDLMSWrapper(wrapperWire(t, hex), 4096)
				if err != nil {
					failure = err
					continue
				}
				fields = append(fields, m.fields)
			}
			if c.Error != nil {
				rocTypedError(t, c.Error.Kind, failure)
			} else {
				require.NoError(t, failure)
			}
			require.Len(t, fields, len(c.Fields))
			for i, f := range fields {
				rocEqualFields(t, c.Fields[i], f)
			}
		})
	}
}
func TestDLMSWrapperSealedReplayMatrix(t *testing.T) {
	for _, c := range wrapperControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile("dlms-wrapper/" + c.File)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
				var extra []CaptureOption
				if c.Transport == "udp" {
					extra = append(extra, WithProtocolDecodeAs("udp", 4059, "dlms-wrapper"))
				}
				es, stats := discoveryReplay(t, bytes.Clone(raw), w, d, o, extra...)
				var msgs, diagnostics []*ProtocolEvent
				for _, e := range es {
					if e.Protocol != "dlms-wrapper" {
						require.Equal(t, "wrapper-hdlc-is-different-carrier", c.Name)
						continue
					}
					if e.Status == "incomplete" {
						diagnostics = append(diagnostics, e)
					} else {
						msgs = append(msgs, e)
					}
				}
				if c.Name == "wrapper-hdlc-is-different-carrier" {
					require.Empty(t, msgs)
					return
				}
				partial := c.Name == "wrapper-short-header" || c.Name == "wrapper-payload-short" || c.Name == "wrapper-budget-header-precheck"
				if partial {
					require.Empty(t, msgs)
					require.Len(t, diagnostics, 1)
					require.Nil(t, diagnostics[0].Session)
					require.Error(t, func() error { _, err := diagnostics[0].GetFields(); return err }())
					return
				}
				if c.Transport == "udp" && c.Name == "wrapper-udp-do-not-stitch" {
					require.Len(t, msgs, 2)
					for i, e := range msgs {
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
						require.Equal(t, wrapperWire(t, c.Steps[i].Hex), e.Raw)
						_, err := e.GetFields()
						rocTypedError(t, "MalformedMessage", err)
						require.Zero(t, e.ResponseTo)
					}
					require.Empty(t, diagnostics)
					return
				}
				if c.Error != nil {
					require.Len(t, msgs, 1)
					f, err := msgs[0].GetFields()
					rocTypedError(t, c.Error.Kind, err)
					require.Nil(t, f)
					require.Nil(t, msgs[0].Session)
					require.Empty(t, diagnostics)
					return
				}
				require.Len(t, msgs, len(c.Fields))
				require.EqualValues(t, len(msgs), stats.Messages)
				for i, e := range msgs {
					require.Empty(t, e.Error)
					require.Equal(t, wrapperProfile(wrapperWire(t, c.CompleteFrames[i])), e.Profile)
					require.Equal(t, wrapperWire(t, c.CompleteFrames[i]), e.Raw)
					f, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, c.Fields[i], f)
					require.Len(t, e.SourceBytes.PacketRefs, len(c.Evidence[i].Refs))
					for j, ref := range e.SourceBytes.PacketRefs {
						require.EqualValues(t, c.Evidence[i].Refs[j], ref.Number)
						require.Equal(t, e.Domain, ref.Domain)
					}
					require.Equal(t, false, f["authentication_verified"])
					f["kind"] = "caller"
					e.Session["kind"] = "caller"
					clear(e.Raw)
					f, err = e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, c.Fields[i], f)
				}
				if pairs, ok := c.Session["pairs"].([]any); ok {
					n := 0
					for _, e := range msgs {
						if e.ResponseTo != 0 {
							n++
						}
					}
					require.Equal(t, len(pairs), n)
					for _, x := range pairs {
						p := x.(map[string]any)
						q, r := int(p["request_message"].(float64)), int(p["response_message"].(float64))
						require.Equal(t, msgs[q].ID, msgs[r].ResponseTo)
						require.Equal(t, msgs[q].ID, msgs[r].TransactionID)
					}
				}
				if c.Name == "wrapper-tcp-split-header-payload" || c.Name == "wrapper-udp-complete-exchange" {
					require.Equal(t, msgs[0].ID, msgs[1].ResponseTo)
				}
				if c.Name == "wrapper-invoke-reuse-ambiguous" {
					require.Zero(t, msgs[2].ResponseTo)
					require.Equal(t, "ambiguous-invoke", msgs[2].Session["Association"])
				}
				wantOutstanding := 0
				if v, ok := c.Session["outstanding_observed_requests"].(float64); ok {
					wantOutstanding = int(v)
				}
				if c.Name == "wrapper-budget-exact-frame" || c.Name == "wrapper-source-dest-width16" {
					wantOutstanding = 1
				}
				if c.Transport == "tcp" && wantOutstanding > 0 {
					require.Len(t, diagnostics, 1)
					require.EqualValues(t, wantOutstanding, diagnostics[0].Session["Outstanding"])
				} else {
					require.Empty(t, diagnostics)
				}
			})
		})
	}
}
func TestDLMSWrapperAnnouncedLimitAndOwnedExchange(t *testing.T) {
	c := wrapperFind(t, "wrapper-get-octet-exchange")
	q, r := wrapperWire(t, c.Steps[0].Hex), wrapperWire(t, c.Steps[1].Hex)
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprint(deferred), func(t *testing.T) {
			newSession := func(b ParserBudget, transport string) *captureSession {
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(transport), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				return s.(*captureSession)
			}
			for _, frame := range []int{len(q) - 1, len(q)} {
				b := DefaultParserBudget()
				b.MaxFrameBytes = frame
				b.MaxMessageBytes = 64
				s := newSession(b, "tcp")
				out := s.Feed(0, time.Unix(100, 0), q[:8])
				if frame < len(q) {
					rocTypedError(t, "ResourceExceeded", out.Err)
					require.False(t, out.NeedMore)
					require.Zero(t, s.Stats().BufferedBytes)
				} else {
					require.True(t, out.NeedMore)
					out = s.Feed(0, time.Unix(100, 0), q[8:])
					require.Nil(t, out.Err)
					require.Len(t, out.Events, 1)
				}
				s.Close("budget")
				require.Zero(t, s.Stats().BufferedBytes)
			}
			b := DefaultParserBudget()
			b.MaxCollectionElements = 2
			s := newSession(b, "tcp")
			out := s.Feed(1, time.Unix(100, 0), r)
			rocTypedError(t, "ResourceExceeded", out.Err)
			require.Len(t, out.Events, 1)
			f, err := out.Events[0].GetFields()
			require.Error(t, err)
			require.Nil(t, f)
			require.Equal(t, "limited", out.Events[0].Status)
			s.Close("value")
			require.Zero(t, s.Stats().BufferedBytes)
			s = newSession(DefaultParserBudget(), "tcp")
			borrowed := bytes.Clone(q)
			out = s.Feed(0, time.Unix(100, 0), borrowed)
			require.Nil(t, out.Err)
			req := out.Events[0]
			clear(borrowed)
			out = s.Feed(1, time.Unix(101, 0), bytes.Clone(r))
			require.Nil(t, out.Err)
			require.Equal(t, req.ID, out.Events[0].ResponseTo)
			require.Empty(t, s.Close("complete"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			for i, e := range []*ProtocolEvent{req, out.Events[0]} {
				f, err := e.GetFields()
				require.NoError(t, err)
				rocEqualFields(t, c.Fields[i], f)
			}
			rocTypedError(t, "FatalSessionError", s.Feed(0, time.Time{}, q).Err)
		})
	}
}

func TestDLMSWrapperProbeBoundaryAndOffportSegmentation(t *testing.T) {
	c := wrapperFind(t, "wrapper-adversarial-udp-coalesced")
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
	require.NoError(t, err)
	wire := wrapperWire(t, c.Steps[0].Hex)
	require.Equal(t, ProbeReject, s.Probe(wire).Verdict)
	out := s.Feed(0, time.Unix(1, 0), wire)
	rocTypedError(t, "MalformedMessage", out.Err)
	require.Len(t, out.Events, 1)
	require.Nil(t, out.Events[0].Session)
	require.Zero(t, out.Events[0].ResponseTo)
	s.Close("udp")
	require.Zero(t, s.Stats().BufferedBytes)
	c = wrapperFind(t, "wrapper-adversarial-offport-split")
	wire = wrapperWire(t, c.CompleteFrames[0])
	for _, chunks := range [][]int{{len(wire)}, {64, len(wire) - 64}, {1, 1, 5, 1, 1, 55, len(wire) - 64}} {
		s, err = NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 4060))
		require.NoError(t, err)
		at := 0
		var events []*ProtocolEvent
		for _, n := range chunks {
			out = s.Feed(1, time.Unix(1, 0), wire[at:at+n])
			at += n
			events = append(events, out.Events...)
			if out.Err != nil {
				require.Equal(t, ErrNeedMore, out.Err.Kind)
			}
		}
		require.Equal(t, len(wire), at)
		require.Len(t, events, 1)
		f, err := events[0].GetFields()
		require.NoError(t, err)
		rocEqualFields(t, c.Fields[0], f)
		require.Zero(t, events[0].ResponseTo)
		require.Equal(t, wire, events[0].Raw)
		require.Empty(t, s.Close("offport"))
		require.Zero(t, s.Stats().BufferedBytes)
	}
}

func TestDLMSWrapperSharedPressureAndLifetime(t *testing.T) {
	c := wrapperFind(t, "wrapper-get-octet-exchange")
	q, r := wrapperWire(t, c.Steps[0].Hex), wrapperWire(t, c.Steps[1].Hex)
	ts := time.Unix(100, 0)
	for _, carrier := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			t.Run(carrier+fmt.Sprint(deferred), func(t *testing.T) {
				makeSession := func(b ParserBudget) *captureSession {
					s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(carrier), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					s.(*captureSession).f.a.config.Deferred = deferred
					return s.(*captureSession)
				}
				// The exact shared ceiling admits the first projection, while another
				// outstanding invoke needs a new slot. Denial must retire old association.
				b := DefaultParserBudget()
				b.MaxMessageBytes = 64
				b.MaxBufferedBytes = 7552
				s := makeSession(b)
				first := s.Feed(0, ts, q)
				require.Nil(t, first.Err)
				require.Len(t, first.Events, 1)
				require.NotZero(t, first.Events[0].TransactionID)
				q2 := wrapperFind(t, "wrapper-adversarial-two-invokes")
				next := wrapperWire(t, q2.Steps[1].Hex)
				denied := s.Feed(0, ts, next)
				rocTypedError(t, "ResourceExceeded", denied.Err)
				require.Len(t, denied.Events, 1)
				f, err := denied.Events[0].GetFields()
				require.Error(t, err)
				require.Nil(t, f)
				late := s.Feed(1, ts.Add(time.Second), r)
				for _, e := range late.Events {
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
				}
				if carrier == "tcp" {
					rocTypedError(t, "FatalSessionError", late.Err)
				} else {
					require.Nil(t, late.Err)
					require.Len(t, late.Events, 1)
					require.Equal(t, "ambiguous-conversation", late.Events[0].Session["Association"])
					f, err := late.Events[0].GetFields()
					require.NoError(t, err)
					rocEqualFields(t, c.Fields[1], f)
				}
				s.Close("pressure")
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
				b.MaxBufferedBytes--
				s = makeSession(b)
				denied = s.Feed(0, ts, q)
				rocTypedError(t, "ResourceExceeded", denied.Err)
				s.Close("under")
				require.Zero(t, s.Stats().BufferedBytes)
				// A too-small field depth never publishes partial fields or a pending ID.
				b = DefaultParserBudget()
				b.MaxRecursionDepth = 1
				s = makeSession(b)
				denied = s.Feed(0, ts, q)
				rocTypedError(t, "ResourceExceeded", denied.Err)
				require.Zero(t, denied.Events[0].TransactionID)
				require.Nil(t, denied.Events[0].Session)
				s.Close("depth")
				require.Zero(t, s.Stats().BufferedBytes)
				if carrier == "udp" {
					b = DefaultParserBudget()
					b.MaxMessageBytes = 64
					b.MaxBufferedBytes = 7552
					s = makeSession(b)
					first = s.Feed(0, ts, q)
					require.Nil(t, first.Err)
					late = s.Feed(1, ts.Add(wrapperIdleTTL), r)
					require.Nil(t, late.Err)
					require.Zero(t, late.Events[0].ResponseTo)
					require.Equal(t, "ambiguous-conversation", late.Events[0].Session["Association"])
					require.EqualValues(t, 512, s.Stats().BufferedBytes)
					fresh := s.Feed(0, ts.Add(wrapperIdleTTL), q)
					require.Nil(t, fresh.Err)
					require.Equal(t, first.Events[0].FlowID, fresh.Events[0].FlowID)
					require.Zero(t, fresh.Events[0].TransactionID)
					require.Equal(t, "ambiguous-conversation", fresh.Events[0].Session["Association"])
					s.Close("expired")
					require.Zero(t, s.Stats().BufferedBytes)
				}
			})
		}
	}
}
