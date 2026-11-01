package pcaputil

import (
	"bytes"
	"encoding/binary"
	"net"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func smb2LayoutFixture(t *testing.T, name string) []byte {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	data, err := trafficfixture.ReadFile(filepath.Join(filepath.Dir(file), "smb2layout", name))
	require.NoError(t, err)
	require.NotEmpty(t, data)
	return data
}

func smb2LayoutSpan(t *testing.T, owner map[string]any, key string, lo, hi uint64) {
	t.Helper()
	ranges, ok := owner["Wire Field Ranges"].(map[string]any)
	require.True(t, ok, key)
	require.Equal(t, []any{lo, hi}, ranges[key], key)
}

func smb2LayoutBody(t *testing.T, name string) map[string]any {
	t.Helper()
	info, err := (&binSMB2{client: 0}).consume(smb2LayoutFixture(t, name), 64)
	require.NoError(t, err)
	body, ok := info["Body"].(map[string]any)
	require.True(t, ok)
	return body
}

func TestSMB2LayoutFixturesKeepWidthSignednessAndRanges(t *testing.T) {
	t.Run("negotiate", func(t *testing.T) {
		body := smb2LayoutBody(t, "negotiate-request.bin")
		require.Equal(t, uint16(36), body["Structure Size"])
		require.Equal(t, uint16(2), body["Dialect Count"])
		require.Equal(t, uint16(1), body["Security Mode"])
		require.Equal(t, uint64(0x0102030405060708), body["Client Start Time"])
		require.NotContains(t, body, "Negotiate Context Offset")
		smb2LayoutSpan(t, body, "Dialect Count", 70, 72)
		smb2LayoutSpan(t, body, "Security Mode", 72, 74)
		smb2LayoutSpan(t, body, "Client Start Time", 96, 104)
		dialects := body["Dialects"].([]map[string]any)
		require.Equal(t, uint16(0x0202), dialects[0]["Dialect"])
		require.Equal(t, uint16(0x0210), dialects[1]["Dialect"])
		smb2LayoutSpan(t, dialects[0], "Dialect", 104, 106)
		smb2LayoutSpan(t, dialects[1], "Dialect", 106, 108)

		response := smb2LayoutBody(t, "negotiate-response.bin")
		require.Equal(t, uint16(0x0210), response["Dialect"])
		require.Equal(t, uint16(0), response["Reserved"])
		require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, response["Server Guid"])
		require.Equal(t, uint32(65536), response["Max Read Size"])
		require.NotContains(t, response, "Negotiate Context Count")
		smb2LayoutSpan(t, response, "Dialect", 72, 74)
		smb2LayoutSpan(t, response, "Server Guid", 76, 92)
		smb2LayoutSpan(t, response, "Max Read Size", 100, 104)
	})

	t.Run("session-tree-logoff", func(t *testing.T) {
		setup := smb2LayoutBody(t, "session-setup-request.bin")
		require.Equal(t, uint16(25), setup["Structure Size"])
		require.Equal(t, byte(1), setup["Security Mode"])
		require.Equal(t, uint64(0), setup["Previous Session Id"])
		require.Equal(t, uint16(0), setup["Security Buffer Length"])
		smb2LayoutSpan(t, setup, "Security Mode", 71, 72)
		smb2LayoutSpan(t, setup, "Previous Session Id", 84, 92)
		session := smb2LayoutBody(t, "session-setup-response.bin")
		require.Equal(t, uint16(0), session["Session Flags"])
		require.Equal(t, uint16(0), session["Security Buffer Offset"])
		require.Equal(t, uint16(0), session["Security Buffer Length"])
		require.NotContains(t, session, "Byte Count")
		smb2LayoutSpan(t, session, "Session Flags", 70, 72)
		smb2LayoutSpan(t, session, "Security Buffer Offset", 72, 74)
		smb2LayoutSpan(t, session, "Security Buffer Length", 74, 76)

		tree := smb2LayoutBody(t, "tree-connect-request.bin")
		require.Equal(t, `\\srv\share`, tree["Path"])
		require.Equal(t, uint16(22), tree["Path Length"])
		smb2LayoutSpan(t, tree, "Path", 76, 98)
		share := smb2LayoutBody(t, "tree-connect-response.bin")
		require.Equal(t, byte(1), share["Share Type"])
		require.Equal(t, byte(0), share["Reserved"])
		require.Equal(t, uint32(0x30), share["Share Flags"])
		require.Equal(t, uint32(8), share["Capabilities"])
		require.Equal(t, uint32(0x001f01ff), share["Maximal Access"])
		smb2LayoutSpan(t, share, "Share Type", 70, 71)
		smb2LayoutSpan(t, share, "Reserved", 71, 72)
		smb2LayoutSpan(t, share, "Share Flags", 72, 76)
		smb2LayoutSpan(t, share, "Capabilities", 76, 80)
		smb2LayoutSpan(t, share, "Maximal Access", 80, 84)

		logoff := smb2LayoutBody(t, "logoff-request.bin")
		require.Equal(t, uint16(4), logoff["Structure Size"])
		require.Equal(t, uint16(0), logoff["Reserved"])
		smb2LayoutSpan(t, logoff, "Reserved", 70, 72)
		disconnect := smb2LayoutBody(t, "tree-disconnect-request.bin")
		require.Equal(t, uint16(4), disconnect["Structure Size"])
		smb2LayoutSpan(t, disconnect, "Structure Size", 68, 70)
	})

	t.Run("create-close-read-write", func(t *testing.T) {
		create := smb2LayoutBody(t, "create-request.bin")
		require.Equal(t, "file.txt", create["File Name"])
		require.Equal(t, uint32(0x00120189), create["Desired Access"])
		require.Equal(t, uint32(1), create["Create Disposition"])
		require.Equal(t, byte(0), create["Security Flags"])
		require.Equal(t, byte(1), create["Requested Oplock Level"])
		require.Equal(t, uint32(2), create["Impersonation Level"])
		require.Equal(t, uint64(0), create["Reserved"])
		require.Equal(t, uint32(0), create["File Attributes"])
		require.Equal(t, uint32(0), create["Share Access"])
		require.Equal(t, uint32(0), create["Create Options"])
		smb2LayoutSpan(t, create, "Desired Access", 92, 96)
		smb2LayoutSpan(t, create, "File Name", 124, 140)
		smb2LayoutSpan(t, create, "Security Flags", 70, 71)
		smb2LayoutSpan(t, create, "Requested Oplock Level", 71, 72)
		smb2LayoutSpan(t, create, "Impersonation Level", 72, 76)
		smb2LayoutSpan(t, create, "Reserved", 84, 92)
		smb2LayoutSpan(t, create, "File Attributes", 96, 100)
		smb2LayoutSpan(t, create, "Share Access", 100, 104)
		smb2LayoutSpan(t, create, "Create Options", 108, 112)
		opened := smb2LayoutBody(t, "create-response.bin")
		require.Equal(t, int64(4096), opened["Allocation Size"])
		require.Equal(t, int64(100), opened["End Of File"])
		require.IsType(t, int64(0), opened["End Of File"])
		require.Equal(t, byte(1), opened["Oplock Level"])
		require.Equal(t, uint64(0), opened["Creation Time"])
		require.Equal(t, uint64(0), opened["Last Access Time"])
		require.Equal(t, uint64(0), opened["Last Write Time"])
		require.Equal(t, uint64(0), opened["Change Time"])
		require.Equal(t, uint32(0), opened["File Attributes"])
		require.Equal(t, uint32(0), opened["Reserved2"])
		smb2LayoutSpan(t, opened, "End Of File", 116, 124)
		smb2LayoutSpan(t, opened, "FileId", 132, 148)
		smb2LayoutSpan(t, opened, "Oplock Level", 70, 71)
		smb2LayoutSpan(t, opened, "Creation Time", 76, 84)
		smb2LayoutSpan(t, opened, "Last Access Time", 84, 92)
		smb2LayoutSpan(t, opened, "Last Write Time", 92, 100)
		smb2LayoutSpan(t, opened, "Change Time", 100, 108)
		smb2LayoutSpan(t, opened, "File Attributes", 124, 128)
		smb2LayoutSpan(t, opened, "Reserved2", 128, 132)

		read := smb2LayoutBody(t, "read-request.bin")
		require.Equal(t, uint32(8), read["Length"])
		require.Equal(t, uint64(100), read["Offset"])
		require.Equal(t, uint32(0), read["Channel"])
		require.Equal(t, byte(0x08), read["Padding"])
		require.Equal(t, byte(0), read["Flags"])
		require.Equal(t, "0102030405060708090a0b0c0d0e0f10", read["FileId"])
		require.Equal(t, uint32(1), read["Minimum Count"])
		require.Equal(t, uint32(0), read["Remaining Bytes"])
		require.Equal(t, uint16(0), read["Read Channel Info Offset"])
		require.Equal(t, uint16(0), read["Read Channel Info Length"])
		require.NotContains(t, read, "Read Channel Info")
		smb2LayoutSpan(t, read, "Length", 72, 76)
		smb2LayoutSpan(t, read, "Offset", 76, 84)
		smb2LayoutSpan(t, read, "Padding", 70, 71)
		smb2LayoutSpan(t, read, "Flags", 71, 72)
		smb2LayoutSpan(t, read, "FileId", 84, 100)
		smb2LayoutSpan(t, read, "Minimum Count", 100, 104)
		smb2LayoutSpan(t, read, "Remaining Bytes", 108, 112)
		smb2LayoutSpan(t, read, "Read Channel Info Offset", 112, 114)
		smb2LayoutSpan(t, read, "Read Channel Info Length", 114, 116)
		data := smb2LayoutBody(t, "read-response.bin")
		require.Equal(t, []byte("abcdefgh"), data["Data"])
		require.Equal(t, uint32(8), data["Data Length"])
		smb2LayoutSpan(t, data, "Data", 84, 92)

		write := smb2LayoutBody(t, "write-request.bin")
		require.Equal(t, []byte("abcdefgh"), write["Data"])
		smb2LayoutSpan(t, write, "Data", 116, 124)
		count := smb2LayoutBody(t, "write-response.bin")
		require.Equal(t, uint32(8), count["Count"])
		smb2LayoutSpan(t, count, "Count", 72, 76)

		closed := smb2LayoutBody(t, "close-request.bin")
		require.Equal(t, uint16(24), closed["Structure Size"])
		smb2LayoutSpan(t, closed, "FileId", 76, 92)
		// SMB2 header offsets 68-72. The published span includes the 4-byte Direct TCP length.
		closeResponse := smb2LayoutBody(t, "close-response.bin")
		require.Equal(t, uint32(0), closeResponse["Reserved"])
		require.Equal(t, uint64(0x0102030405060708), closeResponse["Creation Time"])
		smb2LayoutSpan(t, closeResponse, "Reserved", 72, 76)
		smb2LayoutSpan(t, closeResponse, "Creation Time", 76, 84)
	})

	t.Run("error-ioctl-directory", func(t *testing.T) {
		info, err := (&binSMB2{client: 0}).consume(smb2LayoutFixture(t, "write-error-response.bin"), 64)
		require.NoError(t, err)
		require.Equal(t, true, info["Error Response"])
		errBody := info["Body"].(map[string]any)
		require.Equal(t, uint32(4), errBody["Byte Count"])
		require.Equal(t, []byte{0x11, 0x22, 0x33, 0x44}, errBody["Error Data"])
		require.NotContains(t, errBody, "Count")
		smb2LayoutSpan(t, errBody, "Error Data", 76, 80)

		ioctl := smb2LayoutBody(t, "ioctl-request.bin")
		require.Equal(t, uint32(0x001401fc), ioctl["Ctl Code"])
		require.Equal(t, uint16(0), ioctl["Reserved"])
		require.Equal(t, "0102030405060708090a0b0c0d0e0f10", ioctl["FileId"])
		require.Equal(t, uint32(0), ioctl["Input Offset"])
		require.Equal(t, uint32(0), ioctl["Input Count"])
		require.Equal(t, uint32(0), ioctl["Output Offset"])
		require.Equal(t, uint32(0), ioctl["Output Count"])
		require.Equal(t, uint32(0), ioctl["Flags"])
		require.Equal(t, uint32(0), ioctl["Reserved2"])
		require.Equal(t, "opaque: IOCTL control payload layout is not selected", ioctl["Control Payload Status"])
		require.NotContains(t, ioctl, "Input Buffer")
		smb2LayoutSpan(t, ioctl, "Ctl Code", 72, 76)
		smb2LayoutSpan(t, ioctl, "Reserved", 70, 72)
		smb2LayoutSpan(t, ioctl, "FileId", 76, 92)
		smb2LayoutSpan(t, ioctl, "Input Offset", 92, 96)
		smb2LayoutSpan(t, ioctl, "Input Count", 96, 100)
		smb2LayoutSpan(t, ioctl, "Output Offset", 104, 108)
		smb2LayoutSpan(t, ioctl, "Output Count", 108, 112)
		smb2LayoutSpan(t, ioctl, "Flags", 116, 120)
		smb2LayoutSpan(t, ioctl, "Reserved2", 120, 124)

		query := smb2LayoutBody(t, "query-directory-request.bin")
		require.Equal(t, byte(37), query["File Information Class"])
		require.Equal(t, byte(0), query["Flags"])
		require.Equal(t, uint32(0), query["File Index"])
		require.Equal(t, "00000000000000000000000000000000", query["FileId"])
		require.Equal(t, uint32(0x1000), query["Output Buffer Length"])
		require.Equal(t, "*", query["File Name"])
		smb2LayoutSpan(t, query, "File Name", 100, 102)
		smb2LayoutSpan(t, query, "Flags", 71, 72)
		smb2LayoutSpan(t, query, "File Index", 72, 76)
		smb2LayoutSpan(t, query, "FileId", 76, 92)
		smb2LayoutSpan(t, query, "Output Buffer Length", 96, 100)
		output := smb2LayoutBody(t, "query-directory-response.bin")
		require.Equal(t, []byte{9, 8, 7, 6}, output["Output Buffer"])
		require.Equal(t, "opaque: directory entry chain requires the matched request class", output["Directory Entry Status"])
		smb2LayoutSpan(t, output, "Output Buffer", 76, 80)
	})
}

func TestSMB2CommandBoundaryDoesNotBorrowNextCompound(t *testing.T) {
	// The second command begins with eight data bytes. A WRITE length that
	// ignores NextCommand would treat those header bytes as its payload.
	second := smb2Hdr(6, 0, 12, 0x11, 7)
	second = append(second, make([]byte, 24)...)
	binary.LittleEndian.PutUint16(second[64:], 24)
	first := smb2Hdr(9, 0, 11, 0x11, 7)
	body := make([]byte, 48)
	binary.LittleEndian.PutUint16(body[0:], 49)
	binary.LittleEndian.PutUint16(body[2:], 112)
	binary.LittleEndian.PutUint32(body[4:], 8)
	first = append(first, body...)
	pad := (8 - (len(first) % 8)) % 8
	first = append(first, make([]byte, pad)...)
	binary.LittleEndian.PutUint32(first[20:], uint32(len(first)))
	_, err := (&binSMB2{}).consume(smb2TCP(append(first, second...)), 64)
	require.Error(t, err)

	owned := make([]byte, 48+8)
	binary.LittleEndian.PutUint16(owned[0:], 49)
	binary.LittleEndian.PutUint16(owned[2:], 112)
	binary.LittleEndian.PutUint32(owned[4:], 8)
	copy(owned[48:], []byte("abcdefgh"))
	good := append(smb2Hdr(9, 0, 11, 0x11, 7), owned...)
	pad = (8 - (len(good) % 8)) % 8
	good = append(good, make([]byte, pad)...)
	binary.LittleEndian.PutUint32(good[20:], uint32(len(good)))
	closeBody := make([]byte, 24)
	binary.LittleEndian.PutUint16(closeBody[0:], 24)
	good = append(good, smb2Hdr(6, 4, 12, 0xffffffffffffffff, 0xffffffff)...)
	good = append(good, closeBody...)
	info, err := (&binSMB2{client: 0}).consumeFrom(0, smb2TCP(good), 64)
	require.NoError(t, err)
	commands := info["Commands"].([]map[string]any)
	require.Equal(t, []byte("abcdefgh"), commands[0]["Body"].(map[string]any)["Data"])
	require.NotEqual(t, commands[0]["Body"].(map[string]any)["Wire Field Ranges"].(map[string]any)["Data"], commands[1]["Body"].(map[string]any)["Wire Field Ranges"].(map[string]any)["FileId"])
}

func TestSMB2LayoutRejectsTruncationNegativeSizeAndErrorShape(t *testing.T) {
	short := smb2LayoutFixture(t, "read-request.bin")
	short = short[:len(short)-1]
	binary.BigEndian.PutUint16(short[2:], uint16(len(short)-4))
	_, err := (&binSMB2{}).consume(short, 64)
	require.Error(t, err)

	negative := bytes.Clone(smb2LayoutFixture(t, "create-response.bin"))
	// End Of File is a signed LARGE_INTEGER at Direct TCP offset 116.
	binary.LittleEndian.PutUint64(negative[116:], ^uint64(0))
	_, err = (&binSMB2{}).consume(negative, 64)
	require.Error(t, err)

	overflow := bytes.Clone(smb2LayoutFixture(t, "read-response.bin"))
	binary.LittleEndian.PutUint32(overflow[12:], 0x80000005)
	info, err := (&binSMB2{}).consume(overflow, 64)
	require.NoError(t, err)
	require.NotEqual(t, true, info["Error Response"])
	require.Equal(t, []byte("abcdefgh"), info["Body"].(map[string]any)["Data"])
}

func TestSMB2PublicMutationDoesNotReplaceAcceptedFields(t *testing.T) {
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	result := session.Feed(0, time.Unix(1, 0), smb2LayoutFixture(t, "tree-connect-request.bin"))
	require.Nil(t, result.Err)
	require.Len(t, result.Events, 1)
	event := result.Events[0]
	require.Equal(t, `\\srv\share`, event.Session["Path"])
	event.Session["Path"] = "mutated"
	event.Session["Packet Name"] = "NEGOTIATE"
	require.Equal(t, `\\srv\share`, event.semanticFields["Path"])
	again, err := (&binSMB2{}).consume(smb2LayoutFixture(t, "tree-connect-request.bin"), 64)
	require.NoError(t, err)
	require.Equal(t, `\\srv\share`, again["Path"])
	require.Equal(t, "TREE_CONNECT", again["Packet Name"])
}

func TestSMB2TreeConnectDoesNotCrossVLAN(t *testing.T) {
	request := smb2LayoutFixture(t, "tree-connect-request.bin")
	response := smb2LayoutFixture(t, "tree-connect-response.bin")
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
		src, dst := net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2)
		sport, dport := layers.TCPPort(50000), layers.TCPPort(445)
		if direction == 1 {
			src, dst, sport, dport = dst, src, dport, sport
		}
		ethernet := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeDot1Q}
		ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: src, DstIP: dst}
		tcp := &layers.TCP{SrcPort: sport, DstPort: dport, Seq: seq[domain][direction], ACK: true, PSH: true, Window: 4096}
		seq[domain][direction] += uint32(len(wire))
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
		buffer := gopacket.NewSerializeBuffer()
		tag := &layers.Dot1Q{VLANIdentifier: uint16(80 + domain), Type: layers.EthernetTypeIPv4}
		require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ethernet, tag, ip, tcp, gopacket.Payload(wire)))
		packet := buffer.Bytes()
		require.NoError(t, analyzer.Feed(packet, gopacket.CaptureInfo{Timestamp: time.Unix(int64(100+index), 0), CaptureLength: len(packet), Length: len(packet), InterfaceIndex: 0}, layers.LinkTypeEthernet))
	}
	require.NoError(t, analyzer.Close())
	require.Len(t, events, 3)
	require.NotEqual(t, events[0].FlowID, events[1].FlowID)
	require.Equal(t, "missing-or-ambiguous-request", events[1].Session["Association Status"])
	require.NotContains(t, events[1].Session, "Request Session ID")
	require.Equal(t, "matched", events[2].Session["Association Status"])
	require.Equal(t, `\\srv\share`, events[0].Session["Path"])
	require.NotEqual(t, events[0].Session["Path"], events[1].Session["Body"].(map[string]any)["Share Type"])
}

func TestSMB2ReadRetransmitPublishesOnce(t *testing.T) {
	wire := string(smb2LayoutFixture(t, "read-request.bin"))
	steps := []tcpStep{
		{seq: 99, syn: true}, {seq: 499, ack: 100, syn: true, synack: true, reverse: true}, {seq: 100, ack: 500},
		{seq: 100, data: wire},
		{seq: 100, data: wire},
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
			require.Equal(t, "READ", event.Session["Packet Name"])
			require.Equal(t, uint32(8), event.Session["Length"])
		}
	}
	require.Equal(t, 1, decoded)
}

func TestSMB2CachedDialectDoesNotReplaceCurrentNegotiate(t *testing.T) {
	session, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer session.Close("test")
	first := session.Feed(0, time.Unix(1, 0), smb2LayoutFixture(t, "negotiate-request.bin"))
	require.Nil(t, first.Err)
	second := session.Feed(1, time.Unix(2, 0), smb2LayoutFixture(t, "negotiate-response.bin"))
	require.Nil(t, second.Err)
	require.Equal(t, uint16(0x0210), second.Events[0].Session["Dialect"])
	third := session.Feed(0, time.Unix(3, 0), smb2LayoutFixture(t, "negotiate-request.bin"))
	require.Nil(t, third.Err)
	body := third.Events[0].Session["Body"].(map[string]any)
	require.Equal(t, uint16(2), body["Dialect Count"])
	require.NotContains(t, body, "Dialect")
	require.Equal(t, uint16(0x0210), third.Events[0].Session["Connection Dialect"])
	require.Equal(t, uint16(0x0210), second.Events[0].Session["Dialect"])
}

func TestSMB2CapturedVLAN81QueryDoesNotSelectVLAN80Response(t *testing.T) {
	// Sliced from The-Ultimate-PCAP.pcapng: QUERY_INFO class 7 on VLAN 81, its
	// VLAN 80 response. The two VLAN IDs stay separate domains.
	request := smb2LayoutFixture(t, "vlan81-query-info-request.bin")
	response := smb2LayoutFixture(t, "vlan80-query-info-response.bin")
	parsed, err := (&binSMB2{client: 0}).consume(request, 64)
	require.NoError(t, err)
	require.Equal(t, byte(1), parsed["Body"].(map[string]any)["Info Type"])
	require.Equal(t, byte(7), parsed["Body"].(map[string]any)["File Info Class"])
	wires := [][]byte{request, response, response}
	domains := []int{1, 0, 1}
	directions := []int{0, 1, 1}
	var events []*ProtocolEvent
	analyzer, err := NewPacketAnalyzer(WithOnProtocolMessage(func(event *ProtocolEvent) { events = append(events, event) }))
	require.NoError(t, err)
	defer analyzer.Close()
	seq := [2][2]uint32{{1, 1}, {1, 1}}
	for index, wire := range wires {
		domain, direction := domains[index], directions[index]
		src, dst := net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2)
		sport, dport := layers.TCPPort(50000), layers.TCPPort(445)
		if direction == 1 {
			src, dst, sport, dport = dst, src, dport, sport
		}
		ethernet := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeDot1Q}
		ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: src, DstIP: dst}
		tcp := &layers.TCP{SrcPort: sport, DstPort: dport, Seq: seq[domain][direction], ACK: true, PSH: true, Window: 4096}
		seq[domain][direction] += uint32(len(wire))
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
		buffer := gopacket.NewSerializeBuffer()
		tag := &layers.Dot1Q{VLANIdentifier: uint16(80 + domain), Type: layers.EthernetTypeIPv4}
		require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ethernet, tag, ip, tcp, gopacket.Payload(wire)))
		packet := buffer.Bytes()
		require.NoError(t, analyzer.Feed(packet, gopacket.CaptureInfo{Timestamp: time.Unix(int64(100+index), 0), CaptureLength: len(packet), Length: len(packet), InterfaceIndex: 0}, layers.LinkTypeEthernet))
	}
	require.NoError(t, analyzer.Close())
	require.Len(t, events, 3)
	require.Equal(t, byte(7), events[0].Session["Body"].(map[string]any)["File Info Class"])
	require.NotEqual(t, events[0].FlowID, events[1].FlowID)
	require.Equal(t, "missing-or-ambiguous-request", events[1].Session["Association Status"])
	require.NotContains(t, events[1].Session, "Request File Info Class")
	require.NotContains(t, events[1].Session, "Output Information Type")
	require.Equal(t, events[0].FlowID, events[2].FlowID)
	require.Equal(t, "matched", events[2].Session["Association Status"])
	require.Equal(t, byte(7), events[2].Session["Request File Info Class"])
	require.Equal(t, "FileEaInformation", events[2].Session["Output Information Type"])
}
