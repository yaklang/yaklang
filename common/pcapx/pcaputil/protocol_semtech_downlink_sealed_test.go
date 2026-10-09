package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type semtechDownlinkControl struct {
	Name, File, SHA256 string
	Port               uint16
	Packets            int
	Steps              []struct {
		Dir          int
		Hex          string
		Source       string `json:"src"`
		Dest         string `json:"dst"`
		Sport, Dport uint16
		CaptureBytes int `json:"capture_bytes"`
	}
	Messages []struct {
		Protocol, Transport string
		Raw                 string `json:"raw_hex"`
		Refs                []int  `json:"packet_refs"`
		Fields              map[string]any
	} `json:"expected_messages"`
	Validation []*struct{ Code, Reason string } `json:"static_packet_validation"`
}

func semtechDownlinkControls(t *testing.T) []semtechDownlinkControl {
	t.Helper()
	raw, err := trafficfixture.ReadFile("semtech-downlink/controls.json")
	require.NoError(t, err)
	var d struct{ Cases []semtechDownlinkControl }
	var rows struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.NoError(t, json.Unmarshal(raw, &rows))
	require.Len(t, d.Cases, 55)
	inventories, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for i, c := range d.Cases {
		answerPath := "answers/" + c.Name + ".json"
		a, err := trafficfixture.ReadFile("semtech-downlink/" + answerPath)
		require.NoError(t, err)
		require.JSONEq(t, string(rows.Cases[i]), string(a))
		bound := 0
		for _, inventory := range inventories {
			for _, v := range inventory.Cases {
				if v.Input.OriginalPath != "common/pcapx/pcaputil/semtech-downlink/"+c.File {
					continue
				}
				require.Equal(t, c.SHA256, v.Input.SHA256)
				for _, e := range v.Expectations {
					var b struct {
						Answer  string `json:"answer_file"`
						SHA     string `json:"answer_sha256"`
						Packets int    `json:"packet_count"`
					}
					require.NoError(t, json.Unmarshal(e.PayloadConstraints, &b))
					if b.Answer != answerPath {
						continue
					}
					bound++
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(a)), b.SHA)
					require.Equal(t, c.Packets, b.Packets)
				}
			}
		}
		require.Equal(t, 1, bound, c.Name)
	}
	return d.Cases
}
func semtechDownlinkFields(t *testing.T, fields map[string]any, wire []byte) map[string]any {
	t.Helper()
	f := map[string]any{"Protocol Version": float64(2), "Token": hex.EncodeToString(wire[1:3]), "Identifier": float64(wire[3]), "Context Level": "observed", "Packet Name": fields["kind"], "Downlink Observation": fields}
	if wire[3] == 5 {
		f["Gateway EUI"] = hex.EncodeToString(wire[4:12])
	}
	return f
}
func semtechDownlinkAnswerAt(c semtechDownlinkControl, packet int) int {
	for i, m := range c.Messages {
		if len(m.Refs) == 1 && m.Refs[0] == packet {
			return i
		}
	}
	return -1
}
func semtechDownlinkKind(s string) string {
	if s == "ResourceLimit" {
		return "ResourceExceeded"
	}
	return s
}
func TestSemtechDownlinkSealedCompleteByteOracle(t *testing.T) {
	for _, c := range semtechDownlinkControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			for i, st := range c.Steps {
				if st.CaptureBytes != 0 {
					continue
				} // byte decoder sees a complete provided datagram; capture truncation is tested below
				w := doipDiscoveryWire(t, st.Hex)
				f, err := decodeSemtechDownlink(w, 4096)
				at := semtechDownlinkAnswerAt(c, i+1)
				if at >= 0 {
					require.NoError(t, err)
					rocEqualFields(t, semtechDownlinkFields(t, c.Messages[at].Fields, w), f)
				} else {
					require.NotNil(t, c.Validation[i])
					rocTypedError(t, semtechDownlinkKind(c.Validation[i].Code), err)
					require.Nil(t, f)
				}
			}
		})
	}
}
func TestSemtechDownlinkSealedDatagramMatrix(t *testing.T) {
	for _, c := range semtechDownlinkControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile("semtech-downlink/" + c.File)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
			discoveryMatrix(t, func(t *testing.T, workers int, deferred, observe bool) {
				if c.Steps[0].CaptureBytes != 0 {
					var events []*ProtocolEvent
					var stats ProtocolStats
					err := ReplayPcap(bytes.NewReader(raw), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithProtocolDecodeAs("udp", c.Port, "semtech-udp"), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }))
					require.Error(t, err)
					require.Contains(t, err.Error(), "trunc")
					require.Zero(t, stats.Decoded)
					require.Zero(t, stats.Deferred)
					require.Zero(t, stats.BufferedBytes)
					for _, e := range events {
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						f, err := e.GetFields()
						require.Error(t, err)
						require.Nil(t, f)
					}
					return
				}
				events, stats := discoveryReplay(t, raw, workers, deferred, observe, WithProtocolDecodeAs("udp", c.Port, "semtech-udp"))
				require.Len(t, events, len(c.Steps))
				require.EqualValues(t, len(c.Steps), stats.Messages)
				accepted := 0
				var limited uint64
				bad, context := 0, 0
				for i, e := range events {
					w := doipDiscoveryWire(t, c.Steps[i].Hex)
					require.Equal(t, "semtech-udp", e.Protocol)
					require.Equal(t, semtechDownlinkProfile, e.Profile)
					require.Equal(t, "explicit-decode-as", e.Admission)
					require.Equal(t, "udp", e.Transport)
					require.Equal(t, fmt.Sprintf("%s:%d", c.Steps[i].Source, c.Steps[i].Sport), e.Source)
					require.Equal(t, fmt.Sprintf("%s:%d", c.Steps[i].Dest, c.Steps[i].Dport), e.Destination)
					require.Equal(t, w, e.Raw)
					require.Len(t, e.SourceBytes.PacketRefs, 1)
					require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
					require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
					if i > 0 {
						require.Greater(t, e.ID, events[i-1].ID)
					}
					f, err := e.GetFields()
					at := semtechDownlinkAnswerAt(c, i+1)
					if at < 0 {
						require.NotNil(t, c.Validation[i])
						kind := semtechDownlinkKind(c.Validation[i].Code)
						rocTypedError(t, kind, err)
						require.Nil(t, f)
						require.Empty(t, e.Session)
						if kind == "ResourceExceeded" {
							limited += uint64(len(w))
							require.Equal(t, "limited", e.Status)
						} else if kind == "UnsupportedFeature" {
							context++
							require.Equal(t, "context-required", e.Status)
						} else {
							bad++
							require.Equal(t, "malformed", e.Status)
						}
						continue
					}
					require.NoError(t, err)
					want := semtechDownlinkFields(t, c.Messages[at].Fields, w)
					rocEqualFields(t, want, f)
					require.Equal(t, "unassociated-downlink-observation", e.Session["Association"])
					require.Equal(t, false, e.Session["RF Delivery Proven"])
					status := "decoded"
					if deferred {
						status = "deferred"
					}
					require.Equal(t, status, e.Status)
					require.Equal(t, "message", e.Completeness)
					f["Downlink Observation"].(map[string]any)["kind"] = "changed"
					clear(e.Raw)
					e.Session["Association"] = "changed"
					if e.Fields != nil {
						clear(e.Fields)
					}
					if e.Structured != nil {
						clear(e.Structured)
					}
					f, err = e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, want, f)
					accepted++
				}
				require.Equal(t, len(c.Messages), accepted)
				require.EqualValues(t, bad, stats.Malformed)
				require.EqualValues(t, context, stats.ContextRequired)
				require.Equal(t, limited, stats.LimitedBytes)
				if deferred {
					require.EqualValues(t, accepted, stats.Deferred)
					require.Zero(t, stats.Decoded)
				} else {
					require.EqualValues(t, accepted, stats.Decoded)
					require.Zero(t, stats.Deferred)
				}
			})
		})
	}
}
func TestSemtechDownlinkPublicBudgetOwnership(t *testing.T) {
	var positive semtechDownlinkControl
	for _, c := range semtechDownlinkControls(t) {
		if c.Name == "lora-immediate-full" {
			positive = c
		}
	}
	require.NotEmpty(t, positive.Name)
	wire := doipDiscoveryWire(t, positive.Steps[0].Hex)
	want := semtechDownlinkFields(t, positive.Messages[0].Fields, wire)
	for _, deferred := range []bool{false, true} {
		for _, tc := range []struct {
			Name   string
			Budget ParserBudget
			Pass   bool
		}{
			{"projection-exact", ParserBudget{MaxFrameBytes: 4096, MaxMessageBytes: 4096, MaxBufferedBytes: int(semtechDownlinkProjectionBytes) + 512*len(wire)}, true},
			{"projection-under", ParserBudget{MaxFrameBytes: 4096, MaxMessageBytes: 4096, MaxBufferedBytes: int(semtechDownlinkProjectionBytes) + 512*len(wire) - 1}, false},
			{"frame-under", ParserBudget{MaxFrameBytes: len(wire) - 1}, false},
			{"message-under", ParserBudget{MaxMessageBytes: len(wire) - 1}, false},
			{"field-map", ParserBudget{MaxCollectionElements: 16}, false}, {"field-depth", ParserBudget{MaxRecursionDepth: 3}, false},
		} {
			t.Run(fmt.Sprintf("%s/deferred%t", tc.Name, deferred), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(tc.Budget, WithSessionTransport("udp"), WithSessionPorts(1700, 1701))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				probe := s.Probe(wire)
				require.Zero(t, s.Stats().BufferedBytes)
				if tc.Pass {
					require.Equal(t, ProbeAccept, probe.Verdict)
				} else {
					require.Equal(t, ProbeReject, probe.Verdict)
				}
				before := bytes.Clone(wire)
				r := s.Feed(1, time.Unix(100, 0), before)
				require.Len(t, r.Events, 1)
				e := r.Events[0]
				clear(before)
				require.Zero(t, s.Stats().BufferedBytes)
				require.Zero(t, e.ResponseTo)
				if tc.Pass {
					require.Nil(t, r.Err)
					f, err := e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, want, f)
					clear(e.Raw)
					clear(f)
					f, err = e.GetFields()
					require.NoError(t, err)
					rocEqualFields(t, want, f)
				} else {
					require.NotNil(t, r.Err)
					require.Equal(t, ErrResourceExceeded, r.Err.Kind)
					require.Equal(t, "limited", e.Status)
					require.Empty(t, e.Session)
					f, err := e.GetFields()
					rocTypedError(t, "ResourceExceeded", err)
					require.Nil(t, f)
					require.EqualValues(t, len(wire), s.Stats().LimitedBytes)
				}
				require.Empty(t, s.Close("done"))
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
				require.Equal(t, ErrFatalSessionError, s.Feed(1, time.Unix(101, 0), wire).Err.Kind)
			})
		}
	}
}
