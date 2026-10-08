package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

// Every sealed capture is executed, even when its independent oracle is still
// unmapped. Parity and resource release are compatibility checks; they are not
// a substitute for an independent semantic answer.
func TestFrozenCorpusReplay(t *testing.T) {
	inventories, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	require.NotEmpty(t, inventories)
	require.Len(t, inventories[0].Cases, 694)
	ids := map[string]bool{}
	for _, answers := range inventories {
		for _, c := range answers.Cases {
			require.False(t, ids[c.ID], "duplicate frozen case")
			ids[c.ID] = true
			t.Run(c.ID, func(t *testing.T) {
				raw, err := trafficfixture.ReadFile(filepath.Join("..", "..", "..", c.Input.OriginalPath))
				require.NoError(t, err)
				require.Equal(t, c.Input.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
				var reference []string
				var referenceError string
				for _, deferred := range []bool{false, true} {
					var events []*ProtocolEvent
					var stats ProtocolStats
					err := ReplayPcap(bytes.NewReader(raw), WithTCPReassemblyWorkers(1), WithBinParserConfig(BinParserConfig{Deferred: deferred, MaxBufferedBytes: 32 << 20, MaxMessageBytes: 1 << 20}), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }))
					replayErr := err
					errorText := ""
					if err != nil {
						errorText = err.Error()
					}
					require.Zero(t, stats.BufferedBytes, "parser did not release retained bytes")
					require.Zero(t, stats.CallbackPanics)
					var canonical []string
					counts := map[string]int{}
					byID := map[uint64]*ProtocolEvent{}
					order := map[string]uint64{}
					for _, e := range events {
						require.NotZero(t, e.ID)
						require.NotContains(t, byID, e.ID)
						byID[e.ID] = e
						if e.FlowID != 0 {
							// IDs may be reserved at HTTP headers before a late request body.
							// Check stream byte order within each direction and protocol layer.
							key := fmt.Sprintf("%d/%d/%s/%s", e.FlowID, e.Direction, e.Protocol, e.SourceBytes.Kind)
							require.GreaterOrEqual(t, e.Offset, order[key])
							order[key] = e.Offset
						}
					}
					for _, e := range events {
						status, fields := e.Status, e.Fields
						semanticError, session, expert := e.Error, e.Session, e.ExpertCode
						if status == "decoded" || status == "deferred" {
							fields, err = e.GetFields()
							if e.Status == "decoded" {
								require.NoError(t, err, "eager codec accepted an undecodable message")
							}
							status = "decoded"
							if err != nil {
								var typed *ProtocolError
								status, typed = classifySessionError(err)
								semanticError = err.Error()
								if typed != nil {
									expert = string(typed.Kind)
								}
								partial, decodeErr := e.Decode()
								require.EqualError(t, decodeErr, semanticError)
								fields = protocolFields(partial)
							}
						}
						if e.Protocol != "" {
							counts[e.Protocol+":"+status]++
						}
						// Stateless Syslog repeats message fields in eager Session; capture-first
						// mode exposes them through GetFields. Compare the same decoded facts.
						if e.Protocol == "syslog" {
							if session != nil {
								require.Equal(t, fields, session)
							}
							session = fields
						}
						association := ""
						if e.ResponseTo != 0 {
							parent, ok := byID[e.ResponseTo]
							require.True(t, ok)
							require.Equal(t, parent.Domain, e.Domain)
							require.Equal(t, parent.FlowID, e.FlowID)
							association = fmt.Sprintf("%d/%d/%x", parent.Direction, parent.Offset, sha256.Sum256(parent.Raw))
						}
						wire, err := json.Marshal(map[string]any{"protocol": e.Protocol, "status": status, "direction": e.Direction, "offset": e.Offset, "length": e.Length, "domain": e.Domain, "raw_sha256": fmt.Sprintf("%x", sha256.Sum256(e.Raw)), "fields": fields, "session": session, "error": semanticError, "expert": expert, "response_to": association})
						require.NoError(t, err)
						canonical = append(canonical, string(wire))
					}
					for _, answer := range c.Expectations {
						if answer.Kind == "whole_capture_event_counts" {
							require.NoError(t, replayErr)
							require.Equal(t, answer.Expected.EventCounts, counts, "sealed whole-capture event counts")
						}
					}
					sort.Strings(canonical)
					if !deferred {
						reference = canonical
						referenceError = errorText
					} else {
						require.Equal(t, referenceError, errorText, "capture-reader error parity")
						require.Equal(t, len(reference), len(canonical), "complete message count parity")
						for i := range reference {
							require.Equal(t, reference[i], canonical[i], "message/field/association parity at %d", i)
						}
					}
					t.Logf("case=%s input_sha256=%s deferred=%v messages=%d reader_error=%q oracle_scope=%s", c.ID, c.Input.SHA256, deferred, len(events), errorText, c.ExpectedStatus)
				}
			})
		}
	}
}
