package bin_parser

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestL2TPv3CanonicalWireEntries(t *testing.T) {
	for _, entry := range []string{"L2TPv3IPFields", "L2TPv3UDPFields"} {
		w := []byte{0xc8, 3, 0, 20, 0, 0, 0xf0, 0xf9, 0, 7, 0, 5, 0x80, 8, 0, 0, 0, 0, 0, 6}
		if entry == "L2TPv3IPFields" {
			w = append([]byte{0, 0, 0, 0}, w...)
		}
		v, err := ParseStructured(w, "l2tp3", entry)
		require.NoError(t, err)
		m := v["fields"].(map[string]any)["Message"].(map[string]any)
		require.Equal(t, "HELLO", m["Packet Name"])
		require.Equal(t, uint64(0xf0f9), m["Control Connection ID"])
		for cut := 0; cut < len(w); cut++ {
			_, err = ParseStructured(w[:cut], "l2tp3", entry)
			require.Error(t, err)
		}
	}
}
