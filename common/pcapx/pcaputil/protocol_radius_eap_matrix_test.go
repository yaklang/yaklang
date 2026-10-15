package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type radiusEAPStep struct {
	Direction                                  int
	At                                         int64 `json:"timestamp_unix"`
	Nanos                                      int64
	Wire                                       string `json:"input_hex"`
	Error                                      string
	Fields                                     map[string]any `json:"fields_hexbytes"`
	Legacy                                     map[string]any `json:"legacy_fields_hexstrings"`
	Reply                                      *int           `json:"response_to_step"`
	Transaction                                *int           `json:"transaction_step"`
	Profile, Completeness, Source, Destination string
}
type radiusEAPControl struct {
	Name          string
	CaptureSource string `json:"capture_source"`
	Steps         []radiusEAPStep
	Ports         [2]uint16
	Legacy        bool `json:"legacy_byte_applicable"`
	Budget        struct{ MaxMessageBytes, MaxCollectionElements int }
	Close         struct {
		Profile     string
		Outstanding int
		Error       string
	} `json:"close_expected"`
}

func radiusEAPControls(t *testing.T) []radiusEAPControl {
	t.Helper()
	var x struct{ Cases []radiusEAPControl }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-eap", "RADIUS_EAP_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 50)
	var legacy struct {
		Records map[string]map[string]any `json:"records_by_input_sha256"`
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-eap", "RADIUS_EAP_TEST_ROOT", "legacy-compatibility.json"), &legacy))
	for ci := range x.Cases {
		for si := range x.Cases[ci].Steps {
			w := &x.Cases[ci].Steps[si]
			w.Legacy = legacy.Records[fmt.Sprintf("%x", sha256.Sum256(radiusWire(t, w.Wire)))]
			require.NotNil(t, w.Legacy, "independent compatibility answer")
		}
	}
	radiusNASReconcileHistoricalEAP(t, x.Cases)
	return x.Cases
}
func radiusEAPWant(t *testing.T, c radiusEAPControl, i int) map[string]any {
	t.Helper()
	if c.Steps[i].Fields == nil {
		return nil
	}
	s := cloneSession(c.Steps[i].Fields)
	for _, key := range []string{"Code", "Identifier", "Attribute Count", "Packet Length", "Padding Bytes", "User-Password Bytes"} {
		if v, ok := s[key].(float64); ok {
			s[key] = int(v)
		}
	}
	if v, ok := s["Acct-Status-Type"].(float64); ok {
		s["Acct-Status-Type"] = uint32(v)
	}
	for _, key := range []string{"Authenticator", "Message-Authenticator", "Acct-Session-Id"} {
		if v, ok := s[key].(string); ok {
			s[key] = radiusWire(t, v)
		}
	}
	attrs := make([]map[string]any, 0)
	for _, v := range s["Attributes"].([]any) {
		a := v.(map[string]any)
		attrs = append(attrs, map[string]any{"Type": byte(a["Type"].(float64)), "Raw": radiusWire(t, a["Raw"].(string))})
	}
	s["Attributes"] = attrs
	if v, ok := s["EAP Diagnostics"].([]any); ok {
		d := make([]string, len(v))
		for i, a := range v {
			d[i] = a.(string)
		}
		s["EAP Diagnostics"] = d
	}
	if e, ok := s["EAP"].(map[string]any); ok {
		if v, ok := e["Method Count"].(float64); ok {
			e["Method Count"] = int(v)
		}
		for _, k := range []string{"Vendor-Id", "Vendor-Type"} {
			if v, ok := e[k].(float64); ok {
				e[k] = uint32(v)
			}
		}
		for _, k := range []string{"Desired Types", "Vendor Data"} {
			if v, ok := e[k].(string); ok {
				e[k] = radiusWire(t, v)
			}
		}
		if rows, ok := e["Desired Expanded Types"].([]any); ok {
			out := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				m := r.(map[string]any)
				out = append(out, map[string]any{"Vendor-Id": uint32(m["Vendor-Id"].(float64)), "Vendor-Type": uint32(m["Vendor-Type"].(float64))})
			}
			e["Desired Expanded Types"] = out
		}
	}
	if e, ok := s["EAP"].(map[string]any); ok {
		for _, key := range []string{"Code", "Identifier", "Type", "Value Size"} {
			if v, ok := e[key].(float64); ok {
				e[key] = byte(v)
			}
		}
		if v, ok := e["Length"].(float64); ok {
			e["Length"] = uint16(v)
		}
		for _, key := range []string{"Padding Bytes", "Fragments"} {
			if v, ok := e[key].(float64); ok {
				e[key] = int(v)
			}
		}
		for _, key := range []string{"Raw", "Type Data", "Value", "Name", "Prompt", "Identity", "Notification"} {
			if v, ok := e[key].(string); ok {
				e[key] = radiusWire(t, v)
			}
		}
	}
	return s
}
func radiusEAPCapture(t *testing.T, c radiusEAPControl) []byte {
	b := radiusControl{Name: c.Name, Ports: c.Ports}
	for _, s := range c.Steps {
		b.Steps = append(b.Steps, radiusControlStep{Direction: s.Direction, At: s.At, Wire: s.Wire})
	}
	return radiusControlCapture(t, b)
}
func radiusEAPCheck(t *testing.T, c radiusEAPControl, events []*ProtocolEvent, captured, deferred bool, byteCarrier ...bool) {
	t.Helper()
	legacy := len(byteCarrier) > 0 && byteCarrier[0]
	extra := 0
	if c.Close.Outstanding > 0 {
		extra = 1
	}
	require.Len(t, events, len(c.Steps)+extra)
	ids := map[uint64]bool{}
	for i, w := range c.Steps {
		e := events[i]
		require.Equal(t, "radius", e.Protocol)
		require.Equal(t, w.Profile, e.Profile)
		require.NotZero(t, e.ID)
		require.False(t, ids[e.ID])
		ids[e.ID] = true
		require.NotZero(t, e.FlowID)
		require.Equal(t, events[0].FlowID, e.FlowID)
		require.Equal(t, w.Direction, e.Direction)
		transport := "udp"
		if legacy {
			transport = "tcp"
		}
		require.Equal(t, transport, e.Transport)
		require.Equal(t, CaptureDomain{}, e.Domain)
		require.True(t, time.Unix(w.At, w.Nanos).Equal(e.Timestamp))
		ports := c.Ports
		if legacy {
			ports = [2]uint16{40000, 40001}
		}
		src, dst := fmt.Sprintf("192.0.2.1:%d", ports[0]), fmt.Sprintf("192.0.2.2:%d", ports[1])
		if w.Direction == 1 {
			src, dst = dst, src
		}
		if captured && w.Source != "" {
			src, dst = w.Source, w.Destination
		}
		require.Equal(t, src, e.Source)
		require.Equal(t, dst, e.Destination)
		status := "decoded"
		if deferred {
			status = "deferred"
		}
		if w.Error == "MalformedMessage" {
			status = "malformed"
		}
		if w.Error == "ResourceExceeded" {
			status = "limited"
		}
		if w.Error == "UnsupportedFeature" {
			status = "context-required"
		}
		require.Equal(t, status, e.Status)
		require.Equal(t, w.Completeness, e.Completeness)
		raw := radiusWire(t, w.Wire)
		preview := raw
		if c.Budget.MaxMessageBytes > 0 && len(raw) > c.Budget.MaxMessageBytes {
			preview = raw[:c.Budget.MaxMessageBytes]
			require.Equal(t, "limited", e.Completeness)
			require.Equal(t, "ResourceExceeded", e.ExpertCode)
		}
		require.Equal(t, preview, e.Raw)
		require.Equal(t, len(raw), e.Length)
		if w.Reply == nil {
			require.Zero(t, e.ResponseTo)
		} else {
			require.Equal(t, events[*w.Reply].ID, e.ResponseTo)
		}
		if w.Transaction == nil {
			require.Zero(t, e.TransactionID)
		} else {
			require.Equal(t, events[*w.Transaction].ID, e.TransactionID)
		}
		if captured {
			require.Len(t, e.SourceBytes.PacketRefs, 1)
			require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
			require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(preview)), e.SourceBytes.SHA256)
		}
		want := radiusEAPWant(t, c, i)
		if legacy {
			delete(want, "Observation Scope")
		}
		queryWant := want
		if legacy && w.Error == "" {
			queryWant = radiusEAPLegacyWant(t, w.Legacy)
		}
		require.Equal(t, want, e.Session)
		f, err := e.GetFields()
		rocTypedError(t, w.Error, err)
		if w.Error == "" {
			require.Equal(t, queryWant, f)
		} else {
			require.Nil(t, f)
		}
		if w.Error == "" {
			tree, err := e.Decode()
			require.NoError(t, err)
			require.Equal(t, want, tree["session"])
			mutateRTPSQuery(tree)
			mutateRTPSQuery(f)
		} else {
			require.Nil(t, f)
			require.NotEmpty(t, e.Error)
		}
		tlsDTLSPoison(e, "http")
		f, err = e.GetFields()
		rocTypedError(t, w.Error, err)
		if w.Error == "" {
			require.Equal(t, queryWant, f)
		} else {
			require.Nil(t, f)
		}
	}
	if extra == 1 {
		e := events[len(c.Steps)]
		require.Equal(t, "radius", e.Protocol)
		require.Equal(t, c.Close.Profile, e.Profile)
		require.Equal(t, "incomplete", e.Status)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		require.Equal(t, map[string]any{"Outstanding": c.Close.Outstanding}, e.Session)
		f, err := e.GetFields()
		require.Nil(t, f)
		rocTypedError(t, "NeedMore", err)
	}
}
func radiusEAPBudget(c radiusEAPControl) ParserBudget {
	b := DefaultParserBudget()
	if c.Budget.MaxMessageBytes > 0 {
		b.MaxMessageBytes = c.Budget.MaxMessageBytes
	}
	if c.Budget.MaxCollectionElements > 0 {
		b.MaxCollectionElements = c.Budget.MaxCollectionElements
	}
	return b
}
func TestRADIUSEAPNative(t *testing.T) { radiusNativeCases(t, radiusEAPControls(t)) }
func radiusNativeCases(t *testing.T, controls []radiusEAPControl) {
	for _, c := range controls {
		for _, d := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/d%t", c.Name, d), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(radiusEAPBudget(c), WithSessionTransport("udp"), WithSessionPorts(c.Ports[0], c.Ports[1]))
				require.NoError(t, err)
				defer s.Close("cleanup")
				s.(*captureSession).f.a.config.Deferred = d
				var events []*ProtocolEvent
				total, limited := 0, 0
				for _, w := range c.Steps {
					raw := radiusWire(t, w.Wire)
					n := len(raw)
					total += n
					if w.Error == "ResourceExceeded" {
						limited += n
					}
					o := s.Feed(w.Direction, time.Unix(w.At, w.Nanos), raw)
					clear(raw)
					require.Equal(t, n, o.Consumed)
					if w.Error == "" {
						require.Nil(t, o.Err)
					} else {
						rocTypedError(t, w.Error, o.Err)
					}
					require.Len(t, o.Events, 1)
					events = append(events, o.Events...)
				}
				events = append(events, s.Close("complete")...)
				require.Empty(t, s.Close("again"))
				st := s.Stats()
				require.Zero(t, st.BufferedBytes)
				require.EqualValues(t, total, st.InputBytes)
				require.EqualValues(t, total, st.MessageBytes)
				require.EqualValues(t, limited, st.LimitedBytes)
				radiusEAPCheck(t, c, events, false, d)
			})
		}
	}
}
func TestRADIUSEAPMatrix(t *testing.T) {
	radiusCaptureCases(t, radiusEAPControls(t), "radius-eap", "RADIUS_EAP_TEST_ROOT", "RADIUS_EAP_EXPORT")
}
func radiusCaptureCases(t *testing.T, controls []radiusEAPControl, batch, rootEnv, exportEnv string) {
	for _, c := range controls {
		t.Run(c.Name, func(t *testing.T) {
			raw := radiusEAPCapture(t, c)
			if c.CaptureSource != "" {
				historical, err := trafficfixture.ReadFile(c.CaptureSource)
				require.NoError(t, err)
				require.Equal(t, raw, historical, "exact historical capture reuse")
			} else if root := os.Getenv(exportEnv); root != "" {
				p := filepath.Join(root, batch)
				require.NoError(t, os.MkdirAll(p, 0700))
				require.NoError(t, os.WriteFile(filepath.Join(p, c.Name+".pcap"), raw, 0600))
			} else {
				require.Equal(t, raw, finsReadBatch(t, batch, rootEnv, c.Name+".pcap"))
			}
			for _, workers := range []int{1, 2, 4} {
				for _, d := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%t/o%t", workers, d, observe), func(t *testing.T) {
							var events, originals []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(d), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnProtocolMessage(func(e *ProtocolEvent) {
								snap := *e
								snap.Raw = bytes.Clone(e.Raw)
								snap.Session = cloneSession(e.Session)
								snap.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
								events = append(events, &snap)
								originals = append(originals, e)
								if observe {
									tlsDTLSPoison(e, "http")
									e.ID, e.FlowID = 0, 0
								}
							})}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							a, err := NewPacketAnalyzer(opts...)
							require.NoError(t, err)
							a.conf.binParser.budget = radiusEAPBudget(c)
							if c.Budget.MaxMessageBytes > 0 {
								a.conf.binParser.config.MaxMessageBytes = c.Budget.MaxMessageBytes
							}
							r, err := NewCaptureReader(bytes.NewReader(raw))
							require.NoError(t, err)
							for {
								b, ci, err := r.ReadPacketData()
								if err == io.EOF {
									break
								}
								require.NoError(t, err)
								require.NoError(t, a.Feed(b, ci, r.LinkType()))
								clear(b)
							}
							require.NoError(t, a.Close())
							require.NoError(t, a.Close())
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							if observe {
								require.Equal(t, len(c.Steps), seen)
							}
							radiusEAPCheck(t, c, events, true, d)
							for i := range c.Steps {
								f, err := originals[i].GetFields()
								rocTypedError(t, c.Steps[i].Error, err)
								if c.Steps[i].Error == "" {
									require.Equal(t, radiusEAPWant(t, c, i), f)
								} else {
									require.Nil(t, f)
								}
							}
						})
					}
				}
			}
		})
	}
}

// The compatibility projection follows the fixed RADIUS rule schema. Raw/string
// representations are distinct from the canonical passive Session above.
func radiusEAPLegacyWant(t *testing.T, input map[string]any) map[string]any {
	t.Helper()
	out := cloneSession(input)
	out["Code"] = byte(out["Code"].(float64))
	out["Identifier"] = byte(out["Identifier"].(float64))
	out["Length"] = uint16(out["Length"].(float64))
	out["Authenticator"] = radiusWire(t, out["Authenticator"].(string))
	if a, ok := out["Attributes"].([]any); ok {
		for _, v := range a {
			m := v.(map[string]any)
			typ := byte(m["Type"].(float64))
			m["Type"] = typ
			m["Length"] = byte(m["Length"].(float64))
			if x, ok := m["NAS-Port"].(float64); ok {
				m["NAS-Port"] = uint32(x)
			}
			if x, ok := m["NAS-IP-Address"].(string); ok {
				m["NAS-IP-Address"] = radiusWire(t, x)
			}
			for _, key := range []string{"User-Name", "User-Password", "Message-Authenticator", "Attribute Data"} {
				if x, ok := m[key].(string); ok {
					m[key] = string(radiusWire(t, x))
				}
			}
		}
	}
	return out
}
func TestRADIUSEAPLegacy(t *testing.T) { radiusLegacyCases(t, radiusEAPControls(t), 47) }
func radiusLegacyCases(t *testing.T, controls []radiusEAPControl, expected int) {
	eligible := 0
	defer func() { require.Equal(t, expected, eligible, "all independently admitted byte traces remain required") }()
	for _, c := range controls {
		if !c.Legacy || c.Name == "accounting-forbidden-79-80" {
			continue
		}
		eligible++
		for _, deferred := range []bool{false, true} {
			for _, chunk := range []int{0, 1, 7} {
				t.Run(fmt.Sprintf("%s/d%t/c%d", c.Name, deferred, chunk), func(t *testing.T) {
					s, err := NewProtocolSession(radiusEAPBudget(c))
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = deferred
					var events []*ProtocolEvent
					total := 0
					for _, w := range c.Steps {
						wire := radiusWire(t, w.Wire)
						total += len(wire)
						z := chunk
						if z == 0 {
							z = len(wire)
						}
						for at := 0; at < len(wire); at += z {
							part := bytes.Clone(wire[at:min(at+z, len(wire))])
							n := len(part)
							o := s.Feed(w.Direction, time.Unix(w.At, 0), part)
							clear(part)
							require.Equal(t, n, o.Consumed)
							if len(o.Events) > 0 {
								if w.Error == "" {
									require.Nil(t, o.Err)
								} else {
									rocTypedError(t, w.Error, o.Err)
								}
							} else if o.Err != nil {
								require.Equal(t, ErrNeedMore, o.Err.Kind)
							}
							events = append(events, o.Events...)
						}
					}
					events = append(events, s.Close("complete")...)
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
					require.EqualValues(t, total, s.Stats().InputBytes)
					require.EqualValues(t, total, s.Stats().MessageBytes)
					radiusEAPCheck(t, c, events, false, deferred, true)
				})
			}
		}
	}
}
