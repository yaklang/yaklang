package pcaputil

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSSHInitialEdwardsFieldsAndInvalidLegacySignature(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, malformed := range []bool{false, true} {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
			require.NoError(t, err)
			s.(*captureSession).f.a.config.Deferred = deferred
			ts := time.Unix(1, 0)
			for dir := 0; dir < 2; dir++ {
				require.Nil(t, s.Feed(dir, ts, sshIdent("SSH-2.0-EdwardsTest")).Err)
				require.Nil(t, s.Feed(dir, ts, sshKex("curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")).Err)
			}
			wire := sshValidCurveReply()
			if malformed {
				wire = sshKexDHReply() // unchanged historical four-byte signature
			}
			r := s.Feed(1, ts, wire[:len(wire)-1])
			require.True(t, r.NeedMore)
			r = s.Feed(1, ts, wire[len(wire)-1:])
			if malformed {
				require.NotNil(t, r.Err)
				require.Equal(t, ErrMalformedMessage, r.Err.Kind)
			} else {
				require.Nil(t, r.Err)
				require.Len(t, r.Events, 1)
				e := r.Events[0]
				fields, err := e.GetFields()
				require.NoError(t, err)
				require.Equal(t, e.Session["Initial Packet Fields"], fields)
				payload := fields["Payload"].(map[string]any)
				require.Equal(t, make([]byte, 32), payload["Host Key"].(map[string]any)["Ed25519 Public Key"])
				require.Equal(t, make([]byte, 64), payload["Signature"].(map[string]any)["Ed25519 Signature Bytes"])
				require.Equal(t, false, e.Session["Signature Verified"])
				require.Equal(t, false, e.Session["Peer Identity Verified"])
				payload["Host Key"].(map[string]any)["Ed25519 Public Key"].([]byte)[0] = 255
				again, err := e.GetFields()
				require.NoError(t, err)
				require.True(t, bytes.Equal(make([]byte, 32), again["Payload"].(map[string]any)["Host Key"].(map[string]any)["Ed25519 Public Key"].([]byte)))
			}
			s.Close("Edwards field test")
		}
	}
}

func TestSSHInitialFieldsUseObservedExchangeWithoutLegacyDHReinterpretation(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, known := range []bool{false, true} {
			opts := []ProtocolSessionOption{WithSessionTransport("tcp")}
			if known {
				opts = append(opts, WithSessionClientDirection(0))
			}
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
			require.NoError(t, err)
			s.(*captureSession).f.a.config.Deferred = deferred
			ts := time.Unix(1, 0)
			for dir := 0; dir < 2; dir++ {
				s.Feed(dir, ts, sshIdent("SSH-2.0-Review"))
				r := s.Feed(dir, ts, sshKex("curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none"))
				require.Nil(t, r.Err)
			}
			key := make([]byte, 32)
			for i := range key {
				key[i] = byte(255 - i)
			}
			r := s.Feed(0, ts, sshPkt(append([]byte{30}, sshStr(key)...)))
			require.Nil(t, r.Err)
			require.Len(t, r.Events, 1)
			e := r.Events[0]
			fields, err := e.GetFields()
			require.NoError(t, err)
			payload := fields["Payload"].(map[string]any)
			require.NotContains(t, payload, "SSHKexDHInit", "a shared message code cannot force a fixed-DH layout")
			if known {
				require.Equal(t, key, payload["Client Curve25519 Public Key"], "binary bytes must not become lossy UTF-8 text")
				require.Equal(t, e.Session["Initial Packet Fields"], fields)
			} else {
				require.NotContains(t, payload, "Client Curve25519 Public Key")
				require.Equal(t, sshStr(key), payload["Opaque Message Data"], "unknown preference retains the literal unselected method body")
			}
			e.Fields = map[string]any{"Payload": "edited public cache"}
			e.Structured = map[string]any{"fields": e.Fields}
			e.Session["Kex Algorithm"] = "edited public context"
			again, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, fields, again, "owned selected/generic fields survive mutable public cache edits")
			if known {
				require.Equal(t, "curve25519-sha256", e.DisplayFieldValues().FieldMap()["ssh.kex.selected"][0])
			}
			s.Close("field projection test")
		}
	}
}
