package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"os"
	"path/filepath"
	"testing"
)

type bacnetBoundaryAnswer struct {
	Events []struct {
		ID                                    uint64
		Dir                                   int
		Raw, Protocol, Profile, Status, Error string
		ExpertCode                            string `json:"expert_code"`
		Fields, Session                       map[string]any
		QueryError                            string `json:"query_error"`
		ResponseTo                            uint64 `json:"response_to"`
		TransactionID                         uint64 `json:"transaction_id"`
	}
	Packets int
}

func bacnetBoundaryRead(t *testing.T, name string) []byte {
	t.Helper()
	var b []byte
	var err error
	if root := os.Getenv("BACNET_NPDU_TEST_ROOT"); root != "" {
		b, err = os.ReadFile(filepath.Join(root, name))
	} else {
		b, err = trafficfixture.ReadFile("bacnet-npdu-boundary/" + name)
	}
	require.NoError(t, err)
	return b
}
func bacnetBoundaryCases(t *testing.T) []mmsISOCase {
	t.Helper()
	var m struct {
		Schema int `json:"schema_version"`
		Cases  []mmsISOCase
	}
	require.NoError(t, json.Unmarshal(bacnetBoundaryRead(t, "manifest.json"), &m))
	require.Equal(t, 1, m.Schema)
	require.Len(t, m.Cases, 22)
	if os.Getenv("BACNET_NPDU_TEST_ROOT") == "" {
		batches, err := trafficfixture.AllExpectations()
		require.NoError(t, err)
		frozen := map[string]trafficfixture.FrozenCase{}
		for _, b := range batches {
			for _, c := range b.Cases {
				frozen[c.ID] = c
			}
		}
		for _, c := range m.Cases {
			f, ok := frozen["bacnet-npdu-boundary/"+c.ID]
			require.True(t, ok)
			require.Equal(t, c.SHA256, f.Input.SHA256)
			require.Equal(t, c.Packets, f.Facts.PacketCount)
			require.Len(t, f.Expectations, 1)
			var binding struct {
				Answer string `json:"answer_file"`
				SHA    string `json:"answer_sha256"`
			}
			require.NoError(t, json.Unmarshal(f.Expectations[0].PayloadConstraints, &binding))
			require.Equal(t, c.Answer, binding.Answer)
			require.Equal(t, c.AnswerSHA, binding.SHA)
		}
	}
	return m.Cases
}
func bacnetBoundaryInput(t *testing.T, c mmsISOCase) ([]byte, bacnetBoundaryAnswer) {
	t.Helper()
	raw, answer := bacnetBoundaryRead(t, c.Capture), bacnetBoundaryRead(t, c.Answer)
	require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
	require.Equal(t, c.AnswerSHA, fmt.Sprintf("%x", sha256.Sum256(answer)))
	var a bacnetBoundaryAnswer
	d := json.NewDecoder(bytes.NewReader(answer))
	d.UseNumber()
	require.NoError(t, d.Decode(&a))
	require.Equal(t, c.Packets, a.Packets)
	require.Len(t, a.Events, a.Packets)
	return raw, a
}
func bacnetBoundaryEqual(t *testing.T, want, got any) {
	t.Helper()
	b, err := json.Marshal(got)
	require.NoError(t, err)
	var actual any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	require.NoError(t, d.Decode(&actual))
	b, err = json.Marshal(want)
	require.NoError(t, err)
	var expected any
	d = json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	require.NoError(t, d.Decode(&expected))
	require.Equal(t, expected, actual)
}
func bacnetBoundaryCheck(t *testing.T, a bacnetBoundaryAnswer, events []*ProtocolEvent, deferred, owned bool) {
	t.Helper()
	require.Len(t, events, len(a.Events))
	for i, e := range events {
		w := a.Events[i]
		raw, err := hex.DecodeString(w.Raw)
		require.NoError(t, err)
		require.Equal(t, w.ID, e.ID)
		require.Zero(t, e.ResponseTo)
		require.Zero(t, e.TransactionID)
		require.Equal(t, w.ResponseTo, e.ResponseTo)
		require.Equal(t, w.TransactionID, e.TransactionID)
		require.Equal(t, raw, e.Raw)
		require.Equal(t, len(raw), e.Length)
		require.Equal(t, "udp", e.Transport)
		require.Equal(t, w.Protocol, e.Protocol)
		require.Equal(t, w.Profile, e.Profile)
		require.Equal(t, w.Error, e.Error)
		require.Equal(t, w.ExpertCode, e.ExpertCode)
		require.Equal(t, "captured", e.SourceBytes.Kind)
		require.Len(t, e.SourceBytes.PacketRefs, 1)
		src, dst := "192.0.2.1:40000", "192.0.2.2:47808"
		if w.Dir == 1 {
			src, dst = dst, src
		}
		require.Equal(t, src, e.Source)
		require.Equal(t, dst, e.Destination)
		status := w.Status
		if deferred && w.Protocol == "bacnet" {
			status = "deferred"
		}
		require.Equal(t, status, e.Status)
		if w.Protocol == "" {
			require.Empty(t, e.Fields)
			require.Nil(t, e.Session)
			fields, err := e.GetFields()
			require.Nil(t, fields)
			require.EqualError(t, err, w.QueryError)
			if owned {
				e.Fields = map[string]any{"CALLER": true}
				e.Structured = map[string]any{"fields": e.Fields}
				e.Session = map[string]any{"CALLER": true}
				e.Protocol = "bacnet"
				e.Status = "decoded"
				clear(e.Raw)
				fields, err = e.GetFields()
				require.Nil(t, fields)
				require.EqualError(t, err, w.QueryError)
				result, err := e.Decode()
				require.Nil(t, result)
				require.EqualError(t, err, w.QueryError)
			}
			continue
		}
		require.Equal(t, "message", e.Completeness)
		require.Equal(t, "wire-and-port-hint", e.Admission)
		require.Equal(t, "application-layer.extended_protocols", e.Rule)
		require.Equal(t, "BACnetIP", e.Entry)
		fields, err := e.GetFields()
		require.NoError(t, err)
		bacnetBoundaryEqual(t, w.Fields, fields)
		bacnetBoundaryEqual(t, w.Session, e.Session)
		require.IsType(t, uint8(0), fields["NPDU Control"])
		require.IsType(t, uint16(0), fields["BVLC Length"])
		require.Equal(t, raw, fields["Complete BVLC PDU"])
		require.Equal(t, raw[6:], fields["APDU"])
		if owned {
			fields["APDU"].([]byte)[0] = 255
			fields["Service"] = "CALLER"
			e.Session["APDU"].([]byte)[0] = 255
			e.Session = map[string]any{"CALLER": true}
			e.Fields = map[string]any{"CALLER": true}
			e.Structured = map[string]any{"fields": map[string]any{"CALLER": true}, "metadata": "CALLER"}
			e.Metadata = "CALLER"
			clear(e.Raw)
			again, err := e.GetFields()
			require.NoError(t, err)
			bacnetBoundaryEqual(t, w.Fields, again)
			again["Complete BVLC PDU"].([]byte)[0] = 0
		}
		result, err := e.Decode()
		require.NoError(t, err)
		bacnetBoundaryEqual(t, map[string]any{"fields": w.Fields, "metadata": nil, "session": w.Session}, result)
	}
}
func TestBACnetNPDUSealedReplay(t *testing.T) {
	for _, c := range bacnetBoundaryCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := bacnetBoundaryInput(t, c)
			for _, worker := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d-d%t-o%t", worker, deferred, observe), func(t *testing.T) {
							var events []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithTCPReassemblyWorkers(worker), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							input := bytes.Clone(raw)
							require.NoError(t, ReplayPcap(bytes.NewReader(input), opts...))
							clear(input)
							bacnetBoundaryCheck(t, a, events, deferred, false)
							if observe {
								require.Equal(t, c.Packets, seen)
							}
							accepted := uint64(0)
							for _, expected := range a.Events {
								if expected.Protocol != "" {
									accepted++
								}
							}
							require.Equal(t, accepted, stats.Messages)
							require.Equal(t, uint64(len(events))-accepted, stats.Unknown)
							if deferred {
								require.Equal(t, accepted, stats.Deferred)
								require.Zero(t, stats.Decoded)
							} else {
								require.Equal(t, accepted, stats.Decoded)
								require.Zero(t, stats.Deferred)
							}
							require.Zero(t, stats.Malformed)
							require.Zero(t, stats.Incomplete)
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
						})
					}
				}
			}
		})
	}
}
func TestBACnetNPDUSealedOwnedQueries(t *testing.T) {
	for _, c := range bacnetBoundaryCases(t) {
		t.Run(c.ID, func(t *testing.T) {
			raw, a := bacnetBoundaryInput(t, c)
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprint(deferred), func(t *testing.T) {
					var events []*ProtocolEvent
					input := bytes.Clone(raw)
					require.NoError(t, ReplayPcap(bytes.NewReader(input), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
					clear(input)
					bacnetBoundaryCheck(t, a, events, deferred, true)
				})
			}
		})
	}
}
