package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type wrapperListAnswer struct {
	Wire    string `json:"wire_hex"`
	SHA     string `json:"wire_sha256"`
	Refs    []int  `json:"packetRefs"`
	Fields  map[string]any
	Error   *struct{ Kind, Detail string }
	Default *struct {
		Fields map[string]any
		Error  *struct{ Kind, Detail string }
	} `json:"wire_default_budget_answer"`
}
type wrapperListControl struct {
	Name, Transport, File, SHA256 string
	OriginalPath                  string `json:"original_path"`
	Packets                       int
	Steps                         []struct {
		Dir int
		Hex string
		Ref int `json:"packetRef"`
	}
	Answers []wrapperListAnswer `json:"wholebyteanswers"`
	Limits  struct {
		List   int `json:"list_limit"`
		Octets int `json:"octet_limit"`
		Frame  int `json:"frame_limit"`
	} `json:"static_limits"`
	Targets struct {
		Pairs       [][]int
		Outstanding *int
		Association string
	} `json:"runtime_targets"`
}

func wrapperListMaterial(t *testing.T, name string) []byte {
	t.Helper()
	b, err := trafficfixture.ReadFile("dlms-get-list/" + name)
	require.NoError(t, err)
	return b
}
func wrapperListControls(t *testing.T) []wrapperListControl {
	t.Helper()
	raw := wrapperListMaterial(t, "controls.json")
	var doc struct{ Cases []wrapperListControl }
	var rows struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.NoError(t, json.Unmarshal(raw, &rows))
	require.Len(t, doc.Cases, 51)
	inventories, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for i, c := range doc.Cases {
		a := wrapperListMaterial(t, "answers/"+c.Name+".json")
		require.JSONEq(t, string(rows.Cases[i]), string(a))
		bound := 0
		for _, inventory := range inventories {
			for _, v := range inventory.Cases {
				if v.Input.OriginalPath != "common/pcapx/pcaputil/dlms-get-list/"+c.OriginalPath {
					continue
				}
				require.Equal(t, c.File, v.Input.File)
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
		// The immutable batch records the previous scalar-only support boundary.
		// Reuse its exact capture with a new independently validated full answer;
		// keep the original UnsupportedFeature answer available for historical use.
		if c.Name == "list-data-structure-open" {
			require.Len(t, c.Answers, 1)
			require.Equal(t, "UnsupportedFeature", c.Answers[0].Error.Kind)
			doc.Cases[i].Answers[0].Fields = wrapperStructuredHistoricalFields(t, c.Answers[0].Wire)
			doc.Cases[i].Answers[0].Error = nil
		}

	}
	return doc.Cases
}
func wrapperListBudget(c wrapperListControl) ParserBudget {
	b := DefaultParserBudget()
	if c.Limits.List > 0 {
		b.MaxCollectionElements = c.Limits.List
	}
	if c.Limits.Octets > 0 {
		b.MaxCollectionElements = c.Limits.Octets
	}
	if c.Limits.Frame > 0 {
		b.MaxFrameBytes = c.Limits.Frame
	}
	return b
}
func poisonWrapperListFields(f map[string]any) {
	f["kind"] = "caller"
	for _, key := range []string{"descriptors", "results"} {
		if rows, ok := f[key].([]map[string]any); ok {
			for _, row := range rows {
				row["raw_hex"] = "caller"
				for _, key := range []string{"data", "access_parameters"} {
					if inner, ok := row[key].(map[string]any); ok {
						inner["raw_hex"] = "caller"
					}
				}
			}
		}
	}
}
func TestDLMSWrapperGetListSealedByteOracle(t *testing.T) {
	for _, c := range wrapperListControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			b := wrapperListBudget(c)
			for _, a := range c.Answers {
				w := wrapperWire(t, a.Wire)
				require.Equal(t, a.SHA, fmt.Sprintf("%x", sha256.Sum256(w)))
				_, err := wrapperFrameSize(w, b.MaxFrameBytes)
				var m *wrapperMessage
				if err == nil {
					m, err = decodeDLMSWrapper(w, b.MaxCollectionElements)
				}
				if a.Error != nil {
					rocTypedError(t, a.Error.Kind, err)
					require.Nil(t, m)
				} else {
					require.NoError(t, err)
					require.NotNil(t, m)
					rocEqualFields(t, a.Fields, m.fields)
				}
			}
		})
	}
}
func TestDLMSWrapperGetListSealedMatrix(t *testing.T) {
	for _, c := range wrapperListControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw := wrapperListMaterial(t, c.File)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				var extra []CaptureOption
				if c.Transport == "udp" {
					extra = append(extra, WithProtocolDecodeAs("udp", 4059, "dlms-wrapper"))
				}
				es, stats := discoveryReplay(t, raw, workers, deferred, observe, extra...)
				var msgs, diagnostics []*ProtocolEvent
				for _, e := range es {
					require.Equal(t, "dlms-wrapper", e.Protocol)
					if e.Status == "incomplete" {
						diagnostics = append(diagnostics, e)
					} else {
						msgs = append(msgs, e)
					}
				}
				if c.Name == "list-tcp-incomplete-close" || c.Name == "list-header-budget-only" {
					require.Empty(t, msgs)
					require.Len(t, diagnostics, 1)
					e := diagnostics[0]
					require.Nil(t, e.semanticFields)
					require.Zero(t, e.ResponseTo)
					require.Equal(t, wrapperWire(t, c.Steps[0].Hex), e.Raw)
					require.Equal(t, len(e.Raw), e.Length)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.EqualValues(t, c.Steps[0].Ref, e.SourceBytes.PacketRefs[0].Number)
					f, err := e.GetFields()
					require.EqualError(t, err, "protocol parser: event is incomplete, not an exact message")
					require.Equal(t, "incomplete", e.Completeness)
					require.Empty(t, e.Error)
					require.Contains(t, e.Summary, "exact message boundary was not reached")
					require.EqualValues(t, 1, stats.Incomplete)
					require.Nil(t, f)
					require.Zero(t, stats.Messages)
					return
				}
				if c.Name == "list-udp-never-stitch" {
					require.Len(t, msgs, 2)
					require.Empty(t, diagnostics)
					for i, e := range msgs {
						require.Equal(t, wrapperWire(t, c.Steps[i].Hex), e.Raw)
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						require.EqualValues(t, c.Steps[i].Ref, e.SourceBytes.PacketRefs[0].Number)
						f, err := e.GetFields()
						rocTypedError(t, "MalformedMessage", err)
						require.Nil(t, f)
						require.Zero(t, e.ResponseTo)
					}
					return
				}
				require.Len(t, msgs, len(c.Answers))
				require.EqualValues(t, len(msgs), stats.Messages)
				outstanding := 0
				invalid := false
				offset := [2]uint64{}
				for i, e := range msgs {
					a := c.Answers[i]
					fields, expectedError := a.Fields, a.Error
					if a.Default != nil {
						fields, expectedError = a.Default.Fields, a.Default.Error
					}
					require.Equal(t, "dlms-wrapper-v1-get-list", e.Profile)
					require.Equal(t, wrapperWire(t, a.Wire), e.Raw)
					require.Equal(t, len(wrapperWire(t, a.Wire)), e.Length)
					require.Len(t, e.SourceBytes.PacketRefs, len(a.Refs))
					direction := -1
					for _, s := range c.Steps {
						if s.Ref == a.Refs[0] {
							direction = s.Dir
						}
					}
					require.NotEqual(t, -1, direction)
					require.Equal(t, direction, e.Direction)
					if c.Transport == "tcp" {
						require.Equal(t, offset[direction], e.Offset)
						offset[direction] += uint64(len(e.Raw))
					}
					for j, ref := range e.SourceBytes.PacketRefs {
						require.EqualValues(t, a.Refs[j], ref.Number)
						require.Equal(t, e.Domain, ref.Domain)
					}
					f, err := e.GetFields()
					if expectedError != nil {
						rocTypedError(t, expectedError.Kind, err)
						require.Nil(t, f)
						require.Nil(t, e.Session)
						require.Zero(t, e.ResponseTo)
						invalid = true
						continue
					}
					require.NoError(t, err)
					require.Empty(t, e.Error)
					rocEqualFields(t, fields, f)
					if e.TransactionID == e.ID && fields["command"].(float64) == 192 {
						outstanding++
					}
					if e.ResponseTo != 0 {
						outstanding--
						require.Equal(t, e.ResponseTo, e.TransactionID)
						found := false
						for _, q := range msgs[:i] {
							if q.ID == e.ResponseTo {
								found = true
								require.NotEqual(t, q.Direction, e.Direction)
							}
						}
						require.True(t, found)
					}
					if n, ok := e.Session["Outstanding"].(int); ok {
						outstanding = n
					} else {
						require.Fail(t, "missing typed Outstanding")
					}
					poisonWrapperListFields(f)
					poisonWrapperListFields(e.Session)
					if e.Fields != nil {
						poisonWrapperListFields(e.Fields)
					}
					clear(e.Raw)
					f, err = e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, fields, f)
				}
				if c.Targets.Pairs != nil && c.Limits.Frame == 0 {
					got := 0
					for _, e := range msgs {
						if e.ResponseTo != 0 {
							got++
						}
					}
					require.Equal(t, len(c.Targets.Pairs), got)
					for _, p := range c.Targets.Pairs {
						require.Equal(t, msgs[p[0]-1].ID, msgs[p[1]-1].ResponseTo)
					}
				}
				if c.Targets.Association != "" && c.Limits.Frame == 0 {
					require.Equal(t, c.Targets.Association, msgs[len(msgs)-1].Session["Association"])
				}
				if c.Targets.Outstanding != nil && c.Limits.Frame == 0 {
					require.Equal(t, *c.Targets.Outstanding, outstanding)
				}
				if c.Transport == "tcp" && outstanding > 0 && !invalid {
					require.Len(t, diagnostics, 1)
					require.EqualValues(t, outstanding, diagnostics[0].Session["Outstanding"])
				} else {
					require.Empty(t, diagnostics)
				}
			})
		})
	}
}
func TestDLMSWrapperGetListPublicBudgetAndOwnership(t *testing.T) {
	all := wrapperListControls(t)
	by := map[string]wrapperListControl{}
	for _, c := range all {
		by[c.Name] = c
	}
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprint(deferred), func(t *testing.T) {
			for _, name := range []string{"list-count-budget-preflight", "list-octet-budget-preflight", "list-header-budget-only", "list-frame-budget-late-response"} {
				t.Run(name, func(t *testing.T) {
					c := by[name]
					s, err := NewProtocolSessionWithOptions(wrapperListBudget(c), WithSessionTransport("tcp"), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					s.(*captureSession).f.a.config.Deferred = deferred
					for i, st := range c.Steps {
						o := s.Feed(st.Dir, time.Unix(100+int64(i), 0), wrapperWire(t, st.Hex))
						if i == 0 {
							rocTypedError(t, "ResourceExceeded", o.Err)
						} else {
							rocTypedError(t, "FatalSessionError", o.Err)
							require.Empty(t, o.Events)
						}
						for _, e := range o.Events {
							f, err := e.GetFields()
							rocTypedError(t, "ResourceExceeded", err)
							require.Nil(t, f)
							require.Zero(t, e.ResponseTo)
						}
					}
					s.Close("refused")
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
			c := by["list-udp-refusal-blocks-pending"]
			s, err := NewProtocolSessionWithOptions(wrapperListBudget(c), WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			s.(*captureSession).f.a.config.Deferred = deferred
			for i, st := range c.Steps {
				o := s.Feed(st.Dir, time.Unix(100+int64(i), 0), wrapperWire(t, st.Hex))
				require.Len(t, o.Events, 1)
				e := o.Events[0]
				if i == 1 {
					rocTypedError(t, "ResourceExceeded", o.Err)
					f, err := e.GetFields()
					rocTypedError(t, "ResourceExceeded", err)
					require.Nil(t, f)
					require.Nil(t, e.Raw)
					require.Nil(t, e.Session)
					require.Zero(t, e.ResponseTo)
				} else {
					require.Nil(t, o.Err)
					f, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, c.Answers[i].Fields, f)
					if i == 2 {
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						require.Equal(t, "ambiguous-conversation", e.Session["Association"])
						require.EqualValues(t, 0, e.Session["Outstanding"])
					}
				}
			}
			s.Close("complete")
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
			c = by["list-mixed-data-error"]
			for _, chunk := range []int{1, 7, 64, 4096} {
				s, err = NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				var es []*ProtocolEvent
				for _, st := range c.Steps {
					w := wrapperWire(t, st.Hex)
					for at := 0; at < len(w); {
						n := min(chunk, len(w)-at)
						input := bytes.Clone(w[at : at+n])
						o := s.Feed(st.Dir, time.Unix(100, 0), input)
						clear(input)
						if o.Err != nil {
							require.Equal(t, ErrNeedMore, o.Err.Kind)
						}
						es = append(es, o.Events...)
						at += n
					}
				}
				require.Len(t, es, 2)
				require.Equal(t, es[0].ID, es[1].ResponseTo)
				require.Empty(t, s.Close("complete"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
				for i, e := range es {
					f, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, c.Answers[i].Fields, f)
				}
			}
		})
	}
}

// Both arms use existing ProtocolSession APIs and frozen byte/field answers.
// The same list arm compiled and failed UnsupportedFeature on the branch point.
func TestDLMSWrapperGetListPublicBaseline(t *testing.T) {
	normal := wrapperFind(t, "wrapper-get-octet-exchange")
	var selected wrapperListControl
	for _, c := range wrapperListControls(t) {
		if c.Name == "list-mixed-data-error" {
			selected = c
		}
	}
	for _, row := range []struct {
		name, wire string
		fields     map[string]any
	}{
		{"normal-neighbor", normal.Steps[0].Hex, normal.Fields[0]},
		{"list-positive", selected.Steps[0].Hex, selected.Answers[0].Fields},
	} {
		t.Run(row.name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			probe := s.Probe(wrapperWire(t, row.wire))
			require.Equal(t, ProbeAccept, probe.Verdict)
			require.Equal(t, "dlms-wrapper", probe.Protocol)
			wantVersion := "v1-get-normal"
			if row.name == "list-positive" {
				wantVersion = "v1-get-list"
			}
			require.Equal(t, wantVersion, probe.Version)
			require.Zero(t, s.Stats().BufferedBytes)
			out := s.Feed(0, time.Unix(1, 0), wrapperWire(t, row.wire))
			require.Nil(t, out.Err)
			require.Len(t, out.Events, 1)
			f, err := out.Events[0].GetFields()
			require.NoError(t, err)
			rocEqualFields(t, row.fields, f)
			end := s.Close("baseline")
			require.Len(t, end, 1)
			require.EqualValues(t, 1, end[0].Session["Outstanding"])
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	wrapperListContextAndProjection(t)
}
func wrapperListContextAndProjection(t *testing.T) {
	normal := wrapperFind(t, "wrapper-get-octet-exchange")
	by := map[string]wrapperListControl{}
	for _, c := range wrapperListControls(t) {
		by[c.Name] = c
	}
	list := by["list-mixed-data-error"]
	for _, transport := range []string{"tcp", "udp"} {
		for _, deferred := range []bool{false, true} {
			t.Run(fmt.Sprintf("context/%s/%t", transport, deferred), func(t *testing.T) {
				fresh := func(b ParserBudget) ProtocolSession {
					s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(transport), WithSessionPorts(40000, 4059))
					require.NoError(t, err)
					s.(*captureSession).f.a.config.Deferred = deferred
					return s
				}
				feed := func(s ProtocolSession, dir int, wire string) *ProtocolEvent {
					o := s.Feed(dir, time.Unix(100, 0), wrapperWire(t, wire))
					require.Nil(t, o.Err)
					require.Len(t, o.Events, 1)
					f, err := o.Events[0].GetFields()
					require.NoError(t, err)
					require.NotNil(t, f)
					return o.Events[0]
				}
				finish := func(s ProtocolSession) {
					s.Close("complete")
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				}
				// A syntax-valid normal reply cannot consume a pending list request.
				s := fresh(DefaultParserBudget())
				q := feed(s, 0, list.Steps[0].Hex)
				r := feed(s, 1, normal.Steps[1].Hex)
				require.Zero(t, r.ResponseTo)
				require.EqualValues(t, 1, r.Session["Outstanding"])
				r = feed(s, 1, list.Steps[1].Hex)
				require.Equal(t, q.ID, r.ResponseTo)
				require.EqualValues(t, 0, r.Session["Outstanding"])
				finish(s)
				// Reuse of an invoke across service choices invalidates both observations.
				s = fresh(DefaultParserBudget())
				feed(s, 0, normal.Steps[0].Hex)
				r = feed(s, 0, list.Steps[0].Hex)
				require.Equal(t, "ambiguous-invoke", r.Session["Association"])
				require.Zero(t, r.TransactionID)
				for _, wire := range []string{normal.Steps[1].Hex, list.Steps[1].Hex} {
					r = feed(s, 1, wire)
					require.Zero(t, r.ResponseTo)
					require.Equal(t, "ambiguous-invoke", r.Session["Association"])
					require.EqualValues(t, 0, r.Session["Outstanding"])
				}
				finish(s)
				// Worst selected projection is reserved before expanding a legal large value.
				b := DefaultParserBudget()
				b.MaxBufferedBytes = 200000
				b.MaxFrameBytes = 4096
				b.MaxMessageBytes = 4096
				s = fresh(b)
				feed(s, 0, list.Steps[0].Hex)
				big := by["list-selected-octets-1024"].Steps[0].Hex
				o := s.Feed(1, time.Unix(101, 0), wrapperWire(t, big))
				rocTypedError(t, "ResourceExceeded", o.Err)
				require.LessOrEqual(t, s.Stats().BufferedBytes, int64(b.MaxBufferedBytes))
				for _, e := range o.Events {
					f, err := e.GetFields()
					rocTypedError(t, "ResourceExceeded", err)
					if transport == "tcp" {
						wire := wrapperWire(t, big)
						require.Equal(t, wire[:min(len(wire), b.ProbeBytes)], e.Raw)
						require.Equal(t, len(wire), e.Length)
					} else {
						require.Nil(t, e.Raw)
					}
					require.Nil(t, f)
					require.Nil(t, e.Session)
					require.Zero(t, e.ResponseTo)
				}
				o = s.Feed(1, time.Unix(102, 0), wrapperWire(t, list.Steps[1].Hex))
				if transport == "tcp" {
					rocTypedError(t, "FatalSessionError", o.Err)
					require.Empty(t, o.Events)
				} else {
					require.Nil(t, o.Err)
					require.Len(t, o.Events, 1)
					require.Zero(t, o.Events[0].ResponseTo)
					require.Equal(t, "ambiguous-conversation", o.Events[0].Session["Association"])
					require.EqualValues(t, 0, o.Events[0].Session["Outstanding"])
				}
				finish(s)
				// Nested selected fields require depth4; normal-neighbor depth2 remains valid.
				b = DefaultParserBudget()
				b.MaxRecursionDepth = 3
				s = fresh(b)
				o = s.Feed(0, time.Unix(100, 0), wrapperWire(t, list.Steps[0].Hex))
				rocTypedError(t, "ResourceExceeded", o.Err)
				finish(s)
				b.MaxRecursionDepth = 4
				s = fresh(b)
				q = feed(s, 0, list.Steps[0].Hex)
				r = feed(s, 1, list.Steps[1].Hex)
				require.Equal(t, q.ID, r.ResponseTo)
				finish(s)
				b.MaxRecursionDepth = 2
				s = fresh(b)
				q = feed(s, 0, normal.Steps[0].Hex)
				r = feed(s, 1, normal.Steps[1].Hex)
				require.Equal(t, q.ID, r.ResponseTo)
				finish(s)
			})
		}
	}
}
