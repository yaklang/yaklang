package pcaputil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func testTFTPGenerationMatrix(t *testing.T) {
	const folder = "tftp-generations/"
	b, err := trafficfixture.ReadFile(folder + "controls.json")
	require.NoError(t, err)
	var doc struct {
		Schema string
		Cases  []struct {
			ID, Capture, Answer, SHA256 string
			AnswerSHA                   string `json:"answer_sha256"`
			InputAlias                  string `json:"input_alias"`
			Packets                     int
			Events                      []struct {
				Raw, Error, Source, Destination string
				Direction                       int
				Generation                      int
				Timestamp                       int64 `json:"timestamp_sec"`
				Fields, Session                 map[string]any
				Reply                           uint64   `json:"response_to"`
				Transaction                     uint64   `json:"transaction_id"`
				Refs                            []uint64 `json:"packet_refs"`
			}
		}
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, "owned-tftp-generations/v1", doc.Schema)
	require.Len(t, doc.Cases, 13)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range doc.Cases {
		t.Run(c.ID, func(t *testing.T) {
			answer, err := trafficfixture.ReadFile(folder + c.Answer)
			require.NoError(t, err)
			require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
			var answerDoc map[string]any
			require.NoError(t, json.Unmarshal(answer, &answerDoc))
			encoded, err := json.Marshal(c.Events)
			require.NoError(t, err)
			encodedAnswer, err := json.Marshal(answerDoc["events"])
			require.NoError(t, err)
			// Answer equality below compares complete JSON contracts after retaining
			// the typed test's source fields (input_wire is checked by the verifier).
			var eventsDoc []map[string]any
			require.NoError(t, json.Unmarshal(encoded, &eventsDoc))
			var answerEvents []map[string]any
			require.NoError(t, json.Unmarshal(encodedAnswer, &answerEvents))
			require.Len(t, answerEvents, len(eventsDoc))
			for i, ev := range eventsDoc {
				for k, v := range ev {
					key := k
					switch k {
					case "Generation":
						key = "generation"
					case "Raw":
						key = "raw"
					case "Error":
						key = "error"
					case "Source":
						key = "source"
					case "Destination":
						key = "destination"
					case "Direction":
						key = "direction"
					case "Fields":
						key = "fields"
					case "Session":
						key = "session"
					}
					require.Equal(t, v, answerEvents[i][key])
				}
			}
			bindings := 0
			for _, batch := range all {
				for _, row := range batch.Cases {
					if row.ID == "tftp-generations/"+c.ID {
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
				events, stats := discoveryReplay(t, raw, workers, deferred, observe)
				require.Len(t, events, len(c.Events))
				ids := map[uint64]bool{}
				flows := map[int]uint64{}
				used := map[uint64]bool{}
				for i, e := range events {
					w := c.Events[i]
					require.NotZero(t, e.ID)
					require.False(t, ids[e.ID])
					ids[e.ID] = true
					require.NotZero(t, e.FlowID)
					if id, ok := flows[w.Generation]; ok {
						require.Equal(t, id, e.FlowID)
					} else {
						require.False(t, used[e.FlowID], "a new observed generation needs a distinct flow")
						flows[w.Generation] = e.FlowID
						used[e.FlowID] = true
					}
					require.Equal(t, events[0].Domain, e.Domain)
					require.Equal(t, "tftp", e.Protocol)
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, w.Source, e.Source)
					require.Equal(t, w.Destination, e.Destination)
					require.Equal(t, w.Direction, e.Direction)
					require.Equal(t, w.Timestamp, e.Timestamp.Unix())
					require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
					require.Equal(t, len(w.Raw)/2, e.Length)
					require.Equal(t, w.Reply, e.ResponseTo)
					require.Equal(t, w.Transaction, e.TransactionID)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.Equal(t, w.Refs[0], e.SourceBytes.PacketRefs[0].Number)
					if w.Error != "" {
						require.NotEmpty(t, e.Error)
						require.Nil(t, e.Session)
						fields, err := e.GetFields()
						rocTypedError(t, w.Error, err)
						require.Nil(t, fields)
						continue
					}
					require.Empty(t, e.Error)
					status := "decoded"
					if deferred {
						status = "deferred"
					}
					require.Equal(t, status, e.Status)
					rocEqualFields(t, w.Session, e.Session)
					fields, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, w.Fields, fields)
					fields["Opcode"] = uint16(9999)
					if opts, ok := fields["Options"].([]map[string]any); ok && len(opts) > 0 {
						opts[0]["Value"] = "forged"
					}
					if opts, ok := fields["Options"].([]any); ok && len(opts) > 0 {
						opts[0].(map[string]any)["Value"] = "forged"
					}
					e.Raw[0] ^= 255
					e.Session["Filename"] = "forged"
					if e.Structured != nil {
						e.Structured["fields"] = map[string]any{"Opcode": uint16(9999)}
					}
					again, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, w.Fields, again)
				}
				require.Zero(t, stats.BufferedBytes)
			})
		})
	}
}

func testTFTPRetiredByteRefusal(t *testing.T) {
	raw, err := trafficfixture.ReadFile("tftp-generations/cases/retired-oversize-data.pcap")
	require.NoError(t, err)
	discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
		es, stats := discoveryReplay(t, raw, workers, deferred, observe, WithProtocolBudget(128, 1<<20))
		require.Len(t, es, 11)
		for i, e := range es {
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
			require.Equal(t, "tftp", e.Protocol)
			if i < 5 {
				require.Equal(t, es[0].FlowID, e.FlowID)
			} else {
				require.Equal(t, es[5].FlowID, e.FlowID)
				require.NotEqual(t, es[0].FlowID, e.FlowID)
			}
			if i == 6 {
				require.Equal(t, "limited", e.Status)
				require.Empty(t, e.Raw)
				require.Equal(t, 517, e.Length)
				require.Nil(t, e.Session)
				fields, err := e.GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, fields)
			} else {
				require.Empty(t, e.Error)
			}
		}
		require.Equal(t, true, es[10].Session["Transfer Complete"])
		require.Equal(t, "second", es[10].Session["Filename"])
		require.Equal(t, "192.0.2.2:50001", es[10].Session["Server Transfer ID"])
		require.GreaterOrEqual(t, stats.LimitedBytes, uint64(517))
		require.Zero(t, stats.BufferedBytes)
	})
}

func testTFTPGenerationResourcesAndDomains(t *testing.T) {
	const client = "192.0.2.1:40000"
	request := []byte("\x00\x01first\x00octet\x00blksize\x001024\x00")
	oack := []byte("\x00\x06blksize\x00512\x00")
	for _, tc := range []struct {
		name             string
		maxBytes         int64
		limit            int
		deniedGeneration int
	}{
		{"delta-reservation", 1000, 4096, 0}, {"byte-refusal-atomic", 750, 4096, 3}, {"history-refusal-atomic", 4096, 1, 3},
		{"fixed-history-cap", 65536, 4096, 130},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewDefaultConfig()
			require.NoError(t, WithBinParserConfig(BinParserConfig{MaxBufferedBytes: int(tc.maxBytes), MaxMessageBytes: 64, OnEvent: func(*ProtocolEvent) {}})(c))
			require.NoError(t, c.prepareBinParser())
			a := c.binParser
			a.budget.MaxCollectionElements = tc.limit
			defer c.finishBinParser()
			feed := func(dir int, port string, w []byte) *ProtocolEvent {
				source, dest := client, "192.0.2.2:"+port
				if dir == 1 {
					source, dest = dest, source
				}
				e := &ProtocolEvent{Source: source, Destination: dest, Transport: "udp", Timestamp: time.Unix(100, 0), Length: len(w)}
				require.True(t, a.decodeTFTPDatagram(e, w))
				return e
			}
			var previous uint64
			generations := 3
			if tc.deniedGeneration > generations {
				generations = tc.deniedGeneration
			}
			for gen := 1; gen <= generations; gen++ {
				before := a.stats().BufferedBytes
				r := feed(0, "69", request)
				if gen == tc.deniedGeneration {
					require.NotEmpty(t, r.Error)
					_, err := r.GetFields()
					rocTypedError(t, "ResourceExceeded", err)
					require.Nil(t, r.Session)
					require.Equal(t, previous, r.FlowID)
					require.Equal(t, before, a.stats().BufferedBytes)
					require.Len(t, a.udpSessions.entries, 1)
					v := a.udpSessions.lru.Front().Value.(*binUDPEntry)
					require.True(t, v.flow.tftp.done)
					require.True(t, v.flow.tftp.ambiguous)
					require.Len(t, v.flow.tftp.retired, gen-2)
					late := feed(1, strconv.Itoa(49998+gen), []byte{0, 4, 0, 1})
					_, err = late.GetFields()
					rocTypedError(t, "ContextRequired", err)
					require.Nil(t, late.Session)
					break
				}
				require.Empty(t, r.Error)
				require.NotEqual(t, previous, r.FlowID)
				previous = r.FlowID
				port := strconv.Itoa(49999 + gen)
				osnap := feed(1, port, oack)
				require.Empty(t, osnap.Error)
				require.Equal(t, "192.0.2.2:"+port, osnap.Session["Server Transfer ID"])
				osnap.Session["Filename"] = "mutated"
				require.Empty(t, feed(0, port, []byte{0, 4, 0, 0}).Error)
				data := feed(1, port, []byte{0, 3, 0, 1, 42})
				require.Empty(t, data.Error)
				require.Equal(t, "first", data.Session["Filename"])
				done := feed(0, port, []byte{0, 4, 0, 1})
				require.Empty(t, done.Error)
				require.Equal(t, true, done.Session["Transfer Complete"])
				require.LessOrEqual(t, a.stats().BufferedBytes, tc.maxBytes)
			}
			require.NoError(t, c.finishBinParser())
			require.Zero(t, a.stats().BufferedBytes)
			require.NoError(t, c.finishBinParser())
			require.Zero(t, a.stats().BufferedBytes)
		})
	}
	for _, other := range []CaptureDomain{{Section: 1}, {Interface: 1}, {Encapsulation: "vxlan:42"}} {
		t.Run(fmt.Sprint(other), func(t *testing.T) {
			c := NewDefaultConfig()
			require.NoError(t, WithBinParserConfig(BinParserConfig{OnEvent: func(*ProtocolEvent) {}})(c))
			require.NoError(t, c.prepareBinParser())
			a := c.binParser
			defer c.finishBinParser()
			feed := func(domain CaptureDomain, dir int, port string, w []byte) *ProtocolEvent {
				source, dest := client, "192.0.2.2:"+port
				if dir == 1 {
					source, dest = dest, source
				}
				e := &ProtocolEvent{Domain: domain, Source: source, Destination: dest, Transport: "udp", Timestamp: time.Unix(100, 0), Length: len(w)}
				require.True(t, a.decodeTFTPDatagram(e, w))
				return e
			}
			first := CaptureDomain{}
			require.Empty(t, feed(first, 0, "69", request).Error)
			require.Empty(t, feed(first, 1, "50000", oack).Error)
			require.Empty(t, feed(first, 0, "50000", []byte{0, 4, 0, 0}).Error)
			require.Empty(t, feed(first, 1, "50000", []byte{0, 3, 0, 1, 42}).Error)
			require.Empty(t, feed(first, 0, "50000", []byte{0, 4, 0, 1}).Error)
			next := feed(first, 0, "69", request)
			bad := feed(first, 1, "50000", oack)
			_, err := bad.GetFields()
			rocTypedError(t, "ContextRequired", err)
			require.Nil(t, bad.Session)
			different := feed(other, 0, "69", request)
			require.NotEqual(t, next.FlowID, different.FlowID)
			fresh := feed(other, 1, "50000", oack)
			require.Empty(t, fresh.Error)
			require.Equal(t, different.FlowID, fresh.FlowID)
			require.Equal(t, other, fresh.Domain)
			require.Len(t, a.udpSessions.entries, 2)
			require.NoError(t, c.finishBinParser())
			require.Zero(t, a.stats().BufferedBytes)
		})
	}
}
