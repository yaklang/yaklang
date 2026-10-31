package pcaputil

import (
	"crypto/elliptic"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// These are bounded plaintext syntax controls, not evidence of usable keys,
// safe DH groups, authenticated peers or a completed cryptographic exchange.
func sshSenderRoleControls() []struct {
	name, method string
	client       bool
	wire         []byte
} {
	str := sshStr
	reply := func(code byte, value []byte) []byte {
		p := append([]byte{code}, str(sshHostKeyBlob())...)
		p = append(p, str(value)...)
		sig := append(str([]byte("ssh-ed25519")), str(make([]byte, 64))...)
		return sshPkt(append(p, str(sig)...))
	}
	point := elliptic.Marshal(elliptic.P256(), elliptic.P256().Params().Gx, elliptic.P256().Params().Gy)
	gexGroup := append(append([]byte{31}, str([]byte{23})...), str([]byte{5})...)
	gexRequest := []byte{34}
	for _, n := range []uint32{2048, 3072, 8192} {
		gexRequest = binary.BigEndian.AppendUint32(gexRequest, n)
	}
	return []struct {
		name, method string
		client       bool
		wire         []byte
	}{
		{"curve-init", "curve25519-sha256", true, sshValidCurveInit()},
		{"curve-reply", "curve25519-sha256", false, sshValidCurveReply()},
		{"libssh-reply", "curve25519-sha256@libssh.org", false, sshValidCurveReply()},
		{"P256-init", "ecdh-sha2-nistp256", true, sshPkt(append([]byte{30}, str(point)...))},
		{"P256-reply", "ecdh-sha2-nistp256", false, reply(31, point)},
		{"DH-init", "diffie-hellman-group14-sha256", true, sshPkt(append([]byte{30}, str([]byte{2})...))},
		{"DH-reply", "diffie-hellman-group14-sha256", false, reply(31, []byte{2})},
		{"GEX-old-request", "diffie-hellman-group-exchange-sha256", true, sshPkt(binary.BigEndian.AppendUint32([]byte{30}, 3072))},
		{"GEX-request", "diffie-hellman-group-exchange-sha256", true, sshPkt(gexRequest)},
		{"GEX-group", "diffie-hellman-group-exchange-sha256", false, sshPkt(gexGroup)},
		{"GEX-init", "diffie-hellman-group-exchange-sha256", true, sshPkt(append([]byte{32}, str([]byte{2})...))},
		{"GEX-reply", "diffie-hellman-group-exchange-sha256", false, reply(33, []byte{2})},
	}
}

func TestSSHMethodSenderRoleRefusesOnlyCompleteContradictoryMessage(t *testing.T) {
	for _, tc := range sshSenderRoleControls() {
		for _, client := range []int{0, 1} {
			for _, deferred := range []bool{false, true} {
				for _, chunk := range []int{1, 7, 4096} {
					t.Run(fmt.Sprintf("%s/client%d/deferred%v/chunk%d", tc.name, client, deferred, chunk), func(t *testing.T) {
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(client))
						require.NoError(t, err)
						defer s.Close("sender role control")
						s.(*captureSession).f.a.config.Deferred = deferred
						feed := func(dir int, w []byte) FeedResult {
							var last FeedResult
							for len(w) > 0 {
								n := min(chunk, len(w))
								last = s.Feed(dir, time.Unix(1, 0), w[:n])
								w = w[n:]
								if len(w) > 0 {
									require.True(t, last.Err == nil || last.Err.Kind == ErrNeedMore)
									require.Empty(t, last.Events)
								}
							}
							return last
						}
						// A server may be the first observed SSH endpoint. Its order
						// does not change the independently supplied TCP client role.
						for _, d := range []int{1 - client, client} {
							require.Nil(t, feed(d, sshIdent("SSH-2.0-RoleControl")).Err)
							require.Nil(t, feed(d, sshKex(tc.method, "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")).Err)
						}
						correct := 1 - client
						if tc.client {
							correct = client
						}
						r := feed(1-correct, tc.wire)
						require.NotNil(t, r.Err, "a known sender contradicts this selected method's RFC role")
						require.Equal(t, ErrMalformedMessage, r.Err.Kind)
						require.Len(t, r.Events, 1)
						e := r.Events[0]
						require.Equal(t, "ssh", e.Protocol)
						require.Equal(t, "malformed", e.Status)
						require.Equal(t, "message", e.Session["Error Scope"])
						require.NotContains(t, e.Session, "Host Key Fingerprint")
						require.NotContains(t, e.Session, "Initial Packet Fields")
						// Public compatibility caches cannot turn this private refusal
						// into accepted method fields, or affect later engine recovery.
						e.Session = map[string]any{"Packet Name": "KEX_ECDH_REPLY", "Error Scope": "connection", "Host Key Fingerprint": "forged"}
						e.Fields, e.Structured = map[string]any{"forged": true}, map[string]any{"fields": "forged"}
						e.Protocol, e.Status, e.Error = "tls", "decoded", ""
						fields, err := e.GetFields()
						require.Nil(t, fields)
						require.Error(t, err)
						require.Empty(t, e.DisplayFieldValues().FieldMap()["ssh.host_key.sha256"])
						r = feed(correct, tc.wire)
						require.Nil(t, r.Err, "the complete cleartext boundary survives a role-only refusal")
						require.Len(t, r.Events, 1)
						require.Equal(t, "ssh", r.Events[0].Protocol)
						require.Contains(t, r.Events[0].Session, "Initial Packet Fields")
						_, err = r.Events[0].GetFields()
						require.NoError(t, err)
					})
				}
			}
		}
	}
}

func TestSSHMethodRoleDoesNotInventEvidenceForUnknownMethodsOrMissingClient(t *testing.T) {
	for _, known := range []bool{false, true} {
		for _, method := range []string{"curve25519-sha256", "private@example.test"} {
			if known && method == "curve25519-sha256" {
				continue // The contradictory known method is covered above.
			}
			opts := []ProtocolSessionOption{WithSessionTransport("tcp")}
			if known {
				opts = append(opts, WithSessionClientDirection(0))
			}
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
			require.NoError(t, err)
			for _, d := range []int{1, 0} {
				require.Nil(t, s.Feed(d, time.Unix(1, 0), sshIdent("SSH-2.0-UnknownRole")).Err)
				require.Nil(t, s.Feed(d, time.Unix(1, 0), sshKex(method, "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")).Err)
			}
			r := s.Feed(0, time.Unix(1, 0), sshValidCurveReply())
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			require.NotContains(t, r.Events[0].Session, "Host Key Fingerprint")
			require.NotContains(t, r.Events[0].Session, "Initial Packet Fields")
			require.Equal(t, "KEY_EXCHANGE_31 (layout unconfirmed)", r.Events[0].Session["Packet Name"])
			s.Close("unknown method or role")
		}
	}
}

func TestSSHMethodRoleValidationDoesNotInterpretIgnoredGuess(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
	require.NoError(t, err)
	defer s.Close("ignored guess role")
	for d := 0; d < 2; d++ {
		require.Nil(t, s.Feed(d, time.Unix(1, 0), sshIdent("SSH-2.0-GuessRole")).Err)
	}
	require.Nil(t, s.Feed(0, time.Unix(1, 0), sshKex("curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")).Err)
	guess := sshKex("diffie-hellman-group14-sha256,curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")
	guess[len(guess)-int(guess[4])-5] = 1
	require.Nil(t, s.Feed(1, time.Unix(1, 0), guess).Err)
	r := s.Feed(1, time.Unix(1, 0), sshValidCurveInit())
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, "Ignored guessed key-exchange packet", r.Events[0].Session["Packet Name"])
	require.NotContains(t, r.Events[0].Session, "Initial Packet Fields")
	r = s.Feed(1, time.Unix(1, 0), sshValidCurveReply())
	require.Nil(t, r.Err)
	require.Contains(t, r.Events[0].Session, "Host Key Fingerprint")
}
