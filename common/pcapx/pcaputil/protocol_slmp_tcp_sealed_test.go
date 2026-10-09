package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"testing"
	"time"
)

type slmpTCPControl struct {
	Name, File, SHA256 string
	Port               uint16
	Packets            int
	Steps              []struct {
		Dir int
		Hex string
	}
	Messages []struct {
		Raw         string `json:"raw_hex"`
		Fields      map[string]any
		Error       *string
		Refs        []int `json:"packet_refs"`
		Reply       *int  `json:"response_to_event_index"`
		Transaction *int  `json:"transaction_to_event_index"`
	} `json:"expected_messages"`
	Frames []struct {
		Raw  string `json:"raw_hex"`
		Dir  int
		Refs []int `json:"packet_refs"`
	} `json:"framing_frames"`
	Terminal *string `json:"expected_terminal_error"`
	Options  *struct {
		Frame   int `json:"MaxFrameBytes"`
		Message int `json:"MaxMessageBytes"`
	} `json:"session_options"`
	Unmatched int `json:"expected_unmatched_requests"`
	Partial   int `json:"expected_partial_close"`
}

func slmpTCPControls(t *testing.T) []slmpTCPControl {
	t.Helper()
	raw, err := trafficfixture.ReadFile("slmp-tcp/controls.json")
	require.NoError(t, err)
	var d struct{ Cases []slmpTCPControl }
	var rows struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.NoError(t, json.Unmarshal(raw, &rows))
	require.Len(t, d.Cases, 48)
	inventories, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for i, c := range d.Cases {
		a, err := trafficfixture.ReadFile("slmp-tcp/answers/" + c.Name + ".json")
		require.NoError(t, err)
		require.JSONEq(t, string(rows.Cases[i]), string(a))
		bound := 0
		for _, v := range inventories {
			for _, x := range v.Cases {
				if x.Input.OriginalPath != "common/pcapx/pcaputil/slmp-tcp/"+c.File {
					continue
				}
				require.Equal(t, c.SHA256, x.Input.SHA256)
				for _, y := range x.Expectations {
					var b struct {
						Answer  string `json:"answer_file"`
						SHA     string `json:"answer_sha256"`
						Packets int    `json:"packet_count"`
					}
					require.NoError(t, json.Unmarshal(y.PayloadConstraints, &b))
					if b.Answer == "answers/"+c.Name+".json" {
						bound++
						require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(a)), b.SHA)
						require.Equal(t, c.Packets, b.Packets)
					}
				}
			}
		}
		require.Equal(t, 1, bound, c.Name)
	}
	return d.Cases
}
func slmpTCPCapture(t *testing.T, c slmpTCPControl) []byte {
	t.Helper()
	w, err := trafficfixture.ReadFile("slmp-tcp/" + c.File)
	require.NoError(t, err)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(w)))
	return w
}
func slmpTCPBudget(c slmpTCPControl) ParserBudget {
	b := DefaultParserBudget()
	if c.Options != nil {
		b.MaxFrameBytes = c.Options.Frame
		b.MaxMessageBytes = max(64, c.Options.Message)
	}
	return b
}
func slmpTCPAssertMessages(t *testing.T, c slmpTCPControl, events []*ProtocolEvent, packets bool, refusedPrefix int) {
	t.Helper()
	var msgs, diagnostics []*ProtocolEvent
	for _, e := range events {
		require.Equal(t, "slmp", e.Protocol)
		require.Equal(t, slmpTCPProfile, e.Profile)
		if e.Status == "incomplete" {
			diagnostics = append(diagnostics, e)
		} else {
			msgs = append(msgs, e)
		}
	}
	if c.Options != nil && (c.Options.Frame == 26 || c.Options.Frame == 64) {
		// Frame rejection is terminal for both directions. The original answer
		// explicitly permits no late event after terminal invalidation.
		require.Len(t, msgs, 2)
		f, err := msgs[0].GetFields()
		require.NoError(t, err)
		rocEqualFields(t, c.Messages[0].Fields, f)
		_, err = msgs[1].GetFields()
		rocTypedError(t, "ResourceExceeded", err)
		require.Equal(t, "limited", msgs[1].Status)
		require.Zero(t, msgs[1].ResponseTo)
		require.Zero(t, msgs[1].TransactionID)
		require.Nil(t, msgs[1].Session)
		require.Empty(t, diagnostics)
		require.Equal(t, wrapperWire(t, c.Messages[0].Raw), msgs[0].Raw)
		require.Equal(t, wrapperWire(t, c.Messages[1].Raw)[:refusedPrefix], msgs[1].Raw)
		if packets {
			for i, e := range msgs {
				require.Len(t, e.SourceBytes.PacketRefs, 1)
				require.EqualValues(t, i+4, e.SourceBytes.PacketRefs[0].Number)
			}
		}
		return
	}
	extra := 0
	if c.Terminal != nil && *c.Terminal != "IncompleteMessage" {
		extra = 1
	}
	require.Len(t, msgs, len(c.Messages)+extra, c.Name)
	for i, want := range c.Messages {
		e := msgs[i]
		require.Equal(t, wrapperWire(t, want.Raw), e.Raw)
		require.Equal(t, len(e.Raw), e.Length)
		if packets {
			var refs []int
			for _, p := range e.SourceBytes.PacketRefs {
				refs = append(refs, int(p.Number))
				require.Equal(t, e.Domain, p.Domain)
			}
			require.Equal(t, want.Refs, refs)
		}
		f, err := e.GetFields()
		if want.Error != nil {
			rocTypedError(t, *want.Error, err)
			require.Nil(t, f)
			require.Nil(t, e.Session)
			require.Zero(t, e.TransactionID)
			require.Zero(t, e.ResponseTo)
			continue
		}
		require.NoError(t, err)
		require.Empty(t, e.Error)
		rocEqualFields(t, want.Fields, f)
		if want.Reply != nil {
			require.Equal(t, msgs[*want.Reply].ID, e.ResponseTo)
			require.Equal(t, e.ResponseTo, e.TransactionID)
		} else {
			require.Zero(t, e.ResponseTo)
			if want.Transaction != nil {
				require.Equal(t, msgs[*want.Transaction].ID, e.TransactionID)
			} else if want.Fields["Association"] == "retransmitted-request" {
				require.Equal(t, msgs[0].ID, e.TransactionID)
			} else {
				require.Equal(t, e.ID, e.TransactionID)
			}
		}
		// All public projections and the byte source are owned snapshots. Editing
		// any of them cannot overwrite the private full/deferred field cache.
		clear(e.Raw)
		e.Session["Role"] = "caller"
		if e.Structured != nil {
			e.Structured["fields"] = map[string]any{"caller": true}
		}
		f["Role"] = "caller"
		again, err := e.GetFields()
		require.NoError(t, err)
		rocEqualFields(t, want.Fields, again)
	}
	if extra != 0 {
		e := msgs[len(c.Messages)]
		_, err := e.GetFields()
		rocTypedError(t, *c.Terminal, err)
		require.Nil(t, e.Session)
		require.Zero(t, e.ResponseTo)
	}
	wantDiagnostics := c.Partial
	if c.Unmatched != 0 {
		wantDiagnostics++
	}
	require.Len(t, diagnostics, wantDiagnostics, c.Name)
	if c.Unmatched != 0 {
		found := 0
		for _, e := range diagnostics {
			if e.Session != nil && e.Session["Outstanding"] != nil {
				require.EqualValues(t, c.Unmatched, e.Session["Outstanding"])
				found++
			}
		}
		require.Equal(t, 1, found)
	}
	if c.Partial != 0 {
		require.Len(t, diagnostics, 1)
		require.Equal(t, wrapperWire(t, c.Steps[0].Hex), diagnostics[0].Raw)
		require.Nil(t, diagnostics[0].Session)
		_, err := diagnostics[0].GetFields()
		require.Error(t, err)
	}
}
func TestSLMPTCPSealedPublicSession(t *testing.T) {
	for _, c := range slmpTCPControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			for _, chunk := range []int{0, 1, 13} {
				for _, client := range []int{0, 1} {
					t.Run(fmt.Sprintf("chunk%d/client%d", chunk, client), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(slmpTCPBudget(c), WithSessionTransport("tcp"), WithSessionPorts(41423, c.Port), WithSessionClientDirection(client))
						require.NoError(t, err)
						var events []*ProtocolEvent
						for i, st := range c.Steps {
							w := wrapperWire(t, st.Hex)
							dir := st.Dir ^ client
							size := len(w)
							if chunk != 0 {
								size = chunk
							}
							for at := 0; at < len(w); at += size {
								part := bytes.Clone(w[at:min(len(w), at+size)])
								out := s.Feed(dir, time.Unix(1700000100+int64(i), 0), part)
								events = append(events, out.Events...)
								clear(part)
							}
						}
						events = append(events, s.Close("independent SLMP TCP controls")...)
						require.Nil(t, s.Close("idempotent"))
						require.Zero(t, s.Stats().BufferedBytes)
						prefix := 0
						if c.Options != nil && (c.Options.Frame == 26 || c.Options.Frame == 64) {
							prefix = min(len(wrapperWire(t, c.Messages[1].Raw)), 64)
							if chunk != 0 {
								prefix = min(prefix, ((13+chunk-1)/chunk)*chunk)
							}
						}
						slmpTCPAssertMessages(t, c, events, false, prefix)
					})
				}
			}
		})
	}
}
func TestSLMPTCPSealedCaptureMatrix(t *testing.T) {
	feedOnly := 0
	for _, c := range slmpTCPControls(t) {
		if c.Options != nil && c.Options.Frame == 26 {
			feedOnly++
			continue
		} // See answer applicability: cap26 is only a public Feed budget; cap64 live control covers the same invariant.
		t.Run(c.Name, func(t *testing.T) {
			raw := slmpTCPCapture(t, c)
			discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
				var extra []CaptureOption
				if c.Options != nil {
					extra = append(extra, WithBinParserConfig(BinParserConfig{MaxMessageBytes: c.Options.Message}))
				}
				events, stats := discoveryReplay(t, raw, w, d, o, extra...)
				require.Zero(t, stats.Unknown)
				prefix := 0
				if c.Options != nil && (c.Options.Frame == 26 || c.Options.Frame == 64) {
					prefix = min(len(wrapperWire(t, c.Messages[1].Raw)), 64)
				}
				slmpTCPAssertMessages(t, c, events, true, prefix)
			})
		})
	}
	require.Equal(t, 1, feedOnly)
}
func TestSLMPTCPSealedBudgetsAndOwnership(t *testing.T) {
	c := slmpTCPControls(t)[0]
	q := wrapperWire(t, c.Steps[0].Hex)
	z := wrapperWire(t, c.Steps[1].Hex)
	// Fixed projection overhead and full fields are included before copying raw,
	// allocating pending state or exposing accepted fields.
	need := int(512 + 4096 + 256*int64(len(q)))
	for _, delta := range []int{-1, 0} {
		t.Run(fmt.Sprintf("shared%d", delta), func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxFrameBytes = 64
			b.MaxMessageBytes = 64
			b.MaxBufferedBytes = need + delta
			s, err := NewProtocolSessionWithOptions(b, WithSessionPorts(41423, 5000))
			require.NoError(t, err)
			out := s.Feed(0, time.Unix(1, 0), q)
			if delta < 0 {
				rocTypedError(t, "ResourceExceeded", out.Err)
				require.Len(t, out.Events, 1)
				require.Equal(t, "limited", out.Events[0].Status)
				require.Nil(t, out.Events[0].Session)
				late := s.Feed(1, time.Unix(2, 0), z)
				require.Empty(t, late.Events)
			} else {
				require.Nil(t, out.Err)
				require.Len(t, out.Events, 1)
				reply := s.Feed(1, time.Unix(2, 0), z)
				require.Nil(t, reply.Err)
				require.Equal(t, out.Events[0].ID, reply.Events[0].ResponseTo)
			}
			s.Close("budget")
			require.Zero(t, s.Stats().BufferedBytes)
			require.LessOrEqual(t, s.Stats().PeakBufferedBytes, int64(b.MaxBufferedBytes))
		})
	}
	for _, b := range []ParserBudget{{MaxCollectionElements: 14}, {MaxRecursionDepth: 1}} {
		s, err := NewProtocolSessionWithOptions(b, WithSessionPorts(41423, 5000))
		require.NoError(t, err)
		out := s.Feed(0, time.Unix(1, 0), q)
		rocTypedError(t, "ResourceExceeded", out.Err)
		require.Nil(t, out.Events[0].Session)
		s.Close("structure budget")
		require.Zero(t, s.Stats().BufferedBytes)
	}
}
