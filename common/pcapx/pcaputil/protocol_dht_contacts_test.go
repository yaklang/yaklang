package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type dhtContactControl struct {
	ID, Capture, Answer, SHA256, Error, Source, Destination string
	InputAlias                                              string `json:"input_alias"`
	AnswerSHA                                               string `json:"answer_sha256"`
	Wire                                                    string `json:"wire_hex"`
	WireSHA                                                 string `json:"wire_sha256"`
	Packets, Bytes, Direction                               int
	Seconds                                                 int64 `json:"timestamp_seconds"`
	Nanoseconds                                             int64 `json:"timestamp_ns"`
	Fields                                                  map[string]any
}

// Literal independent answers are sealed with the synthetic captures and
// pinned BEP 5/32 derivation. No expected field is generated from Yak output.
func dhtContactControls(t *testing.T) []dhtContactControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("dht-contacts/controls.json")
	require.NoError(t, err)
	var doc struct {
		Schema string
		Cases  []dhtContactControl
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	require.NoError(t, d.Decode(&doc))
	require.Equal(t, "owned-dht-contacts/v1", doc.Schema)
	require.Len(t, doc.Cases, 21)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range doc.Cases {
		b, err := trafficfixture.ReadFile("dht-contacts/" + c.Answer)
		require.NoError(t, err)
		require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(b)))
		var answer dhtContactControl
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		require.NoError(t, d.Decode(&answer))
		answer.AnswerSHA = c.AnswerSHA
		require.Equal(t, c, answer)
		bound := 0
		for _, batch := range all {
			for _, v := range batch.Cases {
				if v.ID != "dht-contacts/"+c.ID {
					continue
				}
				require.Equal(t, c.InputAlias, v.Input.OriginalPath)
				require.Equal(t, c.SHA256, v.Input.SHA256)
				require.Equal(t, c.Packets, v.Facts.PacketCount)
				for _, e := range v.Expectations {
					var ref struct {
						File string `json:"answer_file"`
						SHA  string `json:"answer_sha256"`
					}
					require.NoError(t, json.Unmarshal(e.PayloadConstraints, &ref))
					require.Equal(t, c.Answer, ref.File)
					require.Equal(t, c.AnswerSHA, ref.SHA)
					bound++
				}
			}
		}
		require.Equal(t, 1, bound, c.ID)
	}
	return doc.Cases
}

func dhtContactEqual(t *testing.T, want, got map[string]any) {
	t.Helper()
	normalize := func(v any) any {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		var x any
		require.NoError(t, d.Decode(&x))
		return x
	}
	require.Equal(t, normalize(want), normalize(got))
}

func dhtContactAssert(t *testing.T, c dhtContactControl, e *ProtocolEvent) {
	t.Helper()
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.TransactionID) // A literal KRPC t is not a matched transaction.
	wire, err := hex.DecodeString(c.Wire)
	require.NoError(t, err)
	f, err := e.GetFields()
	if c.Error != "" {
		// Unrecognized capture queries retain the established non-exact-query
		// error contract. Feed's aggregate refusal is typed UnsupportedFeature.
		require.ErrorContains(t, err, "event is unrecognized, not an exact message")
		rocTypedError(t, c.Error, sessionErrorFromEvents([]*ProtocolEvent{e}))
		require.Nil(t, f)
		require.Empty(t, e.Protocol)
		require.Equal(t, "unrecognized", e.Status)
		require.Nil(t, e.Session)
		require.Nil(t, e.Structured)
		require.Equal(t, wire[:min(len(wire), 64)], e.Raw)
		return
	}
	require.NoError(t, err)
	require.Equal(t, "bittorrent-dht", e.Protocol)
	require.Equal(t, "bittorrent-dht-native", e.Profile)
	require.Equal(t, "udp", e.Transport)
	require.Equal(t, "message", e.Completeness)
	require.Empty(t, e.Error)
	require.Equal(t, wire, e.Raw)
	require.Equal(t, len(wire), e.Length)
	dhtContactEqual(t, c.Fields, f)
	dhtContactEqual(t, c.Fields, e.Session)
	tree, err := e.Decode()
	require.NoError(t, err)
	dhtContactEqual(t, c.Fields, protocolFields(tree))
	dhtContactEqual(t, c.Fields, tree["session"].(map[string]any))
	// Mutate every returned nested record and byte slice. Want's scalar slice
	// also needs in-place mutation, rather than just replacing the map member.
	for _, m := range []map[string]any{f, e.Session, protocolFields(tree)} {
		if flags, ok := m["Want"].([]string); ok {
			flags[0] = "caller-mutated"
		}
		mutateRTPSQuery(m)
	}
	mutateRTPSQuery(e.Fields)
	mutateRTPSQuery(e.Structured)
	f, err = e.GetFields()
	require.NoError(t, err)
	dhtContactEqual(t, c.Fields, f)
	e.Session = nil
	tree, err = e.Decode()
	require.NoError(t, err)
	dhtContactEqual(t, c.Fields, protocolFields(tree))
	dhtContactEqual(t, c.Fields, tree["session"].(map[string]any))
	require.Equal(t, wire, e.Raw)
}

func testDHTContactMatrix(t *testing.T) {
	for _, c := range dhtContactControls(t) {
		t.Run("contacts/"+c.ID, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile(strings.TrimPrefix(c.InputAlias, "common/pcapx/pcaputil/"))
			require.NoError(t, err)
			require.Equal(t, c.Bytes, len(raw))
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("workers%d/deferred%t/observe%t", workers, deferred, observe), func(t *testing.T) {
							var seen atomic.Int64
							opts := []CaptureOption{WithProtocolDeferred(deferred)}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen.Add(1) }))
							}
							input := bytes.Clone(raw)
							events, stats, err := binReplay(t, input, workers, opts...)
							require.NoError(t, err)
							clear(input)
							require.Len(t, events, 1)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							if observe {
								require.EqualValues(t, 1, seen.Load())
							} else {
								require.Zero(t, seen.Load())
							}
							e := events[0]
							require.NotZero(t, e.ID)
							require.Zero(t, e.Direction)
							require.Equal(t, c.Source, e.Source)
							require.Equal(t, c.Destination, e.Destination)
							require.True(t, time.Unix(c.Seconds, c.Nanoseconds).Equal(e.Timestamp))
							require.Equal(t, "captured", e.SourceBytes.Kind)
							require.Len(t, e.SourceBytes.PacketRefs, 1)
							require.EqualValues(t, 1, e.SourceBytes.PacketRefs[0].Number)
							require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
							if c.Error == "" {
								require.Equal(t, c.WireSHA, e.SourceBytes.SHA256)
								status := "decoded"
								if deferred {
									status = "deferred"
								}
								require.Equal(t, status, e.Status)
							}
							dhtContactAssert(t, c, e)
						})
					}
				}
			}
		})
	}
}

func testDHTContactBudgets(t *testing.T) {
	controls := dhtContactControls(t)
	ping := []byte("d1:ad2:id20:ABCDEFGHIJKLMNOPQRSTe1:q4:ping1:t2:aa1:y1:qe")
	wantPing := map[string]any{"Transaction ID": []byte("aa"), "Packet Name": "ping", "Node ID": []byte("ABCDEFGHIJKLMNOPQRST")}
	// Eleven bencode values/keys plus two compact contacts. The dual response
	// has thirteen values/keys plus four contacts; charge both address families.
	for _, c := range controls {
		limit := 0
		switch c.ID {
		case "nodes-v4-two", "nodes-v6-two":
			limit = 13
		case "nodes-dual":
			limit = 17
		default:
			continue
		}
		wire, err := hex.DecodeString(c.Wire)
		require.NoError(t, err)
		for _, deferred := range []bool{false, true} {
			for _, budget := range []int{limit - 1, limit} {
				t.Run(fmt.Sprintf("contacts-budget/%s/deferred%t/elements%d", c.ID, deferred, budget), func(t *testing.T) {
					b := DefaultParserBudget()
					b.MaxCollectionElements = budget
					s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(48000, 6881))
					require.NoError(t, err)
					defer s.Close("cleanup")
					s.(*captureSession).f.a.config.Deferred = deferred
					input := bytes.Clone(wire)
					out := s.Feed(1, time.Unix(1, 0), input)
					clear(input)
					require.Equal(t, len(wire), out.Consumed)
					require.False(t, out.NeedMore)
					require.Len(t, out.Events, 1)
					if budget < limit {
						rocTypedError(t, "ResourceExceeded", out.Err)
						e := out.Events[0]
						require.Equal(t, "limited", e.Status)
						require.Nil(t, e.Session)
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						fields, err := e.GetFields()
						require.Nil(t, fields)
						rocTypedError(t, "ResourceExceeded", err)
						assertOwnedProtocolRefusal(t, e, "ResourceExceeded", err)
					} else {
						require.Nil(t, out.Err)
						dhtContactAssert(t, c, out.Events[0])
					}
					require.Zero(t, s.Stats().BufferedBytes)
					// The refused transaction ID aa cannot poison the next ping aa.
					next := s.Feed(0, time.Unix(2, 0), bytes.Clone(ping))
					require.Nil(t, next.Err)
					require.Len(t, next.Events, 1)
					assertOwnedDatagram(t, next.Events[0], "bittorrent-dht", ping, wantPing)
					require.Empty(t, s.Close("complete"))
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
					require.Zero(t, s.Stats().Flows)
				})
			}
		}
	}
	// Every possible split is a different UDP datagram, never a reassembly
	// fragment. Then a whole hybrid response must recover unchanged.
	for _, c := range controls {
		if c.ID != "peers-hybrid" {
			continue
		}
		wire, err := hex.DecodeString(c.Wire)
		require.NoError(t, err)
		for cut := 1; cut < len(wire); cut++ {
			t.Run(fmt.Sprintf("contacts-datagram-cut/%d", cut), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("udp"), WithSessionPorts(48000, 6881))
				require.NoError(t, err)
				defer s.Close("cleanup")
				for _, part := range [][]byte{wire[:cut], wire[cut:]} {
					out := s.Feed(1, time.Unix(1, 0), part)
					require.Equal(t, len(part), out.Consumed)
					require.False(t, out.NeedMore)
					require.Len(t, out.Events, 1)
					for _, e := range out.Events {
						// A suffix can independently be a valid other protocol (the
						// cut at 13 begins with a legal CoAP envelope). Its recognition
						// is not UDP joining: no partial DHT contact or KRPC t may leak.
						require.NotEqual(t, "bittorrent-dht", e.Protocol)
						fields, err := e.GetFields()
						if e.Protocol == "" {
							require.NotNil(t, out.Err)
							rocTypedError(t, "UnsupportedFeature", out.Err)
							require.Error(t, err)
							require.Nil(t, fields)
							require.Nil(t, e.Session)
						} else {
							require.NotContains(t, fields, "Nodes")
							require.NotContains(t, fields, "Nodes6")
							require.NotContains(t, fields, "Peers")
							require.NotContains(t, fields, "Node ID")
						}
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
					}
				}
				require.Zero(t, s.Stats().BufferedBytes)
				input := bytes.Clone(wire)
				out := s.Feed(1, time.Unix(2, 0), input)
				clear(input)
				require.Nil(t, out.Err)
				require.Len(t, out.Events, 1)
				dhtContactAssert(t, c, out.Events[0])
				require.Empty(t, s.Close("complete"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
				require.Zero(t, s.Stats().Flows)
			})
		}
	}
}
