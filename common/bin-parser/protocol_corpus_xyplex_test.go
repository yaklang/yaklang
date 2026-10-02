package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

type xyplexTestFixture struct {
	name                               string
	reply                              bool
	typeValue, padding                 uint8
	server, returned, reserved, status uint16
	tail                               []byte
}

func (f xyplexTestFixture) entry() string {
	if f.reply {
		return "XYPLEXReply"
	}
	return "XYPLEXRequest"
}
func (f xyplexTestFixture) direction() string {
	if f.reply {
		return "reply"
	}
	return "request"
}
func (f xyplexTestFixture) header() int {
	if f.reply {
		return 4
	}
	return 8
}
func (f xyplexTestFixture) rawName() string {
	if f.reply {
		return "Unparsed XYPLEX Reply"
	}
	return "Unparsed XYPLEX Request"
}
func (f xyplexTestFixture) fields() []snaTestField {
	fields := []snaTestField{snaU8("Protocol Type", f.typeValue), snaU8("Padding", f.padding)}
	if f.reply {
		fields = append(fields, snaU16("Registration Reply", f.status))
	} else {
		fields = append(fields, snaU16("Server Port", f.server), snaU16("Return Port", f.returned), snaU16("Reserved", f.reserved))
	}
	if len(f.tail) > 0 {
		name := "Uninterpreted Request Tail"
		if f.reply {
			name = "Uninterpreted Reply Tail"
		}
		fields = append(fields, snaRaw(name, f.tail))
	}
	return fields
}
func (f xyplexTestFixture) wire() []byte { return snaEncode(f.fields()) }
func xyplexTestFixtures() []xyplexTestFixture {
	return []xyplexTestFixture{
		{name: "request-header", typeValue: 1, server: 7, returned: 8080},
		{name: "reply-ok", reply: true, typeValue: 1, status: 0},
		{name: "reply-queue-full", reply: true, typeValue: 1, status: 5},
		{name: "request-uninterpreted-tail", typeValue: 0xa5, padding: 0xff, server: 0x1234, returned: 0xc001, reserved: 0x55aa, tail: []byte{0, 0xff, 0x61}},
		{name: "reply-unknown-with-tail", reply: true, typeValue: 0xfe, padding: 0x80, status: 0x7fff, tail: []byte{0x61, 0x62, 0, 0xff}},
	}
}

func xyplexTestFields(t *testing.T, node *base.Node, f xyplexTestFixture, offset uint64) {
	t.Helper()
	snaTestFields(t, node, f.fields(), offset)
	first := protocolCorpusFindNode(node, "Protocol Type")
	require.NotNil(t, first)
	info := alljoynTestInfo(t, first.Cfg.GetItem(base.CfgParent).(*base.Node))
	require.Equal(t, "XYPLEX UDP registration header fields", info["Profile"])
	require.Equal(t, f.direction(), info["Direction"])
	require.EqualValues(t, f.header(), info["Header Bytes"])
	require.EqualValues(t, 65527, info["Maximum Datagram Bytes"])
	require.EqualValues(t, len(f.tail), info["Tail Bytes"])
	for _, key := range []string{"Tail Decoded", "Operation Validated", "Transport Provenance Validated", "TCP Session Decoded"} {
		require.Equal(t, false, info[key], key)
	}
	if f.reply {
		require.Equal(t, f.status == 0 || f.status == 5, info["Known Reply Code"])
		label := "unknown"
		if f.status == 0 {
			label = "OK"
		}
		if f.status == 5 {
			label = "Queue Full"
		}
		require.Equal(t, label, info["Reply Label"])
		require.Nil(t, protocolCorpusFindNode(node, "Server Port"))
		require.Nil(t, protocolCorpusFindNode(node, "Return Port"))
	} else {
		require.Equal(t, "serial port identifier", info["Server Port Role"])
		require.Equal(t, "announced TCP port", info["Return Port Role"])
		require.Nil(t, protocolCorpusFindNode(node, "Registration Reply"))
	}
}

func xyplexTestUDP(wire []byte, reply bool) []byte {
	udp := alljoynTestUDP(wire, reply)
	if reply {
		binary.BigEndian.PutUint16(udp, 173)
	} else {
		binary.BigEndian.PutUint16(udp[2:], 173)
	}
	return udp
}

func TestProtocolCorpusXYPLEXFields(t *testing.T) {
	for _, f := range xyplexTestFixtures() {
		t.Run(f.name, func(t *testing.T) {
			t.Logf("companion %s direction=%s %d %x", f.name, f.direction(), len(f.wire()), f.wire())
			for _, entry := range []string{f.entry(), f.entry() + "Carrier"} {
				node := protocolCorpusRequireBoundedRuleParse(t, f.wire(), "xyplex", entry)
				xyplexTestFields(t, node, f, 0)
				require.Equal(t, f.wire(), NodeToBytes(node))
			}
		})
	}
}

func TestProtocolCorpusXYPLEXOriginalEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-xyplex.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "072afe89224b391f71620489a56c1cc683e57e568afda0f190aa3300dca9ffcb", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 1)
	f := xyplexTestFixture{tail: make([]byte, 8)}
	for _, frame := range frames {
		require.Len(t, frame, 58)
		require.Equal(t, byte(17), frame[23])
		require.EqualValues(t, 40101, binary.BigEndian.Uint16(frame[34:]))
		require.EqualValues(t, 173, binary.BigEndian.Uint16(frame[36:]))
		require.Equal(t, make([]byte, 16), frame[42:])
		for _, entry := range []string{f.entry(), f.entry() + "Carrier"} {
			node := protocolCorpusRequireBoundedRuleParse(t, frame[42:], "xyplex", entry)
			xyplexTestFields(t, node, f, 0)
			require.Equal(t, frame[42:], NodeToBytes(node))
		}
		node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, frame, NodeToBytes(node))
		first := protocolCorpusFindNode(node, "Protocol Type")
		require.NotNil(t, first)
		xyplexTestFields(t, first.Cfg.GetItem(base.CfgParent).(*base.Node), f, 42*8)
	}
}

func TestProtocolCorpusXYPLEXCompanionEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-validated/gen-xyplex-valid.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "1c8a37f8fb795b3310ad0315de77c7f63d7d005241fc27eb5d02b63c5968a16a", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	fixtures := xyplexTestFixtures()
	require.Len(t, frames, len(fixtures))
	for index, frame := range frames {
		f := fixtures[index]
		t.Run(f.name, func(t *testing.T) {
			require.Len(t, frame, 42+len(f.wire()))
			require.Equal(t, byte(17), frame[23])
			source, destination := uint16(40101), uint16(173)
			if f.reply {
				source, destination = destination, source
			}
			require.Equal(t, source, binary.BigEndian.Uint16(frame[34:]))
			require.Equal(t, destination, binary.BigEndian.Uint16(frame[36:]))
			require.Equal(t, f.wire(), frame[42:])
			for _, entry := range []string{f.entry(), f.entry() + "Carrier"} {
				node := protocolCorpusRequireBoundedRuleParse(t, frame[42:], "xyplex", entry)
				xyplexTestFields(t, node, f, 0)
				require.Equal(t, frame[42:], NodeToBytes(node))
			}
			node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			first := protocolCorpusFindNode(node, "Protocol Type")
			require.NotNil(t, first)
			xyplexTestFields(t, first.Cfg.GetItem(base.CfgParent).(*base.Node), f, 336)
			require.Equal(t, frame, NodeToBytes(node))
		})
	}
}

func TestProtocolCorpusXYPLEXPortDirection(t *testing.T) {
	for _, f := range xyplexTestFixtures() {
		udp := xyplexTestUDP(f.wire(), f.reply)
		node := protocolCorpusRequireBoundedRuleParse(t, udp, "user_datagram_protocol", "UDP")
		require.Equal(t, udp, NodeToBytes(node))
		first := protocolCorpusFindNode(node, "Protocol Type")
		require.NotNil(t, first)
		xyplexTestFields(t, first.Cfg.GetItem(base.CfgParent).(*base.Node), f, 64)
	}
	f := xyplexTestFixtures()[0]
	udp := xyplexTestUDP(f.wire(), false)
	binary.BigEndian.PutUint16(udp, 173)
	node := protocolCorpusRequireBoundedRuleParse(t, udp, "user_datagram_protocol", "UDP")
	first := protocolCorpusFindNode(node, "Protocol Type")
	require.NotNil(t, first)
	xyplexTestFields(t, first.Cfg.GetItem(base.CfgParent).(*base.Node), f, 64) // destination wins for 173 -> 173.
	// No packet-body heuristic: the same eight bytes have distinct layouts
	// when the caller explicitly chooses a request or reply entry.
	reply := xyplexTestFixture{reply: true, typeValue: f.typeValue, padding: f.padding, status: f.server, tail: f.wire()[4:]}
	node = protocolCorpusRequireBoundedRuleParse(t, f.wire(), "xyplex", reply.entry())
	xyplexTestFields(t, node, reply, 0)
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(xyplexTestFixtures()[1].wire()), "xyplex", f.entry())
	require.ErrorContains(t, err, "8-byte request header")
	for _, reply := range []bool{false, true} {
		f := xyplexTestFixture{reply: reply}
		bad := make([]byte, f.header()-1)
		udp := xyplexTestUDP(bad, reply)
		node := protocolCorpusRequireBoundedRuleParse(t, udp, "user_datagram_protocol", "UDP")
		require.Equal(t, udp, NodeToBytes(node))
		miopTestField(t, node, f.rawName(), bad, 64, uint64(len(udp))*8)
		require.Nil(t, protocolCorpusFindNode(node, "Protocol Type"))
	}
	udp = xyplexTestUDP(make([]byte, 8), false)
	binary.BigEndian.PutUint16(udp[2:], 40102)
	node = protocolCorpusRequireBoundedRuleParse(t, udp, "user_datagram_protocol", "UDP")
	require.Equal(t, udp, NodeToBytes(node))
	require.Nil(t, protocolCorpusFindNode(node, "Protocol Type"))
}

func xyplexTestReject(t *testing.T, wire []byte, f xyplexTestFixture) {
	t.Helper()
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "xyplex", f.entry())
	require.Error(t, err)
	if len(wire) == 0 {
		_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "xyplex", f.entry()+"Carrier")
		require.Error(t, err)
		return
	}
	node := protocolCorpusRequireBoundedRuleParse(t, wire, "xyplex", f.entry()+"Carrier")
	require.Equal(t, wire, NodeToBytes(node))
	miopTestField(t, node, f.rawName(), wire, 0, uint64(len(wire))*8)
	require.Nil(t, protocolCorpusFindNode(node, "Protocol Type"))
}

func TestProtocolCorpusXYPLEXPrefixesValuesAndResources(t *testing.T) {
	for _, f := range xyplexTestFixtures() {
		wire := f.wire()
		for cut := 0; cut < len(wire); cut++ {
			if cut < f.header() {
				xyplexTestReject(t, wire[:cut], f)
			} else {
				short := f
				short.tail = wire[f.header():cut]
				node := protocolCorpusRequireBoundedRuleParse(t, wire[:cut], "xyplex", f.entry())
				xyplexTestFields(t, node, short, 0)
				require.Equal(t, wire[:cut], NodeToBytes(node))
			}
		}
	}
	// The reference has no type/pad/reserved/serial-port validity constraints.
	// Values here prove preservation, not successful registration semantics.
	for _, reply := range []bool{false, true} {
		for _, v := range []uint16{0, 1, 5, 255, 256, 0x7fff, 0xffff} {
			f := xyplexTestFixture{reply: reply, typeValue: uint8(v), padding: uint8(v >> 8), server: v, returned: ^v, reserved: v, status: v}
			node := protocolCorpusRequireBoundedRuleParse(t, f.wire(), "xyplex", f.entry())
			xyplexTestFields(t, node, f, 0)
		}
		f := xyplexTestFixture{reply: reply}
		f.tail = bytes.Repeat([]byte{0xa5}, 65527-f.header())
		require.Len(t, f.wire(), 65527)
		for _, entry := range []string{f.entry(), f.entry() + "Carrier"} {
			node := protocolCorpusRequireBoundedRuleParse(t, f.wire(), "xyplex", entry)
			xyplexTestFields(t, node, f, 0)
			require.Equal(t, f.wire(), NodeToBytes(node))
			_, err := parser.ParseBinary(bytes.NewReader(f.wire()), "xyplex", entry)
			require.ErrorContains(t, err, "explicit datagram boundary")
			_, err = parser.GenerateBinary(map[string]any{}, "xyplex", entry)
			require.ErrorContains(t, err, "explicit datagram boundary")
			for bits := uint64(1); bits < 8; bits++ {
				reader := &alljoynTestBitBoundaryReader{bytes.NewReader(f.wire()), uint64(len(f.wire()))*8 - bits}
				_, err := parser.ParseBinary(reader, "xyplex", entry)
				require.ErrorContains(t, err, "explicit byte boundary")
				require.Equal(t, len(f.wire()), reader.Len())
			}
		}
		f.tail = append(f.tail, 0)
		reader := newProtocolCorpusBoundedReader(f.wire())
		_, err := parser.ParseBinary(reader, "xyplex", f.entry())
		require.ErrorContains(t, err, "implementation profile")
		require.Equal(t, 65528, reader.Len())
		xyplexTestReject(t, f.wire(), f)
	}
}

func TestProtocolCorpusXYPLEXImportOffsetsAndTransactions(t *testing.T) {
	for _, f := range xyplexTestFixtures() {
		for offset := 0; offset < 8; offset++ {
			for _, valid := range []bool{true, false} {
				wire := f.wire()
				if !valid {
					wire = wire[:f.header()-1]
				}
				prefix, padding := "", ""
				if offset > 0 {
					prefix = fmt.Sprintf("    Prefix: uint8,%dbit\n", offset)
					padding = fmt.Sprintf("    Padding: uint8,%dbit\n", 8-offset)
				}
				root := fcoeTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Record").SetMaxLength(%d)
      this.ProcessSubNode("Record")
      this.ProcessSubNode("Suffix")
      if %d > 0 { this.ProcessSubNode("Padding") }
%s    Record: "import:xyplex.yaml;node:%sCarrier"
    Suffix: uint8
%s`, offset, len(wire), offset, prefix, f.entry(), padding))
				fields := []snaTestField{}
				if offset > 0 {
					fields = append(fields, snaBit("Prefix", uint8((1<<offset)-1), uint64(offset)))
				}
				fields = append(fields, snaRaw("Record", wire), snaU8("Suffix", 0x5a))
				if offset > 0 {
					fields = append(fields, snaBit("Padding", 0, uint64(8-offset)))
				}
				input := snaEncode(fields)
				root.Cfg.SetItem(base.CfgLength, uint64(len(input))*8)
				reader := base.NewBitReader(bytes.NewReader(input))
				require.NoError(t, reader.Backup())
				require.NoError(t, root.ParseSubNode(reader, "Envelope"))
				node := base.GetNodeByPath(root, "@Envelope")
				record := protocolCorpusFindNode(node, "Record")
				if valid {
					xyplexTestFields(t, record, f, uint64(offset))
				} else {
					miopTestField(t, record, f.rawName(), wire, uint64(offset), uint64(offset+len(wire)*8))
					require.Nil(t, protocolCorpusFindNode(record, "Protocol Type"))
				}
				miopTestField(t, node, "Suffix", uint8(0x5a), uint64(offset+len(wire)*8), uint64(offset+(len(wire)+1)*8))
				require.Equal(t, input, NodeToBytes(node))
				require.NoError(t, reader.Recovery())
				replay, err := reader.ReadBits(uint64(len(input)) * 8)
				require.NoError(t, err)
				require.Equal(t, input, replay)
				require.ErrorContains(t, reader.Recovery(), "no backup")
			}
		}
	}
}

func TestProtocolCorpusXYPLEXHeldReaderAndIsolation(t *testing.T) {
	for _, f := range xyplexTestFixtures() {
		for _, valid := range []bool{true, false} {
			for _, rollback := range []bool{true, false} {
				wire := f.wire()
				if !valid {
					wire = wire[:f.header()-1]
				}
				reader := base.NewBitReader(bytes.NewReader(append(append([]byte{0xa5}, wire...), 0x5a)))
				_, err := reader.ReadBits(8)
				require.NoError(t, err)
				require.NoError(t, reader.Backup())
				root, err := base.ParseRule("xyplex.yaml")
				require.NoError(t, err)
				root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
				require.NoError(t, root.ParseSubNode(reader, f.entry()+"Carrier"))
				node := base.GetNodeByPath(root, "@"+f.entry()+"Carrier")
				require.Equal(t, wire, NodeToBytes(node))
				if valid {
					xyplexTestFields(t, node, f, 0)
				} else {
					require.Nil(t, protocolCorpusFindNode(node, "Protocol Type"))
					protocolCorpusRequireValue(t, node, f.rawName(), wire)
				}
				if rollback {
					require.NoError(t, reader.Recovery())
					replay, err := reader.ReadBits(uint64(len(wire)) * 8)
					require.NoError(t, err)
					require.Equal(t, wire, replay)
				} else {
					require.NoError(t, reader.PopBackup())
				}
				suffix, err := reader.ReadBits(8)
				require.NoError(t, err)
				require.Equal(t, []byte{0x5a}, suffix)
				require.ErrorContains(t, reader.Recovery(), "no backup")
				_, err = reader.ReadBits(8)
				require.ErrorIs(t, err, io.EOF)
			}
		}
		for _, entry := range []string{f.entry(), f.entry() + "Carrier"} {
			wire := f.wire()
			for cut := 0; cut < len(wire); cut++ {
				root, err := base.ParseRule("xyplex.yaml")
				require.NoError(t, err)
				root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
				reader := base.NewBitReader(bytes.NewReader(wire[:cut]))
				require.NoError(t, reader.Backup())
				require.Error(t, root.ParseSubNode(reader, entry))
				require.NoError(t, reader.Recovery())
				if cut > 0 {
					replay, err := reader.ReadBits(uint64(cut) * 8)
					require.NoError(t, err)
					require.Equal(t, wire[:cut], replay)
				}
			}
		}
	}
	var wait sync.WaitGroup
	errors := make(chan error, 10)
	for index := 0; index < 10; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			f := xyplexTestFixtures()[index%5]
			wire := f.wire()
			valid := index%2 == 0
			if !valid {
				wire = wire[:f.header()-1]
			}
			node, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "xyplex", f.entry()+"Carrier")
			if err == nil {
				_, err = node.Result()
			}
			if err == nil && (!bytes.Equal(wire, NodeToBytes(node)) || (protocolCorpusFindNode(node, "Protocol Type") != nil) != valid || (protocolCorpusFindNode(node, f.rawName()) != nil) == valid) {
				err = fmt.Errorf("XYPLEX context leaked at %d", index)
			}
			errors <- err
		}(index)
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}
