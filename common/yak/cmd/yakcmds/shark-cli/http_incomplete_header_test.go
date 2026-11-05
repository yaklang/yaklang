package sharkcli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func httpIncompleteHeaderFixture(t *testing.T) *pcaputil.ProtocolEvent {
	t.Helper()
	s, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
	require.NoError(t, err)
	s.Feed(0, time.Unix(1, 0), []byte("GET / HTTP/1.1\r\nHost: owned.invalid\r\n\r\n"))
	r := s.Feed(1, time.Unix(2, 0), []byte("HTTP/1.1 200 OK\r\nContent-Length: 8\r\nX-Repeat: one\r\nX-Repeat: two\r\n\r\nbody"))
	require.Empty(t, r.Events)
	events := s.Close("TLS carrier closed")
	require.Len(t, events, 1)
	e := events[0]
	e.Fields, err = e.GetFields()
	require.NoError(t, err)
	return e
}

func TestHTTPIncompleteHeaderProgressiveOwnedBytesAndWorkerIdentity(t *testing.T) {
	e := httpIncompleteHeaderFixture(t)
	p := presentEvents([]*pcaputil.ProtocolEvent{e})
	require.Equal(t, "HTTP/1.1", p.protocol)
	require.Equal(t, "incomplete", p.status)
	require.Equal(t, "headers", p.visibility)
	require.Contains(t, p.summary, "header observed · message incomplete")
	packet := detail(&capturedPacket{number: 1, events: []*pcaputil.ProtocolEvent{e}})
	worker := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false)
	require.Equal(t, packet.fields, worker.fields)
	require.Equal(t, packet.sources, worker.sources)
	for _, d := range []packetDetail{packet, worker, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true), decodeStream(streamDecodeRequest{ID: 1, Events: []*pcaputil.ProtocolEvent{e}})} {
		require.Equal(t, "HTTP/1.1", d.protocol)
		text := detailLines(d.fields)
		require.Contains(t, text, "Message incomplete · exact body boundary not reached")
		require.Contains(t, text, "no complete-message or response association inferred")
		seen := map[string]bool{}
		for _, f := range d.fields {
			if f.text == "X-Repeat: one" || f.text == "X-Repeat: two" {
				require.True(t, f.hasRange)
				require.Positive(t, f.source)
				require.Equal(t, f.text, string(d.sources[f.source-1].Data[f.start:f.end]))
				seen[f.text] = true
			}
			if strings.HasPrefix(f.text, "Observed bytes after header:") {
				require.True(t, f.hasRange)
				require.Equal(t, []byte("body"), d.sources[f.source-1].Data[f.start:f.end])
				seen["unfinished-bytes"] = true
			}
			if strings.HasPrefix(f.text, "Message incomplete") || strings.HasPrefix(f.text, "Diagnostic:") {
				require.False(t, f.hasRange)
			}
		}
		require.Len(t, seen, 3)
		require.True(t, bytes.Equal(e.Raw[len(e.Raw)-4:], []byte("body")))
	}
}

func TestHTTPIncompleteHeaderRejectsContradictoryPublicMetadata(t *testing.T) {
	for _, mode := range []string{"status", "complete", "association", "hash", "header-length", "truncated-header", "source-kind"} {
		t.Run(mode, func(t *testing.T) {
			e := httpIncompleteHeaderFixture(t)
			switch mode {
			case "status":
				e.Status = "decoded"
			case "complete":
				e.Session["Message Complete"] = true
			case "association":
				e.ResponseTo = 123
			case "hash":
				e.Session["Observed Header SHA256"] = strings.Repeat("0", 64)
			case "header-length":
				e.Session["Header Bytes"] = len(e.Raw)
			case "truncated-header":
				e.Raw = e.Raw[:8]
			case "source-kind":
				e.SourceBytes.Kind = "captured"
			}
			require.Nil(t, observedHTTPIncompleteHeader(e))
			p := presentEvents([]*pcaputil.ProtocolEvent{e})
			require.NotEqual(t, "HTTP/1.1", p.protocol)
		})
	}
}

func TestHTTPIncompleteGapContextPrecedesHeaderWithoutByteRanges(t *testing.T) {
	e := httpIncompleteHeaderFixture(t)
	e.SourceBytes.Kind = "decrypted"
	e.Session["TLS Plaintext Gap"] = true
	e.Session["TLS Gap Cause PDU"] = uint64(123)
	e.Session["TLS Gap Direction"] = e.Direction
	e.Session["TLS Gap Version"] = uint16(0x304)
	for _, d := range []packetDetail{detail(&capturedPacket{number: 1, events: []*pcaputil.ProtocolEvent{e}}), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), decodeStream(streamDecodeRequest{ID: 1, Events: []*pcaputil.ProtocolEvent{e}})} {
		text := detailLines(d.fields)
		require.Contains(t, text, "TLS Plaintext Gap: true")
		require.Contains(t, text, "TLS Gap Cause PDU: 123")
		require.Contains(t, text, "TLS Gap Version: TLS 1.3 (0x0304)")
		gap, header := -1, -1
		for i, f := range d.fields {
			if f.text == "TLS Gap Cause PDU: 123" {
				gap = i
			}
			if f.text == "Header" {
				header = i
			}
			if strings.HasPrefix(f.text, "TLS ") {
				require.False(t, f.hasRange)
				require.Zero(t, f.source)
			}
		}
		require.GreaterOrEqual(t, gap, 0)
		require.Greater(t, header, gap)
	}
	e.SourceBytes.Kind = "reassembled"
	require.NotContains(t, detailLines(detail(&capturedPacket{number: 1, events: []*pcaputil.ProtocolEvent{e}}).fields), "TLS Gap Cause PDU:")
}
