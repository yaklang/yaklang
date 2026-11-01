package pcaputil

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSSHOnlyExactCapabilityNamesAreRemovedFromKexPreference(t *testing.T) {
	private := []string{"ext-info-private@example.test", "kex-strict-private@example.test", "EXT-INFO-c", "curve25519-sha256"}
	all := append(append([]string{}, private...), "ext-info-c", "ext-info-s", "kex-strict-c-v00@openssh.com", "kex-strict-s-v00@openssh.com")
	require.Equal(t, strings.Join(private, ","), sshRealKexNames(strings.Join(all, ",")))
}

func TestSSHPrivateMethodNamesDoNotSelectStandardFieldsOrHostKeys(t *testing.T) {
	names := []string{"ext-info-private@example.test", "kex-strict-private@example.test", "diffie-hellman-group-private@example.test", "diffie-hellman-group-exchange-private@example.test", "private-curve25519-sha256@example.test", "private@example.test", "ecdh-private@example.test", "ecdh-sha2-nistp384", "ecdh-sha2-nistp521", "curve448-sha512"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			require.Empty(t, sshKexFamily(name), "an unselected method has no selected field family")
			require.Equal(t, "KEY_EXCHANGE_31 (layout unconfirmed)", sshExchangeMessageName(31, name))
			for _, client := range []int{0, 1} {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionClientDirection(client))
				require.NoError(t, err)
				ts := time.Unix(49, 0)
				for dir := 0; dir < 2; dir++ {
					require.Nil(t, s.Feed(dir, ts, sshIdent("SSH-2.0-KexIdentity")).Err)
				}
				for dir := 0; dir < 2; dir++ {
					r := s.Feed(dir, ts, sshKex(name+",curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none"))
					require.Nil(t, r.Err)
					if dir == 1 {
						require.Equal(t, name, r.Events[0].Session["Kex Algorithm"])
					}
				}
				for _, msg := range []byte{30, 31, 33} {
					// The first string is deliberately ordinary opaque content,
					// with neither a supported exchange layout nor a host key.
					packet := sshPkt(append([]byte{msg}, sshStr([]byte("opaque content"))...))
					r := s.Feed(1-client, ts, packet)
					require.Nil(t, r.Err, "%s message%d", name, msg)
					require.Len(t, r.Events, 1)
					e := r.Events[0]
					require.Equal(t, "ssh", e.Protocol)
					require.Equal(t, name, e.Session["Kex Algorithm"])
					require.Empty(t, e.Session["Kex Family"])
					require.NotContains(t, e.Session, "Host Key Fingerprint")
					require.NotContains(t, e.Session, "Host Key SHA256")
					require.NotContains(t, e.Session, "Host Key Format")
					require.NotContains(t, e.Session, "Initial Packet Fields")
				}
			}
		})
	}
}

func TestSSHSelectedMethodNamesRetainConcreteMessageFamily(t *testing.T) {
	for _, name := range []string{"diffie-hellman-group1-sha1", "diffie-hellman-group14-sha1", "diffie-hellman-group14-sha256", "diffie-hellman-group15-sha512", "diffie-hellman-group16-sha512", "diffie-hellman-group17-sha512", "diffie-hellman-group18-sha512"} {
		require.Equal(t, "DH", sshKexFamily(name))
		require.Equal(t, "KEXDH_REPLY", sshExchangeMessageName(31, name))
	}
	for _, name := range []string{"diffie-hellman-group-exchange-sha1", "diffie-hellman-group-exchange-sha256"} {
		require.Equal(t, "DH", sshKexFamily(name))
		require.Equal(t, "KEX_DH_GEX_REPLY", sshExchangeMessageName(33, name))
	}
	for _, name := range []string{"curve25519-sha256", "curve25519-sha256@libssh.org", "ecdh-sha2-nistp256"} {
		require.Equal(t, "ECDH", sshKexFamily(name))
		require.Equal(t, "KEX_ECDH_REPLY", sshExchangeMessageName(31, name))
	}
}

func TestSSHCapabilityMarkerBeforeRealPreferenceDoesNotDiscardCorrectGuess(t *testing.T) {
	s := &binSSH{client: 0, clientKnown: true}
	for dir := 0; dir < 2; dir++ {
		_, err := s.consume(dir, sshIdent("SSH-2.0-IndicatorGuess"))
		require.NoError(t, err)
	}
	client := sshKex("ext-info-c,curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")
	client[len(client)-int(client[4])-5] = 1
	_, err := s.consume(0, client)
	require.NoError(t, err)
	_, err = s.consume(1, sshKex("curve25519-sha256,ext-info-s", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none"))
	require.NoError(t, err)
	info, err := s.consume(0, sshValidCurveInit())
	require.NoError(t, err)
	require.Equal(t, "KEX_ECDH_INIT", info["Packet Name"])
	require.Contains(t, info, "Initial Packet Fields")
	require.NotEqual(t, "discarded after algorithm mismatch", info["Payload Interpretation"])
}

func TestSSHKnownReplyStillOwnsOpaquePrivateHostKeyFormat(t *testing.T) {
	const format = "private-key@example.test"
	host := append(sshStr([]byte(format)), 1, 2, 3)
	sig := append(sshStr([]byte("ssh-ed25519")), sshStr(make([]byte, 64))...)
	payload := append([]byte{31}, sshStr(host)...)
	payload = append(payload, sshStr(make([]byte, 32))...)
	payload = append(payload, sshStr(sig)...)
	s := &binSSH{client: 0, clientKnown: true, banner: [2]bool{true, true}, selected: map[string]string{"Kex Algorithm": "curve25519-sha256", "Host Key Algorithm": format}}
	info, err := s.consumeMessage(1, sshPkt(payload))
	require.NoError(t, err)
	// The selected complete reply proves ownership of the host-key string,
	// even when its inner key format remains opaque and unauthenticated.
	require.Equal(t, format, info["Host Key Format"])
	require.NotEmpty(t, info["Host Key Fingerprint"])
	require.False(t, info["Peer Identity Verified"].(bool))
}
