package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSSHNegotiationRequiresObservedClientPreference(t *testing.T) {
	for _, client := range []int{0, 1} {
		for _, known := range []bool{false, true} {
			options := []ProtocolSessionOption{WithSessionTransport("tcp")}
			if known {
				options = append(options, WithSessionClientDirection(client))
			}
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), options...)
			require.NoError(t, err)
			ts := time.Unix(1, 0)
			for d := 0; d < 2; d++ {
				r := s.Feed(d, ts, sshIdent("SSH-2.0-RoleEndpoint"))
				require.Nil(t, r.Err)
			}
			clientWire := sshKex("curve25519-sha256,diffie-hellman-group14-sha256", "ssh-ed25519", "aes128-ctr,aes256-ctr", "hmac-sha2-256", "none")
			serverWire := sshKex("diffie-hellman-group14-sha256,curve25519-sha256", "ssh-ed25519", "aes256-ctr,aes128-ctr", "hmac-sha2-256", "none")
			// Direction 0 is always observed first. It can be the server.
			var result FeedResult
			for d := 0; d < 2; d++ {
				wire := serverWire
				if d == client {
					wire = clientWire
				}
				result = s.Feed(d, ts, wire)
				require.Nil(t, result.Err)
			}
			require.Len(t, result.Events, 1)
			info := result.Events[0].Session
			lastOffer := "aes256-ctr,aes128-ctr"
			if client == 1 {
				lastOffer = "aes128-ctr,aes256-ctr"
			}
			require.Equal(t, lastOffer, info["Offered Encryption C2S"], "wire offers survive a separate selected cipher")
			if known {
				require.Equal(t, "curve25519-sha256", info["Kex Algorithm"])
				require.Equal(t, "aes128-ctr", info["Encryption C2S"])
			} else {
				require.NotContains(t, info, "Kex Algorithm", "first observation is not client role evidence")
				require.NotContains(t, info, "Encryption C2S", "directional negotiation cannot be invented")
				require.Equal(t, "client/server role not observed", info["Negotiation Context"])
			}
			result = s.Feed(client, ts, sshValidCurveInit())
			require.Nil(t, result.Err)
			if known {
				require.Equal(t, "KEX_ECDH_INIT", result.Events[0].Session["Packet Name"])
			} else {
				require.Equal(t, "KEY_EXCHANGE_30 (layout unconfirmed)", result.Events[0].Session["Packet Name"])
				require.NotContains(t, result.Events[0].Session, "Initial Packet Fields")
				require.NotContains(t, result.Events[0].Session, "Kex Algorithm")
			}
			s.Close("role evidence test")
		}
	}
}

func TestSSHRepeatedInitialKEXINITCannotReplaceObservedOffers(t *testing.T) {
	s := &binSSH{client: 0, clientKnown: true}
	_, err := s.consume(0, sshIdent("SSH-2.0-RoleClient"))
	require.NoError(t, err)
	_, err = s.consume(1, sshIdent("SSH-2.0-RoleServer"))
	require.NoError(t, err)
	first := sshKex("curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")
	_, err = s.consume(0, first)
	require.NoError(t, err)
	before := s.kex
	for i := 0; i < 140; i++ {
		_, err = s.consume(0, sshKex("diffie-hellman-group14-sha256", "ssh-ed25519", "aes256-ctr", "hmac-sha2-256", "none"))
		require.ErrorContains(t, err, "repeated KEXINIT")
		require.Equal(t, before, s.kex)
		require.Nil(t, s.selected)
	}
	_, err = s.consume(1, first)
	require.NoError(t, err)
	require.Equal(t, "curve25519-sha256", s.selected["Kex Algorithm"])
	require.Equal(t, "aes128-ctr", s.selected["Encryption C2S"])
}

func FuzzSSHClientRoleNegotiationIsolation(f *testing.F) {
	f.Add(uint8(0), uint16(1))
	f.Add(uint8(7), uint16(128))
	f.Fuzz(func(t *testing.T, flags uint8, chunk uint16) {
		client, known := int(flags&1), flags&2 != 0
		kexA, kexB := "curve25519-sha256", "diffie-hellman-group14-sha256"
		if flags&4 != 0 {
			kexA, kexB = kexB, kexA
		}
		opts := []ProtocolSessionOption{WithSessionTransport("tcp")}
		if known {
			opts = append(opts, WithSessionClientDirection(client))
		}
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
		require.NoError(t, err)
		defer s.Close("fuzz role isolation")
		var last *ProtocolEvent
		feed := func(d int, w []byte) {
			for len(w) > 0 {
				n := min(len(w), 1+int(chunk)%128)
				r := s.Feed(d, time.Unix(1, 0), w[:n])
				require.True(t, r.Err == nil || r.Err.Kind == ErrNeedMore, "%v", r.Err)
				for _, e := range r.Events {
					if e.Session["Packet Name"] == "KEXINIT" {
						last = e
					}
				}
				w = w[n:]
			}
		}
		for d := 0; d < 2; d++ {
			feed(d, sshIdent("SSH-2.0-FuzzRole"))
		}
		for d := 0; d < 2; d++ {
			list := kexB + "," + kexA
			if d == client {
				list = kexA + "," + kexB
			}
			feed(d, sshKex(list, "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none"))
		}
		require.NotNil(t, last)
		if known {
			require.Equal(t, kexA, last.Session["Kex Algorithm"])
		} else {
			require.NotContains(t, last.Session, "Kex Algorithm")
			require.Equal(t, "client/server role not observed", last.Session["Negotiation Context"])
		}
	})
}
