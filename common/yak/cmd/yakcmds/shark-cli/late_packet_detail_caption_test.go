package sharkcli

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestLateHTTPDetailCaptionUsesItsOwnRetainedFields(t *testing.T) {
	for _, mode := range []string{"complete", "incomplete-header", "other-packet", "other-interface", "unvalidated-prefix"} {
		t.Run(mode, func(t *testing.T) {
			// Model a captured carrier caption frozen before application EOF.
			// The real TLS 1.2/1.3 late-close captures exercise the same transition
			// independently; this unit isolates history ownership and UI state.
			raw := tcpPacket(t, 12, 100, string(append([]byte{23, 3, 3, 0, 17}, make([]byte, 17)...)), true, false, false, false)
			raw.presentation = packetPresentation{protocol: "TLSv1.3"}
			before := bytes.Clone(raw.data)
			history, err := pcaputil.NewProtocolInspector(16, 1<<20)
			require.NoError(t, err)
			raw.history, raw.reference = history, pcaputil.PacketReference{Number: 12}
			require.Equal(t, "TLSv1.3", quickDetail(raw).protocol)
			session, err := pcaputil.NewProtocolSession(pcaputil.DefaultParserBudget())
			require.NoError(t, err)
			session.Feed(0, time.Unix(1, 0), []byte("GET / HTTP/1.1\r\nHost: owned.invalid\r\n\r\n"))
			response := "HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\nbody"
			if mode == "incomplete-header" {
				response = "HTTP/1.1 200 OK\r\nConnection: close\r\n\r\nbody"
			}
			result := session.Feed(1, time.Unix(2, 0), []byte(response))
			events := append(result.Events, session.Close("TLS carrier closed")...)
			require.Len(t, events, 1)
			e := events[0]
			e.SourceBytes.Kind = "decrypted"
			e.SourceBytes.PacketRefs = []pcaputil.PacketReference{raw.reference}
			switch mode {
			case "other-packet":
				e.SourceBytes.PacketRefs[0].Number++
			case "other-interface":
				e.SourceBytes.PacketRefs[0].Domain.Interface++
			case "unvalidated-prefix":
				e = &pcaputil.ProtocolEvent{ID: 7, Protocol: "http", Transport: "tcp", Status: "incomplete", Raw: []byte("HTTP/1.1 200"), SourceBytes: pcaputil.ByteSource{Kind: "decrypted", PacketRefs: []pcaputil.PacketReference{raw.reference}}}
			}
			history.OnEvent(e)
			entry := packetEntry{packet: raw}
			want := "TLSv1.3"
			if mode == "complete" || mode == "incomplete-header" {
				want = "HTTP/1.1"
			}
			require.Equal(t, want, entry.row().Protocol)
			for _, d := range []packetDetail{quickDetail(raw), detail(raw)} {
				require.Equal(t, want, d.protocol, "caption and fields must use the same retained observation")
				if want == "HTTP/1.1" {
					require.Contains(t, detailLines(d.fields), "200 OK")
				}
			}
			require.Equal(t, before, raw.data)
		})
	}
}

func TestTLSCCSDetailCaptionKeepsObservedConnectionEvidence(t *testing.T) {
	for _, mode := range []string{"TLS", "TLSv1.2", "TLSv1.3", "port-hint", "no-context", "other-packet"} {
		t.Run(mode, func(t *testing.T) {
			wire := []byte{20, 3, 3, 0, 1, 1}
			raw := tcpPacket(t, 12, 100, string(wire), true, false, false, false)
			before := bytes.Clone(raw.data)
			history, err := pcaputil.NewProtocolInspector(16, 1<<20)
			require.NoError(t, err)
			raw.history, raw.reference = history, pcaputil.PacketReference{Number: 12}
			e := &pcaputil.ProtocolEvent{ID: 7, Transport: "tcp", Protocol: "tls", Status: "decoded", Raw: bytes.Clone(wire), SourceBytes: pcaputil.ByteSource{Kind: "reassembled", PacketRefs: []pcaputil.PacketReference{raw.reference}}, Session: map[string]any{"Record Type": byte(20), "Record Version": uint16(0x303), "Record Length": 1, "Content Visibility": "cleartext", "Authentication Verified": false}}
			if mode != "no-context" && mode != "port-hint" {
				label := mode
				if mode == "other-packet" {
					label = "TLS"
					e.SourceBytes.PacketRefs[0].Number++
				}
				raw.application, raw.evidence, raw.connectionStack = label, "validated protocol events", []string{label}
			} else if mode == "port-hint" {
				raw.application, raw.evidence = "TLS", "port hint"
			}
			history.OnEvent(e)
			entry := packetEntry{packet: raw}
			want := entry.row().Protocol
			if mode == "TLS" || mode == "TLSv1.2" || mode == "TLSv1.3" {
				require.Equal(t, mode, want)
			} else if mode == "no-context" || mode == "port-hint" {
				require.Equal(t, "TLS?", want)
			}
			request, err := marshalDecodeRequest(packetDecodeRequest(raw))
			require.NoError(t, err)
			var output bytes.Buffer
			require.NoError(t, runDecodeWorker(bytes.NewReader(request), &output))
			worker, err := decodeWorkerOutput(output.Bytes(), raw)
			require.NoError(t, err)
			for _, d := range []packetDetail{quickDetail(raw), detail(raw), worker} {
				require.Equal(t, want, d.protocol, "a six-byte CCS uses observed connection evidence without claiming negotiation or record authentication")
			}
			require.Equal(t, before, raw.data)
			require.False(t, e.Session["Authentication Verified"] == true)
		})
	}
}
