package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type dlmsListControl struct {
	ID, Capture, Answer, SHA256, Transport string
	AnswerSHA                              string `json:"answer_sha256"`
	InputAlias                             string `json:"input_alias"`
	Packets                                int
	MaxMessageBytes                        int `json:"max_message_bytes"`
	Events                                 []dlmsListAnswer
}
type dlmsListAnswer struct {
	Raw, Error, Profile string
	Direction           int
	Fields              map[string]any
	Reply               *int  `json:"response_to_index"`
	PacketRefs          []int `json:"packet_refs"`
	OmitRaw             bool  `json:"omit_raw"`
}

func dlmsListControls(t *testing.T) []dlmsListControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("dlms-hdlc-list/controls.json")
	require.NoError(t, err)
	var m struct {
		Schema string
		Cases  []dlmsListControl
	}
	require.NoError(t, json.Unmarshal(b, &m))
	require.Equal(t, "owned-dlms-hdlc-list/v1", m.Schema)
	require.Len(t, m.Cases, 46)
	all, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for _, c := range m.Cases {
		answer, err := trafficfixture.ReadFile("dlms-hdlc-list/" + c.Answer)
		require.NoError(t, err)
		require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
		var a dlmsListControl
		require.NoError(t, json.Unmarshal(answer, &a))
		require.Equal(t, c.Events, a.Events)
		bound := 0
		for _, batch := range all {
			for _, input := range batch.Cases {
				if input.ID != "dlms-hdlc-list/"+c.ID {
					continue
				}
				require.Equal(t, c.SHA256, input.Input.SHA256)
				require.Equal(t, c.InputAlias, input.Input.OriginalPath)
				require.Equal(t, c.Packets, input.Facts.PacketCount)
				for _, expectation := range input.Expectations {
					var bind struct {
						File string `json:"answer_file"`
						SHA  string `json:"answer_sha256"`
					}
					require.NoError(t, json.Unmarshal(expectation.PayloadConstraints, &bind))
					if bind.File == c.Answer {
						require.Equal(t, c.AnswerSHA, bind.SHA)
						bound++
					}
				}
			}
		}
		require.Equal(t, 1, bound, c.ID)
	}
	return m.Cases
}

func dlmsListAssert(t *testing.T, c dlmsListControl, events []*ProtocolEvent) {
	t.Helper()
	require.Len(t, events, len(c.Events))
	ids := map[uint64]bool{}
	for i, e := range events {
		w := c.Events[i]
		require.NotZero(t, e.ID)
		require.False(t, ids[e.ID], "event identities cannot be reused")
		ids[e.ID] = true
		require.Equal(t, "dlms", e.Protocol)
		require.Equal(t, w.Profile, e.Profile)
		require.Equal(t, w.Direction, e.Direction)
		require.Zero(t, e.TransactionID, "preserve native HDLC public contract")
		if w.Reply == nil {
			require.Zero(t, e.ResponseTo)
		} else {
			require.NotZero(t, e.ResponseTo)
			require.Equal(t, events[*w.Reply].ID, e.ResponseTo)
			require.Equal(t, events[*w.Reply].FlowID, e.FlowID)
		}
		if c.Transport == "udp" && w.OmitRaw {
			require.Empty(t, e.Raw)
		} else {
			require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
		}
		f, err := e.GetFields()
		rocTypedError(t, w.Error, err)
		if w.Error != "" {
			require.Nil(t, f)
			require.Nil(t, e.Session)
			require.Nil(t, e.Structured)
			require.NotEmpty(t, e.Error)
			continue
		}
		require.Empty(t, e.Error)
		rocEqualFields(t, w.Fields, f)
		rocEqualFields(t, w.Fields, e.Session)
		v, err := e.Decode()
		require.NoError(t, err)
		rocEqualFields(t, w.Fields, protocolFields(v))
		f["Observation"] = "caller-mutated"
		for _, key := range []string{"Data Value", "Access Parameters"} {
			data, ok := f[key].(map[string]any)
			if !ok {
				continue
			}
			data["raw_hex"] = "caller-mutated"
			for _, key := range []string{"elements", "rows"} {
				if children, ok := data[key].([]map[string]any); ok && len(children) > 0 {
					children[0]["raw_hex"] = "caller-mutated-child"
				}
			}
		}
		if list, ok := f["Get List"].(map[string]any); ok {
			list["list_count"] = -1
			for _, key := range []string{"results", "descriptors"} {
				if items, ok := list[key].([]map[string]any); ok && len(items) > 0 {
					items[0]["raw_hex"] = "caller-mutated"
				}
			}
		}
		if block, ok := f["Get Block"].(map[string]any); ok {
			block["raw_data_hex"] = "caller-mutated"
			if data, ok := block["assembled_data"].(map[string]any); ok {
				data["raw_hex"] = "caller-mutated"
				if children, ok := data["elements"].([]map[string]any); ok && len(children) > 0 {
					children[0]["raw_hex"] = "caller-mutated-child"
				}
			}
			if items, ok := block["assembled_results"].([]map[string]any); ok && len(items) > 0 {
				items[0]["raw_hex"] = "caller-mutated-item"
				if data, ok := items[0]["data"].(map[string]any); ok {
					data["raw_hex"] = "caller-mutated-list-data"
				}
			}
		}
		rocEqualFields(t, w.Fields, e.Session)
		again, err := e.GetFields()
		require.NoError(t, err)
		rocEqualFields(t, w.Fields, again)
		e.Session["Observation"] = "public-session-mutated"
		again, err = e.GetFields()
		require.NoError(t, err)
		rocEqualFields(t, w.Fields, again)
		// Restore only the caller's copy for later checks; internal projections
		// and later messages must never depend on that mutation.
		e.Session["Observation"] = w.Fields["Observation"]
	}
}

func TestDLMSHDLCListSealedMatrix(t *testing.T) { dlmsSealedMatrix(t, dlmsListControls(t)) }
func dlmsSealedMatrix(t *testing.T, controls []dlmsListControl) {
	for _, c := range controls {
		t.Run(c.ID, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile(strings.TrimPrefix(c.InputAlias, "common/pcapx/pcaputil/"))
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				var extra []CaptureOption
				if c.Transport == "udp" {
					extra = append(extra, WithProtocolDecodeAs("udp", 4059, "dlms"))
				}
				if c.MaxMessageBytes != 0 {
					extra = append(extra, WithProtocolBudget(c.MaxMessageBytes, 1<<20))
				}
				events, stats := discoveryReplay(t, raw, workers, deferred, observe, extra...)
				dlmsListAssert(t, c, events)
				require.Zero(t, stats.BufferedBytes)
				for i, e := range events {
					require.Equal(t, c.Transport, e.Transport)
					src, dst := "192.0.2.1:40000", "192.0.2.2:4059"
					if c.Events[i].Direction == 1 {
						src, dst = dst, src
					}
					require.Equal(t, src, e.Source)
					require.Equal(t, dst, e.Destination)
					require.Len(t, e.SourceBytes.PacketRefs, len(c.Events[i].PacketRefs))
					for j, ref := range e.SourceBytes.PacketRefs {
						require.EqualValues(t, c.Events[i].PacketRefs[j], ref.Number)
						require.Equal(t, e.Domain, ref.Domain)
					}
				}
			})
		})
	}
}

func TestDLMSHDLCListOwnershipAndChunks(t *testing.T) { dlmsOwnershipAndChunks(t, dlmsListControls(t)) }
func dlmsOwnershipAndChunks(t *testing.T, controls []dlmsListControl) {
	for _, c := range controls {
		for _, deferred := range []bool{false, true} {
			for _, chunk := range []int{1, 7, 64, 2049} {
				if c.Transport == "udp" && chunk != 2049 {
					continue // A UDP datagram is never assembled from independent calls.
				}
				b := DefaultParserBudget()
				if c.MaxMessageBytes != 0 {
					b.MaxMessageBytes, b.MaxFrameBytes = c.MaxMessageBytes, c.MaxMessageBytes
				}
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(c.Transport), WithSessionPorts(40000, 4059))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				if c.Transport == "udp" {
					s.(*captureSession).f.a.datagramDecodeAs = map[uint16]string{4059: "dlms"}
				}
				var events []*ProtocolEvent
				for _, answer := range c.Events {
					w := wrapperWire(t, answer.Raw)
					for at := 0; at < len(w); at += chunk {
						owned := bytes.Clone(w[at:min(len(w), at+chunk)])
						out := s.Feed(answer.Direction, time.Unix(1, 0), owned)
						clear(owned)
						events = append(events, out.Events...)
					}
				}
				events = append(events, s.Close("list-complete")...)
				dlmsListAssert(t, c, events)
				require.Empty(t, s.Close("idempotent"))
				require.Zero(t, s.Stats().BufferedBytes)
			}
		}
	}
}

func TestDLMSHDLCListProjectionAndProbeLimits(t *testing.T) {
	cs := dlmsListControls(t)
	for _, c := range cs {
		if c.ID != "structured-udp" {
			continue
		}
		for _, limits := range []struct{ nodes, depth int }{{4, 64}, {5, 64}, {4096, 7}, {4096, 8}} {
			b := DefaultParserBudget()
			b.MaxCollectionElements, b.MaxRecursionDepth = limits.nodes, limits.depth
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			q := s.Feed(0, time.Unix(1, 0), wrapperWire(t, c.Events[0].Raw))
			require.Nil(t, q.Err)
			out := s.Feed(1, time.Unix(2, 0), wrapperWire(t, c.Events[1].Raw))
			require.Len(t, out.Events, 1)
			if limits.nodes == 4 || limits.depth == 7 {
				f, err := out.Events[0].GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, f)
				require.Nil(t, out.Events[0].Session)
				require.Zero(t, out.Events[0].ResponseTo)
				require.Zero(t, out.Events[0].TransactionID)
			} else {
				dlmsListAssert(t, c, append(q.Events, out.Events...))
			}
			require.Empty(t, s.Close("Data-budget"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
	}
	for _, c := range cs {
		if c.ID != "compact-udp" {
			continue
		}
		q, r := wrapperWire(t, c.Events[0].Raw), wrapperWire(t, c.Events[1].Raw)
		require.Equal(t, ProbeAccept, probeDLMS(q, 2049).Verdict)
		require.Equal(t, ProbeReject, probeDLMS(r, 2049).Verdict)
		for _, transport := range []string{"tcp", "udp"} {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(transport))
			require.NoError(t, err)
			p := s.Probe(q)
			require.Equal(t, ProbeAccept, p.Verdict)
			require.Equal(t, "hdlc-get-list", p.Version)
			require.Equal(t, ProbeReject, s.Probe(r).Verdict)
			damaged := bytes.Clone(q)
			damaged[len(damaged)-2] ^= 1
			require.Equal(t, ProbeReject, s.Probe(damaged).Verdict)
			require.Empty(t, s.Close("probe"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
		need := int(512 + 2*int64(len(q)) + dlmsProjection(r))
		for _, delta := range []int{-1, 0} {
			b := DefaultParserBudget()
			b.MaxFrameBytes, b.MaxMessageBytes, b.MaxBufferedBytes = 2049, 2049, need+delta
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("udp"), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			request := s.Feed(0, time.Unix(1, 0), q)
			require.Nil(t, request.Err)
			out := s.Feed(1, time.Unix(2, 0), r)
			require.Len(t, out.Events, 1)
			if delta == 0 {
				dlmsListAssert(t, c, append(request.Events, out.Events...))
			} else {
				f, err := out.Events[0].GetFields()
				rocTypedError(t, "ResourceExceeded", err)
				require.Nil(t, f)
				require.Nil(t, out.Events[0].Session)
				require.Zero(t, out.Events[0].ResponseTo)
				late := s.Feed(1, time.Unix(3, 0), r)
				require.Len(t, late.Events, 1)
				_, err = late.Events[0].GetFields()
				rocTypedError(t, "ContextRequired", err)
				require.Zero(t, late.Events[0].ResponseTo)
			}
			require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
			require.Empty(t, s.Close("projection"))
			require.Zero(t, s.Stats().BufferedBytes)
		}
	}
}
