package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func socks5ConnectSteps() []sessionStep {
	return []sessionStep{
		{0, []byte{5, 1, 0}},
		{1, []byte{5, 0}},
		{0, []byte{5, 1, 0, 1, 127, 0, 0, 1, 0, 80}},
		{1, []byte{5, 0, 0, 1, 127, 0, 0, 1, 4, 56}},
		{0, []byte("GET /health HTTP/1.1\r\nHost: example.test\r\nContent-Length: 0\r\n\r\n")},
		{1, []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")},
	}
}

func runSOCKS5SessionNames(t *testing.T, steps []sessionStep, chunk int) []string {
	t.Helper()
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	var names []string
	for _, step := range steps {
		for wire := step.wire; len(wire) > 0; {
			n := len(wire)
			if chunk > 0 {
				n = min(n, chunk)
			}
			r := s.Feed(step.dir, ts, wire[:n])
			for _, event := range r.Events {
				if event.Status == "decoded" {
					names = append(names, event.Protocol+":"+event.Entry)
				}
			}
			wire = wire[n:]
		}
	}
	s.Close("FIN")
	return names
}

func TestSOCKS5ProbeUsesWireEvidenceWithoutPortHints(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Equal(t, ProbeNeedMore, s.Probe([]byte{5}).Verdict)
	p := s.Probe([]byte{5, 1, 0})
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "socks5", p.Protocol)
	for _, malformed := range [][]byte{{5, 0}, {5, 1, 0xff}, {5, 1, 9}, {5, 2, 0, 0}} {
		p := s.Probe(malformed)
		require.NotEqual(t, ProbeAccept, p.Verdict, "% x", malformed)
		require.NotEqual(t, "socks5", p.Protocol, "% x", malformed)
	}
}

func TestProtocolSessionSOCKS5ConnectTunnelAndFragmentation(t *testing.T) {
	steps := socks5ConnectSteps()
	assertFragmentation(t, steps, func(chunk int) []string { return runSOCKS5SessionNames(t, steps, chunk) })
	require.Equal(t, []string{
		"socks5:ClientNegotiation",
		"socks5:ServerNegotiation",
		"socks5:Request",
		"socks5:Replies",
		"http:HTTPExact",
		"http:HTTPExact",
	}, runSOCKS5SessionNames(t, steps, 0))
}
