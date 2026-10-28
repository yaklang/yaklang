package pcaputil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func testTFTPNegotiationMatrix(t *testing.T) {
	const folder = "tftp-negotiation/"
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
				Timestamp                       int64 `json:"timestamp_sec"`
				Fields, Session                 map[string]any
				Reply                           uint64   `json:"response_to"`
				Transaction                     uint64   `json:"transaction_id"`
				Refs                            []uint64 `json:"packet_refs"`
			}
		}
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	require.Equal(t, "owned-tftp-negotiation/v1", doc.Schema)
	require.Len(t, doc.Cases, 16)
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
					if row.ID == "tftp-negotiation/"+c.ID {
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
				for i, e := range events {
					w := c.Events[i]
					require.NotZero(t, e.ID)
					require.False(t, ids[e.ID])
					ids[e.ID] = true
					require.NotZero(t, e.FlowID)
					require.Equal(t, events[0].FlowID, e.FlowID)
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

func testTFTPSealedBudgetContext(t *testing.T) {
	// Reuse the published minimum length-boundary capture: the sole change is
	// the explicit caller byte budget. No second copy of its PCAP is stored.
	raw, err := trafficfixture.ReadFile("tftp-negotiation/cases/rrq-size-boundary.pcap")
	require.NoError(t, err)
	discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
		events, stats := discoveryReplay(t, raw, workers, deferred, observe, WithProtocolBudget(128, 1<<20))
		require.Len(t, events, 6)
		for i, e := range events {
			require.Equal(t, events[0].FlowID, e.FlowID)
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
			if i < 3 {
				require.Empty(t, e.Error)
				continue
			}
			kind := "ContextRequired"
			status := "context-required"
			if i == 3 {
				kind, status = "ResourceExceeded", "limited"
				require.Empty(t, e.Raw)
			}
			require.Equal(t, status, e.Status)
			require.Nil(t, e.Session)
			fields, err := e.GetFields()
			rocTypedError(t, kind, err)
			require.Nil(t, fields)
		}
		require.Zero(t, stats.BufferedBytes)
	})
}
