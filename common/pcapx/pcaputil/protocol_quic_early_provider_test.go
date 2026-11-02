package pcaputil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// This is the post-authentication CRYPTO stage. Distinct deterministic test
// secrets verify ClientRandom selection; no fixture is a captured peer session.
func TestQUICEarlySecretProviderAfterObservedHellos(t *testing.T) {
	for _, version := range []uint32{quicVersion1, quicVersion2} {
		for _, suite := range []uint16{0x1301, 0x1302, 0x1303} {
			for _, wrongRandom := range []bool{false, true} {
				t.Run(fmt.Sprintf("v%x/suite%x/wrongRandom%v", version, suite, wrongRandom), func(t *testing.T) {
					n := 32
					if suite == 0x1302 {
						n = 48
					}
					random := bytes.Repeat([]byte{0x31}, 32)
					lookup := bytes.Clone(random)
					if wrongRandom {
						lookup[0] ^= 1
					}
					var text string
					labels := []string{"CLIENT_HANDSHAKE_TRAFFIC_SECRET", "SERVER_HANDSHAKE_TRAFFIC_SECRET", "CLIENT_TRAFFIC_SECRET_0", "SERVER_TRAFFIC_SECRET_0", "CLIENT_EARLY_TRAFFIC_SECRET"}
					for i, label := range labels {
						text += fmt.Sprintf("%s %x %x\n", label, lookup, bytes.Repeat([]byte{byte(i + 1)}, n))
					}
					provider, err := ParseTLSKeyLog(text)
					require.NoError(t, err)
					initial, err := quicInitialKeysVersion([]byte("seedcid8"), version)
					require.NoError(t, err)
					q := &binQUIC{native: true, version: version, byteLimit: 1 << 20, provider: provider, keys: &quicKeyring{version: version, initial: initial}, cids: map[string]int{"seedcid8": 1}}
					share := append([]byte{0, 23, 0, 32}, bytes.Repeat([]byte{1}, 32)...)
					clientShare := append([]byte{0, byte(len(share))}, share...)
					offer := tlsResumptionPSKs([][]byte{[]byte("owned-test-ticket")}, []uint32{0}, []int{n})
					client := tlsResumptionHello(true, nil, suite, tlsResumptionExt{43, []byte{2, 3, 4}}, tlsResumptionExt{10, []byte{0, 2, 0, 23}}, tlsResumptionExt{51, clientShare}, tlsResumptionExt{45, []byte{1, 1}}, tlsResumptionExt{42, nil}, tlsResumptionExt{41, offer})
					server := tlsResumptionHello(false, nil, suite, tlsResumptionExt{43, []byte{3, 4}}, tlsResumptionExt{51, share}, tlsResumptionExt{41, []byte{0, 0}})
					require.NoError(t, q.feedCrypto(0, quicSpaceInitial, 0, quicTLSLevelMessage(1, client), map[string]any{}, 128))
					require.Nil(t, q.keys.early[0], "unselected cipher must not establish an early key")
					require.NoError(t, q.feedCrypto(1, quicSpaceInitial, 0, quicTLSLevelMessage(2, server), map[string]any{}, 128))
					if wrongRandom {
						require.Nil(t, q.keys.early[0])
						require.False(t, q.loadedKeys)
						return
					}
					require.NotNil(t, q.keys.early[0])
					require.True(t, q.loadedKeys, "optional fifth key must not unset complete regular key loading")
					expected, err := quicDeriveTrafficVersion(bytes.Repeat([]byte{5}, n), q.keys.aead, version)
					require.NoError(t, err)
					require.Equal(t, expected, q.keys.early[0])
					require.NotEqual(t, q.keys.app[0][0], q.keys.early[0])
					wire := quicConformancePacket(t, expected, "0-RTT", 1, 0, []byte{1})
					info, err := q.consume(0, wire, 128)
					require.NoError(t, err)
					require.Equal(t, true, info["Authentication Verified"])
					require.Equal(t, uint64(1), info["Packet Number"])
					// A later accepted empty CRYPTO observation cannot reload regular keys
					// merely because the optional early secret made five provider entries.
					loaded := q.keys
					require.NoError(t, q.feedCrypto(1, quicSpaceInitial, uint64(len(quicTLSLevelMessage(2, server))), nil, map[string]any{}, 128))
					require.Same(t, loaded, q.keys)
				})
			}
		}
	}
}
