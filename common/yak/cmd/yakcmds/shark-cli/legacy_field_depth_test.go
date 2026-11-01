package sharkcli

import (
	"bytes"
	"encoding/json"
	"net/netip"
	"strings"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestLegacyFieldHierarchyKeepsIndexedRecordsAndFullFieldPaths(t *testing.T) {
	leaf := func(name string, value any) *base.NodeValue { return &base.NodeValue{Name: name, Value: value} }
	record := func(name string, children ...*base.NodeValue) *base.NodeValue {
		return leaf(name, children)
	}
	records := record("Options", record("Option", leaf("Kind", uint8(1))), record("Option", leaf("Kind", uint8(2))))
	records.ListValue = true
	tree := record("root", record("Package", record("Ethernet", leaf("Type", uint16(0x800)), record("Payload",
		record("IP", leaf("Version", uint8(4)), record("Payload", record("TCP", leaf("Source Port", uint16(1234)), records)))))))
	var fields []field
	flattenFields(tree, 0, &fields)
	depths := map[string]int{}
	var repeated []field
	for _, f := range fields {
		depths[f.text] = f.depth
		if strings.HasPrefix(f.text, "Options.Option.Kind:") {
			repeated = append(repeated, f)
		}
		require.False(t, f.hasRange, "a synthetic tree cannot invent original wire positions")
		require.NotContains(t, []string{"root", "Package", "Payload"}, f.text)
	}
	for _, name := range []string{"Ethernet", "IP", "TCP"} {
		require.Contains(t, depths, name)
		require.Zero(t, depths[name], "wire protocol layers remain sibling headings")
	}
	require.Equal(t, 1, depths["Source Port: 1234"])
	require.Equal(t, 1, depths["Options"])
	require.Equal(t, 2, depths["Options.Option[0]"])
	require.Equal(t, 2, depths["Options.Option[1]"])
	require.Len(t, repeated, 2)
	for _, f := range repeated {
		require.Equal(t, 3, f.depth)
	}
}

func TestLegacyRealPacketHierarchyAndByteRangesSurviveWorker(t *testing.T) {
	raw := tcpPacket(t, 42, 100, "", false, true, false, false)
	local := detail(raw)
	wire, err := json.Marshal(decodeRequest{Number: raw.number, Data: raw.data, Length: raw.ci.Length, Link: raw.link})
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(wire), &output))
	worker, err := decodeWorkerOutput(output.Bytes(), raw)
	require.NoError(t, err)
	require.Equal(t, local.fields, worker.fields)
	source := findField(t, local, "Source")
	require.Equal(t, 1, source.depth)
	require.Equal(t, 6, source.start)
	require.Equal(t, 12, source.end)
	require.True(t, source.hasRange)
	for _, f := range worker.fields {
		if f.hasRange {
			require.GreaterOrEqual(t, f.start, 0)
			require.Greater(t, f.end, f.start)
			require.LessOrEqual(t, f.end, len(raw.data))
		}
	}
}

func TestLegacyFieldHierarchyLimitsDeepAndWideTrees(t *testing.T) {
	root := &base.NodeValue{Name: "IP"}
	// Repeated protocol names reset display indentation but must not reset
	// the independent traversal bound or hide that the view is incomplete.
	root.Value = []*base.NodeValue{root}
	var fields []field
	flattenFields(root, 0, &fields)
	require.LessOrEqual(t, len(fields), 35)
	require.Contains(t, fields[len(fields)-1].text, "view limited")
	children := make([]*base.NodeValue, 9000)
	for i := range children {
		children[i] = &base.NodeValue{Name: "word", Value: uint8(1)}
	}
	fields = nil
	flattenFields(&base.NodeValue{Name: "IP", Value: children}, 0, &fields)
	require.Len(t, fields, 8192)
	require.Contains(t, fields[len(fields)-1].text, "view limited")
}

func TestLegacyAddressFieldsFormatRawStringsWithoutChangingFamilyOrSource(t *testing.T) {
	for _, tc := range []struct {
		section, expected string
		wire              []byte
	}{
		{"IP", "127.0.0.1", []byte{127, 0, 0, 1}},
		{"IPv4", "192.0.2.1", []byte{192, 0, 2, 1}},
		{"Internet Protocol", "127.0.0.1", []byte{127, 0, 0, 1}},
		{"IPv6", "::ffff:192.0.2.1", netip.MustParseAddr("::ffff:192.0.2.1").AsSlice()},
		{"Internet Protocol Version 6", "::ffff:192.0.2.1", netip.MustParseAddr("::ffff:192.0.2.1").AsSlice()},
		{"Ethernet", "41:42:43:44:45:46", []byte("ABCDEF")},
	} {
		for _, value := range []any{tc.wire, string(tc.wire)} {
			node := &base.NodeValue{Name: "Source", Value: value}
			root := &base.NodeValue{Name: tc.section, Value: []*base.NodeValue{node}}
			var fields []field
			flattenFields(root, 0, &fields)
			require.Len(t, fields, 2)
			require.Equal(t, "Source: "+tc.expected, fields[1].text)
			require.Equal(t, value, node.Value, "formatting retains the original raw value and its type")
			require.False(t, fields[1].hasRange, "formatting cannot invent a byte range")
		}
	}
	for _, tc := range []struct {
		section, name string
		wire          []byte
	}{
		{"IP", "Source", []byte{1, 2}},
		{"IP", "Source", netip.MustParseAddr("2001:db8::1").AsSlice()},
		{"IPv6", "Source", []byte{192, 0, 2, 1}},
		{"TCP", "Source", []byte{192, 0, 2, 1}},
		{"Ethernet", "Payload", []byte("ABCDEF")},
	} {
		_, ok := legacyAddressFieldValue(tc.section, tc.name, tc.wire)
		require.False(t, ok, "only complete address words in the actual family are formatted")
	}
}

func TestLegacyLoopbackParserRootFormatsActualIPv4Words(t *testing.T) {
	ethernet := tcpPacket(t, 1, 100, "", false, true, false, false)
	loopback := *ethernet
	loopback.link = layers.LinkTypeNull
	loopback.data = append([]byte{2, 0, 0, 0}, ethernet.data[14:]...)
	loopback.ci.Length, loopback.ci.CaptureLength = len(loopback.data), len(loopback.data)
	var expected [4]byte
	copy(expected[:], loopback.data[16:20])
	local := detail(&loopback)
	require.Contains(t, detailLines(local.fields), "Source: "+netip.AddrFrom4(expected).String())
	wire, err := marshalDecodeRequest(packetDecodeRequest(&loopback))
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, runDecodeWorker(bytes.NewReader(wire), &output))
	worker, err := decodeWorkerOutput(output.Bytes(), &loopback)
	require.NoError(t, err)
	require.Equal(t, local.fields, worker.fields)
	source := findField(t, worker, "Source")
	require.Equal(t, 16, source.start)
	require.Equal(t, 20, source.end)
	require.True(t, source.hasRange)
}
