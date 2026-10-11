package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"google.golang.org/protobuf/encoding/protowire"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func spTestVar(n protowire.Number, v uint64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(nil, n, 0), v)
}
func spTestBytes(n protowire.Number, v []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(nil, n, 2), v)
}
func spTestMetric(name string, typ uint64, field protowire.Number, val uint64, alias *uint64) []byte {
	b := spTestBytes(1, []byte(name))
	if alias != nil {
		b = append(b, spTestVar(2, *alias)...)
	}
	b = append(b, spTestVar(3, 1700000000000)...)
	b = append(b, spTestVar(4, typ)...)
	return append(b, spTestVar(field, val)...)
}
func spTestPayload(seq *uint64, metrics ...[]byte) []byte {
	b := spTestVar(1, 1700000000000)
	for _, m := range metrics {
		b = append(b, spTestBytes(2, m)...)
	}
	if seq != nil {
		b = append(b, spTestVar(3, *seq)...)
	}
	return b
}
func spTestString(s string) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(len(s)))
	return append(b, []byte(s)...)
}
func spTestPacket(h byte, b []byte) []byte {
	w := []byte{h}
	n := len(b)
	for {
		v := byte(n % 128)
		n /= 128
		if n > 0 {
			v |= 128
		}
		w = append(w, v)
		if n == 0 {
			break
		}
	}
	return append(w, b...)
}
func spTestConnect(will []byte, topic string) []byte {
	b := append(spTestString("MQTT"), 4, 14, 0, 30)
	b = append(b, spTestString("node")...)
	b = append(b, spTestString(topic)...)
	v := make([]byte, 2)
	binary.BigEndian.PutUint16(v, uint16(len(will)))
	b = append(b, v...)
	return spTestPacket(0x10, append(b, will...))
}
func spTestPublish(topic string, b []byte) []byte {
	return spTestPacket(0x30, append(spTestString(topic), b...))
}
func TestProtocolSessionSparkplugNodeLifecycle(t *testing.T) {
	seq := uint64(0)
	alias := uint64(1)
	will := spTestPayload(nil, spTestMetric("bdSeq", 4, 11, 0, nil))
	birth := spTestPayload(&seq, spTestMetric("bdSeq", 4, 11, 0, nil), spTestMetric("Node Control/Rebirth", 11, 14, 0, nil), spTestMetric("Temperature", 2, 10, 23, &alias))
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("done")
	require.Nil(t, s.Feed(0, time.Unix(1, 0), spTestConnect(will, "spBv1.0/g/NDEATH/n")).Err)
	require.Nil(t, s.Feed(1, time.Unix(2, 0), []byte{0x20, 2, 0, 0}).Err)
	r := s.Feed(0, time.Unix(3, 0), spTestPublish("spBv1.0/g/NBIRTH/n", birth))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	v, ok := r.Events[0].Session["Sparkplug"].(map[string]any)
	require.True(t, ok, "existing API currently loses Sparkplug payload and birth association")
	require.Equal(t, "matched", v["Context Status"])
	require.Equal(t, uint64(0), v["bdSeq"])
	require.Empty(t, s.Close("complete"))
	require.Zero(t, s.Stats().BufferedBytes)
	require.Nil(t, s.Close("again"))
}

// Compare integer answers without JSON float64 rounding (uint64 max and signed
// Sparkplug conversions are deliberately in the sealed independent answers).
func spAssertTree(t *testing.T, want, got any) {
	t.Helper()
	switch v := got.(type) {
	case uint64:
		n, ok := want.(json.Number)
		require.True(t, ok)
		u, err := strconv.ParseUint(string(n), 10, 64)
		require.NoError(t, err)
		require.Equal(t, u, v)
	case int64:
		n, ok := want.(json.Number)
		require.True(t, ok)
		u, err := strconv.ParseInt(string(n), 10, 64)
		require.NoError(t, err)
		require.Equal(t, u, v)
	case float32:
		n, ok := want.(json.Number)
		require.True(t, ok)
		u, err := strconv.ParseFloat(string(n), 32)
		require.NoError(t, err)
		require.Equal(t, float32(u), v)
	case float64:
		n, ok := want.(json.Number)
		require.True(t, ok)
		u, err := strconv.ParseFloat(string(n), 64)
		require.NoError(t, err)
		require.Equal(t, u, v)
	case []byte:
		s, ok := want.(string)
		require.True(t, ok)
		b, err := base64.StdEncoding.DecodeString(s)
		require.NoError(t, err)
		require.Equal(t, b, v)
	case map[string]any:
		w, ok := want.(map[string]any)
		require.True(t, ok)
		require.Len(t, v, len(w))
		for k, x := range w {
			g, present := v[k]
			require.True(t, present, "missing %s", k)
			spAssertTree(t, x, g)
		}
	default:
		if got != nil && reflect.TypeOf(got).Kind() == reflect.Slice {
			w, ok := want.([]any)
			require.True(t, ok)
			v := reflect.ValueOf(got)
			require.Equal(t, len(w), v.Len())
			for i := range w {
				spAssertTree(t, w[i], v.Index(i).Interface())
			}
			return
		}
		require.Equal(t, want, got)
	}
}
func testSparkplugSealedNodeMatrix(t *testing.T) {
	testSparkplugSealedMatrix(t, "sparkplug-node", 41)
}
func testSparkplugSealedDeviceMatrix(t *testing.T) {
	testSparkplugSealedMatrix(t, "sparkplug-device", 23)
}
func testSparkplugSealedCommandsMatrix(t *testing.T) {
	testSparkplugSealedMatrix(t, "sparkplug-commands", 19)
}
func testSparkplugSealedMatrix(t *testing.T, directory string, count int) {
	var answers struct {
		Profiles map[string]struct {
			Events []struct {
				Record    string
				Direction int
				Session   map[string]any
			}
		}
	}
	raw, err := trafficfixture.ReadFile("testdata/" + directory + "/answers.json")
	require.NoError(t, err)
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&answers))
	require.Len(t, answers.Profiles, count)
	for name, p := range answers.Profiles {
		t.Run(name, func(t *testing.T) {
			check := func(t *testing.T, events []*ProtocolEvent) {
				t.Helper()
				require.Len(t, events, len(p.Events))
				offsets := [2]uint64{}
				for i, e := range events {
					w := p.Events[i]
					wire, err := trafficfixture.ReadFile(strings.TrimPrefix(w.Record, "common/pcapx/pcaputil/"))
					require.NoError(t, err)
					require.Equal(t, "mqtt", e.Protocol)
					require.Equal(t, w.Direction, e.Direction)
					require.Equal(t, wire, e.Raw)
					require.Equal(t, len(wire), e.Length)
					require.Equal(t, offsets[w.Direction], e.Offset)
					offsets[w.Direction] += uint64(len(wire))
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
					require.Empty(t, e.Error)
					spAssertTree(t, w.Session, e.Session)
					tree, err := e.Decode()
					require.NoError(t, err)
					spAssertTree(t, w.Session, tree["session"])
					fields, err := e.GetFields()
					require.NoError(t, err)
					require.NotEmpty(t, fields)
					if spark, ok := w.Session["Sparkplug"]; ok {
						spAssertTree(t, spark, fields["Sparkplug"])
					} else {
						require.NotContains(t, fields, "Sparkplug")
					}
					original := cloneSession(fields)
					e.Raw[0] ^= 255
					e.Session = map[string]any{"Sparkplug": "forged"}
					tree["session"] = map[string]any{"forged": true}
					for k := range fields {
						delete(fields, k)
					}
					e.Structured = map[string]any{"fields": fields}
					fresh, err := e.Decode()
					require.NoError(t, err)
					spAssertTree(t, w.Session, fresh["session"])
					again, err := e.GetFields()
					require.NoError(t, err)
					require.Equal(t, original, again)
				}
			}
			capture, err := trafficfixture.ReadFile("testdata/" + directory + "/captures/" + name + ".pcap")
			require.NoError(t, err)
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("capture/w%d/d%v/o%v", workers, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							var seen atomic.Uint64
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
							}
							require.NoError(t, ReplayPcap(bytes.NewReader(capture), opts...))
							for _, e := range events {
								require.NotEmpty(t, e.SourceBytes.PacketRefs)
								sum := sha256.Sum256(e.Raw)
								require.Equal(t, fmt.Sprintf("%x", sum), e.SourceBytes.SHA256)
							}
							check(t, events)
							require.Zero(t, stats.BufferedBytes)
							if observe {
								require.Positive(t, seen.Load())
							} else {
								require.Zero(t, seen.Load())
							}
						})
					}
				}
			}
			for _, chunk := range []int{0, 1, 7} {
				for _, deferred := range []bool{false, true} {
					t.Run(fmt.Sprintf("session/chunk%d/d%v", chunk, deferred), func(t *testing.T) {
						s, err := NewProtocolSession(DefaultParserBudget())
						require.NoError(t, err)
						defer s.Close("done")
						s.(*captureSession).f.a.config.Deferred = deferred
						var events []*ProtocolEvent
						for i, w := range p.Events {
							wire, err := trafficfixture.ReadFile(strings.TrimPrefix(w.Record, "common/pcapx/pcaputil/"))
							require.NoError(t, err)
							step := chunk
							if step == 0 {
								step = len(wire)
							}
							for at := 0; at < len(wire); at += step {
								b := bytes.Clone(wire[at:min(at+step, len(wire))])
								x := s.Feed(w.Direction, time.Unix(int64(i+1), 0), b)
								if x.Err != nil {
									require.Equal(t, ErrNeedMore, x.Err.Kind)
									require.True(t, x.NeedMore)
								}
								events = append(events, x.Events...)
								for j := range b {
									b[j] ^= 255
								}
							}
						}
						require.Empty(t, s.Close("complete"))
						require.Zero(t, s.Stats().BufferedBytes)
						require.Nil(t, s.Close("again"))
						check(t, events)
					})
				}
			}
		})
	}
}

func testSparkplugResourceAtomicity(t *testing.T) {
	for _, kind := range []string{"fields", "depth", "bytes"} {
		t.Run(kind, func(t *testing.T) {
			budget := DefaultParserBudget()
			switch kind {
			case "fields":
				budget.MaxCollectionElements = 17
			case "depth":
				budget.MaxRecursionDepth = 1
			case "bytes":
				budget.MaxBufferedBytes = 700
				budget.MaxMessageBytes = 512
				budget.MaxFrameBytes = 512
			}
			s, err := NewProtocolSession(budget)
			require.NoError(t, err)
			defer s.Close("done")
			var answers struct {
				Profiles map[string]struct {
					Events []struct {
						Record    string
						Direction int
					}
				}
			}
			b, err := trafficfixture.ReadFile("testdata/sparkplug-node/answers.json")
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(b, &answers))
			var refused bool
			for _, e := range answers.Profiles["level4-lifecycle"].Events[:3] {
				wire, err := trafficfixture.ReadFile(strings.TrimPrefix(e.Record, "common/pcapx/pcaputil/"))
				require.NoError(t, err)
				x := s.Feed(e.Direction, time.Unix(1, 0), wire)
				if x.Err != nil {
					require.Equal(t, ErrResourceExceeded, x.Err.Kind)
					require.Len(t, x.Events, 1)
					_, err = x.Events[0].GetFields()
					var p *ProtocolError
					require.ErrorAs(t, err, &p)
					require.Equal(t, ErrResourceExceeded, p.Kind)
					refused = true
					break
				}
			}
			require.True(t, refused, "selected budget must actually refuse")
			require.Zero(t, s.Stats().BufferedBytes)
			require.Nil(t, s.(*captureSession).f.mqtt, "fatal refusal must release both MQTT and Sparkplug context")
			require.Empty(t, s.Close("refused"))
			require.Nil(t, s.Close("again"))
		})
	}
}

func testSparkplugInterleavedOwnership(t *testing.T) {
	testSparkplugInterleavedProfile(t, "sparkplug-node", "level4-lifecycle")
}
func testSparkplugDeviceInterleavedOwnership(t *testing.T) {
	testSparkplugInterleavedProfile(t, "sparkplug-device", "two-devices-same-name")
}
func testSparkplugInterleavedProfile(t *testing.T, directory, profile string) {
	var answers struct {
		Profiles map[string]struct {
			Events []struct {
				Record    string
				Direction int
				Session   map[string]any
			}
		}
	}
	b, err := trafficfixture.ReadFile("testdata/" + directory + "/answers.json")
	require.NoError(t, err)
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	require.NoError(t, d.Decode(&answers))
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("done")
	for i, w := range answers.Profiles[profile].Events {
		wire, err := trafficfixture.ReadFile(strings.TrimPrefix(w.Record, "common/pcapx/pcaputil/"))
		require.NoError(t, err)
		x := s.Feed(w.Direction, time.Unix(int64(i+1), 0), wire)
		require.Nil(t, x.Err)
		require.Len(t, x.Events, 1)
		e := x.Events[0]
		spAssertTree(t, w.Session, e.Session)
		tree, err := e.Decode()
		require.NoError(t, err)
		fields, err := e.GetFields()
		require.NoError(t, err)
		if v, ok := e.Session["Sparkplug"].(map[string]any); ok {
			if p, ok := v["Payload"].(map[string]any); ok {
				if ms, ok := p["metrics"].([]map[string]any); ok && len(ms) > 0 {
					ms[0]["name"] = "forged"
					ms[0]["alias"] = uint64(999999)
				}
			}
			v["bdSeq"] = uint64(999999)
		}
		for k := range fields {
			delete(fields, k)
		}
		e.Raw[0] ^= 255
		tree["session"] = map[string]any{"forged": true}
		e.Structured = map[string]any{"fields": fields}
		fresh, err := e.Decode()
		require.NoError(t, err)
		spAssertTree(t, w.Session, fresh["session"])
	}
	require.Empty(t, s.Close("complete"))
	require.Zero(t, s.Stats().BufferedBytes)
	require.Nil(t, s.Close("again"))
}
