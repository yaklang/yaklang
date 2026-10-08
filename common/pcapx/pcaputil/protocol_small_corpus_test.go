package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

const smallControlBatch = "validation-small-corpus-df13184f"

// Inputs, independent protocol facts and the offline generator are sealed in
// the batch. These are artificial controls derived from observed failure
// conditions, not claims that the entire upstream corpus has a semantic oracle.
func TestSmallCorpusSealedControls(t *testing.T) {
	raw, err := trafficfixture.ReadBatch(smallControlBatch, "captures/controls.json")
	require.NoError(t, err)
	var inventory struct {
		Cases []struct {
			Name, SHA256 string
			Packets      int
		}
	}
	require.NoError(t, json.Unmarshal(raw, &inventory))
	require.Len(t, inventory.Cases, 26)
	for _, c := range inventory.Cases {
		t.Run(c.Name, func(t *testing.T) {
			wire, err := trafficfixture.ReadBatch(smallControlBatch, "captures/"+c.Name+".pcap")
			require.NoError(t, err)
			require.Equal(t, c.SHA256, fmt.Sprintf("%x", sha256.Sum256(wire)))
			var reference []string
			var referenceError string
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						var mu sync.Mutex
						var events []*ProtocolEvent
						var stats ProtocolStats
						var assembly TCPReassemblyStats
						opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); events = append(events, e); mu.Unlock() }), WithOnProtocolStats(func(s ProtocolStats) { stats = s }), WithOnTCPReassemblyStats(func(s TCPReassemblyStats) { assembly = s })}
						if observe {
							opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
						}
						replayErr := ReplayPcap(bytes.NewReader(wire), opts...)
						require.Zero(t, stats.BufferedBytes)
						require.Zero(t, stats.CallbackPanics)
						require.EqualValues(t, c.Packets, assembly.CapturedPackets)
						if c.Name == "capture-truncated" {
							require.ErrorContains(t, replayErr, "capture contains truncated packets")
							require.EqualValues(t, 1, assembly.TruncatedCaptures)
						} else if c.Name == "ipv4-bad-option-length" {
							require.ErrorContains(t, replayErr, "Invalid IP option type")
							require.EqualValues(t, 1, assembly.DecodeErrors)
						} else {
							require.NoError(t, replayErr)
							require.Zero(t, assembly.TruncatedCaptures)
							require.Zero(t, assembly.DecodeErrors)
						}
						smallAssertFacts(t, c.Name, events)
						canonical := smallCanonicalEvents(t, events)
						errorText := ""
						if replayErr != nil {
							errorText = replayErr.Error()
						}
						if workers == 1 && !deferred && !observe {
							reference, referenceError = canonical, errorText
						} else {
							require.Equal(t, referenceError, errorText)
							require.Equal(t, reference, canonical, "workers=%d deferred=%v observer=%v", workers, deferred, observe)
						}
					}
				}
			}
			if c.Name == "modbus-short-prefix" || c.Name == "modbus-large-mvp" || c.Name == "modbus-large-mvp-coalesced" || c.Name == "tds-login-tail" {
				// Session framing is checked against payloads read from the ZIP
				// capture, including one-byte and coalesced delivery.
				for _, chunk := range []int{0, 1, 2, 7} {
					smallSessionChunks(t, c.Name, wire, chunk)
				}
			}
		})
	}
}

func smallAssertFacts(t *testing.T, name string, events []*ProtocolEvent) {
	t.Helper()
	decoded := func(protocol string) []*ProtocolEvent {
		var out []*ProtocolEvent
		for _, e := range events {
			if e.Protocol == protocol && e.Status != "incomplete" {
				_, err := e.GetFields()
				require.NoError(t, err, "protocol=%s status=%s offset=%d raw=%x session=%v", e.Protocol, e.Status, e.Offset, e.Raw, e.Session)
				require.Contains(t, []string{"decoded", "deferred"}, e.Status)
				out = append(out, e)
			}
		}
		return out
	}
	switch name {
	case "tds-login-queued-no-ack", "tds-login-interleaved-error", "tds-login-server-origin":
		malformed, complete := 0, 0
		for _, e := range events {
			require.NotEqual(t, true, e.Session["Logged In"])
			if e.Status == "decoded" || e.Status == "deferred" {
				complete++
				_, err := e.GetFields()
				require.NoError(t, err)
			}
			if e.Status == "malformed" {
				malformed++
				require.Equal(t, "tds", e.Protocol)
				want := "overlapping LOGIN7 requests"
				if name == "tds-login-server-origin" {
					want = "LOGIN7 must originate from the observed client"
				}
				require.Contains(t, e.Error, want)
			}
		}
		require.Equal(t, 1, malformed)
		want := 3
		if name == "tds-login-interleaved-error" {
			want = 4
		} else if name == "tds-login-server-origin" {
			want = 2
		}
		require.Equal(t, want, complete, "valid messages before rejection remain decoded")
	case "modbus-short-prefix", "modbus-large-mvp", "modbus-large-mvp-coalesced":
		es := decoded("modbus")
		require.Len(t, es, 2)
		require.Len(t, events, 2)
		for _, e := range es {
			tid := 564
			if name == "modbus-large-mvp" || name == "modbus-large-mvp-coalesced" {
				tid = 10613
			}
			require.Equal(t, tid, e.Session["Transaction ID"])
			require.Equal(t, 4, e.Session["Function Code"])
		}
		require.Equal(t, "request", es[0].Session["Role"])
		require.Equal(t, "response", es[1].Session["Role"])
		require.Equal(t, "Read Input Registers", es[1].Session["In Reply To"])
		prefix := "0234"
		if name == "modbus-large-mvp" || name == "modbus-large-mvp-coalesced" {
			prefix = "2975"
		}
		request, err := hex.DecodeString(prefix + "00000006ff0400300028")
		require.NoError(t, err)
		response, err := hex.DecodeString(prefix + "00000053ff0450")
		require.NoError(t, err)
		require.Equal(t, request, es[0].Raw)
		require.Equal(t, append(response, make([]byte, 80)...), es[1].Raw)
	case "tds-login-done-tail", "tds-login-procedure-error", "tds-login-ack-after-done", "tds-login-tail", "tds-login-no-ack", "tds-login-error-done", "tds-login-error-split", "tds-login-wrong-direction", "tds-login-ack-pending", "tds-login-ack-split":
		es := decoded("tds")
		count, total := 4, 4
		if name == "tds-login-ack-split" || name == "tds-login-error-split" {
			count, total = 5, 5
		}
		if name == "tds-login-ack-pending" || name == "tds-login-wrong-direction" {
			total = 5
		}
		require.Len(t, es, count)
		require.Len(t, events, total)
		if total > count {
			closed := events[len(events)-1]
			require.Equal(t, "incomplete", closed.Status)
			require.Equal(t, "tds", closed.Protocol)
			require.Contains(t, closed.Summary, "TDS exchange ended with unmatched requests")
			require.Equal(t, 1, closed.Session["Outstanding"])
		}
		require.Equal(t, "LOGIN7", es[2].Session["Packet Name"])
		require.Equal(t, uint32(0x74000004), es[2].Session["TDS Version"])
		require.Equal(t, "CODEX", es[2].Session["HostName"])
		fields, err := es[2].GetFields()
		require.NoError(t, err)
		require.NotNil(t, fields)

		last := es[len(es)-1]
		switch name {
		case "tds-login-done-tail", "tds-login-procedure-error", "tds-login-ack-after-done", "tds-login-no-ack", "tds-login-error-done", "tds-login-error-split":
			require.Equal(t, false, last.Session["Logged In"])
			require.Equal(t, "rejected", last.Session["Login Result"])
			require.Equal(t, "LOGIN7", last.Session["Matched Request"])
		case "tds-login-wrong-direction":
			require.Equal(t, false, last.Session["Logged In"])
			require.Equal(t, "wrong-direction", last.Session["Association Status"])
		case "tds-login-ack-pending":
			require.Equal(t, false, last.Session["Logged In"])
			require.Equal(t, true, last.Session["Outstanding"])
			require.Equal(t, "pending", last.Session["Login Result"])
		default:
			require.Equal(t, "LOGIN7", last.Session["Matched Request"])
			require.Equal(t, true, last.Session["Logged In"])
			require.Equal(t, "accepted", last.Session["Login Result"])
			if name == "tds-login-ack-split" {
				require.Equal(t, true, es[3].Session["Outstanding"])
				require.Equal(t, false, es[3].Session["Logged In"])
			} else {
				require.Equal(t, []string{"LOGINACK", "DONE"}, last.Session["Token Names"])
				tokens := last.Session["Response Tokens"].([]map[string]any)
				require.Len(t, tokens, 2)
				ack := tokens[0]["Login Acknowledgement"].(map[string]any)
				require.Equal(t, uint64(1), ack["Interface"])
				require.Equal(t, uint32(0x74000004), ack["TDS Version"])
				require.Equal(t, "TEST", ack["Program Name"].(map[string]any)["Text"])
			}
		}
	case "dns-fragmented-tcp":
		es := decoded("dns")
		require.Len(t, es, 2)
		require.Len(t, events, 2)
		for _, e := range es {
			require.Equal(t, "dns-tcp", e.Profile)
			require.EqualValues(t, 4660, e.Session["DNS"].(map[string]any)["ID"])
			dns := e.Session["DNS"].(map[string]any)
			questions := dns["Questions"].([]map[string]any)
			require.Len(t, questions, 1)
			require.Equal(t, "a", questions[0]["Name"])
			require.EqualValues(t, 1, questions[0]["Type"])
			require.EqualValues(t, 1, questions[0]["Class"])
		}
		require.Equal(t, es[0].ID, es[1].ResponseTo)
	case "enip-empty-coap-collision":
		for _, e := range events {
			require.NotEqual(t, "coap", e.Protocol)
		}
	case "ntp-client-and-control", "ipv4-eol-stale-state", "ipv4-bad-option-length":
		es := decoded("ntp")
		if name == "ipv4-eol-stale-state" {
			require.Len(t, es, 2)
			require.Len(t, events, 2)
			require.Equal(t, "192.0.2.1:40000", es[0].Source)
			require.Equal(t, "192.0.2.3:40000", es[1].Source)
		} else {
			require.Len(t, es, 1)
		}
		if name == "ipv4-bad-option-length" {
			require.Len(t, events, 1)
		}
		require.Equal(t, 3, es[0].Session["Mode"])
		for _, e := range events {
			require.NotEqual(t, "incomplete", e.Status)
		}
	case "radius-bad-attribute":
		require.Len(t, events, 1)
		e := events[0]
		require.Equal(t, "radius", e.Protocol)
		_, err := e.GetFields()
		require.Error(t, err)
		require.ErrorContains(t, err, "radius: attribute length smaller than 2")
		status, _ := classifySessionError(err)
		require.Equal(t, "malformed", status)
	case "capture-truncated":
		require.Len(t, events, 1)
		require.Empty(t, events[0].Protocol)
		require.Equal(t, "incomplete", events[0].Status)
	case "vxlan-ntp-control":
		require.Len(t, events, 1)
		require.NotEqual(t, "incomplete", events[0].Status)
	case "geneve-profile":
		require.Len(t, events, 1)
		require.Equal(t, "ntp", events[0].Protocol)
		require.Empty(t, events[0].Error)
		require.Equal(t, "192.0.2.1:40000", events[0].Source)
		require.Equal(t, "192.0.2.2:123", events[0].Destination)
		require.Contains(t, fmt.Sprint(events[0].Domain), "/geneve:192.0.2.1~192.0.2.2:1")
	case "rtp-negotiated-directions":
		require.Len(t, decoded("sip"), 2)
		es := decoded("rtp")
		require.Len(t, es, 1)
		require.Len(t, events, 3)
		require.Equal(t, "recvonly,sendonly", es[0].Session["Negotiated Direction"])
	default:
		t.Fatalf("missing independent control assertion: %s", name)
	}
}

func smallCanonicalEvents(t *testing.T, events []*ProtocolEvent) []string {
	t.Helper()
	byID := map[uint64]*ProtocolEvent{}
	order := map[string]uint64{}
	for _, e := range events {
		require.NotZero(t, e.ID)
		require.NotContains(t, byID, e.ID)
		byID[e.ID] = e
		if e.FlowID != 0 {
			k := fmt.Sprintf("%d/%d/%s/%s", e.FlowID, e.Direction, e.Protocol, e.SourceBytes.Kind)
			require.GreaterOrEqual(t, e.Offset, order[k])
			order[k] = e.Offset
		}
	}
	var result []string
	for _, e := range events {
		status, fields, semanticError, expert := e.Status, e.Fields, e.Error, e.ExpertCode
		if status == "decoded" || status == "deferred" {
			var err error
			fields, err = e.GetFields()
			status = "decoded"
			if err != nil {
				var typed *ProtocolError
				status, typed = classifySessionError(err)
				semanticError = err.Error()
				if typed != nil {
					expert = string(typed.Kind)
				}
				p, _ := e.Decode()
				fields = protocolFields(p)
			}
		}
		parent := ""
		if e.ResponseTo != 0 {
			p := byID[e.ResponseTo]
			require.NotNil(t, p)
			require.Equal(t, p.Domain, e.Domain)
			require.Equal(t, p.FlowID, e.FlowID)
			parent = fmt.Sprintf("%d/%d/%x", p.Direction, p.Offset, sha256.Sum256(p.Raw))
		}
		b, err := json.Marshal(map[string]any{"protocol": e.Protocol, "status": status, "source": e.Source, "destination": e.Destination, "direction": e.Direction, "domain": e.Domain, "offset": e.Offset, "length": e.Length, "raw": fmt.Sprintf("%x", sha256.Sum256(e.Raw)), "fields": smallLogicalReferences(t, fields, byID), "session": smallLogicalReferences(t, e.Session, byID), "error": semanticError, "expert": expert, "parent": parent})
		require.NoError(t, err)
		result = append(result, string(b))
	}
	sort.Strings(result)
	return result
}

func smallSessionChunks(t *testing.T, name string, wire []byte, chunk int) {
	t.Helper()
	r, err := pcapgo.NewReader(bytes.NewReader(wire))
	require.NoError(t, err)
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	var events []*ProtocolEvent
	for {
		b, ci, err := r.ReadPacketData()
		if err != nil {
			require.Equal(t, "EOF", err.Error())
			break
		}
		p := gopacket.NewPacket(b, r.LinkType(), gopacket.Default)
		tcp, ok := p.TransportLayer().(*layers.TCP)
		if !ok || len(tcp.Payload) == 0 {
			continue
		}
		d := 0
		if tcp.SrcPort != 40000 {
			d = 1
		}
		for b := tcp.Payload; len(b) > 0; {
			n := len(b)
			if chunk > 0 {
				n = min(n, chunk)
			}
			result := s.Feed(d, ci.Timestamp, b[:n])
			if result.Err != nil {
				require.Equal(t, ErrNeedMore, result.Err.Kind)
			}
			events = append(events, result.Events...)
			b = b[n:]
		}
	}
	s.Close("FIN")
	require.Zero(t, s.Stats().BufferedBytes)
	smallAssertFacts(t, name, events)
}

// Event IDs are capture-local allocations, not cross-worker semantic IDs.
// Resolve every retained SDP or RTSP SETUP reference to its exact observed PDU before parity
// comparison. Missing/wrong references fail; no association field is discarded.
func smallLogicalReferences(t *testing.T, value any, events map[uint64]*ProtocolEvent) any {
	t.Helper()
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			if key == "SDP Event IDs" || key == "RTSP SETUP Request Event IDs" || key == "RTSP SETUP Event IDs" {
				ids, ok := child.([]uint64)
				require.True(t, ok)
				refs, err := referencedProtocolPDUs(key, ids, events)
				require.NoError(t, err)
				out[key] = refs
			} else {
				out[key] = smallLogicalReferences(t, child, events)
			}
		}
		return out
	case []map[string]any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = smallLogicalReferences(t, v[i], events)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = smallLogicalReferences(t, v[i], events)
		}
		return out
	default:
		return value
	}
}

// Resolve, rather than remove, every association. Invalid targets must fail the
// evidence check even when both A/B sides contain the same invalid reference.
func referencedProtocolPDUs(key string, ids []uint64, events map[uint64]*ProtocolEvent) ([]string, error) {
	protocol := "rtsp"
	if key == "SDP Event IDs" {
		protocol = "sip"
	}
	refs := make([]string, 0, len(ids))
	for _, id := range ids {
		e := events[id]
		if id == 0 || e == nil || e.Protocol != protocol || len(e.Raw) == 0 {
			return nil, fmt.Errorf("invalid %s target %d", key, id)
		}
		if key == "RTSP SETUP Request Event IDs" && !bytes.HasPrefix(e.Raw, []byte("SETUP ")) {
			return nil, fmt.Errorf("RTSP request reference %d is not SETUP", id)
		}
		refs = append(refs, fmt.Sprintf("%s/%s/%v/%d/%d/%x", e.Source, e.Destination, e.Domain, e.Direction, e.Offset, sha256.Sum256(e.Raw)))
	}
	return refs, nil
}
