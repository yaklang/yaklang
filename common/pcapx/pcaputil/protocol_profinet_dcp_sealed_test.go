package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type dcpAssociation struct {
	Pairs     [][]int `json:"candidate_request_response_refs"`
	Unmatched []int   `json:"unmatched_response_refs"`
	Retries   []int   `json:"retransmission_request_refs"`
}

type dcpControl struct {
	Name, File, SHA256 string
	Packets            int
	Steps              []struct {
		Frame     string `json:"frame_hex"`
		Original  int    `json:"capture_original_bytes"`
		Truncated bool   `json:"capture_truncated"`
	}
	Messages []struct {
		Raw    string `json:"raw_hex"`
		Refs   []int  `json:"packet_refs"`
		Fields map[string]any
	} `json:"expected_messages"`
	Errors      []*struct{ Code, Detail string } `json:"static_packet_validation"`
	Association *dcpAssociation                  `json:"association_target"`
	Budget      *struct {
		Frame       int             `json:"max_frame_bytes"`
		Blocks      int             `json:"max_blocks"`
		Refs        []int           `json:"resource_packet_refs"`
		Association *dcpAssociation `json:"association_target"`
	} `json:"budget_target"`
}

func dcpControls(t *testing.T) []dcpControl {
	t.Helper()
	b, err := trafficfixture.ReadFile("dcp-identify/controls.json")
	require.NoError(t, err)
	var doc struct{ Cases []dcpControl }
	var original struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(b, &doc))
	require.NoError(t, json.Unmarshal(b, &original))
	require.Len(t, doc.Cases, 53)
	inventories, err := trafficfixture.AllExpectations()
	require.NoError(t, err)
	for i, c := range doc.Cases {
		answer := "answers/" + c.Name + ".json"
		a, err := trafficfixture.ReadFile("dcp-identify/" + answer)
		require.NoError(t, err)
		require.JSONEq(t, string(original.Cases[i]), string(a))
		bound := 0
		for _, inv := range inventories {
			for _, row := range inv.Cases {
				if row.Input.OriginalPath != "common/pcapx/pcaputil/dcp-identify/"+c.File {
					continue
				}
				require.Equal(t, c.SHA256, row.Input.SHA256)
				for _, e := range row.Expectations {
					var link struct {
						Answer  string `json:"answer_file"`
						SHA     string `json:"answer_sha256"`
						Packets int    `json:"packet_count"`
					}
					require.NoError(t, json.Unmarshal(e.PayloadConstraints, &link))
					if link.Answer == answer {
						bound++
						require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(a)), link.SHA)
						require.Equal(t, c.Packets, link.Packets)
					}
				}
			}
		}
		require.Equal(t, 1, bound, c.Name)
	}
	return doc.Cases
}
func dcpAnswer(c dcpControl, packet int) int {
	for i, m := range c.Messages {
		if len(m.Refs) == 1 && m.Refs[0] == packet {
			return i
		}
	}
	return -1
}
func TestDCPIdentifySealedByteOracle(t *testing.T) {
	for _, c := range dcpControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			for i, st := range c.Steps {
				frame := doipDiscoveryWire(t, st.Frame)
				link, w, _, err := dcpEthernet(frame)
				var f map[string]any
				if err == nil {
					f, err = decodeDCPIdentify(w, 4096)
					if err == nil {
						f["ethernet"] = link
					}
				}
				at := dcpAnswer(c, i+1)
				if at < 0 {
					rocTypedError(t, c.Errors[i].Code, err)
					require.Nil(t, f)
				} else {
					require.NoError(t, err)
					rocEqualFields(t, c.Messages[at].Fields, f)
				}
			}
		})
	}
}
func dcpCheckMessages(t *testing.T, c dcpControl, events []*ProtocolEvent, deferred bool, budget bool) {
	t.Helper()
	require.Len(t, events, len(c.Steps))
	denied := map[int]bool{}
	if budget && c.Budget != nil {
		for _, ref := range c.Budget.Refs {
			denied[ref] = true
		}
	}
	for i, e := range events {
		require.Equal(t, "ethernet", e.Transport)
		require.Len(t, e.SourceBytes.PacketRefs, 1)
		require.EqualValues(t, i+1, e.SourceBytes.PacketRefs[0].Number)
		require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
		require.Zero(t, e.ResponseTo, "passive discovery candidates are not unique request/response transactions")
		require.Zero(t, e.TransactionID)
		at := dcpAnswer(c, i+1)
		f, err := e.GetFields()
		switch {
		case denied[i+1]:
			rocTypedError(t, "ResourceExceeded", err)
			require.Nil(t, f)
			require.Empty(t, e.Session)
		case c.Steps[i].Truncated:
			rocTypedError(t, "NeedMore", err)
			require.Nil(t, f)
			require.Nil(t, e.Raw)
			require.Equal(t, "incomplete", e.Status)
		case at < 0:
			rocTypedError(t, c.Errors[i].Code, err)
			require.Nil(t, f)
			require.Empty(t, e.Session)
		default:
			require.Equal(t, "profinet-dcp", e.Protocol)
			require.Equal(t, dcpIdentifyProfile, e.Profile)
			require.NoError(t, err)
			rocEqualFields(t, c.Messages[at].Fields, f)
			require.Equal(t, doipDiscoveryWire(t, c.Messages[at].Raw), e.Raw)
			require.Equal(t, "message", e.Completeness)
			status := "decoded"
			if deferred {
				status = "deferred"
			}
			require.Equal(t, status, e.Status)
			f["kind"] = "caller mutation"
			if b, ok := f["blocks"].([]map[string]any); ok && len(b) > 0 {
				b[0]["kind"] = "changed"
			}
			clear(e.Raw)
			if e.Fields != nil {
				delete(e.Fields, "blocks")
			}
			e.Session["kind"] = "changed"
			f, err = e.GetFields()
			require.NoError(t, err)
			rocEqualFields(t, c.Messages[at].Fields, f)
		}
	}
	association := c.Association
	if budget && c.Budget != nil && c.Budget.Association != nil {
		association = c.Budget.Association
	}
	if association != nil {
		got := map[int]int{}
		for i, e := range events {
			if id, ok := e.Session["Candidate Request ID"].(uint64); ok {
				for q, r := range events {
					if r.ID == id {
						got[i+1] = q + 1
					}
				}
			}
		}
		require.Len(t, got, len(association.Pairs))
		for _, p := range association.Pairs {
			require.Equal(t, p[0], got[p[1]])
		}
		for _, ref := range association.Unmatched {
			require.NotContains(t, events[ref-1].Session, "Candidate Request ID")
		}
		for _, ref := range association.Retries {
			require.Equal(t, events[0].ID, events[ref-1].Session["Repeated Request ID"])
		}
	}
	if budget && c.Budget != nil {
		for i, e := range events {
			if c.Budget.Frame != 0 && i > 1 {
				require.NotContains(t, e.Session, "Candidate Request ID")
			}
		}
	}
}
func TestDCPIdentifySealedEthernetMatrix(t *testing.T) {
	for _, c := range dcpControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			b, err := trafficfixture.ReadFile("dcp-identify/" + c.File)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(b)))
			discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
				events, stats := dcpReplay(t, c, b, w, d, o)
				dcpCheckMessages(t, c, events, d, false)
				require.EqualValues(t, len(c.Steps), stats.Messages)
			})
		})
	}
}
func TestDCPIdentifyResourceOwnershipAndClose(t *testing.T) {
	for _, c := range dcpControls(t) {
		t.Run(c.Name, func(t *testing.T) {
			for _, deferred := range []bool{false, true} {
				s := discoverySession(t, DefaultParserBudget(), 8805)
				a := s.f.a
				a.config.Deferred = deferred
				var events []*ProtocolEvent
				a.config.OnEvent = func(e *ProtocolEvent) { events = append(events, e) }
				if c.Budget != nil && c.Budget.Frame != 0 {
					a.budget.MaxFrameBytes = c.Budget.Frame
				}
				for i, st := range c.Steps {
					frame := doipDiscoveryWire(t, st.Frame)
					ci := gopacket.CaptureInfo{Timestamp: time.Unix(int64(100+i), 0), CaptureLength: len(frame), Length: st.Original}
					evidence := captureEvidence{Ref: PacketReference{Number: uint64(i + 1)}}
					a.decodeDCPEthernet(frame, evidence, ci)
					clear(frame)
					if c.Budget != nil && c.Budget.Frame != 0 && i == 1 {
						a.dcpMu.Lock()
						for _, ob := range a.dcpObservations {
							require.True(t, ob.blocked)
							require.Empty(t, ob.requests)
						}
						a.dcpMu.Unlock()
						require.EqualValues(t, 512, a.buffered.Load())
					}
				}
				// max_blocks6 is a private target, not a public parser setting. Use the
				// archived byte input with the private block limit; native default remains.
				if c.Budget != nil && c.Budget.Blocks != 0 {
					frame := doipDiscoveryWire(t, c.Steps[0].Frame)
					_, w, _, err := dcpEthernet(frame)
					require.NoError(t, err)
					f, err := decodeDCPIdentifyBounded(w, 4096, c.Budget.Blocks)
					rocTypedError(t, "ResourceExceeded", err)
					require.Nil(t, f)
				}
				dcpCheckMessages(t, c, events, deferred, c.Budget != nil && c.Budget.Frame != 0)
				a.closeDCPObservations()
				a.closeDCPObservations()
				require.Zero(t, a.buffered.Load())
				require.Empty(t, a.dcpObservations)
				s.Close("end")
				require.Empty(t, s.Close("again"))
				require.Zero(t, s.Stats().BufferedBytes)
			}
		})
	}
}

func dcpReplay(t *testing.T, c dcpControl, raw []byte, workers int, deferred, observe bool) ([]*ProtocolEvent, ProtocolStats) {
	t.Helper()
	var events []*ProtocolEvent
	var stats ProtocolStats
	var assembly TCPReassemblyStats
	var seen atomic.Int64
	opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s }), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
	if observe {
		opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.NotEmpty(t, p.Data()); seen.Add(1) }))
	}
	owned := bytes.Clone(raw)
	err := ReplayPcap(bytes.NewReader(owned), opts...)
	clear(owned)
	truncated := 0
	for _, st := range c.Steps {
		if st.Truncated {
			truncated++
		}
	}
	if truncated != 0 {
		require.ErrorContains(t, err, "capture contains truncated packets")
	} else {
		require.NoError(t, err)
	}
	require.EqualValues(t, truncated, assembly.TruncatedCaptures)
	require.EqualValues(t, len(c.Steps), assembly.CapturedPackets)
	require.Zero(t, stats.BufferedBytes)
	require.Zero(t, stats.CallbackPanics)
	if observe {
		require.EqualValues(t, len(c.Steps), seen.Load())
	}
	return events, stats
}
