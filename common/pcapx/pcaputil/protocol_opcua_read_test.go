package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type uaReadCase struct {
	Name  string         `json:"name"`
	Close map[string]any `json:"close"`
	Steps []struct {
		Dir          int            `json:"dir"`
		Hex          string         `json:"hex"`
		Service      uint32         `json:"service"`
		Read         map[string]any `json:"read"`
		Session      map[string]any `json:"session"`
		Contributors []int          `json:"contributors"`
		Error        string         `json:"error"`
		Matched      bool           `json:"matched"`
		Dropped      bool           `json:"dropped"`
	} `json:"steps"`
}

func opcuaReadScalarCodecCases(t *testing.T) {
	opcuaSelectedCodecCases(t, "opcua-read-scalar", "Read", false)
}
func opcuaSelectedCodecCases(t *testing.T, root, field string, browse bool) {
	raw, err := opcuaSelectedInput(root, "material/cases.json")
	require.NoError(t, err)
	var cases []uaReadCase
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&cases))
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			s := &binOPCUA{read: !browse, browse: browse}
			for i, step := range c.Steps {
				w, err := hex.DecodeString(step.Hex)
				require.NoError(t, err)
				out, err := s.consume(step.Dir, w, 1024, 1<<20)
				if step.Error != "" {
					require.Error(t, err, "step %d", i)
					if step.Error == "MalformedMessage" {
						status, _ := classifySessionError(err)
						require.Equal(t, "malformed", status)
					} else {
						require.ErrorContains(t, err, step.Error)
					}
					continue
				}
				require.NoError(t, err, "step %d", i)
				if step.Service != 0 {
					require.Equal(t, step.Service, out["Service Type ID"])
				} else {
					require.NotContains(t, out, "Service Type ID")
				}
				if step.Session != nil {
					requireExactProtocolFields(t, step.Session, out)
				}
				if step.Read != nil {
					requireExactProtocolFields(t, step.Read, out[field].(map[string]any))
					if browse {
						require.Equal(t, "browse-wire-observation", out["Semantic Status"])
					} else {
						require.Equal(t, "read-wire-observation", out["Semantic Status"])
					}
				}
				if step.Service == 634 || step.Service == 530 {
					require.Equal(t, step.Matched, out["Matched"])
				}
			}
		})
	}
}
func opcuaReadScalarSelectedSession(t *testing.T) {
	raw, err := uaReadBasic()
	require.NoError(t, err)
	var c map[string]string
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &obj))
	c = map[string]string{}
	for _, k := range []string{"opn", "request", "response"} {
		var v string
		require.NoError(t, json.Unmarshal(obj[k], &v))
		c[k] = v
	}
	for _, deferred := range []bool{false, true} {
		var events []*ProtocolEvent
		cfg := NewDefaultConfig()
		require.NoError(t, WithBinParserConfig(BinParserConfig{DecodeOPCUARead: true, Deferred: deferred, OnEvent: func(e *ProtocolEvent) { events = append(events, e) }})(cfg))
		require.NoError(t, cfg.prepareBinParser())
		f := &binFlow{a: cfg.binParser, id: 1, endpoints: [2]string{"192.0.2.1:12345", "192.0.2.2:4840"}, ports: [2]uint16{12345, 4840}}
		for i, k := range []string{"opn", "request", "response"} {
			w, err := hex.DecodeString(c[k])
			require.NoError(t, err)
			f.feed(i/2, w, time.Unix(1, 0))
		}
		f.close("FIN")
		f.close("FIN")
		require.Zero(t, f.a.stats().BufferedBytes)
		require.Len(t, events, 3)
		for _, e := range events {
			require.Empty(t, e.Error)
			require.Equal(t, "opcua-read-scalar", e.Profile)
			first, err := e.GetFields()
			require.NoError(t, err)
			expected := cloneSession(first)
			e.Protocol = "forged"
			e.Profile = "forged"
			e.Fields = map[string]any{"forged": true}
			e.Structured = map[string]any{"fields": e.Fields}
			e.Session = map[string]any{"forged": true}
			first["forged"] = true
			again, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, expected, again)
		}
	}
}

func opcuaReadScalarResources(t *testing.T) {
	raw, err := uaReadBasic()
	require.NoError(t, err)
	var fixture map[string]string
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &obj))
	fixture = map[string]string{}
	for _, k := range []string{"opn", "request", "response", "request_body", "response_body"} {
		var v string
		require.NoError(t, json.Unmarshal(obj[k], &v))
		fixture[k] = v
	}
	req, err := hex.DecodeString(fixture["request_body"])
	require.NoError(t, err)
	rsp, err := hex.DecodeString(fixture["response_body"])
	require.NoError(t, err)
	for _, c := range []struct {
		wire     []byte
		response bool
	}{{req[4:], false}, {rsp[4:], true}} {
		var cost int64
		want, err := decodeOPCUARead(c.wire, c.response, 1024, func(n int64) error { cost = n; return nil })
		require.NoError(t, err)
		require.Greater(t, cost, int64(0))
		for _, limit := range []int64{cost - 1, cost} {
			got, err := decodeOPCUARead(c.wire, c.response, 1024, func(n int64) error {
				if n > limit {
					return protocolError(ErrResourceExceeded, "test bounded observation")
				}
				return nil
			})
			if limit < cost {
				require.Nil(t, got)
				rocTypedError(t, "ResourceExceeded", err)
			} else {
				require.NoError(t, err)
				require.Equal(t, want, got)
			}
		}
		got, err := decodeOPCUARead(c.wire, c.response, 0, nil)
		require.Nil(t, got)
		rocTypedError(t, "ResourceExceeded", err)
		for n := 0; n < len(c.wire); n++ {
			_, err := decodeOPCUARead(c.wire[:n], c.response, 1024, nil)
			require.Error(t, err, "cut %d", n)
		}
	}
	opn, _ := hex.DecodeString(fixture["opn"])
	request, _ := hex.DecodeString(fixture["request"])
	response, _ := hex.DecodeString(fixture["response"])
	s := &binOPCUA{read: true}
	_, err = s.consume(0, opn, 1024, 1<<20)
	require.NoError(t, err)
	_, err = s.consume(0, request, 1024, 1<<20)
	require.NoError(t, err)
	require.Len(t, s.appPending, 1)
	// Reject the response during allocation, invalidate its pending exchange,
	// then admit the adjacent response as an unpaired observation.
	s.appReserve = func(int64) error { return protocolError(ErrResourceExceeded, "test response allocation") }
	_, err = s.consume(1, response, 1024, 1<<20)
	rocTypedError(t, "ResourceExceeded", err)
	require.Empty(t, s.appPending)
	s.appReserve = nil
	later := bytes.Clone(response)
	later[16] = 2
	out, err := s.consume(1, later, 1024, 1<<20)
	require.NoError(t, err)
	require.Equal(t, false, out["Matched"])
	// A selected request's Close is observable once and releases all reservations.
	cfg := NewDefaultConfig()
	var events []*ProtocolEvent
	require.NoError(t, WithBinParserConfig(BinParserConfig{DecodeOPCUARead: true, OnEvent: func(e *ProtocolEvent) { events = append(events, e) }})(cfg))
	require.NoError(t, cfg.prepareBinParser())
	f := &binFlow{a: cfg.binParser, id: 1}
	f.feed(0, opn, time.Unix(1, 0))
	f.feed(0, request, time.Unix(1, 0))
	f.close("FIN")
	f.close("FIN")
	require.Len(t, events, 3)
	last := events[2]
	require.Equal(t, "incomplete", last.Status)
	require.Equal(t, map[string]any{"Outstanding Read Requests": 1, "Buffered Secure Messages": 0}, last.Session)
	rocTypedError(t, "NeedMore", last.sessionError)
	last.Status = "decoded"
	last.Profile = "forged"
	last.Fields = map[string]any{"forged": true}
	_, err = last.GetFields()
	rocTypedError(t, "NeedMore", err)
	require.Zero(t, f.a.stats().BufferedBytes)
}

func uaReadBasic() ([]byte, error) {
	raw, err := opcuaReadInput("material/cases.json")
	if err != nil {
		return nil, err
	}
	var cases []uaReadCase
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err = d.Decode(&cases); err != nil {
		return nil, err
	}
	if len(cases) == 0 || len(cases[0].Steps) != 3 {
		return nil, fmt.Errorf("missing sealed Read baseline")
	}
	m := map[string]string{}
	for i, key := range []string{"opn", "request", "response"} {
		m[key] = cases[0].Steps[i].Hex
		if i > 0 {
			m[key+"_body"] = cases[0].Steps[i].Hex[48:]
		}
	}
	return json.Marshal(m)
}
func opcuaReadScalarSealed(t *testing.T) {
	opcuaSelectedSealed(t, "opcua-read-scalar", "Read", 55, false)
}
func opcuaSelectedSealed(t *testing.T, root, field string, profiles int, browse bool) {
	raw, err := opcuaSelectedInput(root, "manifest.json")
	require.NoError(t, err)
	var manifest struct {
		Schema string
		Cases  []struct {
			ID, Capture, SHA256 string
			Packets, Bytes      int
		}
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, root+"/v1", manifest.Schema)
	require.Len(t, manifest.Cases, profiles)
	raw, err = opcuaSelectedInput(root, "material/cases.json")
	require.NoError(t, err)
	var cases []uaReadCase
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	require.NoError(t, d.Decode(&cases))
	require.Len(t, cases, len(manifest.Cases))
	for n, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			m := manifest.Cases[n]
			require.Equal(t, c.Name, m.ID)
			stored, err := opcuaSelectedInput(root, strings.TrimPrefix(m.Capture, "common/pcapx/pcaputil/"+root+"/"))
			require.NoError(t, err)
			require.Equal(t, m.SHA256, fmt.Sprintf("%x", sha256.Sum256(stored)))
			require.Len(t, stored, m.Bytes)
			for _, chunk := range []int{0, 1, 7} {
				var steps []sessionStep
				for _, step := range c.Steps {
					w, err := hex.DecodeString(step.Hex)
					require.NoError(t, err)
					steps = append(steps, sessionStep{step.Dir, w})
				}
				capture := stored
				if chunk != 0 {
					capture = sessionTestPCAP(t, steps, layers.TCPPort(4840), chunk, false, false)
				}
				for _, workers := range []int{1, 2, 4} {
					for _, deferred := range []bool{false, true} {
						for _, observe := range []bool{false, true} {
							t.Run(fmt.Sprintf("chunk%d/w%d/d%t/o%t", chunk, workers, deferred, observe), func(t *testing.T) {
								var mu sync.Mutex
								var events []*ProtocolEvent
								var stats ProtocolStats
								var tcp TCPReassemblyStats
								seen := 0
								opts := []CaptureOption{WithBinParserConfig(BinParserConfig{DecodeOPCUARead: !browse, DecodeOPCUABrowse: browse, Deferred: deferred}), WithTCPReassemblyWorkers(workers), WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); events = append(events, e); mu.Unlock() }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { tcp = s })}
								if observe {
									opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { seen++; require.Nil(t, p.ErrorLayer()) }))
								}
								input := bytes.Clone(capture)
								require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
								clear(input)
								wantClose := c.Close != nil
								count := len(c.Steps)
								inputBytes, droppedBytes := 0, 0
								for _, step := range c.Steps {
									inputBytes += len(step.Hex) / 2
									if step.Dropped {
										count--
										droppedBytes += len(step.Hex) / 2
									}
								}
								if wantClose {
									count++
								}
								require.Len(t, events, count)
								packet := uint64(4)
								var stepRefs [][]uint64
								for _, step := range c.Steps {
									n := len(step.Hex) / 2
									refs := 1
									if chunk > 0 {
										refs = (n + chunk - 1) / chunk
									}
									list := make([]uint64, refs)
									for j := range list {
										list[j] = packet + uint64(j)
									}
									stepRefs = append(stepRefs, list)
									packet += uint64(refs)
								}
								decoded, malformed, context, limited := 0, 0, 0, 0
								eventIndex := 0
								for i, step := range c.Steps {
									if step.Dropped {
										continue
									}
									e := events[eventIndex]
									eventIndex++
									wire, err := hex.DecodeString(step.Hex)
									require.NoError(t, err)
									require.EqualValues(t, i+1, e.ID)
									require.NotZero(t, e.FlowID)
									require.Zero(t, e.ResponseTo)
									require.Zero(t, e.TransactionID)
									require.Equal(t, "opcua", e.Protocol)
									require.Equal(t, "tcp", e.Transport)
									require.Equal(t, root, e.Profile)
									require.Equal(t, step.Dir, e.Direction)
									require.Equal(t, wire, e.Raw)
									require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(wire)), e.SourceBytes.SHA256)
									indices := step.Contributors
									if len(indices) == 0 {
										indices = []int{i}
									}
									var refs []uint64
									var parents []uint64
									for _, j := range indices {
										refs = append(refs, stepRefs[j]...)
										if j != i {
											parents = append(parents, uint64(j+1))
										}
									}
									require.Len(t, e.SourceBytes.PacketRefs, len(refs))
									for j, ref := range e.SourceBytes.PacketRefs {
										require.Equal(t, refs[j], ref.Number)
										require.Equal(t, e.Domain, ref.Domain)
									}
									require.Equal(t, parents, e.SourceBytes.ParentPDUs)
									if step.Error != "" {
										require.NotEmpty(t, e.Error)
										rocTypedError(t, step.Error, e.sessionError)
										if step.Error == "UnsupportedFeature" {
											context++
										} else if step.Error == "ResourceExceeded" {
											limited += len(wire)
										} else {
											malformed++
										}
										e.Protocol, e.Profile, e.Status = "forged", "forged", "decoded"
										e.Error = ""
										e.Fields = map[string]any{"forged": true}
										e.Structured = map[string]any{"fields": e.Fields}
										_, err = e.GetFields()
										rocTypedError(t, step.Error, err)
										continue
									}
									decoded++
									require.Empty(t, e.Error)
									status := "decoded"
									if deferred {
										status = "deferred"
									}
									require.Equal(t, status, e.Status)
									if step.Service != 0 {
										require.Equal(t, step.Service, e.Session["Service Type ID"])
									} else {
										require.NotContains(t, e.Session, "Service Type ID")
									}
									if step.Session != nil {
										requireExactProtocolFields(t, step.Session, e.Session)
									}
									if step.Read != nil {
										requireExactProtocolFields(t, step.Read, e.Session[field].(map[string]any))
									}
									if step.Service == 634 || step.Service == 530 {
										require.Equal(t, step.Matched, e.Session["Matched"])
									}
									fields, err := e.GetFields()
									require.NoError(t, err)
									requireExactProtocolFields(t, e.Session, fields)
									expected := cloneSession(fields)
									fields[field] = "forged"
									e.Fields = map[string]any{"forged": true}
									e.Structured = map[string]any{"fields": e.Fields}
									e.Session = map[string]any{"forged": true}
									e.Profile, e.Protocol = "forged", "forged"
									clear(e.Raw)
									again, err := e.GetFields()
									require.NoError(t, err)
									require.Equal(t, expected, again)
								}
								if wantClose {
									e := events[len(events)-1]
									require.Equal(t, "incomplete", e.Status)
									require.Empty(t, e.Raw)
									requireExactProtocolFields(t, c.Close, e.Session)
									rocTypedError(t, "NeedMore", e.sessionError)
									require.EqualValues(t, 1, stats.Incomplete)
								} else {
									require.Zero(t, stats.Incomplete)
								}
								require.EqualValues(t, malformed, stats.Malformed)
								require.EqualValues(t, context, stats.ContextRequired)
								require.EqualValues(t, limited, stats.LimitedBytes)
								if deferred {
									require.EqualValues(t, decoded, stats.Deferred)
								} else {
									require.EqualValues(t, decoded, stats.Decoded)
								}
								require.Zero(t, stats.BufferedBytes)
								require.EqualValues(t, inputBytes, stats.InputBytes)
								require.EqualValues(t, droppedBytes, stats.UnclassifiedBytes)
								require.Zero(t, tcp.UnreassembledSegments)
								require.Zero(t, tcp.UnreassembledBytes)
								require.Zero(t, tcp.DecodeErrors)
								if observe {
									require.EqualValues(t, tcp.CapturedPackets, seen)
								} else {
									require.Zero(t, seen)
								}
								if chunk == 0 {
									require.EqualValues(t, m.Packets, tcp.CapturedPackets)
								}
							})
						}
					}
				}
			}
		})
	}
}

func opcuaReadInput(name string) ([]byte, error) {
	return opcuaSelectedInput("opcua-read-scalar", name)
}
func opcuaSelectedInput(root, name string) ([]byte, error) {
	env := "OPCUA_TEST_MATERIAL_ROOT"
	if root == "opcua-browse" {
		env = "OPCUA_BROWSE_TEST_MATERIAL_ROOT"
	}
	if override := os.Getenv(env); override != "" {
		return os.ReadFile(filepath.Join(override, name))
	}
	return trafficfixture.ReadFile(root + "/" + name)
}

func opcuaReadScalarAll(t *testing.T) {
	t.Run("Codec", opcuaReadScalarCodecCases)
	t.Run("Session", opcuaReadScalarSelectedSession)
	t.Run("Resources", opcuaReadScalarResources)
	t.Run("Sealed", opcuaReadScalarSealed)
}
