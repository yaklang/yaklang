package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
)

func TestQUICCoalescingExistingAPI(t *testing.T) {
	cli := rfc9001ClientInitial()
	bad := bytes.Clone(cli)
	bad[len(bad)-1] ^= 1
	capture, err := trafficfixture.ReadFile("quic-coalescing/cases/different-CID.pcap")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(capture), 2482)
	wrong := bytes.Clone(capture[1282:2482])
	for _, tc := range []struct {
		name   string
		wires  [][]byte
		errors []string
	}{
		{"adjacent-good", [][]byte{cli, cli}, []string{"", ""}},
		{"bad-first", [][]byte{bad, cli}, []string{"AuthenticationFailed", ""}},
		{"bad-middle", [][]byte{cli, bad, cli}, []string{"", "AuthenticationFailed", ""}},
		{"different-CID", [][]byte{cli, wrong, cli}, []string{"", "MalformedMessage", ""}},
		{"zero-tail", [][]byte{cli, {0, 0, 0}}, []string{"", "MalformedMessage"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := sessionDatagramPCAP(t, []sessionStep{{0, bytes.Join(tc.wires, nil)}}, 14443)
			es, stats := discoveryReplay(t, raw, 1, false, false)
			require.Len(t, es, len(tc.wires), "every structurally bounded packet needs its own observation")
			var flow uint64
			for i, e := range es {
				require.Equal(t, tc.wires[i], e.Raw)
				require.Equal(t, len(tc.wires[i]), e.Length)
				require.Equal(t, uint64(i+1), e.ID)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				if tc.errors[i] != "" {
					require.NotEqual(t, "authenticated-wire-and-observed-CID", e.Admission)
					if tc.name == "different-CID" {
						require.Equal(t, "rejected-coalesced-CID", e.Admission)
						require.Zero(t, e.FlowID)
					}
					_, err := e.GetFields()
					rocTypedError(t, tc.errors[i], err)
					require.Nil(t, e.Session["Frames"])
					continue
				}
				require.Empty(t, e.Error)
				require.Equal(t, "authenticated-wire-and-observed-CID", e.Admission)
				require.Equal(t, true, e.Session["Authentication Verified"])
				require.Equal(t, uint64(2), e.Session["Packet Number"])
				if flow == 0 {
					flow = e.FlowID
					require.NotZero(t, flow)
					require.Nil(t, e.Session["Duplicate"])
				} else {
					require.Equal(t, flow, e.FlowID)
					require.Equal(t, true, e.Session["Duplicate"])
				}
			}
			require.Zero(t, stats.BufferedBytes)
		})
	}
	t.Run("budget-remainder", func(t *testing.T) {
		pressure := append(bytes.Clone(cli), bytes.Repeat([]byte{1}, 200)...)
		binary.BigEndian.PutUint16(pressure[16:18], 0x4000|1382)
		rest := append(pressure, cli...)
		raw := sessionDatagramPCAP(t, []sessionStep{{0, append(bytes.Clone(cli), rest...)}}, 14443)
		es, stats := discoveryReplay(t, raw, 1, false, false, WithProtocolBudget(4096, 48000))
		require.Len(t, es, 2)
		require.Empty(t, es[0].Error)
		_, err := es[1].GetFields()
		rocTypedError(t, "ResourceExceeded", err)
		require.Equal(t, rest, es[1].Raw, "a bounded refusal must preserve the unprocessed remainder")
		require.Nil(t, es[1].Session)
		require.Equal(t, "unparsed-datagram-remainder", es[1].Admission)
		require.Contains(t, es[1].Summary, "authentication not verified")
		require.Zero(t, stats.BufferedBytes)
	})
}

// RFC9000 12.2 boundaries and fixed RFC9001 protection facts. The oracle
// distinguishes a partial encrypted envelope from a complete frame observation.
func testQUICCoalescingMatrix(t *testing.T) {
	const folder = "quic-coalescing/"
	b, err := trafficfixture.ReadFile(folder + "controls.json")
	require.NoError(t, err)
	var doc struct {
		Schema string
		Cases  []struct {
			ID, Capture, SHA256, Answer string
			AnswerSHA                   string `json:"answer_sha256"`
			InputAlias                  string `json:"input_alias"`
			Packets                     int
			Pressure                    bool
			Events                      []struct {
				Raw, Error, Source, Destination string
				Flow, Direction                 int
				TimestampNS                     int64    `json:"timestamp_ns"`
				Reply                           uint64   `json:"response_to"`
				Transaction                     uint64   `json:"transaction_id"`
				Refs                            []uint64 `json:"packet_refs"`
				Fields, Session                 map[string]any
			}
		}
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, "owned-quic-coalescing/v1", doc.Schema)
	require.Len(t, doc.Cases, 10)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range doc.Cases {
		t.Run(c.ID, func(t *testing.T) {
			ab, err := trafficfixture.ReadFile(folder + c.Answer)
			require.NoError(t, err)
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(ab)))
			var answer struct{ Events json.RawMessage }
			require.NoError(t, json.Unmarshal(ab, &answer))
			var root struct{ Cases []map[string]any }
			require.NoError(t, json.Unmarshal(b, &root))
			var answerRows any
			require.NoError(t, json.Unmarshal(answer.Events, &answerRows))
			for _, row := range root.Cases {
				if row["id"] == c.ID {
					require.Equal(t, row["events"], answerRows)
				}
			}
			bindings := 0
			for _, batch := range all {
				for _, row := range batch.Cases {
					if row.ID == "quic-coalescing/"+c.ID {
						bindings++
						require.Equal(t, c.InputAlias, row.Input.OriginalPath)
						require.Equal(t, c.SHA256, row.Input.SHA256)
						require.Equal(t, c.Packets, row.Facts.PacketCount)
					}
				}
			}
			require.Equal(t, 1, bindings)
			raw, err := trafficfixture.ReadFile(folder + c.Capture)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				var opts []CaptureOption
				if c.Pressure {
					opts = append(opts, WithProtocolBudget(4096, 48000))
				}
				es, stats := discoveryReplay(t, raw, workers, deferred, observe, opts...)
				require.Len(t, es, len(c.Events))
				var flow uint64
				for i, e := range es {
					w := c.Events[i]
					require.Equal(t, uint64(i+1), e.ID)
					require.Equal(t, "quic", e.Protocol)
					require.Equal(t, "quic-v1-native", e.Profile)
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, w.Source, e.Source)
					require.Equal(t, w.Destination, e.Destination)
					require.Equal(t, w.Direction, e.Direction)
					require.Equal(t, int64(1_000_000_000)+w.TimestampNS, e.Timestamp.UnixNano())
					require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
					require.Equal(t, len(w.Raw)/2, e.Length)
					require.Equal(t, w.Reply, e.ResponseTo)
					require.Equal(t, w.Transaction, e.TransactionID)
					if w.Flow == 0 {
						require.Zero(t, e.FlowID)
					} else {
						require.NotZero(t, e.FlowID)
						if flow == 0 {
							flow = e.FlowID
						}
						require.Equal(t, flow, e.FlowID)
					}
					require.Equal(t, es[0].Domain, e.Domain)
					require.Equal(t, "captured", e.SourceBytes.Kind)
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(e.Raw)), e.SourceBytes.SHA256)
					require.Len(t, e.SourceBytes.PacketRefs, len(w.Refs))
					for j, ref := range e.SourceBytes.PacketRefs {
						require.Equal(t, w.Refs[j], ref.Number)
						require.Equal(t, e.Domain, ref.Domain)
					}
					rocEqualFields(t, w.Session, e.Session)
					fields, err := e.GetFields()
					if w.Error != "" {
						rocTypedError(t, w.Error, err)
						require.Nil(t, fields)
						require.Equal(t, w.Error, e.ExpertCode)
						status := "malformed"
						if w.Error == "Encrypted" || w.Error == "ContextRequired" || w.Error == "UnsupportedVersion" || w.Error == "ResourceExceeded" {
							status = "context-required"
						}
						require.Equal(t, status, e.Status)
						_, err = e.Decode()
						rocTypedError(t, w.Error, err)
						continue
					}
					require.NoError(t, err)
					require.Empty(t, e.Error)
					status := "decoded"
					if deferred {
						status = "deferred"
					}
					require.Equal(t, status, e.Status)
					rocEqualFields(t, w.Fields, fields)
					fields["First Byte"] = byte(255)
					if v, ok := fields["Protected Payload"].([]any); ok {
						v[0] = byte(255)
					}
					e.Raw[0] ^= 255
					e.Fields = fields
					if e.Structured != nil {
						e.Structured["fields"] = fields
					}
					again, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, w.Fields, again)
				}
				var messageBytes uint64
				for _, w := range c.Events {
					messageBytes += uint64(len(w.Raw) / 2)
				}
				require.Equal(t, messageBytes, stats.MessageBytes, "the complete UDP byte set must remain observable")
				require.EqualValues(t, len(es), stats.Messages)
				require.Zero(t, stats.BufferedBytes)
				limit := int64(128 << 20)
				if c.Pressure {
					limit = 48000
				}
				require.LessOrEqual(t, stats.PeakBufferedBytes, limit)
			})
		})
	}
}

// Reuse the independently protected historical PING/PADDING vector so cap2
// tests outer coalescing rather than the unrelated RFC ClientHello extension list.
func quicCoalescingBudgetInitial(t *testing.T) ([]byte, map[string]any, map[string]any) {
	t.Helper()
	raw, err := trafficfixture.ReadFile("quic-retirement/controls.json")
	require.NoError(t, err)
	var fixtures struct {
		Cases []struct {
			ID     string `json:"id"`
			Events []struct {
				Raw     string
				Session map[string]any
				Fields  map[string]any
			}
		}
	}
	require.NoError(t, json.Unmarshal(raw, &fixtures))
	for _, c := range fixtures.Cases {
		if c.ID != "fresh-CID-after-idle" {
			continue
		}
		for _, e := range c.Events {
			frames, _ := e.Session["Frames"].([]any)
			if len(frames) != 2 || frames[0].(map[string]any)["Frame Type"] != "PING" || e.Session["Duplicate"] == true {
				continue
			}
			wire, err := hex.DecodeString(e.Raw)
			require.NoError(t, err)
			require.Len(t, wire, 1200)
			require.Equal(t, "9fbed178f0b40ec7ad6c3bc36a22639fbfb80db1b021280cbc05d07270cd5a56", fmt.Sprintf("%x", sha256.Sum256(wire)))
			return wire, e.Session, e.Fields
		}
	}
	t.Fatal("missing historical independent PING Initial control")
	return nil, nil, nil
}

func testQUICCoalescingCollectionRefusal(t *testing.T) {
	c := NewDefaultConfig()
	require.NoError(t, WithOnProtocolMessage(func(*ProtocolEvent) {})(c))
	require.NoError(t, c.prepareBinParser())
	a := c.binParser
	a.budget.MaxCollectionElements = 2
	cli, expected, _ := quicCoalescingBudgetInitial(t)
	wrong := rfc9001ClientInitial()
	// CID mismatch cannot turn a collection refusal into a recoverable error.
	wire := bytes.Join([][]byte{cli, cli, wrong, cli}, nil)
	es, ok := a.decodeQUICDatagram(&ProtocolEvent{Transport: "udp", Source: "192.0.2.1:40000", Destination: "192.0.2.2:14443"}, wire)
	require.True(t, ok)
	require.Len(t, es, 3)
	require.Empty(t, es[0].Error)
	require.Empty(t, es[1].Error)
	for i, e := range es[:2] {
		want := cloneSession(expected)
		want["Datagram Offset"] = i * len(cli)
		if i == 1 {
			want["Duplicate"] = true
		}
		require.Equal(t, cli, e.Raw)
		rocEqualFields(t, want, e.Session)
		_, err := e.GetFields()
		require.NoError(t, err)
	}
	fields, err := es[2].GetFields()
	rocTypedError(t, "ResourceExceeded", err)
	require.Nil(t, fields)
	require.Nil(t, es[2].Session)
	require.Equal(t, wire[2400:], es[2].Raw)
	require.Equal(t, es[0].FlowID, es[2].FlowID)
	// No state may be advanced by the rejected remainder, nor re-admitted later.
	later, ok := a.decodeQUICDatagram(&ProtocolEvent{Transport: "udp", Source: "192.0.2.1:40000", Destination: "192.0.2.2:14443"}, cli)
	require.True(t, ok)
	require.Len(t, later, 1)
	fields, err = later[0].GetFields()
	rocTypedError(t, "ContextRequired", err)
	require.Nil(t, fields)
	require.Nil(t, later[0].Session)
	require.Equal(t, es[0].FlowID, later[0].FlowID)
	a.closeUDPSessions()
	require.Zero(t, a.buffered.Load())
	a.closeUDPSessions()
	require.Zero(t, a.buffered.Load())
	require.NoError(t, c.finishBinParser())
}

func testQUICCoalescingCallbackOwnership(t *testing.T) {
	raw, err := trafficfixture.ReadFile("quic-coalescing/cases/malformed-length.pcap")
	require.NoError(t, err)
	b, err := trafficfixture.ReadFile("quic-coalescing/answers/malformed-length.json")
	require.NoError(t, err)
	var want struct {
		Events []struct {
			Raw, Error      string
			Fields, Session map[string]any
		}
	}
	require.NoError(t, json.Unmarshal(b, &want))
	for _, deferred := range []bool{false, true} {
		t.Run(fmt.Sprint(deferred), func(t *testing.T) {
			var stats ProtocolStats
			count := 0
			var flow uint64
			opts := []CaptureOption{WithProtocolDeferred(deferred), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnProtocolMessage(func(e *ProtocolEvent) {
				require.Less(t, count, len(want.Events))
				w := want.Events[count]
				count++
				require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
				rocEqualFields(t, w.Session, e.Session)
				if flow == 0 {
					flow = e.FlowID
				}
				require.Equal(t, flow, e.FlowID)
				fields, err := e.GetFields()
				rocTypedError(t, w.Error, err)
				if w.Error != "" {
					require.Nil(t, fields)
					return
				}
				rocEqualFields(t, w.Fields, fields)
				e.Raw[0] ^= 255
				fields["First Byte"] = byte(255)
				e.Fields = fields
				if e.Structured != nil {
					e.Structured["fields"] = fields
				}
				if cid, ok := e.Session["DCID"].([]byte); ok && len(cid) > 0 {
					cid[0] ^= 255
				}
				e.Session["Packet Number"] = uint64(999999)
				if frames, ok := e.Session["Frames"].([]map[string]any); ok {
					for _, fr := range frames {
						if data, ok := fr["Crypto Data"].([]byte); ok && len(data) > 0 {
							data[0] ^= 255
						}
						fr["Frame Type"] = "edited"
					}
				}
				e.FlowID, e.ResponseTo, e.TransactionID = 999999, 999999, 999999
				again, err := e.GetFields()
				require.NoError(t, err)
				rocEqualFields(t, w.Fields, again)
			})}
			require.NoError(t, ReplayPcap(bytes.NewReader(bytes.Clone(raw)), opts...))
			require.Equal(t, len(want.Events), count)
			require.Zero(t, stats.CallbackPanics)
			require.Zero(t, stats.BufferedBytes)
		})
	}
}
