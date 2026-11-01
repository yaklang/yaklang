package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func espPresentationEvent() *pcaputil.ProtocolEvent {
	w := []byte{0x12, 0x34, 0x56, 0x78, 0, 0, 0, 1, 0x45, 0, 0, 20}
	return &pcaputil.ProtocolEvent{ID: 1, Protocol: "esp", Transport: "network", Profile: "esp-observed-header", Admission: "observed-IP-next-header", Entry: "ESPHeaderFields", Status: "limited", Completeness: "headers", Raw: w, Summary: "ESP clear header; SA context unavailable", SourceBytes: pcaputil.ByteSource{Kind: "captured", SHA256: fmt.Sprintf("%x", sha256.Sum256(w))}, Session: map[string]any{"SPI": uint32(0x12345678), "Sequence": uint32(1), "Header Complete": true, "Algorithm Context Available": false, "Authentication Verified": false, "Extended Sequence Reconstructed": false, "Opaque Body": bytes.Clone(w[8:]), "Observation Scope": "clear ESP header and opaque remainder; no SA, algorithm, peer authentication or replay verdict", "Wire Field Ranges": map[string]any{"SPI": []any{0, 4}, "Sequence": []any{4, 8}, "Opaque Body": []any{8, len(w)}}}}
}

func TestObservedESPHeaderIdentityAndScopeAgreeAcrossNativePacketWorkerAndStream(t *testing.T) {
	for _, version := range []int{4, 6} {
		for _, sequence := range []uint32{0, 0xffffffff} {
			t.Run(fmt.Sprintf("IPv%d/seq%d", version, sequence), func(t *testing.T) {
				w := bytes.Clone(espPresentationEvent().Raw)
				binary.BigEndian.PutUint32(w[4:8], sequence)
				wire := refusalPresentationWire(t, version, layers.IPProtocolESP, w)
				if version == 4 {
					wire = wire[:14+20+len(w)] // IP envelope excludes Ethernet padding.
				}
				ci := gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), Length: len(wire), CaptureLength: len(wire)}
				var input bytes.Buffer
				writer := pcapgo.NewWriter(&input)
				require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
				require.NoError(t, writer.WritePacket(ci, wire))
				var events []*pcaputil.ProtocolEvent
				require.NoError(t, pcaputil.ReplayPcap(bytes.NewReader(input.Bytes()), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) })))
				require.Len(t, events, 1)
				e := events[0]
				require.True(t, validESPHeaderObservation(e))
				before, err := json.Marshal(e)
				require.NoError(t, err)
				raw := &capturedPacket{number: 1, data: wire, ci: ci, link: layers.LinkTypeEthernet, events: events, presentation: presentEvents(events)}
				row := summarize(raw)
				require.Equal(t, "ESP", row.Protocol)
				require.Equal(t, []string{"Ethernet", fmt.Sprintf("IPv%d", version), "ESP"}, row.ProtocolStack)
				require.Equal(t, "limited", row.ProtocolStatus)
				require.Equal(t, "headers", row.Visibility)
				require.Contains(t, row.ProtocolEvidence, "SA context unavailable")
				require.Contains(t, row.ProtocolEvidence, "not verified")
				require.Equal(t, e.Source, row.Source)
				require.Equal(t, e.Destination, row.Destination)
				require.Empty(t, row.ConnectionProtocol)
				require.Zero(t, row.StreamID)
				for _, got := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw), workerMetadataDetail(t, events, true)} {
					require.Equal(t, "ESP", got.protocol)
					require.NotEmpty(t, got.sources)
					require.Equal(t, w, got.sources[0].Data)
					text := detailLines(got.fields)
					require.Contains(t, text, fmt.Sprintf("Sequence: %d", sequence))
					require.Contains(t, text, "Authentication Verified: false")
					require.Contains(t, text, "Extended Sequence Reconstructed: false")
					foundSPI, foundSequence := false, false
					for _, f := range got.fields {
						if strings.HasPrefix(f.text, "SPI:") && f.hasRange {
							foundSPI = true
							require.Equal(t, 1, f.source)
							require.Equal(t, 0, f.start)
							require.Equal(t, 4, f.end)
						}
						if strings.HasPrefix(f.text, "Sequence:") && f.hasRange {
							foundSequence = true
							require.Equal(t, 1, f.source)
							require.Equal(t, 4, f.start)
							require.Equal(t, 8, f.end)
						}
					}
					require.True(t, foundSPI)
					require.True(t, foundSequence)
				}
				stream := workerMetadataDetail(t, events, true)
				for _, f := range stream.fields {
					if f.hasRange {
						require.Positive(t, f.source, "stream header spans belong to the logical ESP PDU, never Source0")
					}
				}
				after, err := json.Marshal(e)
				require.NoError(t, err)
				require.Equal(t, before, after, "display projection does not upgrade or mutate the native observation")
			})
		}
	}
}

func TestESPHeaderPresentationRequiresCompleteByteBoundEvidence(t *testing.T) {
	mutations := []func(*pcaputil.ProtocolEvent){
		func(e *pcaputil.ProtocolEvent) { e.Protocol = "http" },
		func(e *pcaputil.ProtocolEvent) { e.Transport = "udp" },
		func(e *pcaputil.ProtocolEvent) { e.Profile = "" },
		func(e *pcaputil.ProtocolEvent) { e.Admission = "port-hint" },
		func(e *pcaputil.ProtocolEvent) { e.Status = "decoded" },
		func(e *pcaputil.ProtocolEvent) { e.Completeness = "message" },
		func(e *pcaputil.ProtocolEvent) { e.ExpertCode = "NeedMore" },
		func(e *pcaputil.ProtocolEvent) { e.Error = "parent profile unavailable" },
		func(e *pcaputil.ProtocolEvent) { e.Raw = e.Raw[:7] },
		func(e *pcaputil.ProtocolEvent) { e.Raw[0] ^= 0xff },
		func(e *pcaputil.ProtocolEvent) { e.SourceBytes.SHA256 = strings.Repeat("0", 64) },
		func(e *pcaputil.ProtocolEvent) {
			e.Raw[4] ^= 1
			e.SourceBytes.SHA256 = fmt.Sprintf("%x", sha256.Sum256(e.Raw))
		},
		func(e *pcaputil.ProtocolEvent) { e.Session["SPI"] = uint32(0) },
		func(e *pcaputil.ProtocolEvent) { e.Session["SPI"] = "305419896" },
		func(e *pcaputil.ProtocolEvent) { e.Session["SPI"] = float64(305419896.5) },
		func(e *pcaputil.ProtocolEvent) { e.Session["Sequence"] = uint64(1) << 32 },
		func(e *pcaputil.ProtocolEvent) { e.Session["Sequence"] = false },
		func(e *pcaputil.ProtocolEvent) { delete(e.Session, "Sequence") },
		func(e *pcaputil.ProtocolEvent) { e.Session["Header Complete"] = "true" },
		func(e *pcaputil.ProtocolEvent) { e.Session["Algorithm Context Available"] = true },
		func(e *pcaputil.ProtocolEvent) { e.Session["Authentication Verified"] = "false" },
		func(e *pcaputil.ProtocolEvent) { delete(e.Session, "Extended Sequence Reconstructed") },
		func(e *pcaputil.ProtocolEvent) { e.Session["Observation Scope"] = "authenticated decrypted session" },
		func(e *pcaputil.ProtocolEvent) {
			e.Session["Opaque Body"] = base64.StdEncoding.EncodeToString(e.Raw[8:])
		},
		func(e *pcaputil.ProtocolEvent) { e.Session["Opaque Body"] = []byte{0x45} },
		func(e *pcaputil.ProtocolEvent) { e.Session = nil },
	}
	for i, mutate := range mutations {
		e := espPresentationEvent()
		mutate(e)
		require.False(t, validESPHeaderObservation(e), "control %d", i)
		p := presentEvents([]*pcaputil.ProtocolEvent{e})
		require.NotEqual(t, "headers", p.visibility, "an incomplete or spoofed observation cannot gain the header contract, control %d", i)
		if e.Status == "limited" {
			require.Empty(t, eventProtocols([]*pcaputil.ProtocolEvent{e}), "control %d", i)
		}
	}
	require.False(t, validESPHeaderObservation(nil))
}

func TestESPHeaderPresentationAcceptsEmptyOpaqueBodyAndExactIPCFacts(t *testing.T) {
	e := espPresentationEvent()
	e.Raw = bytes.Clone(e.Raw[:8])
	e.Session["Opaque Body"] = []byte{}
	e.Session["Sequence"] = uint32(0)
	binary.BigEndian.PutUint32(e.Raw[4:8], 0)
	e.SourceBytes.SHA256 = fmt.Sprintf("%x", sha256.Sum256(e.Raw))
	e.Session["Wire Field Ranges"] = map[string]any{"SPI": []any{0, 4}, "Sequence": []any{4, 8}, "Opaque Body": []any{8, 8}}
	require.True(t, validESPHeaderObservation(e), "unknown ESN policy permits an observed low sequence word of zero")
	for _, stream := range []bool{false, true} {
		got := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, stream)
		require.Equal(t, "ESP", got.protocol)
		require.Equal(t, e.Raw, got.sources[0].Data)
	}
	// A manual JSON request supplies no parent-owned byte-type facts. An
	// arbitrary base64 text must not become confirmation of the native body.
	request := decodeRequest{Number: 1, Stream: &streamDecodeRequest{ID: 1, Events: []*pcaputil.ProtocolEvent{e}}}
	wire, err := json.Marshal(request)
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(wire), &output))
	response, err := decodeWorkerOutput(output.Bytes(), &capturedPacket{number: 1})
	require.NoError(t, err)
	require.Empty(t, response.protocol)
}
