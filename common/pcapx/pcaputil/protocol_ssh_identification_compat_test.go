package pcaputil

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSSHNonconformantIdentificationNeedsIndependentAdmission(t *testing.T) {
	soft := []byte("SSH-2.0-Cisco-1.25\n")
	_, _, err := sshIdentification(soft)
	require.Error(t, err, "the strict RFC identification API stays strict")
	p := probeSSH(soft, 4096)
	require.Equal(t, ProbeNeedMore, p.Verdict, "a nonconformant banner alone cannot confirm SSH")
	kex := sshKex("curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")
	p = probeSSH(append(append([]byte{}, soft...), kex...), 4096)
	require.Equal(t, ProbeAccept, p.Verdict, "a complete independently validated initial KEXINIT can confirm the observed framing")
	for _, raw := range [][]byte{[]byte("SSH-1.5-Cisco-1.25\n"), []byte("SSH-2.0-bad\x00-version\n"), []byte("SSH-2.0-\n"), append(append([]byte{}, soft...), []byte{0, 0, 0, 8, 4, 20, 0, 0, 0, 0, 0, 0}...)} {
		require.NotEqual(t, ProbeAccept, probeSSH(raw, 4096).Verdict, "control bytes, incomplete fields and a short fabricated packet are not admission")
	}
	state := &binSSH{client: 0, clientKnown: true}
	_, err = state.consume(1, soft)
	require.Error(t, err, "the consumer cannot bypass the native admission guard")
}

func TestSSHConfirmedPeerRetainsNonconformantIdentificationWarning(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
	require.NoError(t, err)
	defer s.Close("compatibility observation test")
	ts := time.Unix(1, 0)
	r := s.Feed(1, ts, []byte("SSH-2.0-Cisco-1.25\n"))
	require.True(t, r.NeedMore)
	require.Empty(t, r.Events, "no premature identity from the nonconformant banner")
	r = s.Feed(0, ts, sshIdent("SSH-2.0-OpenSSH_Review"))
	require.Nil(t, r.Err, "%v", r.Err)
	warningSeen := false
	for _, e := range r.Events {
		if e.Session["Identification"] == "SSH-2.0-Cisco-1.25" {
			warningSeen = true
			require.Equal(t, "nonconformant", e.Session["Identification Conformance"])
			require.Contains(t, e.Session["Identification Warning"], "software")
			fields, err := e.GetFields()
			require.NoError(t, err)
			require.Equal(t, "SSH-2.0-Cisco-1.25", fields["Identification"])
		}
	}
	require.True(t, warningSeen, "the pending observed identification is decoded after independent peer evidence")
	for d := 0; d < 2; d++ {
		r = s.Feed(d, ts, sshKex("curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none"))
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 1)
		require.Equal(t, "nonconformant", r.Events[0].Session["Identification Conformance"], "later good frames do not erase the identification warning")
	}
	require.Equal(t, "curve25519-sha256", r.Events[0].Session["Kex Algorithm"])
	r.Events[0].Session["Identification Warning"] = "edited public snapshot"
	for d := 0; d < 2; d++ {
		r = s.Feed(d, ts, sshNewKeys())
		require.Nil(t, r.Err)
		require.Equal(t, "nonconformant", r.Events[0].Session["Identification Conformance"])
		require.NotEqual(t, "edited public snapshot", r.Events[0].Session["Identification Warning"])
	}
	r = s.Feed(0, ts, []byte{0, 1, 2, 3, 4, 5, 6, 7})
	require.NotNil(t, r.Err)
	require.Equal(t, ErrEncrypted, r.Err.Kind)
	require.Len(t, r.Events, 1)
	require.Equal(t, "nonconformant", r.Events[0].Session["Identification Conformance"], "opaque transport retains connection evidence")
	require.Contains(t, r.Events[0].Session["Identification Warning"], "software")
}

func FuzzSSHObservedIdentificationAdmission(f *testing.F) {
	f.Add([]byte("Cisco-1.25"), uint16(20), true)
	f.Add([]byte{0, '-', 255}, uint16(255), false)
	f.Fuzz(func(t *testing.T, text []byte, cut uint16, serverFirst bool) {
		if len(text) > 200 {
			return
		}
		software := make([]byte, len(text)+1)
		for i, b := range text {
			software[i] = 33 + b%94
		}
		software[len(text)] = '-'
		banner := append(append([]byte("SSH-2.0-"), software...), '\r', '\n')
		original := bytes.Clone(banner)
		_, _, strict := sshIdentification(banner)
		require.Error(t, strict)
		require.NotEqual(t, ProbeAccept, probeSSH(banner, 4096).Verdict)
		packet := sshKex("curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")
		n := int(cut) % len(packet)
		partial := append(bytes.Clone(banner), packet[:n]...)
		require.NotEqual(t, ProbeAccept, probeSSH(partial, 4096).Verdict, "a partial KEXINIT cannot confirm a nonconformant banner")
		complete := append(bytes.Clone(banner), packet...)
		require.Equal(t, ProbeAccept, probeSSH(complete, 4096).Verdict)
		require.Equal(t, original, banner, "validation must not rewrite literal identification bytes")
		s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
		require.NoError(t, err)
		defer s.Close("fuzz observed identification")
		softDir := 0
		if serverFirst {
			softDir = 1
		}
		r := s.Feed(softDir, time.Unix(1, 0), complete)
		require.Nil(t, r.Err)
		require.Len(t, r.Events, 2)
		require.Equal(t, string(original[:len(original)-2]), r.Events[0].Session["Identification"])
		for _, e := range r.Events {
			require.Equal(t, "nonconformant", e.Session["Identification Conformance"])
			require.NotContains(t, e.Session, "Kex Algorithm", "one complete offer is not negotiated agreement")
		}
	})
}

func TestSSHIdentificationWarningSurvivesIncompleteClose(t *testing.T) {
	for _, nonconformant := range []bool{false, true} {
		t.Run(map[bool]string{false: "conformant", true: "nonconformant"}[nonconformant], func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
			require.NoError(t, err)
			ts := time.Unix(1, 0)
			banner := []byte("SSH-2.0-Review_Server\r\n")
			if nonconformant {
				banner = []byte("SSH-2.0-Cisco-1.25\n")
			}
			s.Feed(1, ts, banner)
			r := s.Feed(0, ts, sshIdent("SSH-2.0-OpenSSH_Review"))
			require.Nil(t, r.Err)
			packet := sshKex("curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")
			r = s.Feed(0, ts, packet[:9])
			require.Empty(t, r.Events)
			es := s.Close("captured stream incomplete")
			require.Len(t, es, 1)
			require.Equal(t, "incomplete", es[0].Status)
			require.Equal(t, packet[:9], es[0].Raw)
			require.NotContains(t, es[0].Session, "Packet Name", "a partial packet is not a decoded initial message")
			if nonconformant {
				require.Equal(t, "nonconformant", es[0].Session["Identification Conformance"])
				require.Contains(t, es[0].Session["Identification Warning"], "softwareversion")
			} else {
				require.NotContains(t, es[0].Session, "Identification Warning")
			}
		})
	}
}
