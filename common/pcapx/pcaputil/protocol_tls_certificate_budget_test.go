package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func tlsCertificateBudgetWant(t *testing.T, k string, v any) any {
	t.Helper()
	if k == "DNS Names" && v == nil {
		return []string(nil)
	}
	switch x := v.(type) {
	case map[string]any:
		r := map[string]any{}
		for k, v := range x {
			r[k] = tlsCertificateBudgetWant(t, k, v)
		}
		return r
	case []any:
		if k == "DNS Names" {
			r := make([]string, len(x))
			for i, v := range x {
				r[i] = v.(string)
			}
			return r
		}
		r := make([]map[string]any, len(x))
		for i, v := range x {
			r[i] = tlsCertificateBudgetWant(t, k, v).(map[string]any)
		}
		return r
	case float64:
		switch k {
		case "Record Type", "Type":
			return byte(x)
		case "Record Version", "Negotiated Version", "Cipher Suite":
			return uint16(x)
		}
		return int(x)
	case string:
		if k == "Not Before" || k == "Not After" {
			ts, err := time.Parse(time.RFC3339, x)
			require.NoError(t, err)
			return ts
		}
		return x
	default:
		return x
	}
}

func tlsCertificateBudgetControls(t *testing.T) []tlsAlertCase {
	raw := finsReadBatch(t, "tls-certificate-budget", "TLS_CERTIFICATE_BUDGET_TEST_ROOT", "controls.json")
	var x struct {
		Cases []struct {
			Name  string
			Steps []struct {
				Wire      string `json:"input_hex"`
				SHA       string `json:"input_sha256"`
				Direction int
				At        int64 `json:"timestamp_unix"`
				Offset    uint64
				Error     string
				Fields    map[string]any `json:"correct_fields"`
			}
			After *struct {
				Wire  string `json:"input_hex"`
				Error string
			} `json:"after_refusal_feed"`
		}
	}
	require.NoError(t, json.Unmarshal(raw, &x))
	require.Len(t, x.Cases, 2)
	out := make([]tlsAlertCase, 0, len(x.Cases))
	for _, c := range x.Cases {
		tc := tlsAlertCase{Name: c.Name, Ports: [2]uint16{40000, 443}}
		tc.Budget.Collection = 1
		var n uint64
		for _, s := range c.Steps {
			completeness := "envelope"
			if s.Error != "" {
				completeness = "limited"
			}
			tc.Steps = append(tc.Steps, tlsAlertStep{Wire: s.Wire, SHA: s.SHA, Direction: s.Direction, At: s.At, Offset: s.Offset, Fields: s.Fields, Error: s.Error, Completeness: completeness})
			n += uint64(len(wrapperWire(t, s.Wire)))
		}
		tc.Statistics = map[string]uint64{"Messages": 3, "MessageBytes": n, "InputBytes": n, "UnclassifiedBytes": 0, "Incomplete": 0, "Malformed": 0}
		if c.After != nil {
			tc.Probes = []tlsAlertProbe{{Wire: c.After.Wire, Direction: 1, At: 103, Error: c.After.Error}}
			late := uint64(len(wrapperWire(t, c.After.Wire)))
			tc.Statistics["InputBytes"] += late
			tc.Statistics["UnclassifiedBytes"] = late
			tc.Statistics["LimitedBytes"] = late
		} else {
			tc.Statistics["LimitedBytes"] = 0
		}
		out = append(out, tc)
	}
	return out
}
func tlsCertificateBudgetQuery(t *testing.T, e *ProtocolEvent, want map[string]any, kind string) {
	tlsAlertCheckQuery(t, e, want, kind)
	if kind == "" {
		f, err := e.GetFields()
		require.NoError(t, err)
		radiusEAPMutateStringLists(f)
		mutateRTPSQuery(f)
		decoded, err := e.Decode()
		require.NoError(t, err)
		require.Equal(t, want, decoded["fields"])
		require.Equal(t, want, decoded["session"])
		radiusEAPMutateStringLists(decoded)
		mutateRTPSQuery(decoded)
	}
	tlsAlertCheckQuery(t, e, want, kind)
}
func tlsCertificateBudgetCheckEvent(t *testing.T, e *ProtocolEvent, c tlsAlertCase, w tlsAlertStep, captured, deferred bool, flow uint64) {
	t.Helper()
	var raw []byte
	if w.Wire != "" {
		raw = wrapperWire(t, w.Wire)
	}
	want := tlsCertificateBudgetWant(t, "", w.Fields).(map[string]any)
	require.Equal(t, "tls", e.Protocol)
	require.Equal(t, "tcp", e.Transport)
	require.Equal(t, w.Direction, e.Direction)
	require.Equal(t, w.Offset, e.Offset)
	require.Equal(t, raw, e.Raw)
	require.Equal(t, len(raw), e.Length)
	require.True(t, time.Unix(w.At, 0).Equal(e.Timestamp))
	require.NotZero(t, e.ID)
	require.Equal(t, flow, e.FlowID)
	require.Zero(t, e.TransactionID)
	require.Zero(t, e.ResponseTo)
	require.Equal(t, CaptureDomain{}, e.Domain)
	src, dst := fmt.Sprintf("192.0.2.1:%d", c.Ports[0]), fmt.Sprintf("192.0.2.2:%d", c.Ports[1])
	if w.Direction == 1 {
		src, dst = dst, src
	}
	require.Equal(t, src, e.Source)
	require.Equal(t, dst, e.Destination)
	status := "decoded"
	if deferred {
		status = "deferred"
	}
	switch w.Error {
	case "NeedMore":
		status = "incomplete"
	case "MalformedMessage":
		status = "malformed"
	case "ResourceExceeded":
		status = "limited"
	case "ContextRequired":
		status = "context-required"
	}
	require.Equal(t, status, e.Status)
	require.Equal(t, w.Completeness, e.Completeness)
	require.Equal(t, want, e.Session)
	if captured && len(raw) > 0 {
		require.NotEmpty(t, e.SourceBytes.PacketRefs)
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(raw)), e.SourceBytes.SHA256)
	}
	tlsCertificateBudgetQuery(t, e, want, w.Error)
	radiusEAPMutateStringLists(e.Session)
	tlsDTLSPoison(e, "radius")
	e.TransactionID, e.ResponseTo = 999999, 999999
	tlsCertificateBudgetQuery(t, e, want, w.Error)
}
func TestTLSCertificateBudgetNative(t *testing.T) {
	for _, c := range tlsCertificateBudgetControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			for _, d := range []bool{false, true} {
				for _, chunk := range []int{0, 1, 7} {
					t.Run(fmt.Sprintf("d%t/c%d", d, chunk), func(t *testing.T) {
						b := DefaultParserBudget()
						if c.Budget.Collection > 0 {
							b.MaxCollectionElements = c.Budget.Collection
						}
						s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("tcp"), WithSessionPorts(c.Ports[0], c.Ports[1]))
						require.NoError(t, err)
						defer s.Close("cleanup")
						s.(*captureSession).f.a.config.Deferred = d
						flow := uint64(0)
						ids := map[uint64]bool{}
						for _, w := range c.Steps {
							raw := wrapperWire(t, w.Wire)
							require.Equal(t, w.SHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
							var events []*ProtocolEvent
							var last *ProtocolError
							for at := 0; at < len(raw); {
								n := len(raw) - at
								if chunk > 0 {
									n = min(n, chunk)
								}
								input := bytes.Clone(raw[at : at+n])
								o := s.Feed(w.Direction, time.Unix(w.At, 0), input)
								clear(input)
								at += n
								if at < len(raw) {
									if o.Err != nil {
										rocTypedError(t, "NeedMore", o.Err)
									}
								} else {
									last = o.Err
								}
								events = append(events, o.Events...)
							}
							if w.Error == "" {
								require.Nil(t, last)
							} else {
								rocTypedError(t, w.Error, last)
							}
							require.Len(t, events, 1)
							e := events[0]
							if flow == 0 {
								flow = e.FlowID
							}
							require.False(t, ids[e.ID])
							ids[e.ID] = true
							tlsCertificateBudgetCheckEvent(t, e, c, w, false, d, flow)
						}
						for _, p := range c.Probes {
							o := s.Feed(p.Direction, time.Unix(p.At, 0), wrapperWire(t, p.Wire))
							rocTypedError(t, p.Error, o.Err)
							require.Empty(t, o.Events)
						}
						closed := s.Close("done")
						require.Len(t, closed, len(c.Close))
						for i, w := range c.Close {
							e := closed[i]
							require.False(t, ids[e.ID])
							ids[e.ID] = true
							tlsCertificateBudgetCheckEvent(t, e, c, w, false, d, flow)
						}
						require.Empty(t, s.Close("again"))
						tlsAlertCheckStats(t, s.Stats(), c.Statistics)
					})
				}
			}
		})
	}
}
func TestTLSCertificateBudgetMatrix(t *testing.T) {
	for _, c := range tlsCertificateBudgetControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			generated := tlsAlertCapture(t, c)
			if dir := os.Getenv("TLS_CERTIFICATE_BUDGET_EXPORT"); dir != "" {
				p := filepath.Join(dir, "tls-certificate-budget", c.Name+".pcap")
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0700))
				require.NoError(t, os.WriteFile(p, generated, 0600))
			}
			var raw []byte
			if os.Getenv("TLS_CERTIFICATE_BUDGET_TEST_ROOT") != "" {
				raw = generated
			} else {
				raw = finsReadBatch(t, "tls-certificate-budget", "TLS_CERTIFICATE_BUDGET_TEST_ROOT", c.Name+".pcap")
				require.Equal(t, generated, raw)
			}
			discoveryMatrix(t, func(t *testing.T, workers int, d, o bool) {
				var mu sync.Mutex
				var events []*ProtocolEvent
				var stats ProtocolStats
				var packets atomic.Int64
				b := DefaultParserBudget()
				if c.Budget.Collection > 0 {
					b.MaxCollectionElements = c.Budget.Collection
				}
				opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(d), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnProtocolMessage(func(e *ProtocolEvent) {
					mu.Lock()
					defer mu.Unlock()
					copy := *e
					copy.Raw = bytes.Clone(e.Raw)
					copy.Session = cloneSession(e.Session)
					copy.Fields = cloneSession(e.Fields)
					copy.Structured = cloneSession(e.Structured)
					copy.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
					events = append(events, &copy)
					// Change nested public state while later records are still outstanding.
					radiusEAPMutateStringLists(e.Session)
					mutateRTPSQuery(e.Session)
					tlsDTLSPoison(e, "radius")
				})}
				if o {
					opts = append(opts, WithEveryPacket(func(gopacket.Packet) { packets.Add(1) }))
				}
				a, err := NewPacketAnalyzer(opts...)
				require.NoError(t, err)
				a.conf.binParser.budget = b
				reader, err := NewCaptureReader(bytes.NewReader(raw))
				require.NoError(t, err)
				for {
					packet, ci, readErr := reader.ReadPacketData()
					if readErr == io.EOF {
						break
					}
					require.NoError(t, readErr)
					require.NoError(t, a.Feed(packet, ci, reader.LinkType()))
					clear(packet)
				}
				require.NoError(t, a.Close())
				require.NoError(t, a.Close())
				require.Len(t, events, len(c.Steps)+len(c.Close))
				flow := events[0].FlowID
				ids := map[uint64]bool{}
				for i, w := range append(append([]tlsAlertStep{}, c.Steps...), c.Close...) {
					e := events[i]
					require.False(t, ids[e.ID])
					ids[e.ID] = true
					tlsCertificateBudgetCheckEvent(t, e, c, w, true, d, flow)
					if i < len(c.Steps) {
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						require.Equal(t, uint64(i+4), e.SourceBytes.PacketRefs[0].Number)
					}
				}
				tlsAlertCheckStats(t, stats, c.Statistics)
				if o {
					require.Equal(t, int64(6+len(c.Steps)+len(c.Probes)), packets.Load())
				} else {
					require.Zero(t, packets.Load())
				}
			})
		})
	}
}

func TestTLSCertificateBudgetVectors(t *testing.T) {
	var x struct {
		Cases []struct {
			Name         string
			TLS13        bool `json:"tls13"`
			Max          int
			Wire         string `json:"input_hex"`
			SHA          string `json:"input_sha256"`
			Error        string `json:"expected_error"`
			Certificates []any  `json:"expected_certificates"`
		}
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "tls-certificate-budget", "TLS_CERTIFICATE_BUDGET_TEST_ROOT", "helper-vectors.json"), &x))
	require.Len(t, x.Cases, 6)
	for _, c := range x.Cases {
		t.Run(c.Name, func(t *testing.T) {
			w := wrapperWire(t, c.Wire)
			require.Equal(t, c.SHA, fmt.Sprintf("%x", sha256.Sum256(w)))
			rows, err := tlsCertificates(w, c.TLS13, c.Max)
			rocTypedError(t, c.Error, err)
			if c.Error != "" {
				require.Nil(t, rows)
			} else {
				want := tlsCertificateBudgetWant(t, "Certificates", c.Certificates)
				require.Equal(t, want, rows)
				clear(w)
				require.Equal(t, want, rows)
			}
		})
	}
}
