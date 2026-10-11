package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type dcpServiceControl struct {
	ID      string         `json:"id"`
	Capture string         `json:"capture"`
	Alias   string         `json:"input_alias"`
	SHA     string         `json:"sha256"`
	Wire    string         `json:"wire_hex"`
	Fields  map[string]any `json:"fields"`
	Error   string         `json:"error"`
}

func dcpServiceControls(t *testing.T) []dcpServiceControl {
	t.Helper()
	raw, err := trafficfixture.ReadFile("dcp-services/controls.json")
	require.NoError(t, err)
	var d struct{ Cases []dcpServiceControl }
	require.NoError(t, json.Unmarshal(raw, &d))
	require.Len(t, d.Cases, 42)
	var original struct{ Cases []json.RawMessage }
	require.NoError(t, json.Unmarshal(raw, &original))
	for i, c := range d.Cases {
		answer, err := trafficfixture.ReadFile("dcp-services/answers/" + c.ID + ".json")
		require.NoError(t, err)
		var got map[string]any
		var want map[string]any
		require.NoError(t, json.Unmarshal(answer, &got))
		require.NoError(t, json.Unmarshal(original.Cases[i], &want))
		delete(want, "answer_sha256")
		require.Equal(t, want, got)
	}
	return d.Cases
}
func dcpServiceBytes(t *testing.T, c dcpServiceControl) []byte {
	t.Helper()
	b, e := trafficfixture.ReadFile("dcp-services/" + c.Capture)
	require.NoError(t, e)
	require.Equal(t, c.SHA, fmt.Sprintf("%x", sha256.Sum256(b)))
	return b
}
func testDCPServicesByteOracle(t *testing.T) {
	for _, c := range dcpServiceControls(t) {
		t.Run("services/"+c.ID, func(t *testing.T) {
			w := doipDiscoveryWire(t, c.Wire)
			f, e := decodeDCPServices(w, 4096)
			if c.Error != "" {
				rocTypedError(t, c.Error, e)
				require.Nil(t, f)
				return
			}
			require.NoError(t, e)
			link, _, _, e := dcpEthernet(append(doipDiscoveryWire(t, "0200000000020200000000018892"), w...))
			require.NoError(t, e)
			f["ethernet"] = link
			rocEqualFields(t, c.Fields, f)
		})
	}
}
func checkDCPServiceEvent(t *testing.T, c dcpServiceControl, e *ProtocolEvent) {
	t.Helper()
	require.Equal(t, "profinet-dcp", e.Protocol)
	require.Equal(t, dcpServicesProfile, e.Profile)
	require.Equal(t, "ethernet", e.Transport)
	require.Zero(t, e.ResponseTo)
	require.Zero(t, e.TransactionID)
	require.Equal(t, "02:00:00:00:00:01", e.Source)
	require.Equal(t, "02:00:00:00:00:02", e.Destination)
	require.Len(t, e.SourceBytes.PacketRefs, 1)
	require.EqualValues(t, 1, e.SourceBytes.PacketRefs[0].Number)
	require.Equal(t, e.Domain, e.SourceBytes.PacketRefs[0].Domain)
	f, err := e.GetFields()
	if c.Error != "" {
		rocTypedError(t, c.Error, err)
		require.Nil(t, f)
		require.Empty(t, e.Session)
		require.Empty(t, e.semanticSession)
		return
	}
	require.NoError(t, err)
	rocEqualFields(t, c.Fields, f)
	rocEqualFields(t, c.Fields, e.Session)
	require.Equal(t, doipDiscoveryWire(t, c.Wire), e.Raw)
	d, err := e.Decode()
	require.NoError(t, err)
	rocEqualFields(t, c.Fields, d["fields"].(map[string]any))
	rocEqualFields(t, c.Fields, d["session"].(map[string]any))
	// Public projections, returned trees and caller capture buffers have distinct ownership.
	e.Session["transaction_id"] = uint32(0)
	e.Session["Association"] = "forged"
	e.Session = map[string]any{"forged": true}
	d["session"].(map[string]any)["kind"] = "forged"
	f["kind"] = "forged"
	clear(e.Raw)
	e.Structured = map[string]any{"fields": "forged"}
	d, err = e.Decode()
	require.NoError(t, err)
	rocEqualFields(t, c.Fields, d["fields"].(map[string]any))
	rocEqualFields(t, c.Fields, d["session"].(map[string]any))
	f, err = e.GetFields()
	require.NoError(t, err)
	rocEqualFields(t, c.Fields, f)
}
func testDCPServicesMatrix(t *testing.T) {
	testDCPHistoricalSet(t)
	for _, c := range dcpServiceControls(t) {
		for _, workers := range []int{1, 2, 4} {
			for _, deferred := range []bool{false, true} {
				for _, observe := range []bool{false, true} {
					t.Run(fmt.Sprintf("services/%s/w%d/deferred%t/observe%t", c.ID, workers, deferred, observe), func(t *testing.T) {
						raw := dcpServiceBytes(t, c)
						var events []*ProtocolEvent
						var stats ProtocolStats
						seen := 0
						opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
						if observe {
							opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { require.NotEmpty(t, p.Data()); seen++ }))
						}
						require.NoError(t, ReplayPcap(bytes.NewReader(raw), opts...))
						clear(raw)
						require.Len(t, events, 1)
						checkDCPServiceEvent(t, c, events[0])
						require.Zero(t, stats.BufferedBytes)
						require.Zero(t, stats.CallbackPanics)
						require.EqualValues(t, 1, stats.Messages)
						if observe {
							require.Equal(t, 1, seen)
						}
					})
				}
			}
		}
	}
}
func testDCPServicesResourceOwnership(t *testing.T) {
	testDCPServicesBudgetMatrix(t)
	for _, c := range dcpServiceControls(t) {
		if c.ID != "collection-options-18" && c.ID != "collection-selectors-19" {
			continue
		}
		for _, limit := range []int{18, 19} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("services/%s/limit%d/deferred%t", c.ID, limit, deferred), func(t *testing.T) {
					s := discoverySession(t, DefaultParserBudget(), 8805)
					a := s.f.a
					a.budget.MaxCollectionElements = limit
					a.config.Deferred = deferred
					var events []*ProtocolEvent
					a.config.OnEvent = func(e *ProtocolEvent) { events = append(events, e) }
					frame := append(doipDiscoveryWire(t, "0200000000020200000000018892"), doipDiscoveryWire(t, c.Wire)...)
					ci := gopacket.CaptureInfo{Timestamp: time.Unix(100, 0), CaptureLength: len(frame), Length: len(frame)}
					a.decodeDCPEthernet(frame, captureEvidence{Ref: PacketReference{Number: 1}}, ci)
					clear(frame)
					require.Len(t, events, 1)
					f, e := events[0].GetFields()
					if limit == 18 {
						rocTypedError(t, "ResourceExceeded", e)
						require.Nil(t, f)
						require.Nil(t, events[0].Raw)
						require.Empty(t, events[0].Session)
					} else {
						require.NoError(t, e)
						rocEqualFields(t, c.Fields, f)
					}
					for _, neighbor := range dcpServiceControls(t) {
						if neighbor.ID != "get-ordered-selectors" {
							continue
						}
						next := append(doipDiscoveryWire(t, "0200000000020200000000018892"), doipDiscoveryWire(t, neighbor.Wire)...)
						ci.CaptureLength, ci.Length = len(next), len(next)
						a.decodeDCPEthernet(next, captureEvidence{Ref: PacketReference{Number: 2}}, ci)
						clear(next)
						require.Len(t, events, 2)
						f, err := events[1].GetFields()
						require.NoError(t, err)
						rocEqualFields(t, neighbor.Fields, f)
						require.Zero(t, events[1].ResponseTo)
						require.Zero(t, events[1].TransactionID)
					}
					require.Empty(t, a.dcpObservations, "Get/Set/Hello must not contaminate Identify registry")
					require.Zero(t, a.buffered.Load())
					s.Close("end")
					require.Empty(t, s.Close("again"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
	// Discovery must keep the association context private too, after it is assigned.
	t.Run("services/Identify-private-association", func(t *testing.T) {
		s := discoverySession(t, DefaultParserBudget(), 8805)
		a := s.f.a
		var events []*ProtocolEvent
		a.config.Deferred = true
		a.config.OnEvent = func(e *ProtocolEvent) { events = append(events, e) }
		w := doipDiscoveryWire(t, "0200000000020200000000018892fefe05000102030400000004ffff0000")
		ci := gopacket.CaptureInfo{Timestamp: time.Unix(100, 0), CaptureLength: len(w), Length: len(w)}
		a.decodeDCPEthernet(w, captureEvidence{Ref: PacketReference{Number: 1}}, ci)
		require.Len(t, events, 1)
		e := events[0]
		want := cloneSession(e.Session)
		e.Session["Association"] = "forged"
		e.Session["transaction_id"] = uint32(0)
		d, err := e.Decode()
		require.NoError(t, err)
		require.Equal(t, want, d["session"])
		require.Equal(t, "observed-discovery-request", want["Association"])
		a.closeDCPObservations()
		require.Zero(t, a.buffered.Load())
		s.Close("end")
	})
}

func testDCPHistoricalSet(t *testing.T) {
	b, err := trafficfixture.ReadFile("dcp-services/history-set.json")
	require.NoError(t, err)
	var h struct {
		Alias   string `json:"input_alias"`
		SHA     string `json:"sha256"`
		Answers []struct {
			Packet int
			Fields map[string]any
			Raw    string `json:"raw_hex"`
		}
	}
	require.NoError(t, json.Unmarshal(b, &h))
	require.Len(t, h.Answers, 4)
	raw, err := trafficfixture.ReadFile("../../bin-parser/testdata/protocol-corpus/captures/iti-ics/iti-profinet-dcp.pcap")
	require.NoError(t, err)
	require.Equal(t, h.SHA, fmt.Sprintf("%x", sha256.Sum256(raw)))
	for _, workers := range []int{1, 2, 4} {
		for _, deferred := range []bool{false, true} {
			for _, observe := range []bool{false, true} {
				t.Run(fmt.Sprintf("services/history/w%d/deferred%t/observe%t", workers, deferred, observe), func(t *testing.T) {
					var events []*ProtocolEvent
					var stats ProtocolStats
					seen := 0
					opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
					if observe {
						opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
					}
					require.NoError(t, ReplayPcap(bytes.NewReader(raw), opts...))
					require.Len(t, events, 6)
					require.Zero(t, stats.BufferedBytes)
					require.Zero(t, stats.CallbackPanics)
					if observe {
						require.Equal(t, 6, seen)
					}
					for j := 4; j < 6; j++ {
						e := events[j]
						require.Equal(t, "arp", e.Protocol)
						require.Equal(t, "network", e.Transport)
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						require.EqualValues(t, j+1, e.SourceBytes.PacketRefs[0].Number)
						require.Len(t, e.Raw, 60)
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						f, err := e.GetFields()
						require.NoError(t, err)
						ip := "192.168.0.6"
						if j == 5 {
							ip = "192.168.0.10"
						}
						rocEqualFields(t, map[string]any{"Operation": float64(j - 3), "Sender MAC": "08:00:06:93:cf:32", "Sender IP": ip, "Target IP": "192.168.0.10", "Observation": "unverified-neighbor"}, f)
						require.Equal(t, ip, e.Source)
						require.Equal(t, "192.168.0.10", e.Destination)
					}
					for i, a := range h.Answers {
						e := events[i]
						require.Len(t, e.SourceBytes.PacketRefs, 1)
						require.EqualValues(t, a.Packet, e.SourceBytes.PacketRefs[0].Number)
						require.Zero(t, e.ResponseTo)
						require.Zero(t, e.TransactionID)
						f, err := e.GetFields()
						require.NoError(t, err)
						rocEqualFields(t, a.Fields, f)
						require.Equal(t, doipDiscoveryWire(t, a.Raw), e.Raw)
						require.Equal(t, a.Fields["ethernet"].(map[string]any)["source_mac"], e.Source)
						require.Equal(t, a.Fields["ethernet"].(map[string]any)["destination_mac"], e.Destination)
						want := cloneSession(e.Session)
						if i >= 2 {
							rocEqualFields(t, a.Fields, want)
							require.NotContains(t, want, "Candidate Request ID")
							require.NotContains(t, want, "Association")
						}
						e.Session = map[string]any{"forged": true}
						d, err := e.Decode()
						require.NoError(t, err)
						require.Equal(t, want, d["session"])
						rocEqualFields(t, a.Fields, d["fields"].(map[string]any))
					}
				})
			}
		}
	}
}

func testDCPServicesBudgetMatrix(t *testing.T) {
	for _, c := range dcpServiceControls(t) {
		if c.ID != "collection-options-18" {
			continue
		}
		for _, limit := range []int{67, 68} {
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("services/budget/%s/limit%d/w%d/deferred%t/observe%t", c.ID, limit, workers, deferred, observe), func(t *testing.T) {
							raw := dcpServiceBytes(t, c)
							var events []*ProtocolEvent
							var stats ProtocolStats
							seen := 0
							opts := []CaptureOption{WithProtocolBudget(limit, 16<<20), WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) { seen++ }))
							}
							require.NoError(t, ReplayPcap(bytes.NewReader(raw), opts...))
							clear(raw)
							require.Len(t, events, 1)
							e := events[0]
							require.Zero(t, e.ResponseTo)
							require.Zero(t, e.TransactionID)
							f, err := e.GetFields()
							if limit == 67 {
								rocTypedError(t, "ResourceExceeded", err)
								require.Nil(t, f)
								require.Empty(t, e.Session)
								require.Empty(t, e.semanticSession)
								require.Nil(t, e.Raw)
								require.Equal(t, "limited", e.Status)
							} else {
								require.NoError(t, err)
								rocEqualFields(t, c.Fields, f)
							}
							require.Zero(t, stats.BufferedBytes)
							require.Zero(t, stats.CallbackPanics)
							if observe {
								require.Equal(t, 1, seen)
							}
						})
					}
				}
			}
		}
	}
}
