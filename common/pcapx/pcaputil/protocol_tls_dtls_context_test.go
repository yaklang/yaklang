package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type tlsDTLSContextControl struct {
	Schema string
	Clock  []struct {
		ID, Capture, SHA256 string
		Events              []struct {
			Timestamp                      int64
			Interface, Port, Owner, Packet int
			Wire                           string
			Fields                         map[string]any
		}
	}
	Owned []struct {
		ID, Transport, Wire, SHA256, Error string
		PartialAlert                       bool
		Fields                             map[string]any
	}
}

func tlsDTLSControls(t *testing.T) tlsDTLSContextControl {
	t.Helper()
	var c tlsDTLSContextControl
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "tls-dtls-context", "TLS_DTLS_CONTEXT_TEST_ROOT", "controls.json"), &c))
	require.Equal(t, "owned-tls-dtls-context/v1", c.Schema)
	require.Len(t, c.Clock, 10)
	require.Len(t, c.Owned, 6)
	// Keep the original archived answer intact. The new, separately pinned
	// RFC5246 boundary sidecar supersedes only this exact one-octet input.
	var sidecar struct {
		Records map[string]struct {
			InputSHA string `json:"input_sha256"`
			Feed     struct {
				Fields map[string]any `json:"fields_hexbytes"`
			}
		} `json:"records_by_input_sha256"`
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "tls-alert", "TLS_ALERT_TEST_ROOT", "historical-disposition.json"), &sidecar))
	require.Len(t, sidecar.Records, 1)
	correction := sidecar.Records["64666695579c8a8ac3a8ef2e5573990552739600a4ebeb498c2fb5d0bf8505a2"]
	require.Equal(t, "64666695579c8a8ac3a8ef2e5573990552739600a4ebeb498c2fb5d0bf8505a2", correction.InputSHA)
	corrected := 0
	for i := range c.Owned {
		v := &c.Owned[i]
		if v.SHA256 == correction.InputSHA {
			require.Equal(t, "tcp-bad-alert", v.ID)
			require.Equal(t, "MalformedMessage", v.Error)
			require.Equal(t, "150303000101", v.Wire)
			v.Error = ""
			v.Fields = correction.Feed.Fields
			v.PartialAlert = true
			corrected++
		}
	}
	require.Equal(t, 1, corrected)
	return c
}
func tlsDTLSPoison(e *ProtocolEvent, alias string) {
	clear(e.Fields)
	clear(e.Session)
	e.Protocol, e.Profile, e.Status = alias, "caller", "decoded"
	e.Fields = map[string]any{"Authentication Verified": true, "caller": true}
	e.Session = map[string]any{"Authentication Verified": true, "caller": true}
	e.Structured = map[string]any{"fields": e.Fields, "session": e.Session}
	clear(e.Raw)
}
func tlsDTLSQuery(t *testing.T, e *ProtocolEvent, want map[string]any, kind string) {
	t.Helper()
	f, err := e.GetFields()
	rocTypedError(t, kind, err)
	if kind != "" {
		require.Nil(t, f)
		assertOwnedProtocolRefusal(t, e, kind, err)
		decoded, err := e.Decode()
		rocTypedError(t, kind, err)
		require.Nil(t, decoded)
		return
	}
	rocEqualFields(t, want, f)
	// Nested maps in a returned query, as well as the root, belong to its caller.
	if rows, ok := f["Records"].([]map[string]any); ok {
		for _, r := range rows {
			clear(r)
		}
	}
	clear(f)
	decoded, err := e.Decode()
	require.NoError(t, err)
	rocEqualFields(t, want, decoded["fields"].(map[string]any))
	rocEqualFields(t, want, decoded["session"].(map[string]any))
	clear(decoded["fields"].(map[string]any))
	clear(decoded["session"].(map[string]any))
	again, err := e.GetFields()
	require.NoError(t, err)
	rocEqualFields(t, want, again)
}
func TestTLSAndDTLSOwnedObservation(t *testing.T) {
	for _, c := range tlsDTLSControls(t).Owned {
		t.Run(c.ID, func(t *testing.T) {
			w := wrapperWire(t, c.Wire)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(w)))
			for _, deferred := range []bool{false, true} {
				for _, chunk := range []int{1, 7, 4096} {
					if c.Transport == "udp" && chunk != 4096 {
						continue
					}
					t.Run(fmt.Sprintf("df%t/chunk%d", deferred, chunk), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(c.Transport), WithSessionPorts(40000, 443))
						require.NoError(t, err)
						s.(*captureSession).f.a.config.Deferred = deferred
						var events []*ProtocolEvent
						for at := 0; at < len(w); at += chunk {
							input := bytes.Clone(w[at:min(len(w), at+chunk)])
							out := s.Feed(0, time.Unix(1, 0), input)
							clear(input)
							if out.Err != nil {
								require.Contains(t, []ProtocolErrorKind{ErrNeedMore, ErrMalformedMessage}, out.Err.Kind)
							}
							events = append(events, out.Events...)
						}
						require.Len(t, events, 1)
						e := events[0]
						protocol := "tls"
						if c.Transport == "udp" {
							protocol = "dtls"
						}
						require.Equal(t, protocol, e.Protocol)
						require.Equal(t, c.Transport, e.Transport)
						require.Equal(t, w, e.Raw)
						require.Len(t, e.Raw, e.Length)
						require.Equal(t, 0, e.Direction)
						require.Equal(t, "192.0.2.1:40000", e.Source)
						require.Equal(t, "192.0.2.2:443", e.Destination)
						require.NotZero(t, e.FlowID)
						require.NotZero(t, e.ID)
						require.Zero(t, e.TransactionID)
						require.Zero(t, e.ResponseTo)
						if c.Error == "" {
							status := "decoded"
							if deferred {
								status = "deferred"
							}
							require.Equal(t, status, e.Status)
							require.Empty(t, e.Error)
							rocEqualFields(t, c.Fields, e.Session)
						} else {
							require.Equal(t, "malformed", e.Status)
							require.NotEmpty(t, e.Error)
						}
						tlsDTLSQuery(t, e, c.Fields, c.Error)
						for _, alias := range []string{"caller", "redis", "tls", "dtls"} {
							tlsDTLSPoison(e, alias)
							tlsDTLSQuery(t, e, c.Fields, c.Error)
						}
						if c.Error == "" {
							next := s.Feed(1, time.Unix(2, 0), bytes.Clone(w))
							require.Nil(t, next.Err)
							require.Len(t, next.Events, 1)
							n := next.Events[0]
							require.Equal(t, 1, n.Direction)
							require.Equal(t, e.FlowID, n.FlowID)
							require.NotEqual(t, e.ID, n.ID)
							require.Zero(t, n.ResponseTo)
							require.Zero(t, n.TransactionID)
							require.Equal(t, w, n.Raw)
							rocEqualFields(t, c.Fields, n.Session)
							tlsDTLSQuery(t, n, c.Fields, "")
							tlsDTLSQuery(t, e, c.Fields, "")
						}
						closed := s.Close("end")
						if c.PartialAlert {
							require.Len(t, closed, 2)
							for dir, x := range closed {
								require.Equal(t, dir, x.Direction)
								require.Equal(t, "incomplete", x.Status)
								require.Equal(t, "incomplete", x.Completeness)
								require.Nil(t, x.Raw)
								require.Zero(t, x.Length)
								require.Equal(t, uint64(len(w)), x.Offset)
								require.Equal(t, e.FlowID, x.FlowID)
								require.Zero(t, x.TransactionID)
								require.Zero(t, x.ResponseTo)
								require.True(t, time.Unix(int64(dir+1), 0).Equal(x.Timestamp))
								require.Equal(t, map[string]any{"Alert Buffered Bytes": 1}, x.Session)
								tlsDTLSQuery(t, x, nil, "NeedMore")
								tlsDTLSPoison(x, "radius")
								tlsDTLSQuery(t, x, nil, "NeedMore")
							}
							require.Equal(t, uint64(2), s.Stats().Incomplete)
						} else {
							require.Empty(t, closed)
						}
						require.Empty(t, s.Close("again"))
						require.Zero(t, s.Stats().BufferedBytes)
					})
				}
			}
		})
	}
	// Publicly constructed events retain the documented legacy caller cache.
	for _, protocol := range []string{"tls", "dtls"} {
		t.Run("legacy-"+protocol, func(t *testing.T) {
			e := &ProtocolEvent{Protocol: protocol, Fields: map[string]any{"legacy": true}}
			got, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, map[string]any{"legacy": true}, got)
		})
	}
}
func TestDTLSOwnedConversationClockMatrix(t *testing.T) {
	for _, c := range tlsDTLSControls(t).Clock {
		t.Run(c.ID, func(t *testing.T) {
			raw := finsReadBatch(t, "tls-dtls-context", "TLS_DTLS_CONTEXT_TEST_ROOT", c.Capture)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := discoveryReplay(t, raw, workers, deferred, observe)
				require.Len(t, events, len(c.Events))
				ids := map[uint64]bool{}
				flows := map[int]uint64{}
				for i, w := range c.Events {
					e := events[i]
					require.Equal(t, "dtls", e.Protocol)
					require.Equal(t, "dtls-legacy-udp", e.Profile)
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, "datagram", e.Completeness)
					status := "decoded"
					if deferred {
						status = "deferred"
					}
					require.Equal(t, status, e.Status)
					require.Empty(t, e.Error)
					require.NotZero(t, e.ID)
					require.False(t, ids[e.ID])
					ids[e.ID] = true
					require.NotZero(t, e.FlowID)
					if f, ok := flows[w.Owner]; ok {
						require.Equal(t, f, e.FlowID)
					} else {
						for _, old := range flows {
							require.NotEqual(t, old, e.FlowID)
						}
						flows[w.Owner] = e.FlowID
					}
					require.Equal(t, events[w.Owner].FlowID, e.FlowID)
					require.Equal(t, 0, e.Direction)
					require.Zero(t, e.TransactionID)
					require.Zero(t, e.ResponseTo)
					require.Equal(t, CaptureDomain{Interface: w.Interface}, e.Domain)
					require.Equal(t, fmt.Sprintf("192.0.2.1:%d", w.Port), e.Source)
					require.Equal(t, "192.0.2.2:4444", e.Destination)
					require.True(t, time.Unix(w.Timestamp, 0).Equal(e.Timestamp))
					require.Equal(t, wrapperWire(t, w.Wire), e.Raw)
					require.Len(t, e.Raw, e.Length)
					require.Equal(t, []PacketReference{{Number: uint64(w.Packet), Domain: e.Domain}}, e.SourceBytes.PacketRefs)
					rocEqualFields(t, w.Fields, e.Session)
					tlsDTLSQuery(t, e, w.Fields, "")
					tlsDTLSPoison(e, "redis")
					tlsDTLSQuery(t, e, w.Fields, "")
				}
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.Malformed)
			})
		})
	}
}

func TestTLSAndDTLSOwnedCaptureMatrix(t *testing.T) {
	for _, c := range tlsDTLSControls(t).Owned {
		t.Run(c.ID, func(t *testing.T) {
			w := wrapperWire(t, c.Wire)
			var raw []byte
			if c.Transport == "tcp" {
				raw = sessionTestPCAP(t, []sessionStep{{dir: 0, wire: w}}, 443, 1, false, true)
			} else {
				raw = t20UDPCapture(t, net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2"), 40000, 443, w)
			}
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				events, stats := discoveryReplay(t, raw, workers, deferred, observe)
				expected := 1
				if c.PartialAlert {
					expected = 2
				}
				require.Len(t, events, expected)
				if c.PartialAlert {
					x := events[1]
					require.Equal(t, "tls", x.Protocol)
					require.Equal(t, "tcp", x.Transport)
					require.Equal(t, 0, x.Direction)
					require.Equal(t, "incomplete", x.Status)
					require.Equal(t, "incomplete", x.Completeness)
					require.Equal(t, events[0].FlowID, x.FlowID)
					require.NotEqual(t, events[0].ID, x.ID)
					require.Equal(t, uint64(len(w)), x.Offset)
					require.Nil(t, x.Raw)
					require.Zero(t, x.Length)
					require.Zero(t, x.ResponseTo)
					require.Zero(t, x.TransactionID)
					// The archived capture helper sends one octet per packet after
					// three handshake packets. Close uses the last directional
					// input time; the record event uses its first octet time.
					require.True(t, time.Unix(1700000000, int64(len(w)+2)*1234).Equal(x.Timestamp))
					require.Equal(t, map[string]any{"Alert Buffered Bytes": 1}, x.Session)
					tlsDTLSQuery(t, x, nil, "NeedMore")
					tlsDTLSPoison(x, "radius")
					tlsDTLSQuery(t, x, nil, "NeedMore")
					require.Equal(t, uint64(1), stats.Incomplete)
				}
				e := events[0]
				protocol := "tls"
				src := "192.0.2.1:12345"
				if c.Transport == "udp" {
					protocol = "dtls"
					src = "192.0.2.1:40000"
				}
				require.Equal(t, protocol, e.Protocol)
				require.Equal(t, c.Transport, e.Transport)
				require.Equal(t, src, e.Source)
				require.Equal(t, "192.0.2.2:443", e.Destination)
				require.Equal(t, 0, e.Direction)
				require.Equal(t, CaptureDomain{}, e.Domain)
				require.NotZero(t, e.ID)
				require.NotZero(t, e.FlowID)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				require.Equal(t, w, e.Raw)
				require.Len(t, e.Raw, e.Length)
				require.NotEmpty(t, e.SourceBytes.PacketRefs)
				if c.Error == "" {
					status := "decoded"
					if deferred {
						status = "deferred"
					}
					require.Equal(t, status, e.Status)
					rocEqualFields(t, c.Fields, e.Session)
				} else {
					require.Equal(t, "malformed", e.Status)
					require.NotEmpty(t, e.Error)
				}
				tlsDTLSQuery(t, e, c.Fields, c.Error)
				for _, alias := range []string{"caller", "redis", "tls", "dtls"} {
					tlsDTLSPoison(e, alias)
					tlsDTLSQuery(t, e, c.Fields, c.Error)
				}
				require.Zero(t, stats.BufferedBytes)
			})
		})
	}
}

// Reuse the sealed independently generated TLS1.2/TLS1.3+HTTP2/gRPC capture
// and synthetic key log. This is a provenance/ownership invariant; the native
// codec's separate byte/field oracles remain required, not replaced by it.
func TestTLSOwnedDecryptedEvidence(t *testing.T) {
	var inputs []struct {
		File, SHA256 string
		Bytes        int
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "tls-dtls-context", "TLS_DTLS_CONTEXT_TEST_ROOT", "tls-parent-reused.json"), &inputs))
	require.Len(t, inputs, 2)
	var capture, keylog []byte
	for _, v := range inputs {
		b, err := trafficfixture.ReadFile(strings.TrimPrefix(v.File, "common/pcapx/pcaputil/"))
		require.NoError(t, err)
		require.Len(t, b, v.Bytes)
		require.Equal(t, v.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
		if strings.HasSuffix(v.File, ".keys") {
			keylog = b
		} else {
			capture = b
		}
	}
	keys, err := ParseTLSKeyLog(string(keylog))
	require.NoError(t, err)
	type parentEvidence struct {
		flow                uint64
		ts                  time.Time
		direction           int
		source, destination string
		domain              CaptureDomain
		refs                []PacketReference
	}
	discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
		var canonical []string
		for _, edit := range []bool{false, true} {
			name := "normal"
			if edit {
				name = "edited-parent"
			}
			t.Run(name, func(t *testing.T) {
				parents := map[uint64]parentEvidence{}
				ids := map[uint64]bool{}
				var children []*ProtocolEvent
				var mu sync.Mutex
				var stats ProtocolStats
				var observed atomic.Int64
				opts := []CaptureOption{WithTLSSecrets(keys), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnProtocolMessage(func(e *ProtocolEvent) {
					mu.Lock()
					defer mu.Unlock()
					require.NotZero(t, e.ID)
					require.False(t, ids[e.ID])
					ids[e.ID] = true
					if e.Protocol == "tls" {
						parents[e.ID] = parentEvidence{e.FlowID, e.Timestamp, e.Direction, e.Source, e.Destination, e.Domain, append([]PacketReference(nil), e.SourceBytes.PacketRefs...)}
						if edit {
							tlsDTLSPoison(e, "redis")
							e.ID, e.FlowID = 999999, 999999
							e.Direction = 1 - e.Direction
							e.Timestamp = time.Unix(9, 0)
							e.Source, e.Destination = "caller", "caller"
							e.Domain = CaptureDomain{Interface: 999999}
							for i := range e.SourceBytes.PacketRefs {
								e.SourceBytes.PacketRefs[i].Number = 999999
								e.SourceBytes.PacketRefs[i].Domain = e.Domain
							}
						}
					} else if e.SourceBytes.Kind == "decrypted" {
						children = append(children, e)
					}
				})}
				if observe {
					opts = append(opts, WithEveryPacket(func(gopacket.Packet) { observed.Add(1) }))
				}
				require.NoError(t, ReplayPcap(bytes.NewReader(capture), opts...))
				require.Zero(t, stats.BufferedBytes)
				require.Zero(t, stats.CallbackPanics)
				if observe {
					require.Positive(t, observed.Load())
				}
				require.NotEmpty(t, children)
				messages := 0
				var projection []string
				ordinals := map[string]int{}
				for _, e := range children {
					p, ok := parents[e.SourceBytes.ParentPDU]
					require.True(t, ok, "child keeps actual emitted TLS parent ID before caller edits")
					require.Equal(t, p.flow, e.FlowID)
					require.Equal(t, p.direction, e.Direction)
					require.Equal(t, p.source, e.Source)
					require.Equal(t, p.destination, e.Destination)
					require.Equal(t, p.domain, e.Domain)
					require.True(t, p.ts.Equal(e.Timestamp))
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
					require.Empty(t, e.Error)
					require.Len(t, e.Raw, e.Length)
					require.NotEmpty(t, e.SourceBytes.ParentPDUs)
					expectedRefs := map[PacketReference]bool{}
					for _, id := range e.SourceBytes.ParentPDUs {
						q, ok := parents[id]
						require.True(t, ok)
						require.Equal(t, p.flow, q.flow)
						require.Equal(t, p.direction, q.direction)
						for _, r := range q.refs {
							expectedRefs[r] = true
						}
					}
					gotRefs := map[PacketReference]bool{}
					for _, r := range e.SourceBytes.PacketRefs {
						gotRefs[r] = true
					}
					require.Equal(t, expectedRefs, gotRefs)
					fields, err := e.GetFields()
					require.NoError(t, err)
					decoded, err := e.Decode()
					require.NoError(t, err)
					rocEqualFields(t, fields, decoded["fields"].(map[string]any))
					b, err := json.Marshal(struct {
						Protocol, Source, Destination string
						Direction                     int
						Domain                        CaptureDomain
						Raw                           []byte
						Fields, Session               map[string]any
					}{e.Protocol, e.Source, e.Destination, e.Direction, e.Domain, e.Raw, fields, e.Session})
					require.NoError(t, err)
					key := fmt.Sprintf("%s>%s/%v", e.Source, e.Destination, e.Domain)
					ordinals[key]++
					projection = append(projection, fmt.Sprintf("%s/%09d:%s", key, ordinals[key], b))
					if rows, ok := e.Session["GRPC Messages"].([]map[string]any); ok {
						for _, m := range rows {
							payload := m["Payload"].([]byte)
							require.Len(t, payload, 5)
							require.Equal(t, []byte{10, 3, 'm', '1'}, payload[:4])
							require.Contains(t, []byte{'0', '1', '2'}, payload[4])
							messages++
						}
					}
				}
				require.Equal(t, 12, messages)
				sort.Strings(projection)
				if edit {
					require.Equal(t, canonical, projection, "complete message, field, Session and per-direction order content must survive parent edits")
				} else {
					canonical = projection
				}
			})
		}
	})
}
