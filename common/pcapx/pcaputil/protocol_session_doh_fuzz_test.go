package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Mutated DNS belongs to one valid HTTP/2 stream. An application error on that
// stream must not poison a second, independent DoH exchange or leak its buffers.
func FuzzDoHHTTP2StreamIsolation(f *testing.F) {
	for _, payload := range [][]byte{
		dnsWire(dnsAResponse(0, "one.example", [4]byte{1, 2, 3, 4})),
		dnsWire(dnsQuery(0, "one.example", 1)),
		{0, 0, 0x81, 0x82, 0, 0, 0, 0, 0, 0, 0, 0},
		{0xff},
	} {
		f.Add(byte(0), payload)
		f.Add(byte(1), payload)
	}
	f.Fuzz(func(t *testing.T, split byte, payload []byte) {
		if len(payload) > 4096 {
			return
		}
		budget := DefaultParserBudget()
		budget.MaxFrameBytes = 16384
		budget.MaxMessageBytes = 32768
		budget.MaxBufferedBytes = 1 << 20
		budget.MaxCollectionElements = 128
		session, err := NewProtocolSession(budget)
		require.NoError(t, err)
		s := session.(*captureSession)
		defer session.Close("FIN")
		steps := dohExchangeH2Start()
		steps = append(steps, dohExchangeH2GET(t, 1, "one.example"), dohExchangeH2GET(t, 3, "two.example"))
		steps = append(steps,
			sessionStep{1, h2TestFrame(1, 4, 1, h2TestHeaders(t, ":status", "200", "content-type", "application/dns-message"))},
			sessionStep{1, h2TestFrame(0, 1, 1, payload)},
		)
		steps = append(steps, dohExchangeH2Response(t, 3, "two.example")...)
		chunk := []int{0, 1, 7, 64}[int(split)%4]
		matched := false
		for _, step := range steps {
			wire := step.wire
			for len(wire) > 0 {
				n := len(wire)
				if chunk > 0 && n > chunk {
					n = chunk
				}
				result := session.Feed(step.dir, time.Unix(1, 0), wire[:n])
				for _, event := range result.Events {
					if event.Session["Stream ID"] == uint32(3) && event.Session["Packet Name"] == "Response" && event.Session["Matched Request"] == "two.example" {
						require.Empty(t, event.Error)
						matched = true
					}
				}
				require.LessOrEqual(t, s.f.a.buffered.Load(), int64(s.f.a.config.MaxBufferedBytes))
				wire = wire[n:]
			}
		}
		require.True(t, matched, "malformed DNS on stream 1 must not poison stream 3")
		session.Close("FIN")
		require.Zero(t, s.f.a.buffered.Load())
		require.LessOrEqual(t, s.f.a.peak.Load(), int64(s.f.a.config.MaxBufferedBytes))
	})
}
