package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzTLSEncryptedExtensionsAdmission(f *testing.F) {
	// RFC8701's explicit reserved set is independent of the implementation's
	// bit-pattern check. Keep the existing GREASE seeds as negative controls.
	reserved := map[uint16]bool{0x0a0a: true, 0x1a1a: true, 0x2a2a: true, 0x3a3a: true, 0x4a4a: true, 0x5a5a: true, 0x6a6a: true, 0x7a7a: true, 0x8a8a: true, 0x9a9a: true, 0xaaaa: true, 0xbaba: true, 0xcaca: true, 0xdada: true, 0xeaea: true, 0xfafa: true}
	for _, body := range [][]byte{eeScopeBody(), eeScopeBody(tlsResumptionExt{16, eeScopeALPN("h2")}), eeScopeBody(tlsResumptionExt{43, []byte{3, 3}}), eeScopeBody(tlsResumptionExt{0xaaaa, nil}, tlsResumptionExt{0xaaaa, nil}), eeScopeBody(tlsResumptionExt{0xaaaa, nil}, tlsResumptionExt{0xbbbb, nil}), {0, 1, 0}} {
		f.Add(body, byte(1))
	}
	f.Fuzz(func(t *testing.T, input []byte, budget byte) {
		body := bytes.Clone(input[:min(len(input), 4096)])
		limit := int(budget%8) + 1
		// Independent RFC envelope/placement oracle. Unknown extensions are
		// opaque except the reserved GREASE set; this is not full negotiation.
		valid := len(body) >= 2 && int(binary.BigEndian.Uint16(body)) == len(body)-2
		count := 0
		selected := ""
		seen := map[uint16]bool{}
		if valid {
			for at := 2; at < len(body); {
				if len(body)-at < 4 {
					valid = false
					break
				}
				kind, size := binary.BigEndian.Uint16(body[at:]), int(binary.BigEndian.Uint16(body[at+2:]))
				at += 4
				if size > len(body)-at || seen[kind] {
					valid = false
					break
				}
				seen[kind] = true
				count++
				data := body[at : at+size]
				at += size
				if reserved[kind] || kind == 5 || kind == 13 || kind == 18 || kind == 21 || kind == 33 || kind == 41 || kind == 43 || kind == 44 || kind == 45 || kind == 47 || kind == 48 || kind == 49 || kind == 50 || kind == 51 || kind == 42 && len(data) != 0 {
					valid = false
					break
				}
				if kind == 16 {
					if len(data) < 3 || int(binary.BigEndian.Uint16(data)) != len(data)-2 || int(data[2]) != len(data)-3 || data[2] == 0 || string(data[3:]) != "h2" {
						valid = false
						break
					}
					selected = string(data[3:])
				}
			}
		}
		valid = valid && count <= limit
		state := newBinTLS()
		state.client, state.version, state.suite = 0, 0x304, 0x1301
		state.alpnOfferKnown, state.alpnOffer = true, eeScopeALPN("h2")
		before := *state
		_, err := state.handshakeMessage(1, reliabilityHS(8, body), true, limit)
		if !valid {
			require.Error(t, err)
			require.Equal(t, before, *state)
			return
		}
		require.NoError(t, err)
		require.Equal(t, uint16(0x304), state.version)
		require.Equal(t, selected, state.alpn)
		require.Equal(t, before.alpnOffer, state.alpnOffer)
	})
}
