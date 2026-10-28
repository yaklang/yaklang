package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
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

func espIncompletePresentationEvent(w []byte) *pcaputil.ProtocolEvent {
	e := espPresentationEvent()
	e.Raw = bytes.Clone(w)
	e.Status, e.Completeness, e.ExpertCode = "incomplete", "incomplete", "NeedMore"
	e.Error = "NeedMore: ESP capture is incomplete; only captured clear-header facts are available"
	e.Session["Header Complete"] = len(w) >= 8
	delete(e.Session, "SPI")
	delete(e.Session, "Sequence")
	delete(e.Session, "Opaque Body")
	ranges := map[string]any{}
	if len(w) >= 4 {
		e.Session["SPI"], ranges["SPI"] = binary.BigEndian.Uint32(w[:4]), []any{0, 4}
	}
	if len(w) >= 8 {
		e.Session["Sequence"], ranges["Sequence"] = binary.BigEndian.Uint32(w[4:8]), []any{4, 8}
		e.Session["Opaque Body"], ranges["Opaque Body"] = bytes.Clone(w[8:]), []any{8, len(w)}
	}
	e.Session["Wire Field Ranges"] = ranges
	e.SourceBytes.SHA256 = ""
	if len(w) > 0 {
		e.SourceBytes.SHA256 = fmt.Sprintf("%x", sha256.Sum256(w))
	}
	return e
}

func espIncompleteNativePacket(t *testing.T, version int, w []byte, declared int) *capturedPacket {
	t.Helper()
	payload, next := bytes.Clone(w), layers.IPProtocolESP
	if version == 6 {
		// The complete Destination/HBH shape used by the actual zero-byte
		// witnesses keeps IPv6 Payload Length nonzero without a jumbo profile.
		payload, next = append([]byte{50, 0, 0, 0, 0, 0, 0, 0}, payload...), layers.IPProtocolIPv6Destination
	}
	wire := refusalPresentationWire(t, version, next, payload)
	size := 20
	if version == 6 {
		size = 40
	}
	wire = wire[:14+size+len(payload)]
	if declared > 0 {
		if version == 4 {
			binary.BigEndian.PutUint16(wire[16:18], uint16(declared))
			wire[24], wire[25] = 0, 0
			var sum uint32
			for at := 14; at < 34; at += 2 {
				sum += uint32(binary.BigEndian.Uint16(wire[at : at+2]))
			}
			for sum > 0xffff {
				sum = sum&0xffff + sum>>16
			}
			binary.BigEndian.PutUint16(wire[24:26], ^uint16(sum))
		} else {
			binary.BigEndian.PutUint16(wire[18:20], uint16(declared))
		}
	}
	ci := gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), Length: len(wire), CaptureLength: len(wire)}
	var input bytes.Buffer
	writer := pcapgo.NewWriter(&input)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	require.NoError(t, writer.WritePacket(ci, wire))
	var events []*pcaputil.ProtocolEvent
	require.NoError(t, pcaputil.ReplayPcap(bytes.NewReader(input.Bytes()), pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete: true}), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) { events = append(events, e) })))
	require.Len(t, events, 1)
	require.True(t, validESPIncompleteObservation(events[0]))
	return &capturedPacket{number: 1, data: wire, ci: ci, link: layers.LinkTypeEthernet, events: events, presentation: presentEvents(events)}
}

func assertESPIncompleteInspectors(t *testing.T, raw *capturedPacket, w []byte) {
	t.Helper()
	row := summarize(raw)
	require.Equal(t, "ESP", row.Protocol)
	require.Equal(t, "incomplete", row.ProtocolStatus)
	require.Equal(t, "partial-header", row.Visibility)
	require.Equal(t, 1, countString(row.ProtocolStack, "ESP"))
	require.NotContains(t, row.ProtocolStack, "IPSecESP")
	require.NotContains(t, row.Info, "[incomplete/undecoded]")
	require.NotContains(t, row.Info, "truncated capture", "declared IP extent failure alone does not prove capture loss")
	require.Contains(t, row.ProtocolEvidence, "no complete ESP PDU")
	require.Empty(t, row.ConnectionProtocol)
	require.Zero(t, row.StreamID)
	if len(w) < 8 {
		require.Contains(t, row.Info, fmt.Sprintf("%d/8 bytes observed", len(w)))
		require.Equal(t, "declared ESP; clear header incomplete", row.ProtocolScope)
	} else {
		require.Equal(t, "ESP clear header observed; declared carrier extent incomplete", row.ProtocolScope)
	}
	for _, got := range []packetDetail{detail(raw), quickDetail(raw), workerMetadataDetailForPacket(t, raw), workerMetadataDetail(t, raw.events, true)} {
		require.Equal(t, "ESP", got.protocol)
		if len(w) == 0 {
			require.Empty(t, got.sources)
		} else {
			require.Equal(t, w, got.sources[0].Data)
		}
		spiRange, sequenceRange := false, false
		for _, f := range got.fields {
			if strings.HasPrefix(f.text, "SPI:") && f.hasRange {
				spiRange = true
				require.Equal(t, 1, f.source)
				require.Equal(t, 0, f.start)
				require.Equal(t, 4, f.end)
			}
			if strings.HasPrefix(f.text, "Sequence:") && f.hasRange {
				sequenceRange = true
				require.Equal(t, 1, f.source)
				require.Equal(t, 4, f.start)
				require.Equal(t, 8, f.end)
			}
		}
		require.Equal(t, len(w) >= 4, spiRange)
		require.Equal(t, len(w) >= 8, sequenceRange)
	}
	stream := workerMetadataDetail(t, raw.events, true)
	for _, f := range stream.fields {
		if f.hasRange {
			require.Positive(t, f.source, "an incomplete stream observation has no captured Source0")
		}
	}
}

func countString(values []string, want string) int {
	n := 0
	for _, value := range values {
		if value == want {
			n++
		}
	}
	return n
}

func TestESPIncompleteClearHeaderAndCarrierExtentAcrossInspectors(t *testing.T) {
	w := espPresentationEvent().Raw
	for _, version := range []int{4, 6} {
		for cut := 0; cut < 8; cut++ {
			t.Run(fmt.Sprintf("IPv%d/cut%d", version, cut), func(t *testing.T) {
				assertESPIncompleteInspectors(t, espIncompleteNativePacket(t, version, w[:cut], 0), w[:cut])
			})
		}
		for _, prefix := range [][]byte{w[:8], w} {
			t.Run(fmt.Sprintf("IPv%d/headerComplete%d", version, len(prefix)), func(t *testing.T) {
				assertESPIncompleteInspectors(t, espIncompleteNativePacket(t, version, prefix, 1024), prefix)
			})
		}
	}
	// These public declarations are reproduced with authored carriers, not
	// embedded captures: Sharon249918 has one ESP byte; 231009/277417 each
	// have two complete words while their declared IPv4 extent is unavailable.
	for _, c := range []struct {
		name     string
		spi, seq uint32
		declared int
	}{
		{"Sharon231009", 1111515648, 859009793, 61953},
		{"Sharon277417", 4128768, 4280745983, 8192},
		{"low-sequence-zero", 0x12345678, 0, 1024},
	} {
		t.Run(c.name, func(t *testing.T) {
			prefix := make([]byte, 8)
			binary.BigEndian.PutUint32(prefix[:4], c.spi)
			binary.BigEndian.PutUint32(prefix[4:8], c.seq)
			assertESPIncompleteInspectors(t, espIncompleteNativePacket(t, 4, prefix, c.declared), prefix)
		})
	}
	t.Run("Sharon249918-shape", func(t *testing.T) {
		prefix := []byte{0xc4}
		assertESPIncompleteInspectors(t, espIncompleteNativePacket(t, 4, prefix, 65535), prefix)
	})
}

func TestESPIncompletePresentationRejectsContradictoryPartialFacts(t *testing.T) {
	w := espPresentationEvent().Raw
	mutations := []func(*pcaputil.ProtocolEvent){
		func(e *pcaputil.ProtocolEvent) { e.Status = "limited" },
		func(e *pcaputil.ProtocolEvent) { e.Completeness = "headers" },
		func(e *pcaputil.ProtocolEvent) { e.ExpertCode = "MalformedMessage" },
		func(e *pcaputil.ProtocolEvent) { e.Error = "NeedMore: unrelated parser failure" },
		func(e *pcaputil.ProtocolEvent) { e.Protocol = "tls" },
		func(e *pcaputil.ProtocolEvent) { e.Profile = "" },
		func(e *pcaputil.ProtocolEvent) { e.Admission = "port-hint" },
		func(e *pcaputil.ProtocolEvent) { e.Transport = "udp" },
		func(e *pcaputil.ProtocolEvent) { delete(e.Session, "Header Complete") },
		func(e *pcaputil.ProtocolEvent) { e.Session["Header Complete"] = len(e.Raw) < 8 },
		func(e *pcaputil.ProtocolEvent) { e.Session["Authentication Verified"] = true },
		func(e *pcaputil.ProtocolEvent) { delete(e.Session, "Algorithm Context Available") },
		func(e *pcaputil.ProtocolEvent) { e.Session["Extended Sequence Reconstructed"] = "false" },
		func(e *pcaputil.ProtocolEvent) { e.Session["Observation Scope"] = "complete authenticated session" },
		func(e *pcaputil.ProtocolEvent) { delete(e.Session, "SPI") },
		func(e *pcaputil.ProtocolEvent) { e.Session["SPI"] = "305419896" },
		func(e *pcaputil.ProtocolEvent) { e.Session["SPI"] = uint32(0) },
		func(e *pcaputil.ProtocolEvent) { e.SourceBytes.SHA256 = "" },
		func(e *pcaputil.ProtocolEvent) {
			e.Raw[0] ^= 1
			e.SourceBytes.SHA256 = fmt.Sprintf("%x", sha256.Sum256(e.Raw))
		},
	}
	for _, size := range []int{4, 8} {
		for i, mutate := range mutations {
			e := espIncompletePresentationEvent(w[:size])
			mutate(e)
			require.False(t, validESPIncompleteObservation(e), "size%d/control%d", size, i)
		}
	}
	for _, key := range []string{"SPI", "Sequence", "Opaque Body"} {
		e := espIncompletePresentationEvent(w[:1])
		e.Session[key] = nil
		require.False(t, validESPIncompleteObservation(e), "absent partial fact %s cannot be replaced by a present nil", key)
	}
	e := espIncompletePresentationEvent(nil)
	e.SourceBytes.SHA256 = fmt.Sprintf("%x", sha256.Sum256(nil))
	require.False(t, validESPIncompleteObservation(e), "zero-byte observations have no source hash, not an empty-message hash")
	e = espIncompletePresentationEvent(w[:8])
	e.Session["Opaque Body"] = ""
	require.False(t, validESPIncompleteObservation(e), "binary type facts cannot come from a same-shaped string")
}

func TestESPIncompleteDecoderMarkerRequiresExactPhysicalPrefix(t *testing.T) {
	w := espPresentationEvent().Raw[:3]
	frame := refusalPresentationWire(t, 4, layers.IPProtocolESP, w)
	frame = frame[:14+20+len(w)]
	e := espIncompletePresentationEvent(w)
	presentation := presentEvents([]*pcaputil.ProtocolEvent{e})
	packet := gopacket.NewPacket(frame, pcaputil.CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	require.NotNil(t, packet.ErrorLayer())
	why := packet.ErrorLayer().Error().Error()
	require.True(t, espIncompleteDecoderCovered(packet, presentation, why))
	require.False(t, espIncompleteDecoderCovered(packet, presentation, "unrelated decoder error"))
	changed := bytes.Clone(frame)
	changed[len(changed)-1] ^= 1
	packet = gopacket.NewPacket(changed, pcaputil.CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	require.False(t, espIncompleteDecoderCovered(packet, presentation, why), "same length is not the same captured ESP prefix")
	other := refusalPresentationWire(t, 4, layers.IPProtocolTCP, w)
	packet = gopacket.NewPacket(other, pcaputil.CaptureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
	require.False(t, espIncompleteDecoderCovered(packet, presentation, why), "an ESP label cannot cover a different child decoder failure")
}
