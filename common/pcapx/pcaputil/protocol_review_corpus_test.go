package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"io"

	"sort"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/internal/trafficfixture"
)

type reviewChunkReader struct {
	r    io.Reader
	size int
}

func (r reviewChunkReader) Read(p []byte) (int, error) { return r.r.Read(p[:min(len(p), r.size)]) }

// These controls have independently declared wire semantics and CC0 provenance.
// Canonical parity also includes fields, source byte hashes, per-flow order and
// request/response edges; matching protocol names alone cannot pass this matrix.
func TestReviewAttachmentCorpusMatrix(t *testing.T) {
	root := "testdata/protocol-sessions/pr5013-review/"
	var manifest struct {
		Captures []struct {
			Name string `json:"file"`
			Hash string `json:"sha256"`
		} `json:"captures"`
	}
	data, err := trafficfixture.ReadFile(root + "generated-manifest.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &manifest))
	for _, c := range manifest.Captures {
		t.Run(c.Name, func(t *testing.T) {
			raw, err := trafficfixture.ReadFile(root + c.Name)
			require.NoError(t, err)
			require.Equal(t, c.Hash, fmt.Sprintf("%x", sha256.Sum256(raw)))
			var reference []string
			for _, deferred := range []bool{false, true} {
				for _, workers := range []int{1, 2, 4} {
					for _, public := range []bool{false, true} {
						for _, chunk := range []int{1, 7, 65536} {
							t.Run(fmt.Sprintf("deferred%v/workers%d/public%v/readchunk%d", deferred, workers, public, chunk), func(t *testing.T) {
								budget := 32 << 20
								if strings.Contains(c.Name, "zookeeper-pending-budget") {
									budget = 65536
								}
								var events []*ProtocolEvent
								var stats ProtocolStats
								opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithBinParserConfig(BinParserConfig{Deferred: deferred, MaxBufferedBytes: budget, MaxMessageBytes: min(budget, 1<<20)}), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
								if public {
									opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
								}
								require.NoError(t, ReplayPcap(reviewChunkReader{bytes.NewReader(raw), chunk}, opts...))
								require.Zero(t, stats.BufferedBytes)
								require.Zero(t, stats.CallbackPanics)
								ids := map[uint64]*ProtocolEvent{}
								flowOrder := map[uint64]uint64{}
								var canonical []string
								for _, e := range events {
									require.NotZero(t, e.ID)
									require.NotContains(t, ids, e.ID)
									ids[e.ID] = e
									if e.FlowID != 0 {
										require.Greater(t, e.ID, flowOrder[e.FlowID])
										flowOrder[e.FlowID] = e.ID
									}
								}
								for _, e := range events {
									fields := e.Fields
									status := e.Status
									if status == "decoded" || status == "deferred" {
										fields, err = e.GetFields()
										require.NoError(t, err)
										status = "complete"
									}
									association := ""
									if e.ResponseTo != 0 {
										parent, ok := ids[e.ResponseTo]
										require.True(t, ok)
										require.Equal(t, parent.Domain, e.Domain)
										require.Equal(t, parent.FlowID, e.FlowID)
										association = fmt.Sprintf("%d/%d/%x", parent.Direction, parent.Offset, sha256.Sum256(parent.Raw))
									}
									wire, err := json.Marshal(map[string]any{"protocol": e.Protocol, "status": status, "direction": e.Direction, "offset": e.Offset, "length": e.Length, "domain": e.Domain, "raw": fmt.Sprintf("%x", sha256.Sum256(e.Raw)), "source_sha": e.SourceBytes.SHA256, "fields": fields, "session": e.Session, "error": e.Error, "expert": e.ExpertCode, "response_to": association})
									require.NoError(t, err)
									canonical = append(canonical, string(wire))
								}
								reviewAssertControl(t, c.Name, events, stats)
								sort.Strings(canonical)
								if reference == nil {
									reference = canonical
								} else {
									require.Equal(t, reference, canonical, "worker/callback/deferred/container-read parity")
								}
							})
						}
					}
				}
			}
		})
	}
}

func reviewAssertControl(t *testing.T, name string, events []*ProtocolEvent, stats ProtocolStats) {
	t.Helper()
	complete := func(e *ProtocolEvent) bool { return e.Status == "decoded" || e.Status == "deferred" }
	if strings.Contains(name, "vxlan") {
		require.Len(t, events, 2)
		domains := map[CaptureDomain]bool{}
		for _, e := range events {
			require.True(t, complete(e))
			require.Equal(t, "dns", e.Protocol)
			domains[e.Domain] = true
			require.Contains(t, []string{"/vxlan:192.0.2.1~192.0.2.2:7", "/vxlan:192.0.2.1~192.0.2.2:9"}, e.Domain.Encapsulation)
			dns := e.Session["DNS"].(map[string]any)
			require.EqualValues(t, 0, dns["ID"])
			require.Equal(t, "vxlan.example.test", dns["Questions"].([]map[string]any)[0]["Name"])
		}
		require.Len(t, domains, 2)
	} else if strings.Contains(name, "two-interfaces") {
		require.Len(t, events, 2)
		seen := map[int]bool{}
		for _, e := range events {
			require.True(t, complete(e))
			require.Equal(t, "http", e.Protocol)
			require.Zero(t, e.Direction)
			fields, err := e.GetFields()
			require.NoError(t, err)
			encoded, err := json.Marshal(fields)
			require.NoError(t, err)
			path := map[int]string{0: "/interface-zero", 1: "/interface-one"}[e.Domain.Interface]
			require.NotEmpty(t, path)
			require.Contains(t, string(encoded), path)
			seen[e.Domain.Interface] = true
		}
		require.Len(t, seen, 2)
		require.EqualValues(t, 2, stats.Flows)
	} else if strings.Contains(name, "close-") {
		require.Len(t, events, 2)
		request, response := events[0], events[1]
		require.True(t, complete(request))
		require.Zero(t, request.Direction)
		require.Equal(t, 1, response.Direction)
		fin := strings.Contains(name, "-fin")
		if fin {
			require.True(t, complete(response))
			require.Zero(t, stats.Incomplete)
			fields, err := response.GetFields()
			require.NoError(t, err)
			if strings.HasPrefix(name, "doh") {
				require.Equal(t, "doh", response.Protocol)
				require.Equal(t, request.ID, response.ResponseTo)
				require.Equal(t, "matched", response.Session["Association Status"])
				require.Equal(t, "doh.example.test", response.Session["Matched Request"])
				require.Equal(t, []string{"192.0.2.99"}, response.Session["A Records"])
			} else {
				require.Equal(t, "close-body", fields["Message"].(map[string]any)["HTTP Response"].(map[string]any)["Body"].(map[string]any)["Octets"])
			}
		} else {
			require.Equal(t, "incomplete", response.Status)
			require.EqualValues(t, 1, stats.Incomplete)
			require.Zero(t, response.ResponseTo)
		}
	} else if strings.Contains(name, "zookeeper") {
		require.GreaterOrEqual(t, len(events), 3)
		require.True(t, complete(events[0]))
		require.True(t, complete(events[1]))
		require.Equal(t, "connected", events[1].Session["Handshake Status"])
		if strings.Contains(name, "-legal") {
			require.LessOrEqual(t, stats.PeakBufferedBytes, int64(65536))
			limited := 0
			requests := 0
			for _, e := range events {
				if e.ExpertCode == string(ErrResourceExceeded) {
					// Connection budgets retain the established context-required status.
					require.Equal(t, "context-required", e.Status)
					limited++
					require.Equal(t, string(ErrResourceExceeded), e.ExpertCode)
				}
				if complete(e) && e.Direction == 0 && e.Offset > 0 {
					requests++
					require.Len(t, e.Session["Path"], 4096)
				}
			}
			require.Equal(t, 1, limited)
			require.Greater(t, requests, 0)
			require.Less(t, requests, 24)
		} else {
			require.Len(t, events, 3)
			require.Equal(t, "malformed", events[2].Status)
			require.Equal(t, string(ErrMalformedMessage), events[2].ExpertCode)
			require.Contains(t, events[2].Error, "invalid Jute path/watch")
			require.Zero(t, stats.LimitedBytes)
		}
	} else {
		t.Fatalf("missing independent control oracle: %s", name)
	}
}
