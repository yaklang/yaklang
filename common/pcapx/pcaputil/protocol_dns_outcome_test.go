package pcaputil

import (
	"encoding/binary"
	"fmt"

	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func TestDNSTCPFinalOutcomeStats(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		maxItems     int
	}{
		{"short-a-rdata", "malformed", 0},
		{"valid-a-rdata", "decoded", 0},
		{"two-questions", "decoded", 0},
		{"two-questions", "limited", 1},
	} {
		wire, err := trafficfixture.ReadFile("../../bin-parser/testdata/protocol-recognition-regressions/dns/" + tc.name + ".tcp.bin")
		require.NoError(t, err)
		for _, deferred := range []bool{false, true} {
			for _, chunk := range []int{0, 1, 7} {
				t.Run(fmt.Sprintf("%s/%s/deferred=%t/chunk=%d", tc.name, tc.status, deferred, chunk), func(t *testing.T) {
					s, err := NewProtocolSession(ParserBudget{MaxCollectionElements: tc.maxItems})
					require.NoError(t, err)
					defer s.Close("FIN")
					f := s.(*captureSession).f
					f.ports = [2]uint16{40000, 53}
					f.a.config.Deferred = deferred
					var events []*ProtocolEvent
					step := chunk
					if step == 0 {
						step = len(wire)
					}
					for off := 0; off < len(wire); off += step {
						r := s.Feed(1, time.Unix(1, 0), wire[off:min(off+step, len(wire))])
						events = append(events, r.Events...)
					}
					require.Len(t, events, 1)
					e := events[0]
					status := tc.status
					if deferred && status == "decoded" {
						status = "deferred"
					}
					require.Equal(t, "dns", e.Protocol)
					require.Equal(t, status, e.Status, "%s", e.Error)
					stats := s.Stats()
					require.EqualValues(t, 1, stats.Messages)
					switch status {
					case "decoded":
						require.EqualValues(t, 1, stats.Decoded)
						require.Zero(t, stats.Deferred+stats.Malformed+stats.LimitedBytes)
					case "deferred":
						require.EqualValues(t, 1, stats.Deferred)
						require.Zero(t, stats.Decoded+stats.Malformed+stats.LimitedBytes)
					case "malformed":
						require.Contains(t, e.Error, "DNS address length")
						require.EqualValues(t, 1, stats.Malformed)
						require.Zero(t, stats.Decoded+stats.Deferred+stats.LimitedBytes)
					case "limited":
						require.Equal(t, string(ErrResourceExceeded), e.ExpertCode)
						require.EqualValues(t, len(wire), stats.LimitedBytes)
						require.Zero(t, stats.Decoded+stats.Deferred+stats.Malformed)
					}
				})
			}
		}
	}
}

func TestDNSTCPOutcomeCaptureReplay(t *testing.T) {
	for _, name := range []string{"short-a-rdata", "valid-a-rdata"} {
		wire, err := trafficfixture.ReadFile("../../bin-parser/testdata/protocol-recognition-regressions/dns/" + name + ".tcp.bin")
		require.NoError(t, err)
		// Wrap the same specification-derived message in real Ethernet/IP/TCP
		// framing so worker replay and direct Feed use the same outcome counts.
		capture := binTestPcap(t, []tcpStep{
			{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true},
			{seq: 1, data: string(wire[:7]), reverse: true},
			{seq: 8, data: string(wire[7:]), reverse: true},
		}, 53, false, true)
		for _, deferred := range []bool{false, true} {
			for _, workers := range []int{1, 4} {
				t.Run(fmt.Sprintf("%s/deferred=%t/workers=%d", name, deferred, workers), func(t *testing.T) {
					events, stats, err := binReplay(t, capture, workers, WithProtocolDeferred(deferred))
					require.NoError(t, err)
					require.Len(t, events, 1)
					require.Zero(t, stats.BufferedBytes)
					if name == "short-a-rdata" {
						require.Equal(t, "malformed", events[0].Status)
						require.EqualValues(t, 1, stats.Malformed)
						require.Zero(t, stats.Decoded+stats.Deferred)
					} else {
						require.Empty(t, events[0].Error)
						require.EqualValues(t, 1, stats.Decoded+stats.Deferred)
						require.Zero(t, stats.Malformed)
					}
				})
			}
		}
	}
}

// Reject before entering the rule VM: a native collection limit must not spend
// its CPU budget replaying the same rejected message through another parser.
type dnsBudgetRuleProbe struct {
	stream_parser.DefParser
	calls  int
	reject bool
}

func (p *dnsBudgetRuleProbe) OnRoot(node *base.Node) error {
	p.calls++
	if p.reject {
		return fmt.Errorf("unexpected DNS rule parse")
	}
	return p.DefParser.OnRoot(node)
}

func TestDNSTCPBudgetRejectsBeforeRuleDecode(t *testing.T) {
	original := base.ParserRegistration("default")
	defer base.RegisterParser("default", original)
	probe := &dnsBudgetRuleProbe{}
	base.RegisterParser("default", probe)
	txt := caaDNSControl(make([]byte, 4097))
	binary.BigEndian.PutUint16(txt[13:], 16)
	binary.BigEndian.PutUint16(txt[19:], 16)
	txt = append(binary.BigEndian.AppendUint16(nil, uint16(len(txt))), txt...)
	records, err := trafficfixture.ReadFile("../../bin-parser/testdata/protocol-recognition-regressions/dns/two-questions.tcp.bin")
	require.NoError(t, err)
	feed := func(t *testing.T, wire []byte, limit int) (*ProtocolEvent, ProtocolStats) {
		t.Helper()
		s, err := NewProtocolSession(ParserBudget{MaxCollectionElements: limit})
		require.NoError(t, err)
		s.(*captureSession).f.ports = [2]uint16{40000, 53}
		r := s.Feed(1, time.Unix(1, 0), wire)
		require.Len(t, r.Events, 1)
		s.Close("EOF")
		require.Zero(t, s.Stats().BufferedBytes)
		return r.Events[0], s.Stats()
	}
	for _, tc := range []struct {
		name  string
		wire  []byte
		limit int
	}{
		{"txt-count", txt, 4096}, {"record-count", records, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe.calls, probe.reject = 0, true
			e, stats := feed(t, tc.wire, tc.limit)
			require.Equal(t, "limited", e.Status)
			require.Equal(t, string(ErrResourceExceeded), e.ExpertCode)
			require.Zero(t, probe.calls, "resource rejection invoked rule parser")
			require.Nil(t, e.Fields)
			require.Nil(t, e.Structured)
			require.Zero(t, stats.Decoded+stats.Deferred+stats.Malformed)
			require.EqualValues(t, len(tc.wire), stats.LimitedBytes)
			require.Equal(t, tc.wire, e.Raw)
			probe.calls, probe.reject = 0, false
			normal, stats := feed(t, records, 4096)
			require.Equal(t, "decoded", normal.Status, "%s", normal.Error)
			require.Positive(t, probe.calls, "valid message bypassed custom parser")
			require.NotEmpty(t, normal.Fields)
			require.EqualValues(t, 1, stats.Decoded)
			require.Zero(t, stats.LimitedBytes+stats.Malformed)
		})
	}
}
