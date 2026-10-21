package pcaputil

// Unexecuted external missing-behavior design. Public session API only;
// application unsupported is expected to remain a decoded MQTT observation.
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSparkplugDataSetCandidateExistingAPI(t *testing.T) {
	raw := spDataSetSource(t, "candidate-vectors.json")
	require.Equal(t, "85d3b7ecb4f079e161ca9797e8f92e558b986e3f72ab33028fee79227fd6cfb8", fmt.Sprintf("%x", sha256.Sum256(raw)))
	type step struct {
		Direction int    `json:"direction"`
		Time      int64  `json:"timestamp_unix"`
		Wire      string `json:"wire_hex"`
		SHA       string `json:"wire_sha256"`
	}
	var g struct {
		Traces map[string][]step `json:"existing_API_traces"`
	}
	require.NoError(t, json.Unmarshal(raw, &g))
	for _, name := range []string{"node_DataSet_birth_feature", "device_DataSet_birth_feature", "ordinary_node_and_device_scalar_neighbor"} {
		t.Run(name, func(t *testing.T) {
			s, e := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionPorts(40000, 1883))
			require.NoError(t, e)
			defer s.Close("cleanup")
			var last *ProtocolEvent
			for _, st := range g.Traces[name] {
				b, e := hex.DecodeString(st.Wire)
				require.NoError(t, e)
				require.Equal(t, st.SHA, fmt.Sprintf("%x", sha256.Sum256(b)))
				r := s.Feed(st.Direction, time.Unix(st.Time, 0), b)
				require.Nil(t, r.Err)
				require.False(t, r.NeedMore)
				require.Equal(t, len(b), r.Consumed)
				require.Len(t, r.Events, 1)
				last = r.Events[0]
				require.Equal(t, "mqtt", last.Protocol)
				require.Equal(t, "decoded", last.Status)
				require.Equal(t, b, last.Raw)
			}
			require.NotNil(t, last)
			sp, ok := last.Session["Sparkplug"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "matched", sp["Context Status"], "current scalar-only runtime should fail here on the DataSet feature, not at compilation or MQTT framing")
			vals, ok := sp["Metrics"].([]map[string]any)
			require.True(t, ok)
			require.NotEmpty(t, vals)
			v := vals[len(vals)-1]
			if name == "ordinary_node_and_device_scalar_neighbor" {
				require.Equal(t, "Temperature", v["Name"])
				require.Equal(t, uint64(7), v["Datatype"])
				require.Equal(t, uint64(42), v["Value"])
			} else {
				require.Equal(t, "Table", v["Name"])
				require.Equal(t, uint64(16), v["Datatype"])
				require.Equal(t, map[string]any{"num_of_columns": uint64(2), "columns": []string{"count", "label"}, "types": []uint64{7, 12}, "rows": []any{[]any{uint64(7), "ok"}, []any{uint64(8), "next"}}}, v["Value"])
			}
			require.Empty(t, s.Close("complete"))
			require.Zero(t, s.Stats().BufferedBytes)
			require.Empty(t, s.Close("again"))
		})
	}
}

func spDataSetSource(t *testing.T, name string) []byte {
	t.Helper()
	if root := os.Getenv("SPARKPLUG_DATASET_SOURCE_ROOT"); root != "" {
		b, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		return b
	}
	b, err := trafficfixture.ReadFile("testdata/sparkplug-dataset/" + name)
	require.NoError(t, err)
	return b
}

func TestSparkplugDataSetNodeStateExistingAPI(t *testing.T) {
	raw := spDataSetSource(t, "node-state-controls.json")
	require.Equal(t, "8d27e69a31cbf9f53258251973edc7932ff45c85e7e4483b3fbac406f993f404", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var g struct{ Steps []spDataSetStep }
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&g))
	require.Len(t, g.Steps, 6)
	spDataSetStateTrace(t, g.Steps)
}

type spDataSetStep struct {
	Name                                             string `json:"name"`
	Input                                            string `json:"input_hex"`
	SHA                                              string `json:"input_sha256"`
	Direction                                        int    `json:"direction"`
	Time                                             int64  `json:"timestamp_unix"`
	Offset                                           uint64 `json:"offset"`
	Length                                           int    `json:"length"`
	Source, Destination, Protocol, Transport, Status string
	Session                                          map[string]any
}

func spDataSetStateTrace(t *testing.T, steps []spDataSetStep) {
	spDataSetStateTraceBudget(t, steps, DefaultParserBudget())
}

func spDataSetStateTraceBudget(t *testing.T, steps []spDataSetStep, budget ParserBudget) {
	t.Helper()
	for _, mode := range []bool{false, true} {
		for _, chunk := range []int{0, 1, 3, 17} {
			t.Run(fmt.Sprintf("d%v/chunk%d", mode, chunk), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(budget, WithSessionTransport("tcp"), WithSessionPorts(40000, 1883))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = mode
				var history []struct {
					event   *ProtocolEvent
					session map[string]any
					fields  map[string]any
				}
				recheck := func() {
					for _, h := range history {
						v, e := h.event.Decode()
						require.NoError(t, e)
						spAssertTree(t, h.session, v["session"])
						spDataSetTypes(t, v["session"].(map[string]any))
						f, e := h.event.GetFields()
						require.NoError(t, e)
						require.Equal(t, h.fields, f)
					}
				}
				ids := map[uint64]bool{}
				var flow uint64
				for _, w := range steps {
					t.Run(w.Name, func(t *testing.T) {
						wire, err := hex.DecodeString(w.Input)
						require.NoError(t, err)
						require.Equal(t, w.SHA, fmt.Sprintf("%x", sha256.Sum256(wire)))
						var ev []*ProtocolEvent
						n := chunk
						if n == 0 {
							n = len(wire)
						}
						for at := 0; at < len(wire); {
							end := at + n
							if end > len(wire) {
								end = len(wire)
							}
							input := append([]byte(nil), wire[at:end]...)
							r := s.Feed(w.Direction, time.Unix(w.Time, 0), input)
							require.Equal(t, len(input), r.Consumed)
							if end < len(wire) {
								require.NotNil(t, r.Err)
								require.Equal(t, ErrNeedMore, r.Err.Kind)
								require.Empty(t, r.Events)
								require.True(t, r.NeedMore)
							} else {
								require.Nil(t, r.Err)
								require.False(t, r.NeedMore)
							}
							ev = append(ev, r.Events...)
							for i := range input {
								input[i] ^= 255
							}
							at = end
						}
						require.Len(t, ev, 1)
						e := ev[0]
						require.NotZero(t, e.ID)
						require.False(t, ids[e.ID])
						ids[e.ID] = true
						if flow == 0 {
							flow = e.FlowID
						}
						require.Equal(t, flow, e.FlowID)
						require.NotZero(t, flow)
						require.Equal(t, w.Protocol, e.Protocol)
						require.Equal(t, w.Transport, e.Transport)
						if mode {
							require.Equal(t, "deferred", e.Status)
						} else {
							require.Equal(t, w.Status, e.Status)
						}
						require.Equal(t, time.Unix(w.Time, 0), e.Timestamp)
						require.Equal(t, w.Direction, e.Direction)
						require.Equal(t, w.Source, e.Source)
						require.Equal(t, w.Destination, e.Destination)
						require.Equal(t, w.Offset, e.Offset)
						require.Equal(t, w.Length, e.Length)
						require.Equal(t, wire, e.Raw)
						require.Zero(t, e.TransactionID)
						require.Zero(t, e.ResponseTo)
						require.Empty(t, e.Error)
						spAssertTree(t, w.Session, e.Session)
						spDataSetTypes(t, e.Session)
						tree, err := e.Decode()
						require.NoError(t, err)
						spAssertTree(t, w.Session, tree["session"])
						fields, err := e.GetFields()
						require.NoError(t, err)
						if sp, ok := w.Session["Sparkplug"]; ok {
							spAssertTree(t, sp, fields["Sparkplug"])
							spDataSetTypes(t, map[string]any{"Sparkplug": fields["Sparkplug"]})
						} else {
							require.NotContains(t, fields, "Sparkplug")
						}
						pristine := cloneSession(fields)
						// Mutate public raw, every nested field graph and returned decode. Later
						// events and fresh queries must still own their original bytes and rows.
						for i := range e.Raw {
							e.Raw[i] ^= 255
						}
						spDataSetPoison(e.Session)
						spDataSetPoison(fields)
						spDataSetPoison(tree)
						e.Structured = map[string]any{"fields": fields}
						fresh, err := e.Decode()
						require.NoError(t, err)
						spAssertTree(t, w.Session, fresh["session"])
						again, err := e.GetFields()
						require.NoError(t, err)
						require.Equal(t, pristine, again)
						history = append(history, struct {
							event   *ProtocolEvent
							session map[string]any
							fields  map[string]any
						}{e, w.Session, pristine})
						recheck()
					})
				}
				require.Empty(t, s.Close("complete"))
				recheck()
				require.Zero(t, s.Stats().BufferedBytes)
				require.Nil(t, s.Close("again"))
			})
		}
	}
}

func spDataSetPoison(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			spDataSetPoison(c)
			x[k] = "poison"
		}
	case []map[string]any:
		for _, c := range x {
			spDataSetPoison(c)
		}
	case []any:
		for i, c := range x {
			spDataSetPoison(c)
			x[i] = "poison"
		}
	case [][]any:
		for _, r := range x {
			spDataSetPoison(r)
		}
	case []string:
		for i := range x {
			x[i] = "poison"
		}
	case []uint64:
		for i := range x {
			x[i] = 999999
		}
	case []byte:
		for i := range x {
			x[i] ^= 255
		}
	}
}

func TestSparkplugDataSetResourceAtomicity(t *testing.T) {
	var g struct {
		Vectors []struct {
			Name   string `json:"name"`
			Metric string `json:"metric_hex"`
		}
	}
	require.NoError(t, json.Unmarshal(spDataSetSource(t, "cell-controls.json"), &g))
	metric, err := hex.DecodeString(g.Vectors[0].Metric)
	require.NoError(t, err)
	for _, kind := range []string{"depth", "fields", "bytes"} {
		t.Run(kind, func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxFrameBytes = 512
			b.MaxMessageBytes = 512
			switch kind {
			case "depth":
				b.MaxRecursionDepth = 4
			case "fields":
				b.MaxCollectionElements = 40
			case "bytes":
				b.MaxBufferedBytes = 4096
			}
			for _, dataset := range []bool{false, true} {
				t.Run(fmt.Sprintf("dataset%v", dataset), func(t *testing.T) {
					s, e := NewProtocolSessionWithOptions(b, WithSessionTransport("tcp"), WithSessionPorts(40000, 1883))
					require.NoError(t, e)
					defer s.Close("cleanup")
					will := spTestPayload(nil, spTestMetric("bdSeq", 4, 11, 0, nil))
					for i, w := range [][]byte{spTestConnect(will, "spBv1.0/g/NDEATH/n"), {0x20, 2, 0, 0}} {
						r := s.Feed(i, time.Unix(int64(i+1), 0), w)
						require.Nil(t, r.Err)
						require.Len(t, r.Events, 1)
					}
					seq := uint64(0)
					var table []byte
					if dataset {
						table = metric
					}
					// Both contexts use the same independent timestamp and valid producer
					// controls. DataSet is tested in NBIRTH rather than a name+alias NDATA.
					bd := append(spTestBytes(1, []byte("bdSeq")), spTestVar(3, 1000)...)
					bd = append(bd, spTestVar(4, 4)...)
					bd = append(bd, spTestVar(11, 0)...)
					rebirth := append(spTestBytes(1, []byte("Node Control/Rebirth")), spTestVar(3, 1000)...)
					rebirth = append(rebirth, spTestVar(4, 11)...)
					rebirth = append(rebirth, spTestVar(14, 0)...)
					if !dataset {
						table = append(spTestBytes(1, []byte("Table")), spTestVar(3, 1000)...)
						table = append(table, spTestVar(4, 7)...)
						table = append(table, spTestVar(10, 42)...)
					}
					birth := spTestPublish("spBv1.0/g/NBIRTH/n", spTestPayload(&seq, bd, rebirth, table))
					r := s.Feed(0, time.Unix(3, 0), birth)
					require.Equal(t, len(birth), r.Consumed)
					require.Len(t, r.Events, 1)
					if dataset {
						require.NotNil(t, r.Err)
						require.Equal(t, ErrResourceExceeded, r.Err.Kind)
						status := "limited"
						if kind == "bytes" {
							status = "context-required"
						}
						require.Equal(t, status, r.Events[0].Status)
						_, e := r.Events[0].GetFields()
						var pe *ProtocolError
						require.ErrorAs(t, e, &pe)
						require.Equal(t, ErrResourceExceeded, pe.Kind)
						require.Nil(t, s.(*captureSession).f.mqtt)
						require.Zero(t, s.Stats().BufferedBytes)
					} else {
						require.Nil(t, r.Err)
						require.Equal(t, "matched", r.Events[0].Session["Sparkplug"].(map[string]any)["Context Status"])
						require.Equal(t, []map[string]any{{"Name": "bdSeq", "Datatype": uint64(4), "Value": int64(0)}, {"Name": "Node Control/Rebirth", "Datatype": uint64(11), "Value": false}, {"Name": "Table", "Datatype": uint64(7), "Value": uint64(42)}}, r.Events[0].Session["Sparkplug"].(map[string]any)["Metrics"])
					}
					require.Empty(t, s.Close("complete"))
					require.Zero(t, s.Stats().BufferedBytes)
					require.Nil(t, s.Close("again"))
				})
			}
		})
	}
}

func TestSparkplugDataSetCellSessionsExistingAPI(t *testing.T) {
	raw := spDataSetSource(t, "cell-session-controls.json")
	require.Equal(t, "6952a028725192c4e6d652c46ce15ce13f420e17e7ca657452674a8e00a1998f", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var g struct {
		Cases []struct {
			Name  string
			Steps []spDataSetStep
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&g))
	require.Len(t, g.Cases, 14)
	for _, c := range g.Cases {
		t.Run(c.Name, func(t *testing.T) { require.Len(t, c.Steps, 3); spDataSetStateTrace(t, c.Steps) })
	}
}

func TestSparkplugDataSetCapturedMatrix(t *testing.T) {
	spDataSetCapturedMatrix(t, spDataSetSource, "sparkplug-dataset", "df422a396267c4d2582507dcfebe31be6136596400a9a90090a0ccac5206a6b7", 25)
}

func spDataSetCapturedMatrix(t *testing.T, source func(*testing.T, string) []byte, profile, bindingHash string, cases int) {
	bound := source(t, "source-bindings.json")
	require.Equal(t, bindingHash, fmt.Sprintf("%x", sha256.Sum256(bound)))
	var bindings struct {
		AnswersSHA string `json:"answers_sha256"`
		Captures   []struct {
			Case           string `json:"case"`
			SHA            string `json:"sha256"`
			Bytes, Packets int
			PayloadSHA     []string `json:"payload_sha256"`
			Times          []int64  `json:"timestamp_unix"`
		}
	}
	require.NoError(t, json.Unmarshal(bound, &bindings))
	require.Len(t, bindings.Captures, cases)
	raw := source(t, "answers.json")
	require.Equal(t, bindings.AnswersSHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
	if profile == "sparkplug-dataset" {
		raw = spDataSetCommandLegacyCapturedAnswers(t, raw)
	}
	if profile == "sparkplug-template" {
		raw = spTemplatePartialLegacyAnswers(t, raw)
	}
	var g struct {
		Profiles map[string]struct {
			Events []struct {
				Record    string
				Direction int
				Session   map[string]any
			}
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&g))
	require.Len(t, g.Profiles, cases)
	for name, p := range g.Profiles {
		t.Run(name, func(t *testing.T) {
			cap := source(t, "captures/"+name+".pcap")
			var expectedSHA string
			var expectedBytes, expectedPackets int
			var payloads []string
			var timestamps []int64
			for _, b := range bindings.Captures {
				if b.Case == name {
					require.Empty(t, expectedSHA)
					expectedSHA = b.SHA
					expectedBytes = b.Bytes
					expectedPackets = b.Packets
					payloads = b.PayloadSHA
					timestamps = b.Times
				}
			}
			require.NotEmpty(t, expectedSHA)
			require.Equal(t, expectedBytes, len(cap))
			require.Equal(t, expectedSHA, fmt.Sprintf("%x", sha256.Sum256(cap)))
			require.Len(t, payloads, len(p.Events))
			require.Len(t, timestamps, len(p.Events))
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%v/o%v", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var seen atomic.Uint64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
							}
							require.NoError(t, ReplayPcap(bytes.NewReader(cap), opts...))
							require.Len(t, events, len(p.Events))
							offsets := [2]uint64{}
							ids := map[uint64]bool{}
							var flow uint64
							var history []struct {
								event   *ProtocolEvent
								session map[string]any
								fields  map[string]any
							}
							recheck := func() {
								for _, h := range history {
									q, err := h.event.Decode()
									require.NoError(t, err)
									spAssertTree(t, h.session, q["session"])
									spDataSetTypes(t, q["session"].(map[string]any))
									f, err := h.event.GetFields()
									require.NoError(t, err)
									require.Equal(t, h.fields, f)
								}
							}
							for i, e := range events {
								w := p.Events[i]
								var wire []byte
								prefix := "common/pcapx/pcaputil/testdata/" + profile + "/"
								if strings.HasPrefix(w.Record, prefix) {
									wire = source(t, strings.TrimPrefix(w.Record, prefix))
								} else {
									var err error
									wire, err = trafficfixture.ReadFile(strings.TrimPrefix(w.Record, "common/pcapx/pcaputil/"))
									require.NoError(t, err)
								}
								require.Equal(t, "mqtt", e.Protocol)
								require.Equal(t, "tcp", e.Transport)
								if deferred {
									require.Equal(t, "deferred", e.Status)
								} else {
									require.Equal(t, "decoded", e.Status)
								}
								require.Equal(t, w.Direction, e.Direction)
								require.Equal(t, payloads[i], fmt.Sprintf("%x", sha256.Sum256(e.Raw)))
								require.Equal(t, time.Unix(timestamps[i], int64(i+3)*1000), e.Timestamp)
								source, dest := "192.0.2.1:40000", "192.0.2.2:1883"
								if w.Direction == 1 {
									source, dest = dest, source
								}
								require.Equal(t, source, e.Source)
								require.Equal(t, dest, e.Destination)
								require.Empty(t, e.Error)
								require.Equal(t, wire, e.Raw)
								require.Equal(t, len(wire), e.Length)
								require.Equal(t, offsets[w.Direction], e.Offset)
								offsets[w.Direction] += uint64(len(wire))
								require.NotZero(t, e.ID)
								require.False(t, ids[e.ID])
								ids[e.ID] = true
								if flow == 0 {
									flow = e.FlowID
								}
								require.NotZero(t, flow)
								require.Equal(t, flow, e.FlowID)
								require.Zero(t, e.TransactionID)
								require.Zero(t, e.ResponseTo)
								require.NotEmpty(t, e.SourceBytes.PacketRefs)
								require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wire)), e.SourceBytes.SHA256)
								spAssertTree(t, w.Session, e.Session)
								spDataSetTypes(t, e.Session)
								fields, err := e.GetFields()
								require.NoError(t, err)
								if sp, ok := w.Session["Sparkplug"]; ok {
									spAssertTree(t, sp, fields["Sparkplug"])
									spDataSetTypes(t, map[string]any{"Sparkplug": fields["Sparkplug"]})
								} else {
									require.NotContains(t, fields, "Sparkplug")
								}
								pristine := cloneSession(fields)
								tree, err := e.Decode()
								require.NoError(t, err)
								spAssertTree(t, w.Session, tree["session"])
								spDataSetPoison(fields)
								spDataSetPoison(tree)
								spDataSetPoison(e.Session)
								for j := range e.Raw {
									e.Raw[j] ^= 255
								}
								e.Structured = map[string]any{"fields": fields}
								fresh, err := e.Decode()
								require.NoError(t, err)
								spAssertTree(t, w.Session, fresh["session"])
								again, err := e.GetFields()
								require.NoError(t, err)
								require.Equal(t, pristine, again)
								history = append(history, struct {
									event   *ProtocolEvent
									session map[string]any
									fields  map[string]any
								}{e, w.Session, pristine})
								recheck()
							}
							recheck()
							require.Zero(t, stats.BufferedBytes)
							if observe {
								require.Equal(t, uint64(expectedPackets), seen.Load())
							} else {
								require.Zero(t, seen.Load())
							}
						})
					}
				}
			}
		})
	}
}

func TestSparkplugDataSetCommandBoundaryExistingAPI(t *testing.T) {
	spDataSetCommandLegacyCases(t)
}
func TestSparkplugDataSetOriginalCandidatesExistingAPI(t *testing.T) {
	spDataSetCaseFile(t, "addon-controls.json", "dad520bf2e8364eacc041f4115e3628467b4de8e19fc98e0ad2172ab9a692d8e", 8)
}
func spDataSetCaseFile(t *testing.T, name, hash string, n int) {
	t.Helper()
	raw := spDataSetSource(t, name)
	require.Equal(t, hash, fmt.Sprintf("%x", sha256.Sum256(raw)))
	var g struct {
		Cases []struct {
			Name  string
			Steps []spDataSetStep
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&g))
	require.Len(t, g.Cases, n)
	for _, c := range g.Cases {
		t.Run(c.Name, func(t *testing.T) { spDataSetStateTrace(t, c.Steps) })
	}
}

func TestSparkplugDataSetPackedTypesGlobalBudget(t *testing.T) {
	// Independent count: Payload5 + bdSeq4 + Rebirth4 + Table5 +
	// DataSet(num1 + columns14 + packed field1 + packed elements14) = 48.
	// Without charging packed elements the same message would only count34.
	ds := spTestVar(1, 14)
	columns := []string{}
	types := []uint64{}
	packed := []byte{}
	for i := uint64(1); i <= 14; i++ {
		name := fmt.Sprintf("c%d", i)
		columns = append(columns, name)
		types = append(types, i)
		packed = append(packed, byte(i))
		ds = append(ds, spTestBytes(2, []byte(name))...)
	}
	ds = append(ds, spTestBytes(3, packed)...)
	table := spTestBytes(1, []byte("Table"))
	table = append(table, spTestVar(2, 1)...)
	table = append(table, spTestVar(3, 1000)...)
	table = append(table, spTestVar(4, 16)...)
	table = append(table, spTestBytes(17, ds)...)
	bd := spTestBytes(1, []byte("bdSeq"))
	bd = append(bd, spTestVar(3, 1000)...)
	bd = append(bd, spTestVar(4, 4)...)
	bd = append(bd, spTestVar(11, 0)...)
	reb := spTestBytes(1, []byte("Node Control/Rebirth"))
	reb = append(reb, spTestVar(3, 1000)...)
	reb = append(reb, spTestVar(4, 11)...)
	reb = append(reb, spTestVar(14, 0)...)
	seq := uint64(0)
	birth := spTestPublish("spBv1.0/g/NBIRTH/n", spTestPayload(&seq, bd, reb, table))
	for _, limit := range []int{47, 48} {
		t.Run(fmt.Sprintf("fields%d", limit), func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxCollectionElements = limit
			s, e := NewProtocolSessionWithOptions(b, WithSessionTransport("tcp"), WithSessionPorts(40000, 1883))
			require.NoError(t, e)
			defer s.Close("cleanup")
			will := spTestPayload(nil, spTestMetric("bdSeq", 4, 11, 0, nil))
			for i, w := range [][]byte{spTestConnect(will, "spBv1.0/g/NDEATH/n"), {0x20, 2, 0, 0}} {
				r := s.Feed(i, time.Unix(int64(i+1), 0), w)
				require.Nil(t, r.Err)
				require.Len(t, r.Events, 1)
			}
			r := s.Feed(0, time.Unix(3, 0), birth)
			require.Len(t, r.Events, 1)
			require.Equal(t, len(birth), r.Consumed)
			require.Zero(t, r.Events[0].TransactionID)
			require.Zero(t, r.Events[0].ResponseTo)
			if limit == 47 {
				require.NotNil(t, r.Err)
				require.Equal(t, ErrResourceExceeded, r.Err.Kind)
				require.Equal(t, "limited", r.Events[0].Status)
				require.Nil(t, s.(*captureSession).f.mqtt)
				require.Zero(t, s.Stats().BufferedBytes)
			} else {
				require.Nil(t, r.Err)
				sp := r.Events[0].Session["Sparkplug"].(map[string]any)
				require.Equal(t, "matched", sp["Context Status"])
				values := []map[string]any{{"Name": "bdSeq", "Datatype": uint64(4), "Value": int64(0)}, {"Name": "Node Control/Rebirth", "Datatype": uint64(11), "Value": false}, {"Name": "Table", "Datatype": uint64(16), "Value": map[string]any{"num_of_columns": uint64(14), "columns": columns, "types": types, "rows": []any{}}}}
				require.Equal(t, values, sp["Metrics"])
				fields, e := r.Events[0].GetFields()
				require.NoError(t, e)
				require.Equal(t, sp, fields["Sparkplug"])
			}
			require.Empty(t, s.Close("complete"))
			require.Zero(t, s.Stats().BufferedBytes)
			require.Nil(t, s.Close("again"))
		})
	}
}

// JSON equality alone would accept positive int/uint/float substitutions.
// Keep the selected public types explicit without weakening the shared oracle.
func spDataSetTypes(t *testing.T, session map[string]any) {
	spTemplateTypes(t, session)
	t.Helper()
	spArrayDraftTyped(t, session)
	raw, present := session["Sparkplug"]
	if !present {
		return
	}
	sp, ok := raw.(map[string]any)
	require.True(t, ok)
	if p, present := sp["Payload"]; present {
		payload, ok := p.(map[string]any)
		require.True(t, ok)
		for _, k := range []string{"timestamp", "seq"} {
			if x, present := payload[k]; present {
				_, ok := x.(uint64)
				require.True(t, ok, "Payload.%s type", k)
			}
		}
		if x, present := payload["metrics"]; present {
			ms, ok := x.([]map[string]any)
			require.True(t, ok)
			for _, m := range ms {
				for _, k := range []string{"alias", "timestamp", "datatype", "int_value", "long_value"} {
					if x, present := m[k]; present {
						_, ok := x.(uint64)
						require.True(t, ok, "metric.%s type", k)
					}
				}
				if x, present := m["dataset_value"]; present {
					_, ok := x.([]byte)
					require.True(t, ok, "dataset_value remains owned bytes")
				}
			}
		}
	}
	raw, present = sp["Metrics"]
	if !present {
		return
	}
	ms, ok := raw.([]map[string]any)
	require.True(t, ok)
	for _, m := range ms {
		typ, ok := m["Datatype"].(uint64)
		require.True(t, ok)
		if typ != 16 || m["Value"] == nil {
			continue
		}
		v, ok := m["Value"].(map[string]any)
		require.True(t, ok)
		require.Len(t, v, 4)
		n, ok := v["num_of_columns"].(uint64)
		require.True(t, ok)
		columns, ok := v["columns"].([]string)
		require.True(t, ok)
		types, ok := v["types"].([]uint64)
		require.True(t, ok)
		require.Equal(t, int(n), len(columns))
		require.Len(t, types, len(columns))
		rows, ok := v["rows"].([]any)
		require.True(t, ok)
		for _, r := range rows {
			row, ok := r.([]any)
			require.True(t, ok)
			require.Len(t, row, len(types))
			for i, x := range row {
				ok := false
				switch types[i] {
				case 1, 2, 3, 4:
					_, ok = x.(int64)
				case 5, 6, 7, 8, 13:
					_, ok = x.(uint64)
				case 9:
					_, ok = x.(float32)
				case 10:
					_, ok = x.(float64)
				case 11:
					_, ok = x.(bool)
				case 12, 14:
					_, ok = x.(string)
				}
				require.True(t, ok, "column%d type%d", i, types[i])
			}
		}
	}
}
