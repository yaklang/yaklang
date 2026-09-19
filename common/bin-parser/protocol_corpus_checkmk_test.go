package bin_parser

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// The independent text oracle compares every row and column, retaining order,
// duplicate section names, empty sections and empty explicitly delimited cells.
func protocolCorpusCheckmkFields(t *testing.T, node *base.Node, wire []byte) (int, []string) {
	t.Helper()
	require.True(t, bytes.HasSuffix(wire, []byte{'\n'}))
	lines := strings.Split(string(wire[:len(wire)-1]), "\n")
	parsed := protocolCorpusFindNode(node, "Lines")
	require.NotNil(t, parsed)
	require.Len(t, parsed.Children, len(lines))
	section, separator := "", -1
	var names []string
	for i, line := range lines {
		child := parsed.Children[i]
		protocolCorpusRequireValue(t, child, "Text", line)
		info := child.Cfg.GetItem("additionInfo").(map[string]any)
		if strings.HasPrefix(line, "<<<") {
			require.True(t, strings.HasSuffix(line, ">>>"))
			parts := strings.Split(line[3:len(line)-3], ":")
			section, separator = parts[0], -1
			if len(parts) == 2 {
				var err error
				separator, err = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(parts[1], "sep("), ")"))
				require.NoError(t, err)
			}
			names = append(names, section)
			require.Equal(t, "section", info["Kind"])
		} else {
			require.Equal(t, "row", info["Kind"])
			var columns []string
			switch separator {
			case -1:
				columns = strings.Fields(line)
			case 0:
				columns = []string{line}
			default:
				columns = strings.Split(line, string(rune(separator)))
			}
			require.Equal(t, columns, info["Columns"], "line %d, section %s", i+1, section)
		}
		require.Equal(t, section, info["Section Name"])
		require.EqualValues(t, separator, info["Separator"])
	}
	require.EqualValues(t, len(names), node.Cfg.GetItem("additionInfo").(map[string]any)["Section Count"])
	return len(lines), names
}

func TestProtocolCorpusCheckmkEveryRecordAndCompleteReport(t *testing.T) {
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-checkmk.pcap")
	require.Len(t, frames, 98)
	var segments []soapCorpusTCPSegment
	var initial, final uint32
	var flow string
	for index, frame := range frames {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		envelope := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		// A frame boundary is not the end of this EOF-delimited report.
		require.Nil(t, protocolCorpusFindNode(envelope, "CheckmkReport"))
		if tcp.SrcPort == 6556 {
			if tcp.SYN {
				require.Equal(t, 2, index+1)
				initial = tcp.Seq + 1
			}
			if tcp.FIN {
				require.Equal(t, 96, index+1)
				final = tcp.Seq
			}
		}
		if len(tcp.Payload) == 0 {
			continue
		}
		require.EqualValues(t, 6556, tcp.SrcPort)
		direction := packet.NetworkLayer().NetworkFlow().String() + "|" + tcp.TransportFlow().String()
		if flow == "" {
			flow = direction
		}
		require.Equal(t, flow, direction)
		protocolCorpusRequireValue(t, envelope, "Remaining Payload", tcp.Payload)
		segments = append(segments, soapCorpusTCPSegment{frame: index + 1, seq: tcp.Seq, payload: bytes.Clone(tcp.Payload)})
	}
	require.Len(t, segments, 46)
	wire, err := soapCorpusReassembleTCP(segments)
	require.NoError(t, err)
	require.Len(t, wire, 13758)
	require.Equal(t, initial, segments[0].seq)
	require.Equal(t, initial+uint32(len(wire)), final, "SYN and FIN bound the entire captured report")
	for _, segment := range segments {
		offset := int(segment.seq - initial)
		require.Equal(t, segment.payload, wire[offset:offset+len(segment.payload)])
	}
	node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.checkmk", "CheckmkReport")
	lineCount, names := protocolCorpusCheckmkFields(t, node, wire)
	require.Equal(t, 338, lineCount)
	require.Equal(t, []string{"check_mk", "df", "df", "nfsmounts", "cifsmounts", "mounts", "ps", "mem", "cpu", "uptime", "lnx_if", "lnx_if", "tcp_conn_stats", "diskstat", "kernel", "md", "vbox_guest", "job", "local"}, names)
	// The protocol itself has no report length/end marker. When the transport
	// supplied an expected length, every shorter prefix, including a complete
	// header or a line-aligned prefix, must fail that external boundary check.
	for cut := 0; cut < len(wire); cut++ {
		_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire[:cut]), "application-layer.checkmk", map[string]any{"checkmkReportLength": len(wire)}, "CheckmkReport")
		require.ErrorContains(t, err, "external boundary", "cut %d/%d", cut, len(wire))
	}
	protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "application-layer.checkmk", "CheckmkReport", map[string]any{"checkmkReportLength": len(wire)})
}

func TestProtocolCorpusCheckmkSectionsLimitsAndReportIsolation(t *testing.T) {
	first := []byte("<<<sample:sep(58)>>>\na::c:\n<<<sample>>>\n one\t two  \n\n<<<empty>>>\n")
	second := []byte("<<<sample:sep(0)>>>\n a b : c \n<<<sample:sep(9)>>>\na\t\tb\t\n")
	for _, wire := range [][]byte{first, second} {
		node := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.checkmk", "CheckmkReport")
		protocolCorpusCheckmkFields(t, node, wire)
	}
	// Reset section context even if a caller-supplied map contains stale data.
	_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader([]byte("unframed row\n")), "application-layer.checkmk", map[string]any{"checkmkSection": "stale", "checkmkSeparator": 58}, "CheckmkReport")
	require.ErrorContains(t, err, "row precedes section")
	_, err = parser.ParseBinary(struct{ io.Reader }{bytes.NewReader(first)}, "application-layer.checkmk", "CheckmkReport")
	require.ErrorContains(t, err, "explicit boundary")
	for _, bad := range []string{"", "row\n", "<<<>>>\n", "<<<bad name>>>\n", "<<<x>>\n", "<<<<host>>>>\n", "<<<x:cached(1,2)>>>\n", "<<<x:sep(-1)>>>\n", "<<<x:sep()>>>\n", "<<<x:sep(128)>>>\n", "<<<x:sep(10)>>>\n", "<<<x:sep(9):sep(58)>>>\n", "<<<x>>>\nbad\x00row\n", "<<<x>>>\nunterminated"} {
		t.Run(fmt.Sprintf("invalid-%x", []byte(bad)), func(t *testing.T) {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader([]byte(bad)), "application-layer.checkmk", "CheckmkReport")
			require.Error(t, err)
		})
	}
	for _, config := range []map[string]any{{"checkmkReportLimit": 0}, {"checkmkReportLimit": len(first) - 1}, {"checkmkLineLimit": 0}, {"checkmkLineLimit": 10}} {
		_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(first), "application-layer.checkmk", config, "CheckmkReport")
		require.Error(t, err)
	}
	// A shorter, line-aligned input is a valid smaller report if its caller
	// explicitly supplies that boundary; no nonexistent terminator is inferred.
	protocolCorpusRequireBoundedRuleParse(t, []byte("<<<empty>>>\n"), "application-layer.checkmk", "CheckmkReport")
}

func TestProtocolCorpusTCPReassemblyRejectsConflictingOverlap(t *testing.T) {
	segment := func(frame int, seq uint32, data string) soapCorpusTCPSegment {
		return soapCorpusTCPSegment{frame: frame, seq: seq, payload: []byte(data)}
	}
	for _, segments := range [][]soapCorpusTCPSegment{
		{segment(1, 100, "abcde"), segment(2, 102, "cde"), segment(3, 104, "efg")},
		{segment(3, 104, "efg"), segment(2, 102, "cde"), segment(1, 100, "abcde")},
	} {
		wire, err := soapCorpusReassembleTCP(segments)
		require.NoError(t, err)
		require.Equal(t, []byte("abcdefg"), wire)
	}
	for _, conflict := range []soapCorpusTCPSegment{segment(2, 100, "abcXe"), segment(2, 102, "cXe"), segment(2, 104, "Xfg")} {
		_, err := soapCorpusReassembleTCP([]soapCorpusTCPSegment{segment(1, 100, "abcde"), conflict})
		require.ErrorContains(t, err, "overlap differs")
	}
	_, err := soapCorpusReassembleTCP([]soapCorpusTCPSegment{segment(1, 100, "abcde"), segment(2, 106, "g")})
	require.ErrorContains(t, err, "TCP gap")
}
