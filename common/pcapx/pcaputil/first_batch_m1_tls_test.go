package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"strings"
	"testing"
)

func TestFirstBatchT09(t *testing.T) {
	keylog, err := os.ReadFile("testdata/protocol-sessions/first-batch-m1/tls-h2-bidi.keys")
	require.NoError(t, err)
	keys, err := ParseTLSKeyLog(string(keylog))
	require.NoError(t, err)
	capture, err := os.ReadFile("testdata/protocol-sessions/first-batch-m1/tls-h2-bidi.pcap")
	require.NoError(t, err)
	for _, hasKey := range []bool{false, true} {
		for _, deferred := range []bool{false, true} {
			for _, workers := range []int{1, 2, 4} {
				t.Run(fmt.Sprintf("key=%t/deferred=%t/workers=%d", hasKey, deferred, workers), func(t *testing.T) {
					opts := []CaptureOption{WithProtocolDeferred(deferred)}
					if hasKey {
						opts = append(opts, WithTLSSecrets(keys))
					}
					events, stats, err := binReplay(t, capture, workers, opts...)
					require.NoError(t, err)
					require.Zero(t, stats.BufferedBytes)
					messages, auth, certs := 0, 0, 0
					versions := map[uint16]bool{}
					for _, e := range events {
						require.Empty(t, e.Error, "%s %s", e.Protocol, e.Summary)
						if e.Protocol == "tls" {
							if v, ok := e.Session["Negotiated Version"].(uint16); ok {
								versions[v] = true
							}
							if e.Session["Authentication Verified"] == true {
								auth++
							}
							require.Equal(t, "not-evaluated", e.Session["Certificate Trust"])
							if rows, ok := e.Session["Handshake Messages"].([]map[string]any); ok {
								for _, row := range rows {
									if _, ok := row["Certificates"]; ok {
										certs++
									}
								}
							}
						}
						if ms, ok := e.Session["GRPC Messages"].([]map[string]any); ok {
							for _, m := range ms {
								b := m["Payload"].([]byte)
								require.Len(t, b, 5)
								require.Equal(t, []byte{10, 3, 'm', '1'}, b[:4])
								require.Contains(t, []byte{'0', '1', '2'}, b[4])
								messages++
							}
							require.Equal(t, "decrypted", e.SourceBytes.Kind)
							require.NotZero(t, e.SourceBytes.ParentPDU)
							require.NotEmpty(t, e.SourceBytes.ParentPDUs)
						}
					}
					require.True(t, versions[0x303])
					require.True(t, versions[0x304])
					if hasKey {
						require.Equal(t, 12, messages)
						require.Greater(t, auth, 12)
						require.Equal(t, 2, certs)
					} else {
						require.Zero(t, messages)
						require.Zero(t, auth)
						require.Equal(t, 1, certs)
					}
				})
			}
		}
	}
	t.Run("wrong-key-no-plaintext", func(t *testing.T) {
		wrong := &TLSKeyLog{secrets: map[string][]byte{}}
		for k, v := range keys.secrets {
			wrong.secrets[k] = bytes.Repeat([]byte{0xff}, len(v))
		}
		events, stats, err := binReplay(t, capture, 2, WithTLSSecrets(wrong))
		require.NoError(t, err)
		require.Zero(t, stats.BufferedBytes)
		fail := 0
		for _, e := range events {
			require.NotEqual(t, "decrypted", e.SourceBytes.Kind)
			if e.ExpertCode == string(ErrAuthenticationFailed) {
				fail++
			}
		}
		require.Positive(t, fail)
	})
	t.Run("keylog-validation", func(t *testing.T) {
		_, err := ParseTLSKeyLog("CLIENT_RANDOM 00 11")
		require.Error(t, err)
		_, err = ParseTLSKeyLog(stringsRepeat(1<<20 + 1))
		require.Error(t, err)
		for key, v := range keys.secrets {
			label, random, _ := strings.Cut(key, ":")
			r, err := hex.DecodeString(random)
			require.NoError(t, err)
			copyKey, ok := keys.LookupTLSSecret(label, r)
			require.True(t, ok)
			copyKey[0] ^= 1
			again, ok := keys.LookupTLSSecret(label, r)
			require.True(t, ok)
			require.Equal(t, v, again)
			break
		}
	})
}
func stringsRepeat(n int) string { return string(bytes.Repeat([]byte{'x'}, n)) }

func TestFirstBatchT09KeyEpoch(t *testing.T) {
	state := newBinTLS()
	state.version = 0x304
	state.client = 0
	state.app[0] = true
	secret := bytes.Repeat([]byte{3}, 32)
	require.NoError(t, state.install13(0, secret))
	seal := func(payload []byte) []byte {
		header := []byte{23, 3, 3, 0, 0}
		binary.BigEndian.PutUint16(header[3:], uint16(len(payload)+16))
		nonce := bytes.Clone(state.iv[0])
		for i := 0; i < 8; i++ {
			nonce[11-i] ^= byte(state.seq[0] >> uint(8*i))
		}
		return append(header, state.keys[0].Seal(nil, nonce, payload, header)...)
	}
	wire := seal([]byte{'a', 23})
	bad := bytes.Clone(wire)
	bad[len(bad)-1] ^= 1
	_, _, err := state.open(0, bad)
	require.Error(t, err)
	require.Zero(t, state.seq[0])
	plain, typ, err := state.open(0, wire)
	require.NoError(t, err)
	require.Equal(t, byte(23), typ)
	require.Equal(t, []byte{'a'}, plain)
	require.Equal(t, uint64(1), state.seq[0])
	update := seal([]byte{24, 0, 0, 1, 0, 22})
	plain, typ, err = state.open(0, update)
	require.NoError(t, err)
	require.Equal(t, byte(22), typ)
	_, err = state.handshakeMessage(0, plain, true)
	require.NoError(t, err)
	require.Zero(t, state.seq[0])
	require.NotEqual(t, secret, state.secret[0])
	_, _, err = state.open(0, wire)
	require.Error(t, err)
	require.Zero(t, state.seq[0])
	plain, _, err = state.open(0, seal([]byte{'b', 23}))
	require.NoError(t, err)
	require.Equal(t, []byte{'b'}, plain)
	require.Zero(t, state.seq[1])
}
