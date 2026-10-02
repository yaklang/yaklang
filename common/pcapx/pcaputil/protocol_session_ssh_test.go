package pcaputil

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func sshIdent(s string) []byte {
	return []byte(s + "\r\n")
}

func sshStr(b []byte) []byte {
	w := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(w, uint32(len(b)))
	copy(w[4:], b)
	return w
}

func sshPkt(payload []byte) []byte {
	pad := 8 - (5+len(payload))%8
	if pad < 4 {
		pad += 8
	}
	plen := 1 + len(payload) + pad
	w := make([]byte, 4+plen)
	binary.BigEndian.PutUint32(w, uint32(plen))
	w[4] = byte(pad)
	copy(w[5:], payload)
	return w
}

func sshKex(kex, host, enc, mac, comp string) []byte {
	p := []byte{20}
	p = append(p, make([]byte, 16)...)
	for _, s := range []string{kex, host, enc, enc, mac, mac, comp, comp, "", ""} {
		p = append(p, sshStr([]byte(s))...)
	}
	p = append(p, 0, 0, 0, 0, 0)
	return sshPkt(p)
}

func sshKexDHInit() []byte {
	return sshPkt(append([]byte{30}, sshStr([]byte{2})...))
}

func sshHostKeyBlob() []byte {
	inner := append(sshStr([]byte("ssh-ed25519")), sshStr(make([]byte, 32))...)
	return inner
}

func sshKexDHReply() []byte {
	host := sshHostKeyBlob()
	p := []byte{31}
	p = append(p, sshStr(host)...)
	p = append(p, sshStr(make([]byte, 32))...)
	sig := append(sshStr([]byte("ssh-ed25519")), sshStr([]byte{1, 2, 3, 4})...)
	p = append(p, sshStr(sig)...)
	return sshPkt(p)
}

func sshNewKeys() []byte {
	return sshPkt([]byte{21})
}

func TestProtocolSessionSSHHandshakeAndNewkeys(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	banner := sshIdent("SSH-2.0-OpenSSH_8.9")
	p := s.Probe(banner)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "ssh", p.Protocol)

	r := s.Feed(0, ts, []byte("SSH-2.0-"))
	require.True(t, r.NeedMore)
	require.Equal(t, ErrNeedMore, r.Err.Kind)
	r = s.Feed(0, ts, []byte("OpenSSH_8.9\r\n"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "identification", r.Events[0].Session["Packet Name"])
	require.Equal(t, "SSH-2.0-OpenSSH_8.9", r.Events[0].Session["Identification"])

	r = s.Feed(1, ts, sshIdent("SSH-2.0-sshd"))
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "identification", r.Events[0].Session["Packet Name"])

	clientKex := sshKex("curve25519-sha256,diffie-hellman-group14-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")
	serverKex := sshKex("diffie-hellman-group14-sha256,curve25519-sha256", "ssh-ed25519,rsa-sha2-256", "aes128-ctr", "hmac-sha2-256", "none")
	r = s.Feed(0, ts, clientKex)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "KEXINIT", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, serverKex)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "curve25519-sha256", r.Events[0].Session["Kex Algorithm"])
	require.Equal(t, "ssh-ed25519", r.Events[0].Session["Host Key Algorithm"])
	require.Equal(t, "aes128-ctr", r.Events[0].Session["Encryption C2S"])
	require.Equal(t, "ECDH", r.Events[0].Session["Kex Family"])

	r = s.Feed(0, ts, sshKexDHInit())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "KEXDH_INIT", r.Events[0].Session["Packet Name"])

	r = s.Feed(1, ts, sshKexDHReply())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "KEXDH_REPLY", r.Events[0].Session["Packet Name"])
	require.Equal(t, "ssh-ed25519", r.Events[0].Session["Host Key Format"])
	sum := sha256.Sum256(sshHostKeyBlob())
	require.Equal(t, "SHA256:"+base64.RawStdEncoding.EncodeToString(sum[:]), r.Events[0].Session["Host Key Fingerprint"])

	r = s.Feed(0, ts, sshNewKeys())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, "NEWKEYS", r.Events[0].Session["Packet Name"])
	r = s.Feed(1, ts, sshNewKeys())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Encrypted"])
	require.Equal(t, "ssh->encrypted", r.Events[0].Session["Protocol Transition"])

	r = s.Feed(0, ts, []byte("USERAUTH"))
	require.NotNil(t, r.Err)
	require.Equal(t, ErrEncrypted, r.Err.Kind)
}

func TestProtocolSessionSSHFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.NotEqual(t, "ssh", s.Probe([]byte("HTTP/1.1")).Protocol)
	require.Equal(t, ProbeNeedMore, s.Probe([]byte("SSH")).Verdict)
	require.Equal(t, ProbeReject, s.Probe([]byte("SSH-1.5-old\r\n")).Verdict)

	r := s.Feed(0, ts, []byte("GET / HTTP/1.1\r\n\r\n"))
	require.True(t, r.Err != nil || r.State == "undetected" || len(r.Events) == 0 || r.Events[0].Protocol != "ssh")

	s2, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	require.Nil(t, s2.Feed(0, ts, sshIdent("SSH-2.0-c")).Err)
	require.Nil(t, s2.Feed(1, ts, sshIdent("SSH-2.0-s")).Err)
	bad := make([]byte, 8)
	binary.BigEndian.PutUint32(bad, 3)
	r = s2.Feed(0, ts, bad)
	require.NotNil(t, r.Err)
	require.NotEqual(t, ErrNeedMore, r.Err.Kind)

	s3, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	r = s3.Feed(0, ts, []byte("SSH-2.0-"))
	require.True(t, r.NeedMore || r.Err != nil && r.Err.Kind == ErrNeedMore)
}

func TestProtocolSessionSSHFragmentation(t *testing.T) {
	steps := []sessionStep{
		{0, sshIdent("SSH-2.0-OpenSSH_8.9")},
		{1, sshIdent("SSH-2.0-sshd")},
		{0, sshKex("curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")},
		{1, sshKex("curve25519-sha256", "ssh-ed25519", "aes128-ctr", "hmac-sha2-256", "none")},
		{0, sshKexDHInit()},
		{1, sshKexDHReply()},
		{0, sshNewKeys()},
		{1, sshNewKeys()},
	}
	assertFragmentation(t, steps, func(chunk int) []string {
		s, err := NewProtocolSession(DefaultParserBudget())
		require.NoError(t, err)
		ts := time.Unix(1, 0)
		var names []string
		for _, st := range steps {
			for w := st.wire; len(w) > 0; {
				n := len(w)
				if chunk > 0 {
					n = min(n, chunk)
				}
				r := s.Feed(st.dir, ts, w[:n])
				for _, e := range r.Events {
					if e.Status == "decoded" || e.Status == "deferred" {
						names = append(names, fmt.Sprint(e.Session["Packet Name"]))
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}
