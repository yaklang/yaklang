package bin_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type dicomUserNativeItem struct {
	kind  byte
	value []byte
}

type dicomUserNativeFixture struct {
	pdu               []byte
	items             []dicomUserNativeItem
	userValueOffset   int
	wantItemCount     int
	presentationItems []byte
}

func dicomUserNativeItemBytes(item dicomUserNativeItem) []byte {
	return dicomTestItem(item.kind, item.value)
}

func dicomUserNativeJoin(items ...dicomUserNativeItem) []byte {
	var result []byte
	for _, item := range items {
		result = append(result, dicomUserNativeItemBytes(item)...)
	}
	return result
}

func dicomUserNativeMixedFixture(pduKind byte, extensionCount int) dicomUserNativeFixture {
	maximum := make([]byte, 4)
	binary.BigEndian.PutUint32(maximum, 2*1024*1024)
	known := []dicomUserNativeItem{
		{kind: 0x55, value: []byte("NATIVE_USER_1")},
		{kind: 0x52, value: []byte("1.2.826.0.1.3680043.10.42")},
		{kind: 0x51, value: maximum},
	}
	extensions := make([]dicomUserNativeItem, extensionCount)
	for i := range extensions {
		var value []byte
		switch i % 3 {
		case 1:
			value = bytes.Repeat([]byte{byte(i + 1)}, 8)
		case 2:
			value = bytes.Repeat([]byte{byte(i + 1)}, 128)
		}
		extensions[i] = dicomUserNativeItem{kind: byte(0xe0 + i%16), value: value}
	}

	// User-information sub-items may occur in any order. Keep Maximum Length at
	// the end so every strict prefix of the small fixture is also invalid.
	middle := len(extensions) / 2
	items := make([]dicomUserNativeItem, 0, len(extensions)+len(known))
	items = append(items, extensions[:1]...)
	items = append(items, known[0])
	items = append(items, extensions[1:middle]...)
	items = append(items, known[1])
	items = append(items, extensions[middle:]...)
	items = append(items, known[2])
	user := dicomUserNativeJoin(items...)

	contextKind := byte(0x20)
	syntaxItems := 2
	if pduKind == 2 {
		contextKind = 0x21
		syntaxItems = 1
	}
	presentation := dicomTestContext(contextKind, 1, 0)
	application := dicomTestItem(0x10, []byte("1.2.840.10008.3.1.1.1"))
	return dicomUserNativeFixture{
		pdu:               dicomTestAssociation(pduKind, presentation, user),
		items:             items,
		userValueOffset:   6 + 68 + len(application) + len(presentation) + 4,
		wantItemCount:     3 + syntaxItems + len(items),
		presentationItems: presentation,
	}
}

func dicomUserNativeDirectChild(t *testing.T, node *base.Node, name string) *base.Node {
	t.Helper()
	for _, child := range node.Children {
		if child.Name == name {
			return child
		}
	}
	t.Fatalf("missing direct child %q below %q", name, node.Name)
	return nil
}

func dicomUserNativeRequireSpan(t *testing.T, node *base.Node, start, end uint64) {
	t.Helper()
	require.True(t, stream_parser.NodeHasResult(node), node.Name)
	require.Equal(t, [2]uint64{start, end}, stream_parser.GetNodeResultPos(node), node.Name)
}

func dicomUserNativeRequireItems(t *testing.T, root *base.Node, fixture dicomUserNativeFixture, pduOffset int) {
	t.Helper()
	dicom := protocolCorpusFindNode(root, "DICOM")
	require.NotNil(t, dicom)
	list := protocolCorpusFindNode(dicom, "User Information")
	require.NotNil(t, list)
	require.True(t, list.Cfg.GetBool(stream_parser.CfgIsList))
	require.Len(t, list.Children, len(fixture.items))

	cursor := pduOffset + fixture.userValueOffset
	for index, want := range fixture.items {
		item := list.Children[index]
		require.Equal(t, "User Item", item.Name)
		start := uint64(cursor) * 8
		itemType := dicomUserNativeDirectChild(t, item, "Item Type")
		reserved := dicomUserNativeDirectChild(t, item, "Item Reserved")
		length := dicomUserNativeDirectChild(t, item, "Item Length")
		dicomUserNativeRequireSpan(t, itemType, start, start+8)
		dicomUserNativeRequireSpan(t, reserved, start+8, start+16)
		dicomUserNativeRequireSpan(t, length, start+16, start+32)
		protocolCorpusRequireValue(t, item, "Item Type", uint64(want.kind))
		protocolCorpusRequireValue(t, item, "Item Reserved", uint64(0))
		protocolCorpusRequireValue(t, item, "Item Length", uint64(len(want.value)))

		payloadStart := start + 32
		payloadEnd := payloadStart + uint64(len(want.value))*8
		switch want.kind {
		case 0x51:
			maximum := dicomUserNativeDirectChild(t, item, "Maximum PDU Length")
			dicomUserNativeRequireSpan(t, maximum, payloadStart, payloadEnd)
			protocolCorpusRequireValue(t, item, "Maximum PDU Length", uint64(binary.BigEndian.Uint32(want.value)))
		case 0x52:
			implementation := dicomUserNativeDirectChild(t, item, "Implementation Class")
			uid := dicomUserNativeDirectChild(t, implementation, "UID")
			dicomUserNativeRequireSpan(t, uid, payloadStart, payloadEnd)
			protocolCorpusRequireValue(t, implementation, "UID", string(want.value))
		case 0x55:
			version := dicomUserNativeDirectChild(t, item, "Implementation Version")
			dicomUserNativeRequireSpan(t, version, payloadStart, payloadEnd)
			protocolCorpusRequireValue(t, item, "Implementation Version", string(want.value))
		default:
			extension := dicomUserNativeDirectChild(t, item, "Extension Data")
			if len(want.value) == 0 {
				// Legacy retains the raw template child but does not synthesize an
				// empty value or a zero-length field configuration.
				require.False(t, stream_parser.NodeHasResult(extension))
				require.False(t, extension.Cfg.Has(base.CfgLength))
			} else {
				dicomUserNativeRequireSpan(t, extension, payloadStart, payloadEnd)
				require.Equal(t, uint64(len(want.value))*8, extension.Cfg.GetUint64(base.CfgLength))
				protocolCorpusRequireValue(t, item, "Extension Data", want.value)
			}
		}
		cursor += 4 + len(want.value)
	}
	require.Equal(t, pduOffset+len(fixture.pdu), cursor)
}

func dicomUserNativeConfig(legacy bool) map[string]any {
	return map[string]any{"dicomUserLegacy": legacy}
}

func TestProtocolCorpusDICOMUserNativeLegacyTreeDifferential(t *testing.T) {
	for _, pduKind := range []byte{1, 2} {
		for _, extensionCount := range []int{8, 128} {
			for _, imported := range []bool{false, true} {
				name := fmt.Sprintf("pdu-%d/extensions-%d/import-%t", pduKind, extensionCount, imported)
				t.Run(name, func(t *testing.T) {
					fixture := dicomUserNativeMixedFixture(pduKind, extensionCount)
					wire, rule, entry, offset := fixture.pdu, dicomRule, "DICOM", 0
					if imported {
						wire = ipv4TCPFrame(t, 41000, 104, fixture.pdu)
						rule, entry, offset = "ethernet", "Ethernet", 54
						require.Equal(t, fixture.pdu, wire[offset:])
					}

					native := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, rule, entry, dicomUserNativeConfig(false))
					legacy := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, rule, entry, dicomUserNativeConfig(true))
					for _, root := range []*base.Node{native, legacy} {
						dicomUserNativeRequireItems(t, root, fixture, offset)
						require.Equal(t, wire, NodeToBytes(root))
					}

					nativeDICOM := protocolCorpusFindNode(native, "DICOM")
					legacyDICOM := protocolCorpusFindNode(legacy, "DICOM")
					require.NotNil(t, nativeDICOM)
					require.NotNil(t, legacyDICOM)
					legacyCount := legacyDICOM.Ctx.GetItem("dicomItemCount")
					nativeCount := nativeDICOM.Ctx.GetItem("dicomItemCount")
					// Yak integer literals and addition retain Go int here. Comparing
					// the raw interface values prevents a native uint64 write-back from
					// looking equivalent through a numeric conversion helper.
					require.IsType(t, int(0), legacyCount)
					require.Equal(t, fixture.wantItemCount, legacyCount)
					require.Equal(t, legacyCount, nativeCount)

					nativeTree, nativeValue := dicomNativeSnapshot(t, native, wire)
					legacyTree, legacyValue := dicomNativeSnapshot(t, legacy, wire)
					require.Equal(t, legacyTree, nativeTree, "complete tree/config/span/context shape")
					require.Equal(t, legacyValue, nativeValue, "complete Result tree and origins")
				})
			}
		}
	}
}

func TestProtocolCorpusDICOMUserNativeLegacyRejectedBodies(t *testing.T) {
	maximum := dicomUserNativeItem{kind: 0x51, value: []byte{0, 1, 0, 0}}
	implementation := dicomUserNativeItem{kind: 0x52, value: []byte("1.2.826.0.1.3680043.10.42")}
	version := dicomUserNativeItem{kind: 0x55, value: []byte("NATIVE_USER_1")}
	for _, tc := range []struct {
		name, reason string
		items        []dicomUserNativeItem
	}{
		{"missing-maximum", "required user information", []dicomUserNativeItem{implementation}},
		{"missing-implementation", "required user information", []dicomUserNativeItem{maximum}},
		{"duplicate-maximum", "required user information", []dicomUserNativeItem{maximum, implementation, maximum}},
		{"duplicate-implementation", "required user information", []dicomUserNativeItem{implementation, maximum, implementation}},
		{"duplicate-version", "required user information", []dicomUserNativeItem{maximum, version, implementation, version}},
		{"empty-implementation-uid", "UID length", []dicomUserNativeItem{maximum, {kind: 0x52}}},
		{"invalid-implementation-uid", "invalid UID character", []dicomUserNativeItem{maximum, {kind: 0x52, value: []byte("1.2x3")}}},
		{"leading-zero-implementation-uid", "leading zero", []dicomUserNativeItem{maximum, {kind: 0x52, value: []byte("1.02.3")}}},
		{"empty-version", "implementation version length", []dicomUserNativeItem{maximum, implementation, {kind: 0x55}}},
		{"long-version", "implementation version length", []dicomUserNativeItem{maximum, implementation, {kind: 0x55, value: bytes.Repeat([]byte{'V'}, 17)}}},
		{"control-version", "invalid implementation version character", []dicomUserNativeItem{maximum, implementation, {kind: 0x55, value: []byte{'V', 0}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := dicomTestAssociation(1, dicomTestContext(0x20, 1, 0), dicomUserNativeJoin(tc.items...))
			for _, legacy := range []bool{false, true} {
				_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), dicomRule, dicomUserNativeConfig(legacy), "DICOM")
				require.ErrorContains(t, err, tc.reason, "legacy=%t", legacy)
			}
		})
	}

	// Rebuild the enclosing lengths for every strict User Information body
	// prefix. This reaches the nested list instead of failing at the PDU boundary.
	small := dicomUserNativeMixedFixture(1, 2)
	fullUser := dicomUserNativeJoin(small.items...)
	for cut := 0; cut < len(fullUser); cut++ {
		wire := dicomTestAssociation(1, small.presentationItems, fullUser[:cut])
		for _, legacy := range []bool{false, true} {
			_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), dicomRule, dicomUserNativeConfig(legacy), "DICOM")
			require.Error(t, err, "User Information prefix %d/%d legacy=%t", cut, len(fullUser), legacy)
		}
	}
}

func TestProtocolCorpusDICOMUserNativeTCPFailureRestoresPayload(t *testing.T) {
	fixture := dicomUserNativeMixedFixture(1, 8)
	// A duplicate required item is detected after all item bytes are available.
	badItems := append([]dicomUserNativeItem(nil), fixture.items...)
	badItems = append(badItems, dicomUserNativeItem{kind: 0x51, value: []byte{0, 1, 0, 0}})
	badPDU := dicomTestAssociation(1, fixture.presentationItems, dicomUserNativeJoin(badItems...))
	frame := ipv4TCPFrame(t, 41000, 104, badPDU)
	var trees [][]dicomNativeTreeSnapshot
	var values []dicomNativeValueSnapshot
	for _, legacy := range []bool{false, true} {
		root := protocolCorpusRequireBoundedRuleParseWithConfig(t, frame, "ethernet", "Ethernet", dicomUserNativeConfig(legacy))
		require.Nil(t, protocolCorpusFindNode(root, "DICOM"))
		remaining := protocolCorpusFindNode(root, "Remaining Payload")
		require.NotNil(t, remaining)
		protocolCorpusRequireValue(t, root, "Remaining Payload", badPDU)
		require.Equal(t, [2]uint64{54 * 8, uint64(len(frame)) * 8}, stream_parser.GetNodeResultPos(remaining))
		tree, value := dicomNativeSnapshot(t, root, frame)
		trees = append(trees, tree)
		values = append(values, value)
	}
	require.Equal(t, trees[1], trees[0])
	require.Equal(t, values[1], values[0])
}

func dicomUserNativeStructuredValue(value *base.NodeValue) any {
	if value.IsValue() {
		return value.Value
	}
	if value.IsList() {
		result := make([]any, 0, len(value.Children()))
		for _, child := range value.Children() {
			result = append(result, dicomUserNativeStructuredValue(child))
		}
		return result
	}
	result := make(map[string]any, len(value.Children()))
	for _, child := range value.Children() {
		result[child.Name] = dicomUserNativeStructuredValue(child)
	}
	return result
}

func TestProtocolCorpusDICOMUserNativeAssociationGenerateCompatibility(t *testing.T) {
	fixture := dicomUserNativeMixedFixture(1, 8)
	parsed := protocolCorpusRequireBoundedRuleParseWithConfig(t, fixture.pdu, dicomRule, "DICOM", dicomUserNativeConfig(true))
	value, err := parsed.Result()
	require.NoError(t, err)
	input, ok := dicomUserNativeStructuredValue(value).(map[string]any)
	require.True(t, ok)

	generated, err := parser.GenerateBinary(input, dicomRule, "DICOM")
	require.NoError(t, err)
	require.Equal(t, fixture.pdu, NodeToBytes(generated))
	dicomUserNativeRequireItems(t, generated, fixture, 0)
}
