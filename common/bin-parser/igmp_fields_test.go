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

func parseIGMPLegacyComplete(t *testing.T, wire []byte) *base.NodeValue {
	t.Helper()
	node, err := binaryparser.ParseBinary(newProtocolCorpusBoundedReader(wire), "igmp", "IGMP")
	require.NoError(t, err)
	value, err := node.Result()
	require.NoError(t, err)
	return value
}

func TestIGMPCanonicalAndLegacyConsumeEveryRecordSourceAndAuxiliaryWord(t *testing.T) {
	requireIGMPCanonicalWireScope(t)
}

func requireIGMPCanonicalWireScope(t *testing.T) {
	t.Helper()
	var profile *ProtocolInfo
	for index := range ProtocolCatalog {
		if ProtocolCatalog[index].Name == "IGMP Membership Wire Fields" {
			profile = &ProtocolCatalog[index]
			break
		}
	}
	require.NotNil(t, profile)
	require.Equal(t, "network_igmp.yaml", profile.RuleFile)
	require.Equal(t, "IGMPFields", profile.EntryNode)
	require.Equal(t, statusPartial, profile.Status)
	w, err := hex.DecodeString("220012340000000201010002ef010203c0000201c0000202deadbeef06000000ef040506")
	require.NoError(t, err)
	v, err := ParseStructured(w, "network_igmp", "IGMPFields")
	require.NoError(t, err)
	shared, err := stream_parser.DecodeIGMPWire(w, len(w), 4096)
	require.NoError(t, err)
	require.Equal(t, shared.Fields, v["fields"].(map[string]any)["Message"])
	legacy := parseRule(t, w, "igmp", "IGMP")
	records := mustChild(t, legacy, "Records").Children()
	require.Len(t, records, 2)
	require.Equal(t, uint64(6), uintVal(t, records[1].Child("Record Type")))
	require.Equal(t, []byte{0xde, 0xad, 0xbe, 0xef}, bytesVal(t, records[0].Child("Auxiliary Data")))
	for cut := 0; cut < len(w); cut++ {
		_, err = ParseStructured(w[:cut], "network_igmp", "IGMPFields")
		require.Error(t, err)
		_, err = binaryparser.ParseBinary(bytes.NewReader(w[:cut]), "igmp", "IGMP")
		require.Error(t, err)
	}
}

func TestIGMPLegacyOpenQueryCannotSelectAnEightByteVersion(t *testing.T) {
	w := []byte{0x11, 0x82, 0, 0, 0, 0, 0, 0, 2, 0x95, 0, 1, 192, 0, 2, 1}
	open := parseRule(t, w, "igmp", "IGMP")
	require.Equal(t, uint64(0x82), uintVal(t, open.Child("Max Resp Time")), "legacy label is a raw octet")
	require.Nil(t, open.Child("SQRV"))
	require.Nil(t, open.Child("Number of Sources"))
	bounded, err := binaryparser.ParseBinary(newProtocolCorpusBoundedReader(w), "igmp", "IGMP")
	require.NoError(t, err)
	value, err := bounded.Result()
	require.NoError(t, err)
	require.Len(t, mustChild(t, value, "Sources").Children(), 1)
	for size := 9; size <= 11; size++ {
		_, err = binaryparser.ParseBinary(newProtocolCorpusBoundedReader(w[:size]), "igmp", "IGMP")
		require.Error(t, err)
	}
}

func TestIGMPLegacyLargeVectorsAreConsumedOrExplicitlyRefused(t *testing.T) {
	w := make([]byte, 8+260*8)
	w[0] = 0x22
	binary.BigEndian.PutUint16(w[6:8], 260)
	for at := 8; at < len(w); at += 8 {
		w[at] = 1
		copy(w[at+4:at+8], []byte{239, 1, 2, 3})
	}
	n := parseRule(t, w, "igmp", "IGMP")
	require.Len(t, mustChild(t, n, "Records").Children(), 260)
	query := make([]byte, 12+65*4)
	query[0] = 0x11
	binary.BigEndian.PutUint16(query[10:12], 65)
	node, err := binaryparser.ParseBinary(newProtocolCorpusBoundedReader(query), "igmp", "IGMP")
	require.NoError(t, err)
	n, err = node.Result()
	require.NoError(t, err)
	require.Len(t, mustChild(t, n, "Sources").Children(), 65)
	bad := []byte{0x22, 0, 0, 0, 0, 0, 0xff, 0xff}
	_, err = binaryparser.ParseBinary(bytes.NewReader(bad), "igmp", "IGMP")
	require.Error(t, err)
}

func TestIGMPLegacyReportCollectionBudgetIncludesEveryRecordsSources(t *testing.T) {
	wire := []byte{0x22, 0, 0, 0, 0, 0, 0, 2}
	for i := 0; i < 2; i++ {
		record := make([]byte, 8+2048*4)
		record[0], record[4] = 1, 239
		binary.BigEndian.PutUint16(record[2:4], 2048)
		wire = append(wire, record...)
	}
	_, err := binaryparser.ParseBinary(newProtocolCorpusBoundedReader(wire), "igmp", "IGMP")
	require.ErrorContains(t, err, "igmp: source collection budget exceeded")
}
