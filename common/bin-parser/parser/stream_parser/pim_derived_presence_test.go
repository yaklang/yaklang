package stream_parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func pimDerivedPresenceMessage(typ byte) ([]byte, int, int) {
	w := []byte{0x20 | typ, 0, 0, 0, 1, 0, 0, 32, 239, 1, 2, 3, 1, 0, 192, 0, 2, 1}
	if typ == 9 {
		w = append(w, 1, 0, 192, 0, 2, 2) // State Refresh Originator.
	}
	metric := len(w)
	w = append(w, make([]byte, 8)...) // Complete legal zero metric preference/metric.
	flags := -1
	if typ == 9 {
		w = append(w, 32, 64)
		flags = len(w)
		w = append(w, 0, 10) // Complete legal false flags and interval.
	}
	return w, metric, flags
}

func TestPIMDerivedFieldsRequireTheirCompleteSourceWords(t *testing.T) {
	for _, typ := range []byte{5, 9} {
		wire, metric, flags := pimDerivedPresenceMessage(typ)
		for cut := 0; cut <= len(wire); cut++ {
			fields, _ := DecodePIMControl(wire[:cut], PIMWireContext{}, 64)
			for _, key := range []string{"Metric Preference", "RPT"} {
				_, present := fields[key]
				require.Equal(t, cut >= metric+4, present, "type%d cut%d field%s", typ, cut, key)
			}
			if typ == 9 {
				for _, key := range []string{"Prune Indicator", "Prune Now", "Assert Override"} {
					_, present := fields[key]
					require.Equal(t, cut >= flags+1, present, "cut%d field%s", cut, key)
				}
			}
		}
		for _, corrupt := range []func([]byte){
			func(w []byte) { w[5] = 1 },   // Unselected group address encoding.
			func(w []byte) { w[7] = 33 },  // Impossible IPv4 group mask.
			func(w []byte) { w[8] = 192 }, // Complete group address is not multicast.
		} {
			bad := bytes.Clone(wire)
			corrupt(bad)
			fields, err := DecodePIMControl(bad, PIMWireContext{}, 64)
			require.Error(t, err)
			for _, key := range []string{"Metric Preference", "RPT", "Prune Indicator", "Prune Now", "Assert Override"} {
				require.NotContains(t, fields, key, "later source words are not located after an address failure")
			}
		}
		fields, err := DecodePIMControl(wire, PIMWireContext{}, 64)
		require.NoError(t, err)
		require.Equal(t, uint64(0), fields["Metric Preference"])
		require.Equal(t, false, fields["RPT"], "complete legal zero words remain fields")
	}
}

func TestPIMRegisterFlagsAreAbsentUntilTheirWholeWordExists(t *testing.T) {
	for cut := 4; cut <= 8; cut++ {
		wire := make([]byte, cut)
		wire[0] = 0x21
		fields, err := DecodePIMControl(wire, PIMWireContext{}, 32)
		require.Error(t, err, "the encapsulated header is missing")
		for _, key := range []string{"Border", "Null Register"} {
			_, present := fields[key]
			require.Equal(t, cut == 8, present)
			if cut == 8 {
				require.Equal(t, false, fields[key])
			}
		}
	}
}
