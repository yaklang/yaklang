package pcaputil

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestSMB2IOCTLResponsePayloadsFromCapture(t *testing.T) {
	t.Run("network interfaces", func(t *testing.T) {
		info, err := (&binSMB2{}).consume(smb2LayoutFixture(t, "ioctl-network-response.bin"), 64)
		require.NoError(t, err)
		body := info["Body"].(map[string]any)
		require.Equal(t, uint32(0x001401fc), body["Ctl Code"])
		require.Equal(t, "decoded: FSCTL_QUERY_NETWORK_INTERFACE_INFO", body["Control Payload Status"])
		entries := body["Network Interfaces"].([]map[string]any)
		require.Len(t, entries, 3)
		require.Equal(t, uint32(152), entries[0]["Next"])
		require.Equal(t, uint32(7), entries[0]["IfIndex"])
		require.Equal(t, uint64(1_000_000_000), entries[0]["Link Speed"])
		require.Equal(t, uint16(23), entries[0]["Address Family"])
		require.Equal(t, uint32(0), entries[2]["Next"])
		smb2LayoutSpan(t, entries[0], "IfIndex", 120, 124)
		smb2LayoutSpan(t, entries[2], "Next", 420, 424)
	})

	t.Run("dfs version 3", func(t *testing.T) {
		info, err := (&binSMB2{}).consume(smb2LayoutFixture(t, "ioctl-dfs-response.bin"), 64)
		require.NoError(t, err)
		body := info["Body"].(map[string]any)
		require.Equal(t, uint32(0x00060194), body["Ctl Code"])
		// The captured entries set ReferralEntryFlags 0x0002. MS-DFSC 2.2.5.3.2
		// NameListReferral padding is 0 or 16 bytes with no length, so the
		// following strings are not a v3 target path.
		require.Contains(t, body["Control Payload Status"], "opaque")
		require.NotContains(t, body, "DFS Referral")
		require.NotContains(t, body["Control Payload Status"], `\AD2024`)
		require.NotEmpty(t, body["Output Buffer"])
	})

	t.Run("pipe stays opaque", func(t *testing.T) {
		info, err := (&binSMB2{}).consume(smb2LayoutFixture(t, "ioctl-pipe-response.bin"), 64)
		require.NoError(t, err)
		body := info["Body"].(map[string]any)
		require.Equal(t, uint32(0x0011c017), body["Ctl Code"])
		require.Equal(t, "opaque: FSCTL_PIPE_TRANSCEIVE payload is not an SMB2 layout", body["Control Payload Status"])
		require.NotEmpty(t, body["Output Buffer"])
		require.NotContains(t, body, "Network Interfaces")
		require.NotContains(t, body, "DFS Referral")
	})

	t.Run("truncated interface fails closed", func(t *testing.T) {
		wire := append([]byte(nil), smb2LayoutFixture(t, "ioctl-network-response.bin")...)
		wire = wire[:len(wire)-1]
		binary.BigEndian.PutUint16(wire[2:], uint16(len(wire)-4))
		binary.LittleEndian.PutUint32(wire[4+100:], uint32(455))
		_, err := (&binSMB2{}).consume(wire, 64)
		require.Error(t, err)
	})
}

func TestSMB2QueryDirectoryFileIdBothChain(t *testing.T) {
	response := smb2LayoutFixture(t, "query-directory-fileidboth-response.bin")
	alone, err := (&binSMB2{}).consume(response, 64)
	require.NoError(t, err)
	directory := smb2NamedCommand(alone, "QUERY_DIRECTORY")
	require.Equal(t, "opaque: directory entry chain requires the matched request class", directory["Body"].(map[string]any)["Directory Entry Status"])
	require.NotContains(t, directory["Body"], "Directory Entries")

	request := smb2DirectoryRequestMatching(response)
	state := &binSMB2{client: 0}
	_, err = state.consumeFrom(0, request, 64)
	require.NoError(t, err)
	matched, err := state.consumeFrom(1, response, 64)
	require.NoError(t, err)
	directory = smb2NamedCommand(matched, "QUERY_DIRECTORY")
	require.Equal(t, "matched", directory["Association Status"])
	require.Equal(t, byte(37), directory["Request File Information Class"])
	body := directory["Body"].(map[string]any)
	require.Equal(t, "decoded: FileIdBothDirectoryInformation from matched request", body["Directory Entry Status"])
	entries := body["Directory Entries"].([]map[string]any)
	require.GreaterOrEqual(t, len(entries), 2)
	require.Equal(t, uint32(0x70), entries[0]["Next Entry Offset"])
	require.Equal(t, ".", entries[0]["File Name"])
	require.Equal(t, uint32(0x10), entries[0]["File Attributes"])
	require.Equal(t, int64(0), entries[0]["End Of File"])
	require.Equal(t, uint32(0), entries[len(entries)-1]["Next Entry Offset"])

	// A NextEntryOffset that lands in the following command is not a directory entry.
	compound := append([]byte(nil), response...)
	pad := (8 - ((len(compound) - 4) % 8)) % 8
	compound = append(compound, make([]byte, pad)...)
	binary.LittleEndian.PutUint32(compound[4+20:], uint32(len(compound)-4))
	binary.BigEndian.PutUint16(compound[2:], uint16(len(compound)-4+64))
	tail := smb2Hdr(6, 1, 99, 1, 1)
	tail = append(tail, make([]byte, 60)...)
	binary.LittleEndian.PutUint16(tail[64:], 60)
	compound = append(compound, tail...)
	binary.BigEndian.PutUint16(compound[2:], uint16(len(compound)-4))
	// Claim the original output now includes the next command.
	binary.LittleEndian.PutUint32(compound[4+68:], uint32(len(compound)-4-72))
	_, err = state.consumeFrom(1, compound, 64)
	require.Error(t, err)
}

func TestSMB2QueryDirectoryClassOtherThanFileIdBothStaysOpaque(t *testing.T) {
	response := smb2LayoutFixture(t, "query-directory-fileidboth-response.bin")
	request := smb2DirectoryRequestMatching(response)
	request[70] = 38
	state := &binSMB2{client: 0}
	_, err := state.consumeFrom(0, request, 64)
	require.NoError(t, err)
	matched, err := state.consumeFrom(1, response, 64)
	require.NoError(t, err)
	body := smb2NamedCommand(matched, "QUERY_DIRECTORY")["Body"].(map[string]any)
	require.Equal(t, byte(38), smb2NamedCommand(matched, "QUERY_DIRECTORY")["Request File Information Class"])
	require.Contains(t, body["Directory Entry Status"], "not selected")
	require.NotContains(t, body, "Directory Entries")
	require.NotContains(t, body, "File Name")
	require.NotContains(t, body, "Next Entry Offset")
}

func TestSMB2QueryDirectoryNoMoreFilesIsError(t *testing.T) {
	info, err := (&binSMB2{}).consume(smb2LayoutFixture(t, "query-directory-no-more-files.bin"), 64)
	require.NoError(t, err)
	var found bool
	for _, command := range smb2AllCommands(info) {
		if command["Packet Name"] != "QUERY_DIRECTORY" || command["Header"].(map[string]any)["Status"] != uint32(0x80000006) {
			continue
		}
		found = true
		require.Equal(t, true, command["Error Response"])
		require.NotContains(t, command["Body"], "Directory Entries")
		require.NotContains(t, command["Body"], "Output Buffer")
	}
	require.True(t, found)
}

func TestSMB2QueryDirectoryDoesNotCrossVLAN(t *testing.T) {
	response := smb2LayoutFixture(t, "query-directory-fileidboth-response.bin")
	request := smb2DirectoryRequestMatching(response)
	wires := [][]byte{request, response, response}
	domains := []int{0, 1, 0}
	directions := []int{0, 1, 1}
	var events []*ProtocolEvent
	analyzer, err := NewPacketAnalyzer(WithOnProtocolMessage(func(event *ProtocolEvent) { events = append(events, event) }))
	require.NoError(t, err)
	defer analyzer.Close()
	seq := [2][2]uint32{{1, 1}, {1, 1}}
	for index, wire := range wires {
		domain, direction := domains[index], directions[index]
		src, dst := net.IPv4(192, 0, 2, 10), net.IPv4(192, 0, 2, 20)
		sport, dport := layers.TCPPort(51000), layers.TCPPort(445)
		if direction == 1 {
			src, dst, sport, dport = dst, src, dport, sport
		}
		ethernet := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 3}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 4}, EthernetType: layers.EthernetTypeDot1Q}
		ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: src, DstIP: dst}
		tcp := &layers.TCP{SrcPort: sport, DstPort: dport, Seq: seq[domain][direction], ACK: true, PSH: true, Window: 65535}
		seq[domain][direction] += uint32(len(wire))
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
		buffer := gopacket.NewSerializeBuffer()
		tag := &layers.Dot1Q{VLANIdentifier: uint16(80 + domain), Type: layers.EthernetTypeIPv4}
		require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ethernet, tag, ip, tcp, gopacket.Payload(wire)))
		packet := buffer.Bytes()
		require.NoError(t, analyzer.Feed(packet, gopacket.CaptureInfo{Timestamp: time.Unix(int64(200+index), 0), CaptureLength: len(packet), Length: len(packet)}, layers.LinkTypeEthernet))
	}
	require.NoError(t, analyzer.Close())
	require.Len(t, events, 3)
	foreign := smb2NamedCommand(events[1].Session, "QUERY_DIRECTORY")
	require.Equal(t, "missing-or-ambiguous-request", foreign["Association Status"])
	require.NotContains(t, foreign["Body"], "Directory Entries")
	local := smb2NamedCommand(events[2].Session, "QUERY_DIRECTORY")
	require.Equal(t, "matched", local["Association Status"])
	require.Contains(t, local["Body"].(map[string]any)["Directory Entry Status"], "decoded: FileIdBothDirectoryInformation")
}

func TestSMB2NetworkInterfaceRetransmitPublishesOnce(t *testing.T) {
	wire := string(smb2LayoutFixture(t, "ioctl-network-response.bin"))
	steps := []tcpStep{
		{seq: 99, syn: true}, {seq: 499, ack: 100, syn: true, synack: true, reverse: true}, {seq: 100, ack: 500},
		{seq: 500, data: wire, reverse: true},
		{seq: 500, data: wire, reverse: true},
	}
	var events []*ProtocolEvent
	err := ReplayPcap(bytes.NewReader(binTestPcap(t, steps, 445, false, true)), WithOnProtocolMessage(func(event *ProtocolEvent) {
		events = append(events, event)
	}))
	require.NoError(t, err)
	decoded := 0
	for _, event := range events {
		if event.Protocol == "smb2" && (event.Status == "decoded" || event.Status == "deferred") {
			decoded++
			require.Equal(t, "IOCTL", event.Session["Packet Name"])
			require.Len(t, event.Session["Body"].(map[string]any)["Network Interfaces"], 3)
		}
	}
	require.Equal(t, 1, decoded)
}

func smb2DirectoryRequestMatching(response []byte) []byte {
	payload := response[4:]
	for at := 0; at+88 <= len(payload); {
		if binary.LittleEndian.Uint32(payload[at:at+4]) != 0x424d53fe {
			break
		}
		next := int(binary.LittleEndian.Uint32(payload[at+20 : at+24]))
		end := len(payload)
		if next != 0 {
			end = at + next
		}
		if binary.LittleEndian.Uint16(payload[at+12:at+14]) == 14 && payload[at+16]&1 != 0 {
			body := make([]byte, 32)
			binary.LittleEndian.PutUint16(body[0:], 33)
			body[2] = 37
			copy(body[8:24], payload[at+72:at+88])
			flags := binary.LittleEndian.Uint32(payload[at+16 : at+20])
			var tid uint32
			var sid uint64
			if flags&2 == 0 {
				tid = binary.LittleEndian.Uint32(payload[at+36 : at+40])
			}
			sid = binary.LittleEndian.Uint64(payload[at+40 : at+48])
			header := smb2Hdr(14, 0, binary.LittleEndian.Uint64(payload[at+24:at+32]), sid, tid)
			return smb2TCP(append(header, body...))
		}
		if next == 0 {
			break
		}
		at = end
	}
	panic("directory response command not found")
}

func smb2AllCommands(info map[string]any) []map[string]any {
	if info["Compound"] == true {
		return info["Commands"].([]map[string]any)
	}
	return []map[string]any{info}
}

func smb2NamedCommand(info map[string]any, name string) map[string]any {
	for _, command := range smb2AllCommands(info) {
		if command["Packet Name"] == name {
			return command
		}
	}
	return info
}
