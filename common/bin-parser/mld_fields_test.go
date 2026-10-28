package bin_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	binaryparser "github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func parseMLDLegacyComplete(t *testing.T, wire []byte) *base.NodeValue {
	t.Helper()
	node, err := binaryparser.ParseBinary(newProtocolCorpusBoundedReader(wire), "internet_control_message_protocol_v6", "ICMPV6")
	require.NoError(t, err)
	value, err := node.Result()
	require.NoError(t, err)
	return value
}

func TestMLDCanonicalAndLegacyConsumeAllRecordComponents(t *testing.T) {
	requireMLDCanonicalWireScope(t)
}

func TestMLDLegacyCollectionThresholdsNeverSilentlySkipValidVectors(t *testing.T) {
	group, err := hex.DecodeString("ff3e0000000000000000000000001234")
	require.NoError(t, err)
	report := append([]byte{143, 0, 0x12, 0x34, 0, 0, 0, 1, 1, 0, 0, 65}, group...)
	for i := 0; i < 65; i++ {
		address := make([]byte, 16)
		binary.BigEndian.PutUint16(address[14:], uint16(i+1))
		report = append(report, address...)
	}
	n := parseRule(t, report, "internet_control_message_protocol_v6", "ICMPV6")
	record := mustChild(t, n, "Multicast Listener Report v2", "Records").Children()[0]
	require.Len(t, record.Child("Sources").Children(), 65)
	query := append([]byte{130, 0, 0x12, 0x34, 0, 1, 0, 0}, group...)
	query = append(query, 2, 1, 0, 65)
	query = append(query, report[28:]...)
	n = parseMLDLegacyComplete(t, query)
	require.Len(t, mustChild(t, n, "Multicast Listener Query", "Sources").Children(), 65)
	// The former nrec<256 condition silently skipped the entire valid vector.
	report = []byte{143, 0, 0x12, 0x34, 0, 0, 1, 1}
	for i := 0; i < 257; i++ {
		report = append(report, 1, 0, 0, 0)
		report = append(report, group...)
	}
	n = parseRule(t, report, "internet_control_message_protocol_v6", "ICMPV6")
	require.Len(t, mustChild(t, n, "Multicast Listener Report v2", "Records").Children(), 257)
}

func requireMLDCanonicalWireScope(t *testing.T) {
	t.Helper()
	var catalog *ProtocolInfo
	for index := range ProtocolCatalog {
		if ProtocolCatalog[index].Name == "ICMPv6 MLD" {
			catalog = &ProtocolCatalog[index]
			break
		}
	}
	require.NotNil(t, catalog)
	require.Equal(t, "network_icmp.yaml", catalog.RuleFile)
	require.Equal(t, "MLDFields", catalog.EntryNode)
	require.Equal(t, statusPartial, catalog.Status)
	w, err := hex.DecodeString("8f00123400000002" +
		"01010002ff3e0000000000000000000000001234" +
		"20010db8000000000000000000000001" +
		"00000000000000000000ffffc0000201deadbeef" +
		"06000000ff0200000000000000000000000000fb")
	require.NoError(t, err)
	v, err := ParseStructured(w, "network_icmp", "MLDFields")
	require.NoError(t, err)
	fields := v["fields"].(map[string]any)["Message"].(map[string]any)
	shared, err := stream_parser.DecodeMLDWire(w, len(w), 4096)
	require.NoError(t, err)
	require.Equal(t, shared.Fields, fields)
	legacy := parseRule(t, w, "internet_control_message_protocol_v6", "ICMPV6")
	records := mustChild(t, legacy, "Multicast Listener Report v2", "Records").Children()
	require.Len(t, records, 2)
	require.Equal(t, uint64(6), uintVal(t, records[1].Child("Record Type")))
	require.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, bytesVal(t, records[0].Child("Auxiliary Data")))
	for cut := 0; cut < len(w); cut++ {
		_, err := ParseStructured(w[:cut], "network_icmp", "MLDFields")
		require.Error(t, err, "canonical cut%d", cut)
		_, err = binaryparser.ParseBinary(bytes.NewReader(w[:cut]), "internet_control_message_protocol_v6", "ICMPV6")
		require.Error(t, err, "legacy cut%d", cut)
	}
}

func TestMLDCanonicalQueryFloatingCodesAndLegacyV2Sources(t *testing.T) {
	w, err := hex.DecodeString("82001234ffff0000ff3e000000000000000000000000abcd" + "faff0001" + "20010db8000000000000000000000005")
	require.NoError(t, err)
	n := parseMLDLegacyComplete(t, w)
	query := mustChild(t, n, "Multicast Listener Query")
	require.Nil(t, query.Child("Maximum Response Delay"), "v2 carries a code, not a raw linear millisecond delay")
	require.Equal(t, uint64(65535), uintVal(t, query.Child("Maximum Response Code")))
	require.Len(t, query.Child("Sources").Children(), 1)
	v, err := ParseStructured(w, "network_icmp", "MLDFields")
	require.NoError(t, err)
	fields := v["fields"].(map[string]any)["Message"].(map[string]any)
	mld := fields["MLD"].(map[string]any)
	require.Equal(t, uint32(8387584), mld["Maximum Response Delay (ms)"])
	require.Equal(t, uint16(31744), mld["Query Interval (s)"])
	for length := 25; length <= 27; length++ {
		_, err := binaryparser.ParseBinary(newProtocolCorpusBoundedReader(w[:length]), "internet_control_message_protocol_v6", "ICMPV6")
		require.Error(t, err)
	}
	// An unbounded reader can contain a following message. Common bytes alone
	// cannot select the query version or reinterpret the word as a timer.
	unbounded := parseRule(t, w, "internet_control_message_protocol_v6", "ICMPV6")
	common := mustChild(t, unbounded, "Multicast Listener Query")
	require.Equal(t, uint64(65535), uintVal(t, common.Child("Response Code/Delay Word")))
	require.Nil(t, common.Child("Number of Sources"))
}

func TestMLDLegacyReportCollectionBudgetIncludesEveryRecordsSources(t *testing.T) {
	// Neither record alone exceeds the limit; their combined records and
	// sources do. A per-record-only check would consume an unbounded list.
	wire := []byte{143, 0, 0, 0, 0, 0, 0, 2}
	for i := 0; i < 2; i++ {
		record := make([]byte, 20+2048*16)
		record[0] = 1
		binary.BigEndian.PutUint16(record[2:4], 2048)
		record[4], record[5] = 0xff, 0x3e
		wire = append(wire, record...)
	}
	_, err := binaryparser.ParseBinary(newProtocolCorpusBoundedReader(wire), "internet_control_message_protocol_v6", "ICMPV6")
	require.ErrorContains(t, err, "MLD source collection budget")
}
