package stream_parser

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func nf9Set(id uint16, body []byte) []byte {
	w := binary.BigEndian.AppendUint16(nil, id)
	w = binary.BigEndian.AppendUint16(w, uint16(4+len(body)))
	return append(w, body...)
}

func nf9Template(id uint16, fields ...uint16) []byte {
	w := binary.BigEndian.AppendUint16(nil, id)
	w = binary.BigEndian.AppendUint16(w, uint16(len(fields)/2))
	for _, f := range fields {
		w = binary.BigEndian.AppendUint16(w, f)
	}
	return nf9Set(0, w)
}

func nf9Message(count uint16, sets ...[]byte) []byte {
	w := []byte{0, 9, byte(count >> 8), byte(count), 0, 0, 3, 232, 0x65, 0x53, 0xf1, 0, 0, 0, 0, 7, 0, 0, 0, 19}
	for _, s := range sets {
		w = append(w, s...)
	}
	return w
}

func nf9Sample() (template, data []byte) {
	template = nf9Template(256, 8, 4, 12, 4, 7, 2, 11, 2, 2, 8)
	body := []byte{192, 0, 2, 1, 198, 51, 100, 2, 0xcf, 8, 1, 0xbb, 0x10, 0, 0, 0, 0, 0, 0, 3}
	return template, nf9Set(256, body)
}

func TestNetFlowV9SameMessageAndCallerContextUseOwnedRanges(t *testing.T) {
	template, data := nf9Sample()
	for _, late := range []bool{false, true} {
		sets := [][]byte{template, data}
		if late {
			sets[0], sets[1] = sets[1], sets[0]
		}
		w := nf9Message(2, sets...)
		decoded, err := DecodeNetFlowV9Wire(w, nil, 128)
		require.NoError(t, err)
		require.Equal(t, true, decoded.Fields["Record Count Verified"])
		require.Equal(t, uint64(1), decoded.Fields["Data Record Count"])
		index, offset := 1, uint64(52)
		if late {
			index, offset = 0, 24
		}
		row := decoded.Fields["FlowSets"].([]map[string]any)[index]
		require.Equal(t, "same PDU", row["Template Scope"])
		values := row["Records"].([]map[string]any)[0]["Fields"].([]map[string]any)
		require.Equal(t, "192.0.2.1", values[0]["Value"])
		require.Equal(t, "198.51.100.2", values[1]["Value"])
		require.Equal(t, uint64(53000), values[2]["Value"])
		require.Equal(t, uint64(443), values[3]["Value"])
		require.Equal(t, uint64(1)<<60|3, values[4]["Value"])
		require.Equal(t, []any{offset, offset + 4}, values[0]["Wire Field Ranges"].(map[string]any)["Value"])
		require.NotContains(t, values[0]["Wire Field Ranges"], "Field Type", "schema identifiers are not current data bytes")
		before, err := json.Marshal(decoded.Fields)
		require.NoError(t, err)
		clear(w)
		after, err := json.Marshal(decoded.Fields)
		require.NoError(t, err)
		require.Equal(t, before, after)
		lookup := decoded.Templates[256]
		require.Equal(t, 20, lookup.RecordLength)
		require.Len(t, lookup.Fields, 5)
		decoded.Fields["FlowSets"].([]map[string]any)[1-index]["Templates"].([]map[string]any)[0]["Fields"].([]map[string]any)[0]["Field Type"] = uint64(99)
		require.Equal(t, uint16(8), lookup.Fields[0].Type)
	}
	only, err := DecodeNetFlowV9Wire(nf9Message(1, template), nil, 128)
	require.NoError(t, err)
	missing, err := DecodeNetFlowV9Wire(nf9Message(1, data), nil, 128)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.Equal(t, true, missing.Fields["Fixed Header Complete"])
	require.Equal(t, true, missing.Fields["Wire Framing Complete"])
	require.Nil(t, missing.Templates)
	resolved, err := DecodeNetFlowV9Wire(nf9Message(1, data), only.Templates, 128)
	require.NoError(t, err)
	require.Equal(t, "observed caller-supplied context", resolved.Fields["FlowSets"].([]map[string]any)[0]["Template Scope"])
	ownedTemplate := only.Templates[256]
	ownedTemplate.Fields[0].Type = 99
	require.Equal(t, uint16(8), resolved.Templates[256].Fields[0].Type)
}

func TestNetFlowV9TransactionalErrorsAndAmbiguousPadding(t *testing.T) {
	template, data := nf9Sample()
	good := nf9Message(2, template, data)
	for cut := 0; cut < len(good); cut++ {
		// A cut exactly after a valid template still has a header declaring
		// two records. It may not install the otherwise complete template.
		result, err := DecodeNetFlowV9Wire(good[:cut], nil, 128)
		require.Error(t, err, "accepted prefix %d", cut)
		require.Nil(t, result.Templates)
	}
	for _, w := range [][]byte{append(bytes.Clone(good), 0), nf9Message(3, template, data), nf9Message(1, nf9Template(255, 8, 4)), nf9Message(1, nf9Template(256, 8, 0))} {
		result, err := DecodeNetFlowV9Wire(w, nil, 128)
		require.Error(t, err)
		require.Nil(t, result.Templates)
	}
	for _, pad := range [][]byte{{0, 0, 0}, {7, 8, 9}} {
		w := nf9Message(2, nf9Template(300, 4, 1), nf9Set(300, append([]byte{6}, pad...)))
		result, err := DecodeNetFlowV9Wire(w, nil, 128)
		require.NoError(t, err)
		row := result.Fields["FlowSets"].([]map[string]any)[1]
		require.Equal(t, uint64(1), row["Record Count"])
		require.Equal(t, pad, row["Padding Bytes"])
		if pad[0] != 0 {
			require.Contains(t, row["Conformance Warning"], "does not require")
		}
	}
	ambiguous := nf9Message(6, nf9Template(300, 4, 1), nf9Set(300, []byte{6, 0, 0, 0}), nf9Set(300, []byte{17, 0, 0, 0}))
	result, err := DecodeNetFlowV9Wire(ambiguous, nil, 128)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.Contains(t, err.Error(), "ambiguous")
	require.Nil(t, result.Templates)
	for _, row := range result.Fields["FlowSets"].([]map[string]any)[1:] {
		require.NotContains(t, row, "Records")
	}
}

func TestNetFlowV9OptionsNamespaceAndConflictingDefinitions(t *testing.T) {
	// Options scope type1 has no relationship to ordinary IE1 octet count.
	options := nf9Set(1, []byte{1, 44, 0, 4, 0, 4, 0, 1, 0, 4, 0, 2, 0, 4})
	result, err := DecodeNetFlowV9Wire(nf9Message(2, options, nf9Set(300, []byte{1, 2, 3, 4, 0, 0, 0, 43})), nil, 128)
	require.NoError(t, err)
	fields := result.Fields["FlowSets"].([]map[string]any)[1]["Records"].([]map[string]any)[0]["Fields"].([]map[string]any)
	require.Equal(t, true, fields[0]["Scope"])
	require.Equal(t, "opaque", fields[0]["Value Kind"])
	require.Equal(t, []byte{1, 2, 3, 4}, fields[0]["Value"])
	require.Equal(t, false, fields[1]["Scope"])
	require.Equal(t, uint64(43), fields[1]["Value"])
	require.Equal(t, true, result.Fields["Unselected Field Semantics"])
	zeroScope := nf9Set(1, []byte{1, 44, 0, 4, 0, 4, 0, 1, 0, 0, 0, 2, 0, 4})
	z, err := DecodeNetFlowV9Wire(nf9Message(2, zeroScope, nf9Set(300, []byte{0, 0, 0, 43})), nil, 128)
	require.NoError(t, err)
	zf := z.Fields["FlowSets"].([]map[string]any)[1]["Records"].([]map[string]any)[0]["Fields"].([]map[string]any)
	require.Equal(t, "scope", zf[0]["Value Kind"])
	require.Empty(t, zf[0]["Wire Field Ranges"])
	require.Equal(t, uint64(43), zf[1]["Value"])
	a := nf9Template(300, 4, 1)
	b := nf9Template(300, 8, 4)
	conflict, err := DecodeNetFlowV9Wire(nf9Message(3, a, b, nf9Set(300, []byte{1, 2, 3, 4})), nil, 128)
	require.ErrorIs(t, err, ErrObservedWireContext)
	require.Nil(t, conflict.Templates)
	identical, err := DecodeNetFlowV9Wire(nf9Message(3, a, a, nf9Set(300, []byte{6, 0, 0, 0})), nil, 128)
	require.NoError(t, err)
	require.Equal(t, uint64(2), identical.Fields["Template Record Count"])
}

func TestNetFlowV9InvalidCallerContextCannotCrossRecordBounds(t *testing.T) {
	_, data := nf9Sample()
	for _, schema := range []NetFlowV9Template{
		{ID: 256, RecordLength: 1, Fields: []NetFlowV9Field{{Type: 8, Length: 65535}}},
		{ID: 257, RecordLength: 4, Fields: []NetFlowV9Field{{Type: 8, Length: 4}}},
		{ID: 256, RecordLength: 4, Fields: []NetFlowV9Field{{Type: 8, Length: 4, Scope: true}}},
	} {
		result, err := DecodeNetFlowV9Wire(nf9Message(1, data), map[uint16]NetFlowV9Template{256: schema}, 128)
		require.ErrorIs(t, err, ErrObservedWireContext)
		require.Nil(t, result.Templates)
	}
	_, err := DecodeNetFlowV9Wire(nf9Message(100, data), nil, 16)
	require.ErrorIs(t, err, ErrObservedWireBudget)
}

func FuzzNetFlowV9WireBoundsAndTransactionalSchemas(f *testing.F) {
	template, data := nf9Sample()
	f.Add(nf9Message(2, template, data))
	f.Add(nf9Message(1, data))
	f.Add(nf9Message(2, nf9Template(300, 4, 1), nf9Set(300, []byte{6, 0, 0, 0})))
	f.Add(nf9Message(2, nf9Set(1, []byte{1, 44, 0, 4, 0, 4, 0, 1, 0, 0, 0, 2, 0, 4}), nf9Set(300, []byte{0, 0, 0, 43})))
	f.Add(nf9Message(6, nf9Template(300, 4, 1), nf9Set(300, []byte{6, 0, 0, 0}), nf9Set(300, []byte{17, 0, 0, 0})))
	f.Fuzz(func(t *testing.T, w []byte) {
		if len(w) > 65527 {
			t.Skip()
		}
		before := bytes.Clone(w)
		result, err := DecodeNetFlowV9Wire(w, nil, 128)
		require.Equal(t, before, w)
		if err != nil {
			require.Nil(t, result.Templates)
			return
		}
		require.Equal(t, true, result.Fields["Wire Framing Complete"])
		require.Equal(t, true, result.Fields["Record Count Verified"])
		require.NotNil(t, result.Templates)
		copy := CloneNetFlowV9Templates(result.Templates)
		for id, schema := range copy {
			if len(schema.Fields) > 0 {
				schema.ID = 0
				copy[id] = schema
				_, e := DecodeNetFlowV9Wire(w, copy, 128)
				require.True(t, e != nil && (errors.Is(e, ErrObservedWireContext) || errors.Is(e, ErrObservedWireBudget)))
				break
			}
		}
	})
}
