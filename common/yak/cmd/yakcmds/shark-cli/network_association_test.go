package sharkcli

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Authored base exchanges exercise the real capture receiver. PW7 remains
// unsupported: retaining that refusal must not turn its PPP bytes into TCP.
func associationAVP(typ uint16, value []byte) []byte {
	w := make([]byte, 6+len(value))
	binary.BigEndian.PutUint16(w, uint16(len(w))|0x8000)
	binary.BigEndian.PutUint16(w[4:], typ)
	copy(w[6:], value)
	return w
}
func associationNumber(typ uint16, value uint32, size int) []byte {
	w := make([]byte, 4)
	binary.BigEndian.PutUint32(w, value)
	return associationAVP(typ, w[4-size:])
}
func associationControl(udp bool, cid uint32, ns, nr, mt uint16, avps ...[]byte) []byte {
	w := make([]byte, 12)
	binary.BigEndian.PutUint16(w, 0xc803)
	binary.BigEndian.PutUint32(w[4:], cid)
	binary.BigEndian.PutUint16(w[8:], ns)
	binary.BigEndian.PutUint16(w[10:], nr)
	w = append(w, associationNumber(0, uint32(mt), 2)...)
	for _, v := range avps {
		w = append(w, v...)
	}
	binary.BigEndian.PutUint16(w[2:], uint16(len(w)))
	if !udp {
		w = append([]byte{0, 0, 0, 0}, w...)
	}
	return w
}
func associationExchanges(udp bool, base uint32) [][]byte {
	setup := func(id uint32) [][]byte {
		return [][]byte{associationAVP(7, []byte("peer")), associationAVP(60, []byte{192, 0, 2, 1}), associationNumber(61, id, 4), associationAVP(62, []byte{0, 7})}
	}
	return [][]byte{
		associationControl(udp, 0, 0, 0, 1, setup(100+base)...), associationControl(udp, 100+base, 0, 1, 2, setup(200+base)...), associationControl(udp, 200+base, 1, 1, 3),
		associationControl(udp, 200+base, 2, 1, 10, associationNumber(63, 300+base, 4), associationNumber(64, 0, 4), associationNumber(15, 1, 4), associationNumber(68, 7, 2), associationAVP(66, []byte("pw")), associationNumber(71, 2, 2)),
		associationControl(udp, 100+base, 1, 3, 11, associationNumber(63, 400+base, 4), associationNumber(64, 300+base, 4), associationNumber(71, 3, 2)),
		associationControl(udp, 200+base, 3, 2, 12, associationNumber(63, 300+base, 4), associationNumber(64, 400+base, 4)),
	}
}
func associationCapture(t *testing.T, udp, reuse bool, dataCopies ...int) string {
	t.Helper()
	var wire bytes.Buffer
	writer := pcapgo.NewWriter(&wire)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	number := 0
	write := func(w []byte, dir int, initial bool) {
		number++
		src, dst := net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2)
		sport, dport := uint16(40000), uint16(50000)
		if initial {
			dport = 1701
		}
		if dir == 1 {
			src, dst = dst, src
			sport, dport = dport, sport
		}
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
		ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: 115, SrcIP: src, DstIP: dst}
		serial := []gopacket.SerializableLayer{eth, ip}
		if udp {
			ip.Protocol = layers.IPProtocolUDP
			u := &layers.UDP{SrcPort: layers.UDPPort(sport), DstPort: layers.UDPPort(dport)}
			require.NoError(t, u.SetNetworkLayerForChecksum(ip))
			serial = append(serial, u)
		}
		serial = append(serial, gopacket.Payload(w))
		b := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, serial...))
		require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(int64(number), 0), Length: len(b.Bytes()), CaptureLength: len(b.Bytes())}, b.Bytes()))
	}
	for _, base := range []uint32{0, 1000} {
		ws := associationExchanges(udp, base)
		for i, w := range ws {
			write(w, []int{0, 1, 0, 0, 1, 0}[i], i == 0)
		}
		data := make([]byte, 4)
		binary.BigEndian.PutUint32(data, 400+base)
		if udp {
			data = append([]byte{0, 3, 0, 0}, data...)
		}
		copies := 1
		if len(dataCopies) > 0 {
			copies = dataCopies[0]
		}
		for i := 0; i < copies; i++ {
			write(append(data, []byte{0xc0, 0x21, 9, 1, 0, 4}...), 0, false)
		}
		write(ws[0], 0, true) // a late SCCRQ retransmission must not undo the selected responder port
		if !reuse {
			break
		}
	}
	p := filepath.Join(t.TempDir(), "authored-l2tp-association.pcap")
	require.NoError(t, os.WriteFile(p, wire.Bytes(), 0600))
	return p
}
func replayAssociation(t *testing.T, udp, reuse bool, dataCopies ...int) ([]*capturedPacket, *captureSession) {
	t.Helper()
	cfg := captureConfig{input: associationCapture(t, udp, reuse, dataCopies...), snaplen: 65535, count: 100, retentionPackets: 100}
	src, err := openCapture(cfg)
	require.NoError(t, err)
	defer src.Close()
	s := startCapture(context.Background(), src, cfg, false)
	var packets []*capturedPacket
	for p := range s.packets {
		packets = append(packets, p)
	}
	require.NoError(t, s.result())
	return packets, s
}

func TestIP115AssociationRetainsRefusalAndSixSetupMessages(t *testing.T) {
	for _, udp := range []bool{false, true} {
		t.Run(map[bool]string{false: "IP115", true: "dynamic-UDP"}[udp], func(t *testing.T) {
			packets, s := replayAssociation(t, udp, false)
			require.Len(t, packets, 8)
			p := packets[6]
			require.NotZero(t, p.conversationID)
			require.Zero(t, p.streamID)
			v := datagramConversation(p)
			require.NotNil(t, v)
			require.Len(t, v.events, 1)
			require.Len(t, v.associationProofs, 6)
			require.Empty(t, v.associationUnavailable)
			require.Equal(t, "context-required", v.events[0].Status)
			require.Contains(t, v.events[0].Error, "unselected L2TP pseudowire")
			wantTransport := "network"
			if udp {
				wantTransport = "udp"
			}
			require.Equal(t, wantTransport, v.transport)
			control := datagramConversation(packets[7])
			require.NotNil(t, control)
			if udp {
				require.Equal(t, [2]string{"192.0.2.1:40000", "192.0.2.2:50000"}, control.endpoints)
			}
			u := &tui{ring: newPacketRing(100), streams: s.streams, width: 180, height: 54, pane: 3, inspect: p, stream: v, streamPacket: p.number, streamApplication: true, streamPinned: true}
			u.ring.add(p)
			var text strings.Builder
			for _, line := range u.streamLines() {
				text.WriteString(line.text + "\n")
			}
			require.Contains(t, text.String(), "PROTOCOL MESSAGES")
			require.Contains(t, text.String(), "unselected L2TP pseudowire")
			require.NotContains(t, text.String(), "TCP payload beginning")
			if !udp {
				require.Contains(t, v.description(), "IP › L2TPv3")
				require.NotContains(t, text.String(), "UDP")
			}
			evidence := datagramEvidenceRows(v)
			text.Reset()
			for _, line := range evidence {
				text.WriteString(line.text + "\n")
			}
			require.Contains(t, text.String(), "Setup PDU #")
			require.Contains(t, text.String(), "authentication not verified")
			request, err := marshalDecodeRequest(decodeRequest{Stream: &streamDecodeRequest{ID: v.id, Events: v.events}})
			require.NoError(t, err)
			var output bytes.Buffer
			require.NoError(t, runDecodeWorker(bytes.NewReader(request), &output))
			detail, err := decodeWorkerOutput(output.Bytes(), &capturedPacket{number: v.id})
			require.NoError(t, err)
			var fields strings.Builder
			for _, f := range detail.fields {
				fields.WriteString(f.text + "\n")
			}
			require.Contains(t, fields.String(), "unselected L2TP pseudowire")
			require.Contains(t, fields.String(), "Session Setup Complete: true")
			v.chunks[0].data[0] ^= 1
			again := datagramConversation(p)
			require.NotEqual(t, v.chunks[0].data, again.chunks[0].data, "display snapshots cannot mutate retained wire bytes")
		})
	}
}

func TestNetworkAssociationGenerationAndDomainIsolation(t *testing.T) {
	packets, s := replayAssociation(t, false, true)
	require.Len(t, packets, 16)
	first, second := packets[6], packets[14]
	require.NotEqual(t, first.conversationID, second.conversationID)
	for _, p := range []*capturedPacket{first, second} {
		v := datagramConversation(p)
		require.Len(t, v.events, 1)
		require.Equal(t, p.conversationID, v.events[0].FlowID)
		require.Len(t, v.associationProofs, 6)
	}
	foreign := *first
	foreign.conversationDomain.Interface++
	foreign.connectionStack = nil
	require.Empty(t, datagramConversation(&foreign).events)
	s.streams.annotate(&foreign)
	require.Empty(t, foreign.connectionStack)
	// Equal numeric IDs in different transport namespaces cannot alias metadata.
	e := &pcaputil.ProtocolEvent{ID: 999, FlowID: first.conversationID, Transport: "udp", Protocol: "http3", Status: "decoded", Timestamp: time.Unix(30, 0), Domain: first.conversationDomain}
	s.streams.observeProtocol(e)
	row := s.streams.projectDatagram(packetSummary{}, first)
	require.Equal(t, "L2TPv3", row.ConnectionProtocol)
	other := *first
	other.conversationTransport = "udp"
	row = s.streams.projectDatagram(packetSummary{}, &other)
	require.Equal(t, "QUIC", row.ConnectionProtocol)
}

func TestAssociationProofEvictionAndByteMismatchRemainUnavailable(t *testing.T) {
	packets, _ := replayAssociation(t, false, false)
	p := *packets[6]
	data := p.protocolEvents()[0]
	history, err := pcaputil.NewProtocolInspector(1, 1<<20)
	require.NoError(t, err)
	history.OnEvent(data)
	p.history = history
	v := datagramConversation(&p)
	require.Len(t, v.events, 1)
	require.Empty(t, v.associationProofs)
	require.Len(t, v.associationUnavailable, 6)
	// A retained PDU with the requested number but other bytes is not proof.
	history, err = pcaputil.NewProtocolInspector(8, 1<<20)
	require.NoError(t, err)
	proof := mapList(data.Session["Context Proof"])[0]
	id := number(proof["PDU"])
	history.OnEvent(&pcaputil.ProtocolEvent{ID: id, FlowID: 1, Protocol: "l2tp", Transport: "network", Status: "decoded", Domain: data.Domain, Raw: []byte{1, 2, 3}, Session: map[string]any{"Version": uint64(3)}})
	history.OnEvent(data)
	p.history = history
	v = datagramConversation(&p)
	require.Empty(t, v.associationProofs)
	require.Len(t, v.associationUnavailable, 6)
	// Identical wire bytes from another capture domain are also unavailable.
	realProof, err := packets[0].history.Details(id)
	require.NoError(t, err)
	realProof.Domain.Interface++
	history, err = pcaputil.NewProtocolInspector(8, 1<<20)
	require.NoError(t, err)
	history.OnEvent(realProof)
	history.OnEvent(data)
	p.history = history
	v = datagramConversation(&p)
	require.Empty(t, v.associationProofs)
	require.Len(t, v.associationUnavailable, 6)
}

func TestUnboundNetworkEnvelopeDoesNotCreateConversation(t *testing.T) {
	for _, e := range []*pcaputil.ProtocolEvent{{Protocol: "l2tp", Transport: "network", Profile: "l2tp3-bounded-wire-envelope", Session: map[string]any{"Envelope Verified": true}}, {Protocol: "icmp", Transport: "network", FlowID: 5}} {
		p := &capturedPacket{}
		recordProtocolConversation(p, e)
		require.Zero(t, p.conversationID)
	}
}

func TestAssociationSetupEvidenceSurvivesMessageDetailBudget(t *testing.T) {
	packets, _ := replayAssociation(t, false, false, 64)
	v := datagramConversation(packets[6])
	require.Len(t, v.events, 64)
	limited := false
	for _, e := range v.events {
		limited = limited || e.Status == "limited"
	}
	require.True(t, limited, "the test must exhaust retained message-detail bytes")
	require.Len(t, v.associationProofs, 6)
	require.Empty(t, v.associationUnavailable)
	for _, p := range v.associationProofs {
		require.Len(t, p.hash, 64)
		require.NotEmpty(t, p.refs)
	}
}
