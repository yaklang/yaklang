package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func dtlsHelloCarrierTyped(t *testing.T, k string, v any) any {
	t.Helper()
	if k == "Packet Refs" && v == nil {
		return []PacketReference(nil)
	}
	switch x := v.(type) {
	case map[string]any:
		r := map[string]any{}
		for k, v := range x {
			r[k] = dtlsHelloCarrierTyped(t, k, v)
		}
		if _, ok := x["Value"]; ok {
			r["Type"] = uint16(x["Type"].(float64))
		}
		return r
	case []any:
		if k == "Cipher Suites" {
			r := make([]uint16, len(x))
			for i, v := range x {
				r[i] = uint16(v.(float64))
			}
			return r
		}
		r := make([]map[string]any, len(x))
		for i, v := range x {
			r[i] = dtlsHelloCarrierTyped(t, k, v).(map[string]any)
		}
		return r
	case float64:
		switch k {
		case "Content Type", "Type":
			return byte(x)
		case "Version", "Epoch", "Message Seq", "Hello Version", "Negotiated Version":
			return uint16(x)
		case "Sequence":
			return uint64(x)
		}
		return int(x)
	case string:
		if k == "Value" {
			b, e := hex.DecodeString(x)
			require.NoError(t, e)
			return b
		}
		return x
	default:
		return x
	}
}

type tlsHelloDTLSCase struct {
	Name, Error  string
	Wire         string         `json:"input_hex"`
	SHA          string         `json:"input_sha256"`
	ErrorMessage string         `json:"error_message"`
	Fields       map[string]any `json:"fields_hexbytes"`
}

func tlsHelloDTLSControls(t *testing.T) []tlsHelloDTLSCase {
	var x struct{ Cases []tlsHelloDTLSCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "tls-hello-collection", "TLS_HELLO_COLLECTION_TEST_ROOT", "dtls/controls.json"), &x))
	require.Len(t, x.Cases, 2)
	return x.Cases
}
func tlsHelloDTLSCheck(t *testing.T, c tlsHelloDTLSCase, e *ProtocolEvent, d, captured bool) {
	t.Helper()
	raw := wrapperWire(t, c.Wire)
	require.Equal(t, c.SHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Equal(t, "dtls", e.Protocol)
	require.Equal(t, "dtls-legacy-udp", e.Profile)
	require.Equal(t, "udp", e.Transport)
	require.Equal(t, 0, e.Direction)
	require.Zero(t, e.Offset)
	require.Equal(t, raw, e.Raw)
	require.Equal(t, len(raw), e.Length)
	require.NotZero(t, e.ID)
	require.NotZero(t, e.FlowID)
	require.Zero(t, e.TransactionID)
	require.Zero(t, e.ResponseTo)
	require.Equal(t, CaptureDomain{}, e.Domain)
	require.Equal(t, "192.0.2.1:41000", e.Source)
	require.Equal(t, "192.0.2.2:4433", e.Destination)
	ts := time.Unix(100, 0)
	if captured {
		ts = time.Unix(1700000000, 0)
	}
	require.True(t, ts.Equal(e.Timestamp))
	want := dtlsHelloCarrierTyped(t, "", c.Fields).(map[string]any)
	if captured {
		refs := []PacketReference{{Number: 1, Domain: e.Domain}}
		require.Equal(t, refs, e.SourceBytes.PacketRefs)
		require.Equal(t, c.SHA, e.SourceBytes.SHA256)
		if c.Error == "" {
			want["Records"].([]map[string]any)[0]["Handshakes"].([]map[string]any)[0]["Packet Refs"] = refs
		}
	}
	status, complete := "decoded", "datagram"
	if d {
		status = "deferred"
	}
	if c.Error != "" {
		status, complete = "limited", "limited"
	}
	require.Equal(t, status, e.Status)
	require.Equal(t, complete, e.Completeness)
	require.Equal(t, want, e.Session)
	tlsCertificateBudgetQuery(t, e, want, c.Error)
	if c.Error != "" {
		_, err := e.GetFields()
		var pe *ProtocolError
		require.ErrorAs(t, err, &pe)
		require.Equal(t, c.ErrorMessage, pe.Message)
		require.Equal(t, "ResourceExceeded: "+c.ErrorMessage, e.Error)
	} else {
		require.Empty(t, e.Error)
	}
	radiusEAPMutateStringLists(e.Session)
	mutateRTPSQuery(e.Session)
	tlsDTLSPoison(e, "radius")
	e.TransactionID, e.ResponseTo = 999999, 999999
	tlsCertificateBudgetQuery(t, e, want, c.Error)
}
func tlsHelloDTLSStats(t *testing.T, c tlsHelloDTLSCase, s ProtocolStats) {
	t.Helper()
	n := uint64(len(wrapperWire(t, c.Wire)))
	lim := uint64(0)
	if c.Error != "" {
		lim = n
	}
	tlsAlertCheckStats(t, s, map[string]uint64{"Messages": 1, "InputBytes": n, "MessageBytes": n, "UnclassifiedBytes": 0, "LimitedBytes": lim, "Incomplete": 0, "Malformed": 0})
}
func TestTLSHelloCollectionDTLSNative(t *testing.T) {
	for _, c := range tlsHelloDTLSControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			for _, d := range []bool{false, true} {
				t.Run(fmt.Sprintf("d%t", d), func(t *testing.T) {
					b := DefaultParserBudget()
					b.MaxCollectionElements = 1
					s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(41000, 4433))
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = d
					input := wrapperWire(t, c.Wire)
					n := len(input)
					o := s.Feed(0, time.Unix(100, 0), input)
					clear(input)
					require.Equal(t, n, o.Consumed)
					require.False(t, o.NeedMore)
					if c.Error == "" {
						require.Nil(t, o.Err)
					} else {
						rocTypedError(t, c.Error, o.Err)
					}
					if o.Err != nil {
						require.Equal(t, c.ErrorMessage, o.Err.Message)
					}
					require.Len(t, o.Events, 1)
					tlsHelloDTLSCheck(t, c, o.Events[0], d, false)
					require.Empty(t, s.Close("done"))
					require.Empty(t, s.Close("again"))
					tlsHelloDTLSStats(t, c, s.Stats())
				})
			}
		})
	}
}
func TestTLSHelloCollectionDTLSMatrix(t *testing.T) {
	for _, c := range tlsHelloDTLSControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			generated := t20UDPCapture(t, net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2"), 41000, 4433, wrapperWire(t, c.Wire))
			if dir := os.Getenv("TLS_HELLO_COLLECTION_EXPORT"); dir != "" {
				p := filepath.Join(dir, "tls-hello-collection", "dtls", c.Name+".pcap")
				require.NoError(t, os.MkdirAll(filepath.Dir(p), 0700))
				require.NoError(t, os.WriteFile(p, generated, 0600))
			}
			raw := generated
			if os.Getenv("TLS_HELLO_COLLECTION_TEST_ROOT") == "" {
				raw = finsReadBatch(t, "tls-hello-collection", "TLS_HELLO_COLLECTION_TEST_ROOT", "dtls/"+c.Name+".pcap")
				require.Equal(t, generated, raw)
			}
			discoveryMatrix(t, func(t *testing.T, workers int, d, o bool) {
				var mu sync.Mutex
				var events []*ProtocolEvent
				var stats ProtocolStats
				var packets atomic.Int64
				opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(d), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnProtocolMessage(func(e *ProtocolEvent) {
					mu.Lock()
					defer mu.Unlock()
					v := *e
					v.Raw = bytes.Clone(e.Raw)
					v.Session = cloneSession(e.Session)
					v.Fields = cloneSession(e.Fields)
					v.Structured = cloneSession(e.Structured)
					v.SourceBytes.PacketRefs = append([]PacketReference(nil), e.SourceBytes.PacketRefs...)
					events = append(events, &v)
					mutateRTPSQuery(e.Session)
					tlsDTLSPoison(e, "radius")
				})}
				if o {
					opts = append(opts, WithEveryPacket(func(gopacket.Packet) { packets.Add(1) }))
				}
				a, err := NewPacketAnalyzer(opts...)
				require.NoError(t, err)
				a.conf.binParser.budget.MaxCollectionElements = 1
				reader, err := NewCaptureReader(bytes.NewReader(raw))
				require.NoError(t, err)
				for {
					packet, ci, e := reader.ReadPacketData()
					if e == io.EOF {
						break
					}
					require.NoError(t, e)
					require.NoError(t, a.Feed(packet, ci, reader.LinkType()))
					clear(packet)
				}
				require.NoError(t, a.Close())
				require.NoError(t, a.Close())
				require.Len(t, events, 1)
				tlsHelloDTLSCheck(t, c, events[0], d, true)
				tlsHelloDTLSStats(t, c, stats)
				if o {
					require.Equal(t, int64(1), packets.Load())
				} else {
					require.Zero(t, packets.Load())
				}
			})
		})
	}
}
