package stream_parser

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type pimTestRangedValue struct {
	value      any
	start, end int
}

func pimTestRangedFields(root map[string]any) map[string]pimTestRangedValue {
	out := map[string]pimTestRangedValue{}
	var walk func(any, string)
	walk = func(value any, path string) {
		switch row := value.(type) {
		case map[string]any:
			if ranges, ok := row["Wire Field Ranges"].(map[string]any); ok {
				for name, raw := range ranges {
					span := raw.([]any)
					out[path+"/"+name] = pimTestRangedValue{row[name], int(span[0].(uint64)), int(span[1].(uint64))}
				}
			}
			for name, child := range row {
				if name != "Wire Field Ranges" {
					walk(child, path+"/"+name)
				}
			}
		case []map[string]any:
			for i, child := range row {
				walk(child, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	walk(root, "")
	return out
}

func pimTestProgressiveMessages() [][]byte {
	hello := []byte{0x20, 0, 0, 0,
		0, 1, 0, 2, 0, 105,
		0, 2, 0, 4, 0x80, 5, 0, 10,
		0, 24, 0, 12, 1, 0, 192, 0, 2, 1, 1, 0, 192, 0, 2, 1}
	join := []byte{0x23, 0, 0, 0, 1, 0, 192, 0, 2, 2, 0, 1, 0, 210,
		1, 0, 0, 32, 239, 1, 2, 3, 0, 1, 0, 1,
		1, 0, 4, 32, 192, 0, 2, 10, 1, 0, 7, 32, 192, 0, 2, 10}
	bootstrap := []byte{0x24, 0, 0, 0, 0, 7, 30, 1, 1, 0, 192, 0, 2, 1,
		1, 0, 0, 24, 239, 1, 2, 0, 2, 2, 0, 0,
		1, 0, 192, 0, 2, 2, 0, 105, 1, 0,
		1, 0, 192, 0, 2, 3, 0, 105, 2, 0}
	assert, _, _ := pimDerivedPresenceMessage(5)
	refresh, _, _ := pimDerivedPresenceMessage(9)
	return [][]byte{hello, join, bootstrap, assert, refresh}
}

func TestPIMEveryCutRetainsEveryEarlierCompleteFieldOccurrence(t *testing.T) {
	for _, wire := range pimTestProgressiveMessages() {
		full, err := DecodePIMControl(wire, PIMWireContext{}, 4096)
		require.NoError(t, err)
		want := pimTestRangedFields(full)
		for cut := 4; cut <= len(wire); cut++ {
			partial, _ := DecodePIMControl(wire[:cut], PIMWireContext{}, 4096)
			actual := pimTestRangedFields(partial)
			for path, field := range want {
				if field.end <= cut {
					require.Contains(t, actual, path, "type%d cut%d discarded a complete field", wire[0]&15, cut)
					require.Equal(t, field, actual[path], "type%d cut%d %s", wire[0]&15, cut, path)
				} else {
					require.NotContains(t, actual, path, "type%d cut%d manufactured an unread field", wire[0]&15, cut)
				}
			}
			for path, field := range actual {
				require.Contains(t, want, path)
				require.GreaterOrEqual(t, field.start, 0)
				require.LessOrEqual(t, field.end, cut)
			}
			if wire[0]&15 == 4 && cut < len(wire) {
				for _, group := range partial["Groups"].([]map[string]any) {
					require.NotContains(t, group, "RP Set Complete In Message", "declared counts do not supply missing RP fields")
				}
			}
		}
	}
}

func TestPIMLaterBadHelloOptionRetainsPriorOptionsAndItsOwnHeader(t *testing.T) {
	wire := []byte{0x20, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 2, 0, 105, 0, 19, 0, 4, 1, 2}
	m, err := DecodePIMControl(wire, PIMWireContext{}, 64)
	require.ErrorContains(t, err, "option crosses datagram")
	options := m["Options"].([]map[string]any)
	require.Len(t, options, 3)
	require.Equal(t, uint64(0), options[0]["Option Type"], "complete unknown zero-length option remains valid")
	require.Equal(t, uint64(105), options[1]["Hold Time"])
	require.Equal(t, uint64(19), options[2]["Option Type"])
	require.Equal(t, uint64(4), options[2]["Option Length"])
	require.NotContains(t, options[2], "DR Priority")
	require.Equal(t, wire[18:], m["Unparsed Body"])
	for cut := 0; cut < 4; cut++ {
		fields, err := DecodePIMControl(wire[:cut], PIMWireContext{}, 64)
		require.Error(t, err)
		require.Nil(t, fields, "preserve the unselected short common-header boundary")
	}
}

func TestPIMBadAddressRetainsOnlyItsLocatedPrefix(t *testing.T) {
	wire, _, _ := pimDerivedPresenceMessage(5)
	for _, test := range []struct {
		at, value, stop int
		address         bool
	}{{5, 1, 6, false}, {7, 33, 8, false}, {8, 192, 12, true}} {
		bad := bytes.Clone(wire)
		bad[test.at] = byte(test.value)
		fields, err := DecodePIMControl(bad, PIMWireContext{}, 64)
		require.Error(t, err)
		group := fields["Group"].(map[string]any)
		require.Equal(t, uint64(1), group["Address Family"])
		_, hasAddress := group["Address"]
		require.Equal(t, test.address, hasAddress)
		require.Nil(t, fields["Source"], "do not scan for a later plausible source")
		require.NotContains(t, fields, "Metric Preference")
		require.Equal(t, bad[test.stop:], fields["Unparsed Body"])
	}
}

func TestPIMTrailingBytesDoNotEraseCompleteLocatedFields(t *testing.T) {
	wire, _, _ := pimDerivedPresenceMessage(5)
	m, err := DecodePIMControl(append(bytes.Clone(wire), 0xaa), PIMWireContext{}, 64)
	require.ErrorContains(t, err, "trailing bytes")
	require.Equal(t, uint64(0), m["Metric Preference"])
	require.Equal(t, []byte{0xaa}, m["Unparsed Body"])
	require.Equal(t, pimPrefixScope, m["Observation Scope"])
}
