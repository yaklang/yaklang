package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

const megacoTestRule = "application-layer.megaco"

const megacoTestV2Wire = "MEGACO/2 <mg-tr>:16384\nReply = 174091{\nContext = 255{\nModify = MUX/255\n}\n}\n"

// Independent wire literals constructed directly from RFC 3525 Annex B.
// These assert textual structure, not successful operations, valid SDP sessions,
// package-specific values, or correspondence between transactions.
var megacoTestFixtures = []struct{ name, wire string }{
	{"service-change", `MEGACO/1 [192.0.2.10]:2944 Transaction=1{Context=-{ServiceChange=ROOT{Services{Method=Restart,Reason="901 restart",Profile=demo/1,Version=1}}}}`},
	{"multiple-commands", "MEGACO/1 <mg.example> Transaction=42{Context=${Priority=7,Add=rtp/${Media{Stream=1{LocalControl{Mode=SendReceive,ReservedGroup=OFF,foo/bar=[1,2]},Local{v=0\r\ns=example\\}text\r\n}}},Signals{tonegen/play{Duration=100,SignalType=Brief}}},Modify=rtp/1{Events=7{al/of{Stream=1}},EventBuffer{al/of},DigitMap=digits,Audit{Media,Signals}}},Context=1{Subtract=rtp/2{Audit{}},AuditValue=rtp/3{Audit{Media,Statistics}}}}"},
	{"compact", `!/1 mg1 T=3{C=1{O-W-A=rtp/${M{O{MO=SR}}},MV=aaln/1,AC=rtp/1{AT{M,SG}},N=aaln/1{OE=9{20260906T12345678:al/of{ST=1,foo="x,y"}}}}}`},
	{"reply", `MEGACO/1 mg1 Reply=42{ImmAckRequired,Context=1{Add=rtp/1{Media{LocalControl{Mode=SendReceive}},Statistics{rtp/ps=10},Packages{root-1}},Modify=rtp/2,Notify=aaln/1,ServiceChange=ROOT{Services{Version=1,Profile=demo/1}}}}`},
	{"pending-and-ack", `MEGACO/1 [2001:db8::1] Pending=7{} TransactionResponseAck{1,3-7}`},
	{"error", `MEGACO/1 mg1 Error = 500 {"unavailable context"}`},
	{"comments", "; leading\r\nmEgAcO/01 ; sender\r\n[001.002.003.004]:00080 ; body\nT=0{C=-{S=ROOT}} ; tail\r\n"},
}

func megacoTestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	return protocolCorpusRequireBoundedRuleParse(t, wire, megacoTestRule, entry)
}

func megacoTestTree(t *testing.T, n *base.Node, start, end uint64) {
	t.Helper()
	value, err := n.Result()
	require.NoError(t, err, n.Name)
	require.Same(t, n, value.Origin, n.Name)
	if stream_parser.NodeHasResult(n) {
		require.Equal(t, [2]uint64{start, end}, stream_parser.GetNodeResultPos(n), n.Name)
		return
	}
	require.Len(t, value.Children(), len(n.Children), n.Name)
	position := start
	for i, child := range n.Children {
		require.Same(t, n, child.Cfg.GetItem(base.CfgParent))
		require.Same(t, n.Ctx, child.Ctx)
		require.Same(t, child, value.Children()[i].Origin)
		length := stream_parser.CalcNodeConsumedLength(child)
		megacoTestTree(t, child, position, position+length)
		position += length
	}
	require.Equal(t, end, position, n.Name)
}

func megacoTestMessage(t *testing.T, n *base.Node) *base.Node {
	t.Helper()
	field := protocolCorpusFindNode(n, "Protocol Marker")
	require.NotNil(t, field)
	return field.Cfg.GetItem(base.CfgParent).(*base.Node)
}

func megacoTestInfo(t *testing.T, n *base.Node) {
	t.Helper()
	message := megacoTestMessage(t, n)
	info := message.Cfg.GetItem("additionInfo").(map[string]any)
	version, err := protocolCorpusFindNode(n, "Version").Result()
	require.NoError(t, err)
	profile := "RFC 3525 v1 bounded text structural subset"
	if version.Value == "2" || version.Value == "02" {
		profile = "ITU-T H.248.1 (05/2002) v2 command-only AMMS replies"
	}
	require.Equal(t, profile, info["Profile"])
	for _, key := range []string{"Binary Encoding Decoded", "SDP Semantics Decoded", "Package Semantics Decoded", "Transaction Outcome Validated", "Session State Validated"} {
		require.Equal(t, false, info[key], key)
	}
}

func megacoTestField(t *testing.T, n *base.Node, name, want string, start, end uint64) {
	t.Helper()
	latTestField(t, n, name, "string", start, end, want)
}

func TestProtocolCorpusMegacoFields(t *testing.T) {
	for _, f := range megacoTestFixtures {
		for _, entry := range []string{"Megaco", "MegacoCarrier"} {
			t.Run(f.name+"/"+entry, func(t *testing.T) {
				wire := []byte(f.wire)
				node := megacoTestParse(t, wire, entry)
				require.Equal(t, wire, NodeToBytes(node))
				megacoTestInfo(t, node)
				megacoTestTree(t, megacoTestMessage(t, node), 0, uint64(len(wire))*8)
			})
		}
	}
	f := megacoTestFixtures[0]
	n := megacoTestParse(t, []byte(f.wire), "Megaco")
	for _, field := range []struct{ name, text string }{{"Protocol Marker", "MEGACO"}, {"Version", "1"}, {"Address", "192.0.2.10"}, {"Port", "2944"}, {"Transaction ID", "1"}, {"Context ID", "-"}, {"Termination ID", "ROOT"}, {"Service Change Method", "Restart"}, {"Service Change Reason", `"901 restart"`}} {
		start := strings.Index(f.wire, field.text)
		if field.name == "Transaction ID" {
			start = strings.Index(f.wire, "Transaction=1") + len("Transaction=")
		}
		megacoTestField(t, n, field.name, field.text, uint64(start)*8, uint64(start+len(field.text))*8)
	}
	transaction := protocolCorpusFindNode(n, "Transaction ID")
	require.Equal(t, uint64(1), transaction.Cfg.GetItem("additionInfo").(map[string]any)["Numeric Value"])
	compound := megacoTestParse(t, []byte(megacoTestFixtures[1].wire), "Megaco")
	protocolCorpusRequireValue(t, compound, "SDP Text", []byte("v=0\r\ns=example\\}text\r\n"))
	compact := megacoTestParse(t, []byte(megacoTestFixtures[2].wire), "Megaco")
	command := protocolCorpusFindNode(compact, "Command")
	require.Equal(t, "Add", command.Cfg.GetItem("additionInfo").(map[string]any)["Command Kind"])
}

func TestProtocolCorpusMegacoGrammarBranches(t *testing.T) {
	for _, body := range []string{
		`T=1{C=-{Priority=65535,Emergency,Topology{aaln/1,aaln/2,BW},CA{TP,PR,EG},A=ROOT}}`,
		`T=1{C=-{A=ROOT{Media{TerminationState{ServiceStates=InService,Buffer=LockStep,foo/bar>0},LocalControl{Mode=Loopback,ReservedValue=ON,foo/bar=[1:2],foo/baz={"a b",c}},Local{},Remote{v=0}},Mux=H221{aaln/1,aaln/2},Modem[V32b,SN]{foo/bar#0}}}}`,
		`T=1{C=-{A=ROOT{Signals{SignalList=7{tone/play{SignalType=Brief,Duration=10,NotifyCompletion={TO,IBE,IBS,OR},KeepActive},tone/stop{SY=OO}},tone/play{Media=1,M=2}},Events=7{foo/bar{KeepActive,DigitMap=digits,Media=1,M=2}}}}}`,
		`T=1{C=-{N=ROOT{OE=*{foo/bar{DigitMap=1,DM=2,Media=1,M=2}},ER=401{}}}}`,
		`T=1{C=-{SC=ROOT{SV{MT=HO,RE="903",AD=[192.0.2.4]:1234,DL=4294967295,20260906T23595999}}}}`,
		`P=1{C=1{AV=ROOT{Media ; between token and delimiter
,Signals},S=ROOT{ER=500{}},N=ROOT{ER=500{}},SC=ROOT{ER=500{}},ER=500{}}}`,
		`P=1{ER=500{}}`, `K{4294967295-0}`, `PN=4294967295{}`,
	} {
		wire := []byte("MEGACO/1 mg1 " + body)
		n := megacoTestParse(t, wire, "Megaco")
		require.Equal(t, wire, NodeToBytes(n))
		megacoTestTree(t, megacoTestMessage(t, n), 0, uint64(len(wire))*8)
	}
	// V4hex admits leading zeroes, including its RFC2373 embedded IPv6 form.
	wire := []byte("MEGACO/1 [::ffff:001.002.003.004] PN=0{}")
	n := megacoTestParse(t, wire, "Megaco")
	protocolCorpusRequireValue(t, n, "Address", "::ffff:001.002.003.004")
	require.Equal(t, wire, NodeToBytes(n))
}

func TestProtocolCorpusMegacoV2ReplyProfile(t *testing.T) {
	for _, wire := range []string{megacoTestV2Wire, `!/02 mg1 P=1{IA,C=1{A=ROOT,MV=rtp/1,MF=rtp/2,S=aaln/1},C=-{MF=ROOT}}`} {
		for _, entry := range []string{"Megaco", "MegacoCarrier"} {
			n := megacoTestParse(t, []byte(wire), entry)
			require.Equal(t, []byte(wire), NodeToBytes(n))
			megacoTestTree(t, megacoTestMessage(t, n), 0, uint64(len(wire))*8)
			megacoTestInfo(t, n)
		}
	}
	n := megacoTestParse(t, []byte(megacoTestV2Wire), "Megaco")
	for _, field := range []struct{ name, text string }{{"Version", "2"}, {"Address", "mg-tr"}, {"Port", "16384"}, {"Transaction ID", "174091"}, {"Context ID", "255"}, {"Command Name", "Modify"}, {"Termination ID", "MUX/255"}} {
		start := strings.Index(megacoTestV2Wire, field.text)
		megacoTestField(t, n, field.name, field.text, uint64(start)*8, uint64(start+len(field.text))*8)
	}
	for cut := 0; cut < len(megacoTestV2Wire)-1; cut++ {
		megacoTestReject(t, []byte(megacoTestV2Wire[:cut]), "")
	}
	// V2's real AMMS reply branch is supported; this does not enable its
	// expanded individual-audit/descriptor grammar or all v1 structures.
	for _, body := range []string{`T=1{C=-{A=ROOT}}`, `PN=1{}`, `K{1}`, `ER=500{}`, `P=1{ER=500{}}`, `P=1{C=1{ER=500{}}}`, `P=1{C=1{MF=ROOT{M{O{MO=IN}}}}}`, `P=1{C=1{N=ROOT}}`, `P=1{C=1{AV=ROOT}}`, `P=1{C=1{SC=ROOT}}`, `P=1{C=1{Priority=1,MF=ROOT}}`, `P=1{C=1{CA{PR},MF=ROOT}}`, `P=1{C=1{MF=ROOT}}PN=2{}`} {
		megacoTestReject(t, []byte("MEGACO/2 mg1 "+body), "unsupported v2")
	}
	for _, body := range []string{`P=1{C=1{}}`, `P=1{C=0{MF=ROOT}}`, `P=4294967296{C=1{MF=ROOT}}`, `P=1{C=1{MF=ROOT,}}`} {
		megacoTestReject(t, []byte("MEGACO/2 mg1 "+body), "")
	}
}

func megacoTestReject(t *testing.T, wire []byte, reason string) {
	t.Helper()
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), megacoTestRule, "Megaco")
	require.Error(t, err)
	if reason != "" {
		require.ErrorContains(t, err, reason)
	}
	if len(wire) == 0 {
		return
	}
	n := megacoTestParse(t, wire, "MegacoCarrier")
	require.Equal(t, wire, NodeToBytes(n))
	latTestField(t, n, "Unparsed Megaco Payload", "raw", 0, uint64(len(wire))*8, wire)
	require.Nil(t, protocolCorpusFindNode(n, "Protocol Marker"))
	require.False(t, n.Cfg.Has("additionInfo"))
}

func TestProtocolCorpusMegacoAllOriginalRecords(t *testing.T) {
	const path = "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-megaco.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "5adece609846c5505c04960ce052abeebfda33c88fdaf2381e2edc9ee0c40fd3", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 1)
	for _, frame := range frames {
		require.Len(t, frame, 101)
		require.Equal(t, uint16(0x800), binary.BigEndian.Uint16(frame[12:14]))
		require.Equal(t, byte(17), frame[23])
		require.Equal(t, uint16(2944), binary.BigEndian.Uint16(frame[36:38]))
		require.Equal(t, uint16(67), binary.BigEndian.Uint16(frame[38:40]))
		wire := frame[42:]
		require.Equal(t, "MEGACO/1 [10.0.0.1]:2944 Transaction = 1 { Context = - { }}", string(wire))
		require.Len(t, wire, 59)
		// Both equals delimiters exist. The empty Context is the first
		// definite Annex B actionRequest violation, not a missing equals.
		megacoTestReject(t, wire, "empty Context structure")
		node := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, frame, NodeToBytes(node))
	}
}

func TestProtocolCorpusMegacoCompanionRecords(t *testing.T) {
	const path = "testdata/protocol-corpus/captures/generated-validated/gen-megaco-valid.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "d73a1f18d4aae1d04a3d588aa9f6e83f0e4f7e43fb72db195114bcebb6731ad3", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, len(megacoTestFixtures))
	for i, frame := range frames {
		require.GreaterOrEqual(t, len(frame), 42)
		require.Equal(t, byte(0x45), frame[14])
		require.Equal(t, byte(17), frame[23])
		require.Equal(t, len(frame)-14, int(binary.BigEndian.Uint16(frame[16:18])))
		require.Equal(t, len(frame)-34, int(binary.BigEndian.Uint16(frame[38:40])))
		require.Equal(t, megacoTestFixtures[i].wire, string(frame[42:]))
		for _, entry := range []string{"Megaco", "MegacoCarrier"} {
			n := megacoTestParse(t, frame[42:], entry)
			require.Equal(t, frame[42:], NodeToBytes(n))
			megacoTestInfo(t, n)
			megacoTestTree(t, megacoTestMessage(t, n), 0, uint64(len(frame)-42)*8)
		}
	}
}

func TestProtocolCorpusMegacoOtherOriginalCaptures(t *testing.T) {
	// The capture's roadmap label is not a wire protocol filter: T.38's
	// session signaling has 130 real Megaco datagrams among 1552 records.
	const t38 = "testdata/protocol-corpus/captures/ndpi/ndpi-t38.pcap"
	data, err := os.ReadFile(t38)
	require.NoError(t, err)
	require.Equal(t, "b402d3ee4ccb32fba3c75a90fdb8ae50ab9a58f9f5e51a72d9d873b18a68d99e", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, t38)
	require.Len(t, frames, 1552)
	count := 0
	for index, frame := range frames {
		if len(frame) < 42 || binary.BigEndian.Uint16(frame[12:14]) != 0x800 || frame[23] != 17 {
			continue
		}
		ihl := int(frame[14]&15) * 4
		require.GreaterOrEqual(t, ihl, 20)
		udp := 14 + ihl
		if binary.BigEndian.Uint16(frame[udp:udp+2]) != 2944 && binary.BigEndian.Uint16(frame[udp+2:udp+4]) != 2944 {
			continue
		}
		count++
		t.Run(fmt.Sprintf("t38-frame-%d", index+1), func(t *testing.T) {
			require.Zero(t, binary.BigEndian.Uint16(frame[20:22])&0x3fff)
			length := int(binary.BigEndian.Uint16(frame[udp+4 : udp+6]))
			require.Equal(t, ihl+length, int(binary.BigEndian.Uint16(frame[16:18])))
			require.LessOrEqual(t, udp+length, len(frame))
			wire := frame[udp+8 : udp+length]
			n := megacoTestParse(t, wire, "Megaco")
			require.Equal(t, wire, NodeToBytes(n))
			protocolCorpusRequireValue(t, n, "Protocol Marker", "!")
			protocolCorpusRequireValue(t, n, "Version", "1")
			require.NotNil(t, protocolCorpusFindNode(n, "Command"))
			megacoTestInfo(t, n)
			megacoTestTree(t, megacoTestMessage(t, n), 0, uint64(len(wire))*8)
		})
	}
	require.Equal(t, 130, count)

	// All four SCTP records are inspected. Only record 1 contains a DATA
	// chunk (PPID 7); its MEGACO/2 is an explicitly supported AMMS Reply. The
	// single SCTP padding octet 0x67 is NOT part of the 75-byte message.
	const sctp = "testdata/protocol-corpus/captures/ndpi/ndpi-sctp.cap"
	data, err = os.ReadFile(sctp)
	require.NoError(t, err)
	require.Equal(t, "9f188aa749ffe872d9e077aa909a06db5cbd14033b29e84c11b65d10f520d7df", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames = protocolCorpusAuditPackets(t, sctp)
	require.Len(t, frames, 4)
	for i, frame := range frames {
		require.Equal(t, byte(0x45), frame[14])
		require.Equal(t, byte(132), frame[23])
		require.Equal(t, []byte{0, 3, 4, 5}[i], frame[46])
		if i != 0 {
			continue
		}
		require.Equal(t, byte(3), frame[47]) // complete unfragmented DATA
		require.Equal(t, uint16(91), binary.BigEndian.Uint16(frame[48:50]))
		require.Equal(t, uint32(7), binary.BigEndian.Uint32(frame[58:62]))
		wire := frame[62:137]
		require.Equal(t, megacoTestV2Wire, string(wire))
		require.Equal(t, byte(0x67), frame[137])
		for _, entry := range []string{"Megaco", "MegacoCarrier"} {
			n := megacoTestParse(t, wire, entry)
			require.Equal(t, wire, NodeToBytes(n))
			megacoTestInfo(t, n)
			megacoTestTree(t, megacoTestMessage(t, n), 0, uint64(len(wire))*8)
			protocolCorpusRequireValue(t, n, "Version", "2")
			protocolCorpusRequireValue(t, n, "Transaction ID", "174091")
			protocolCorpusRequireValue(t, n, "Context ID", "255")
			protocolCorpusRequireValue(t, n, "Termination ID", "MUX/255")
		}
		megacoTestReject(t, frame[62:], "unsupported")
	}
}

func TestProtocolCorpusMegacoPrefixesAndNegatives(t *testing.T) {
	for _, f := range megacoTestFixtures {
		t.Run(f.name, func(t *testing.T) {
			for cut := 0; cut < len(f.wire); cut++ {
				// A complete first transaction or trailing layout is itself a
				// valid shorter message; it must not be labeled truncation.
				valid := false
				if f.name == "pending-and-ack" {
					end := strings.Index(f.wire, " TransactionResponseAck")
					valid = cut == end || cut == end+1
				} else if f.name == "comments" {
					end := strings.Index(f.wire, " ; tail")
					valid = cut == end || cut == end+1 || cut == len(f.wire)-1
				}
				if valid {
					n := megacoTestParse(t, []byte(f.wire[:cut]), "Megaco")
					require.Equal(t, []byte(f.wire[:cut]), NodeToBytes(n))
					continue
				}
				megacoTestReject(t, []byte(f.wire[:cut]), "")
			}
		})
	}
	for _, body := range []string{
		`T=1{C=-{}}`, `T=1{}`, `PN=1{C=-{A=ROOT}}`, `K{}`, `T=4294967296{C=-{A=ROOT}}`, `T=1{C=0{A=ROOT}}`, `T=1{C=4294967295{A=ROOT}}`,
		`T=1{C=-{A=ROOT{}}}`, `T=1{C=-{X=ROOT}}`, `T=1{C=-{A=ROOT{Unknown{a=b}}}}`, `T=1{C=-{A=ROOT{Media{}}}}`, `T=1{C=-{A=ROOT{Media{LocalControl{Mode=bogus}}}}}`,
		`T=1{C=-{A=ROOT{Media{Stream=1{LocalControl{Mode=IN}},LocalControl{Mode=IN}}}}}`, `T=1{C=-{A=ROOT{Signals{SignalList=1{SignalList=2{foo/bar}}}}}}`,
		`T=1{C=-{AV=ROOT}}`, `T=1{C=-{AC=ROOT{AT{DM}}}}`, `T=1{C=-{N=ROOT{Signals{}}}}`, `T=1{C=-{SC=ROOT{SV{MT=RS}}}}`, `T=1{C=-{SC=ROOT{SV{MT=RS,RE=""}}}}`,
		`T=1{C=-{SC=ROOT{SV{MT=RS,RE=901}}}}`, `T=1{C=-{SC=ROOT{SV{MT=RS,MT=GR,RE="901"}}}}`, `T=1{C=-{A=ROOT{DM={1|2}}}}`, `T=1{C=-{A=ROOT{E=1{foo/bar{EM{E}}}}}}`,
		`T=1{C=-{A=ROOT,Priority=1}}`, `T=1{C=-{CA{PR},Priority=1}}`, `T=1{C=-{A=ROOT,}}`, `P=1{C=1{ER=500{},A=ROOT}}`, `ER=500{unquoted}`, `ER=10000{}`,
		`K{1 -2}`, `K{1- 2}`, `T=1{C=-{CA{PR,Priority}}}`, `P=1{C=1{A=ROOT{SA{foo/bar=1,FOO/BAR=2}}}}`,
		`T=1{C=-{A=ROOT{M{O{foo/bar=[1 :2]}}}}}`, `T=1{C=-{A=ROOT{M{O{foo/bar=[1: 2]}}}}}`,
		`T=1{C=-{A=ROOT{SG{SL=1{foo/bar}}}}}`, `T=1{C=-{A=ROOT{SG{SL=1{foo/bar{DR=1}}}}}}`, `T=1{C=-{A=ROOT{MD[SN,SynchISDN]}}}`,
		`T=1{C=-{SC=ROOT{SV{MT=RS,RE="901",PF=demo/+1}}}}`, `P=1{C=1{A=ROOT{PG{root-+1}}}}`,
	} {
		t.Run(body, func(t *testing.T) { megacoTestReject(t, []byte("MEGACO/1 mg1 "+body), "") })
	}
	for _, wire := range []string{"MEGACO/2 mg1 PN=1{}", "MEGACO/ 1 mg1 PN=1{}", "MEGACO/1[192.0.2.1] PN=1{}", "MEGACO/1 [256.0.0.1] PN=1{}", "MEGACO/1 [+1.2.3.4] PN=1{}", "MEGACO/1 [2001:::1] PN=1{}", "MEGACO/1 [fe80::1%en0] PN=1{}", "MEGACO/1 <a_b> PN=1{}", "MEGACO/1 [1.2.3.4]:65536 PN=1{}", "MEGACO/1 mg1 PN=1{};unterminated", "MEGACO/1 mg1 PN=1{}\x00"} {
		megacoTestReject(t, []byte(wire), "")
	}
}

func TestProtocolCorpusMegacoResourcesAndIsolation(t *testing.T) {
	baseWire := []byte("!/1 mg1 PN=0{}")
	maximum := append(bytes.Clone(baseWire), bytes.Repeat([]byte{' '}, 65527-len(baseWire))...)
	node := megacoTestParse(t, maximum, "Megaco")
	require.Equal(t, maximum, NodeToBytes(node))
	tooLong := newProtocolCorpusBoundedReader(append(maximum, ' '))
	_, err := parser.ParseBinary(tooLong, megacoTestRule, "Megaco")
	require.ErrorContains(t, err, "65527")
	require.Equal(t, 65528, tooLong.Len())
	many := []byte("!/1 mg1 " + strings.Repeat("PN=1{}", 2800))
	megacoTestReject(t, many, "field-node resource limit")
	for _, entry := range []string{"Megaco", "MegacoCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(baseWire), megacoTestRule, entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, megacoTestRule, entry)
		require.Error(t, err)
		for extra := uint64(1); extra < 8; extra++ {
			r := &giopCarrierTestBitReader{Reader: bytes.NewReader(append(bytes.Clone(baseWire), 0)), bits: uint64(len(baseWire))*8 + extra}
			_, err = parser.ParseBinary(r, megacoTestRule, entry)
			require.ErrorContains(t, err, "byte")
			require.Equal(t, len(baseWire)+1, r.Len())
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			wire := []byte(megacoTestFixtures[i%len(megacoTestFixtures)].wire)
			n := megacoTestParse(t, wire, "MegacoCarrier")
			megacoTestInfo(t, n)
			require.Equal(t, wire, NodeToBytes(n))
		}(i)
	}
	wg.Wait()
}

func TestProtocolCorpusMegacoImportedHeldReader(t *testing.T) {
	for _, messageWire := range []string{megacoTestFixtures[0].wire, megacoTestV2Wire} {
		for offset := uint64(0); offset < 8; offset++ {
			for _, valid := range []bool{false, true} {
				t.Run(fmt.Sprintf("offset%d/valid%t", offset, valid), func(t *testing.T) {
					wire := []byte(messageWire)
					if !valid {
						wire[len(wire)-1] = '!'
					}
					var packed bytes.Buffer
					w := base.NewBitWriter(&packed)
					if offset > 0 {
						require.NoError(t, w.WriteBits([]byte{0x55}, offset))
					}
					require.NoError(t, w.WriteBits(wire, uint64(len(wire))*8))
					require.NoError(t, w.WriteBits([]byte{0xd3}, 8))
					if offset > 0 {
						require.NoError(t, w.WriteBits([]byte{0}, 8-offset))
					}
					root := latTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Message").SetMaxLength(%d)
      this.ProcessSubNode("Message")
      this.ProcessSubNode("Sentinel")
      if %d > 0 { this.ProcessSubNode("Padding") }
    Prefix: uint8,%dbit
    Message: "import:application-layer/megaco.yaml;node:MegacoCarrier"
    Sentinel: uint8
    Padding: uint8,%dbit
`, offset, len(wire), offset, offset, 8-offset))
					root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
					r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
					require.NoError(t, r.Backup())
					require.NoError(t, root.ParseSubNode(r, "Envelope"))
					n := base.GetNodeByPath(root, "@Envelope")
					message := protocolCorpusFindNode(n, "Message")
					if valid {
						megacoTestTree(t, megacoTestMessage(t, message), offset, offset+uint64(len(wire))*8)
						megacoTestInfo(t, message)
					} else {
						latTestField(t, message, "Unparsed Megaco Payload", "raw", offset, offset+uint64(len(wire))*8, wire)
						require.Nil(t, protocolCorpusFindNode(message, "Protocol Marker"))
					}
					protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
					require.Equal(t, packed.Bytes(), NodeToBytes(n))
					require.NoError(t, r.Recovery())
					replay, err := r.ReadBits(uint64(packed.Len()) * 8)
					require.NoError(t, err)
					require.Equal(t, packed.Bytes(), replay)
					require.ErrorContains(t, r.Recovery(), "no backup")
					_, err = r.ReadBits(8)
					require.ErrorIs(t, err, io.EOF)
				})
			}
		}
	}
}

func TestProtocolCorpusMegacoPhysicalTruncation(t *testing.T) {
	wire := []byte(megacoTestFixtures[0].wire)
	for _, entry := range []string{"Megaco", "MegacoCarrier"} {
		for cut := 0; cut < len(wire); cut++ {
			root, err := base.ParseRule("application-layer/megaco.yaml")
			require.NoError(t, err)
			root.Cfg.SetItem(base.CfgLength, uint64(len(wire))*8)
			r := base.NewBitReader(bytes.NewReader(wire[:cut]))
			require.NoError(t, r.Backup())
			require.Error(t, root.ParseSubNode(r, entry))
			require.NoError(t, r.Recovery())
			if cut > 0 {
				got, err := r.ReadBits(uint64(cut) * 8)
				require.NoError(t, err)
				require.Equal(t, wire[:cut], got)
			}
			require.ErrorContains(t, r.PopBackup(), "no backup")
		}
	}
}
