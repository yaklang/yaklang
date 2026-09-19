package pcaputil

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func rfcHex(s string) []byte {
	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			return -1
		}
		return r
	}, s)
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// RFC 9001 Appendix A.2 protected Client Initial.
func rfc9001ClientInitial() []byte {
	return rfcHex(`
c000000001088394c8f03e5157080000 449e7b9aec34d1b1c98dd7689fb8ec11
d242b123dc9bd8bab936b47d92ec356c 0bab7df5976d27cd449f63300099f399
1c260ec4c60d17b31f8429157bb35a12 82a643a8d2262cad67500cadb8e7378c
8eb7539ec4d4905fed1bee1fc8aafba1 7c750e2c7ace01e6005f80fcb7df6212
30c83711b39343fa028cea7f7fb5ff89 eac2308249a02252155e2347b63d58c5
457afd84d05dfffdb20392844ae81215 4682e9cf012f9021a6f0be17ddd0c208
4dce25ff9b06cde535d0f920a2db1bf3 62c23e596d11a4f5a6cf3948838a3aec
4e15daf8500a6ef69ec4e3feb6b1d98e 610ac8b7ec3faf6ad760b7bad1db4ba3
485e8a94dc250ae3fdb41ed15fb6a8e5 eba0fc3dd60bc8e30c5c4287e53805db
059ae0648db2f64264ed5e39be2e20d8 2df566da8dd5998ccabdae053060ae6c
7b4378e846d29f37ed7b4ea9ec5d82e7 961b7f25a9323851f681d582363aa5f8
9937f5a67258bf63ad6f1a0b1d96dbd4 faddfcefc5266ba6611722395c906556
be52afe3f565636ad1b17d508b73d874 3eeb524be22b3dcbc2c7468d54119c74
68449a13d8e3b95811a198f3491de3e7 fe942b330407abf82a4ed7c1b311663a
c69890f4157015853d91e923037c227a 33cdd5ec281ca3f79c44546b9d90ca00
f064c99e3dd97911d39fe9c5d0b23a22 9a234cb36186c4819e8b9c5927726632
291d6a418211cc2962e20fe47feb3edf 330f2c603a9d48c0fcb5699dbfe58964
25c5bac4aee82e57a85aaf4e2513e4f0 5796b07ba2ee47d80506f8d2c25e50fd
14de71e6c418559302f939b0e1abd576 f279c4b2e0feb85c1f28ff18f58891ff
ef132eef2fa09346aee33c28eb130ff2 8f5b766953334113211996d20011a198
e3fc433f9f2541010ae17c1bf202580f 6047472fb36857fe843b19f5984009dd
c324044e847a4f4a0ab34f719595de37 252d6235365e9b84392b061085349d73
203a4a13e96f5432ec0fd4a1ee65accd d5e3904df54c1da510b0ff20dcc0c77f
cb2c0e0eb605cb0504db87632cf3d8b4 dae6e705769d1de354270123cb11450e
fc60ac47683d7b8d0f811365565fd98c 4c8eb936bcab8d069fc33bd801b03ade
a2e1fbc5aa463d08ca19896d2bf59a07 1b851e6c239052172f296bfb5e724047
90a2181014f3b94a4e97d117b4381303 68cc39dbb2d198065ae3986547926cd2
162f40a29f0c3c8745c0f50fba3852e5 66d44575c29d39a03f0cda721984b6f4
40591f355e12d439ff150aab7613499d bd49adabc8676eef023b15b65bfc5ca0
6948109f23f350db82123535eb8a7433 bdabcb909271a6ecbcb58b936a88cd4e
8f2e6ff5800175f113253d8fa9ca8885 c2f552e657dc603f252e1a8e308f76f0
be79e2fb8f5d5fbbe2e30ecadd220723 c8c0aea8078cdfcb3868263ff8f09400
54da48781893a7e49ad5aff4af300cd8 04a6b6279ab3ff3afb64491c85194aab
760d58a606654f9f4400e8b38591356f bf6425aca26dc85244259ff2b19c41b9
f96f3ca9ec1dde434da7d2d392b905dd f3d1f9af93d1af5950bd493f5aa731b4
056df31bd267b6b90a079831aaf579be 0a39013137aac6d404f518cfd4684064
7e78bfe706ca4cf5e9c5453e9f7cfd2b 8b4c8d169a44e55c88d4a9a7f9474241
e221af44860018ab0856972e194cd934
`)
}

// RFC 9001 Appendix A.3 protected Server Initial.
func rfc9001ServerInitial() []byte {
	return rfcHex(`
cf000000010008f067a5502a4262b500 4075c0d95a482cd0991cd25b0aac406a
5816b6394100f37a1c69797554780bb3 8cc5a99f5ede4cf73c3ec2493a1839b3
dbcba3f6ea46c5b7684df3548e7ddeb9 c3bf9c73cc3f3bded74b562bfb19fb84
022f8ef4cdd93795d77d06edbb7aaf2f 58891850abbdca3d20398c276456cbc4
2158407dd074ee
`)
}

func TestQUICInitialSecretsRFC9001(t *testing.T) {
	dcid := rfcHex("8394c8f03e515708")
	keys, err := quicInitialKeys(dcid)
	require.NoError(t, err)
	require.Equal(t, rfcHex("1f369613dd76d5467730efcbe3b1a22d"), keys[0].key)
	require.Equal(t, rfcHex("fa044b2f42a3fd3b46fb255c"), keys[0].iv)
	require.Equal(t, rfcHex("9f50449e04a0e810283a1e9933adedd2"), keys[0].hp)
	require.Equal(t, rfcHex("cf3a5331653c364c88f0f379b6067e37"), keys[1].key)
	require.Equal(t, rfcHex("0ac1493ca1905853b0bba03e"), keys[1].iv)
	require.Equal(t, rfcHex("c206b8d9b9f0f37644430b490eeaa314"), keys[1].hp)
}

func TestProtocolSessionQUICDecryptClientServerInitial(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	cli := rfc9001ClientInitial()
	p := s.Probe(cli)
	require.Equal(t, ProbeAccept, p.Verdict)
	require.Equal(t, "quic", p.Protocol)
	ts := time.Unix(1, 0)
	r := s.Feed(0, ts, cli)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Decrypted"])
	require.Equal(t, "removed", r.Events[0].Session["Header Protection"])
	require.Equal(t, "Initial", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint64(2), r.Events[0].Session["Packet Number"])
	require.Equal(t, rfcHex("8394c8f03e515708"), r.Events[0].Session["DCID"])
	frames, _ := r.Events[0].Session["Frames"].([]map[string]any)
	require.NotEmpty(t, frames)
	require.Equal(t, "CRYPTO", frames[0]["Frame Type"])
	crypto, _ := frames[0]["Crypto Data"].([]byte)
	require.Contains(t, string(crypto), "example.com")

	r = s.Feed(1, ts, rfc9001ServerInitial())
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Decrypted"])
	require.Equal(t, "Initial", r.Events[0].Session["Packet Name"])
	require.Equal(t, uint64(1), r.Events[0].Session["Packet Number"])
	frames, _ = r.Events[0].Session["Frames"].([]map[string]any)
	require.Equal(t, "ACK", frames[0]["Frame Type"])
	require.Equal(t, "CRYPTO", frames[1]["Frame Type"])
}

func TestProtocolSessionQUICMissingKeysHandshake(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, rfc9001ClientInitial()).Err)
	hs := quicLongPacket(2, 1, rfcHex("8394c8f03e515708"), rfcHex("f067a5502a4262b5"), nil, 0, bytesRepeat(0xaa, 32))
	r := s.Feed(1, ts, hs)
	require.NotNil(t, r.Err)
	require.Equal(t, ErrEncrypted, r.Err.Kind)
	require.Equal(t, true, r.Events[0].Session["Encrypted"])
	require.Equal(t, true, r.Events[0].Session["Missing Keys"])
	require.Equal(t, "handshake-keys-missing", r.Events[0].Session["Key Reason"])
	require.Equal(t, "Handshake", r.Events[0].Session["Packet Name"])
	require.Nil(t, r.Events[0].Session["Frames"])
}

func TestQUICChaCha20ShortHeaderUnprotect(t *testing.T) {
	secret := rfcHex("9ac312a7f877468ebe69422748ad00a15443f18203a07d6060f688f30f21632b")
	tk, err := quicDeriveTraffic(secret, quicAEADChaCha20Poly1305)
	require.NoError(t, err)
	require.Equal(t, rfcHex("c6d98ff3441c3fe1b2182094f69caa2ed4b716b65488960a7a984979fb23e1c8"), tk.key)
	require.Equal(t, rfcHex("e0459b3474bdd0e44a41c144"), tk.iv)
	require.Equal(t, rfcHex("25a282b9e82f06f21f488917a4fc8f1b73573685608597d0efcb076b0ab7a7a4"), tk.hp)
	mask, err := quicHeaderMask(tk, rfcHex("5e5cd55c41f69080575d7999c25a5bfb"))
	require.NoError(t, err)
	require.Equal(t, rfcHex("aefefe7d03"), mask)
	pkt := rfcHex("4cfe4189655e5cd55c41f69080575d7999c25a5bfb")
	h, err := quicParseHeader(pkt, true)
	require.NoError(t, err)
	plain, pn, pnLen, first, phase, uerr := quicUnprotect(pkt, h, tk, [2]*quicTrafficKeys{tk, nil}, 654360564-1)
	require.NoError(t, uerr)
	require.Equal(t, uint64(654360564), pn)
	require.Equal(t, 3, pnLen)
	require.Equal(t, byte(0x42), first)
	require.Equal(t, 0, phase)
	require.Equal(t, []byte{0x01}, plain)
}

func TestProtocolSessionQUICChaCha20KeyPhaseAndKeyLog(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	secret := rfcHex("9ac312a7f877468ebe69422748ad00a15443f18203a07d6060f688f30f21632b")
	require.NoError(t, ApplyQUICKeys(s, QUICKeyMaterial{
		AEAD:   quicAEADChaCha20Poly1305,
		KeyLog: "SERVER_TRAFFIC_SECRET_0 00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff " + hex.EncodeToString(secret) + "\n",
	}))
	ku, err := quicHKDFExpandLabel(secret, "quic ku", 32)
	require.NoError(t, err)
	require.Equal(t, rfcHex("1223504755036d556342ee9361d253421a826c9ecdf3c7148684b36b714881f9"), ku)

	ts := time.Unix(1, 0)
	require.Nil(t, s.Feed(0, ts, rfc9001ClientInitial()).Err)
	cs := s.(*captureSession)
	cs.f.quic.spaces[quicSpaceApplication].largest = 654360564 - 1
	cs.f.quic.spaces[quicSpaceApplication].init = true
	pkt := rfcHex("4cfe4189655e5cd55c41f69080575d7999c25a5bfb")
	r := s.Feed(1, ts, pkt)
	require.Nil(t, r.Err, "%v", r.Err)
	require.Equal(t, true, r.Events[0].Session["Decrypted"])
	require.Equal(t, 0, r.Events[0].Session["Key Phase"])
	require.Equal(t, quicAEADChaCha20Poly1305, r.Events[0].Session["Cipher"])
	frames, _ := r.Events[0].Session["Frames"].([]map[string]any)
	require.Equal(t, "PING", frames[0]["Frame Type"])
}

func TestProtocolSessionQUICDecryptFailClosed(t *testing.T) {
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	ts := time.Unix(1, 0)
	bad := append([]byte(nil), rfc9001ClientInitial()...)
	bad[len(bad)-1] ^= 0xff
	r := s.Feed(0, ts, bad)
	require.NotNil(t, r.Err)
	require.Equal(t, ErrEncrypted, r.Err.Kind)
	require.Equal(t, true, r.Events[0].Session["Encrypted"])
	require.Equal(t, "Initial", r.Events[0].Session["Packet Name"])
	require.Nil(t, r.Events[0].Session["Frames"])
	require.NotEqual(t, true, r.Events[0].Session["Decrypted"])
}

func TestProtocolSessionQUICDecryptFragmentation(t *testing.T) {
	steps := []sessionStep{
		{0, rfc9001ClientInitial()},
		{1, rfc9001ServerInitial()},
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
						names = append(names, fmt.Sprintf("%v:%v", e.Session["Packet Name"], e.Session["Packet Number"]))
					}
				}
				w = w[n:]
			}
		}
		return names
	})
}
