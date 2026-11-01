package pcaputil

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Literal RFC4253 vectors; C2S/S2C are independent even when the endpoint
// implementation normally advertises the same preferences in both directions.
func sshAEADKex(encC2S, encS2C, macC2S, macS2C string) []byte {
	payload := append([]byte{20}, make([]byte, 16)...)
	for _, value := range []string{"curve25519-sha256", "ssh-ed25519", encC2S, encS2C, macC2S, macS2C, "none", "none", "", ""} {
		payload = append(payload, sshStr([]byte(value))...)
	}
	return sshPkt(append(payload, 0, 0, 0, 0, 0))
}

func TestSSHOpenSSHAEADMACSelectionAndDirectionalNEWKEYS(t *testing.T) {
	cases := []struct {
		name, c2s, s2c, clientMAC, serverMAC, wantC2S, wantS2C string
	}{
		{"gcm128-common-MAC-is-not-used", "aes128-gcm@openssh.com", "aes128-gcm@openssh.com", "hmac-sha2-256", "hmac-sha2-256", "<implicit>", "<implicit>"},
		{"gcm256-disjoint-MAC-is-valid", "aes256-gcm@openssh.com", "aes256-gcm@openssh.com", "hmac-sha2-256", "hmac-sha2-512", "<implicit>", "<implicit>"},
		{"chacha-disjoint-MAC-is-valid", "chacha20-poly1305@openssh.com", "chacha20-poly1305@openssh.com", "hmac-sha2-256", "hmac-sha2-512", "<implicit>", "<implicit>"},
		{"gcm-C2S-CTR-S2C", "aes128-gcm@openssh.com", "aes128-ctr", "hmac-sha2-256", "hmac-sha2-256", "<implicit>", "hmac-sha2-256"},
		{"CTR-C2S-chacha-S2C", "aes128-ctr", "chacha20-poly1305@openssh.com", "hmac-sha2-256", "hmac-sha2-256", "hmac-sha2-256", "<implicit>"},
		{"unknown-name-never-infers-AEAD", "private-gcm@example.test", "private-poly1305@example.test", "hmac-sha2-256", "hmac-sha2-256", "hmac-sha2-256", "hmac-sha2-256"},
		{"RFC5647-name-keeps-its-explicit-MAC", "AEAD_AES_128_GCM", "AEAD_AES_128_GCM", "AEAD_AES_128_GCM", "AEAD_AES_128_GCM", "AEAD_AES_128_GCM", "AEAD_AES_128_GCM"},
	}
	for _, tc := range cases {
		for _, client := range []int{0, 1} {
			for _, deferred := range []bool{false, true} {
				for _, chunk := range []int{0, 1, 7} {
					t.Run(fmt.Sprintf("%s/client=%d/deferred=%t/chunk=%d", tc.name, client, deferred, chunk), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(client))
						require.NoError(t, err)
						defer s.Close("AEAD negotiation")
						cs := s.(*captureSession)
						cs.f.a.config.Deferred = deferred
						feed := func(direction int, wire []byte) *ProtocolEvent {
							var event *ProtocolEvent
							for len(wire) > 0 {
								n := len(wire)
								if chunk > 0 {
									n = min(n, chunk)
								}
								r := s.Feed(direction, time.Unix(1, 0), wire[:n])
								require.True(t, r.Err == nil || r.Err.Kind == ErrNeedMore, "%v", r.Err)
								for _, next := range r.Events {
									event = next
								}
								wire = wire[n:]
							}
							require.NotNil(t, event)
							return event
						}
						for d := 0; d < 2; d++ {
							feed(d, sshIdent("SSH-2.0-AEADTest"))
						}
						// Server-first observations must still use the actual client
						// preference and directional cipher/MAC fields.
						first := feed(1-client, sshAEADKex(tc.c2s, tc.s2c, tc.serverMAC, tc.serverMAC))
						require.NotContains(t, first.Session, "MAC C2S")
						second := feed(client, sshAEADKex(tc.c2s, tc.s2c, tc.clientMAC, tc.clientMAC))
						require.Equal(t, tc.clientMAC, second.Session["Offered MAC C2S"])
						require.Equal(t, tc.clientMAC, second.Session["Offered MAC S2C"])
						require.Equal(t, tc.wantC2S, second.Session["MAC C2S"])
						require.Equal(t, tc.wantS2C, second.Session["MAC S2C"])
						fields, err := second.GetFields()
						require.NoError(t, err)
						payload := fields["Payload"].(map[string]any)
						require.Equal(t, tc.clientMAC, payload["MAC C2S"], "GetFields is this sender's literal KEXINIT, not selected context")
						require.Equal(t, tc.clientMAC, payload["MAC S2C"])
						decoded, err := second.Decode()
						require.NoError(t, err)
						require.Equal(t, fields, decoded["fields"])
						filter, err := CompileDisplayFilter(`ssh.kex.selected == "curve25519-sha256"`)
						require.NoError(t, err)
						require.True(t, filter.Match(second))
						// Public display/cache edits must not alter private observed
						// negotiation or the immutable wire-field owner.
						second.Session["MAC C2S"] = "forged-public-context"
						second.Session["Kex Algorithm"] = "forged-public-context"
						second.Fields, second.Structured = map[string]any{"forged": true}, map[string]any{"forged": true}
						again, err := second.GetFields()
						require.NoError(t, err)
						require.Equal(t, fields, again)
						require.True(t, filter.Match(second))
						require.Equal(t, tc.wantC2S, second.semanticSession["MAC C2S"])
						require.Equal(t, tc.wantC2S, cs.f.ssh.selected["MAC C2S"])
						for _, step := range []sessionStep{{client, sshValidCurveInit()}, {1 - client, sshValidCurveReply()}, {1 - client, sshNewKeys()}} {
							event := feed(step.dir, step.wire)
							require.Equal(t, tc.wantC2S, event.Session["MAC C2S"])
							require.Equal(t, tc.wantS2C, event.Session["MAC S2C"])
						}
						opaque := s.Feed(1-client, time.Unix(1, 0), []byte("opaque SSH ciphertext"))
						require.NotNil(t, opaque.Err)
						require.Equal(t, ErrEncrypted, opaque.Err.Kind)
						last := feed(client, sshNewKeys())
						require.Equal(t, true, last.Session["Encrypted"])
						require.Equal(t, tc.wantC2S, last.Session["MAC C2S"])
						require.Equal(t, tc.wantS2C, last.Session["MAC S2C"])
						rekey := s.Feed(client, time.Unix(1, 0), sshIdent("SSH-2.0-NotANewConnection"))
						require.NotNil(t, rekey.Err)
						require.Equal(t, ErrEncrypted, rekey.Err.Kind, "protected rekey/application bytes must stay opaque")
					})
				}
			}
		}
	}
}

func TestSSHNegotiationRefusalDoesNotCommitOffersOrGuessState(t *testing.T) {
	for _, client := range []int{0, 1} {
		for _, cipher := range []string{"aes128-ctr", "private-gcm@example.test", "AEAD_AES_128_GCM"} {
			t.Run(fmt.Sprintf("client=%d/cipher=%s", client, cipher), func(t *testing.T) {
				s := &binSSH{client: client, clientKnown: true}
				for d := 0; d < 2; d++ {
					_, err := s.consume(d, sshIdent("SSH-2.0-AtomicNegotiation"))
					require.NoError(t, err)
				}
				first := sshAEADKex(cipher, cipher, "hmac-sha2-256", "hmac-sha2-256")
				_, err := s.consume(client, first)
				require.NoError(t, err)
				before := *s
				bad := sshAEADKex(cipher, cipher, "hmac-sha2-512", "hmac-sha2-512")
				// A follows flag must not become pending on negotiation failure.
				bad[len(bad)-int(bad[4])-5] = 1
				_, err = s.consume(1-client, bad)
				require.ErrorContains(t, err, "no common MAC C2S")
				require.Equal(t, before, *s)
				info, err := s.consume(1-client, first)
				require.NoError(t, err)
				require.Equal(t, "hmac-sha2-256", info["MAC C2S"])
				require.Equal(t, "hmac-sha2-256", info["MAC S2C"])
			})
		}
	}
}

func TestSSHOpenSSHAEADStillRequiresCompleteLiteralMACVectors(t *testing.T) {
	for _, cipher := range []string{"aes128-gcm@openssh.com", "aes256-gcm@openssh.com", "chacha20-poly1305@openssh.com"} {
		s := &binSSH{client: 0, clientKnown: true}
		for d := 0; d < 2; d++ {
			_, err := s.consume(d, sshIdent("SSH-2.0-CompleteVectors"))
			require.NoError(t, err)
		}
		first := sshAEADKex(cipher, cipher, "hmac-sha2-256", "hmac-sha2-256")
		_, err := s.consume(0, first)
		require.NoError(t, err)
		before := *s
		full := sshAEADKex(cipher, cipher, "hmac-sha2-512", "hmac-sha2-512")
		payload := full[5 : len(full)-int(full[4])]
		for _, cut := range []int{1, 5, 17, len(payload) - 1} {
			_, err := s.consume(1, sshPkt(bytes.Clone(payload[:cut])))
			require.Error(t, err, "AEAD does not make truncated mandatory KEXINIT vectors acceptable")
			require.Equal(t, before, *s)
		}
		_, err = s.consume(1, full)
		require.NoError(t, err)
		require.Equal(t, "<implicit>", s.selected["MAC C2S"])
		require.Equal(t, "<implicit>", s.selected["MAC S2C"])
	}
}

func TestSSHOpenSSHAEADDoesNotHideOppositeDirectionMACFailure(t *testing.T) {
	for _, tc := range []struct{ c2s, s2c, refused string }{
		{"aes128-gcm@openssh.com", "aes128-ctr", "MAC S2C"},
		{"aes128-ctr", "chacha20-poly1305@openssh.com", "MAC C2S"},
	} {
		s := &binSSH{client: 0, clientKnown: true}
		for dir := 0; dir < 2; dir++ {
			_, err := s.consume(dir, sshIdent("SSH-2.0-MixedMAC"))
			require.NoError(t, err)
		}
		_, err := s.consume(0, sshAEADKex(tc.c2s, tc.s2c, "hmac-sha2-256", "hmac-sha2-256"))
		require.NoError(t, err)
		before := *s
		_, err = s.consume(1, sshAEADKex(tc.c2s, tc.s2c, "hmac-sha2-512", "hmac-sha2-512"))
		require.ErrorContains(t, err, "no common "+tc.refused)
		require.Equal(t, before, *s)
	}
}
