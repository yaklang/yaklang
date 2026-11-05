package pcaputil

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/chacha20poly1305"
	"testing"
)

// Independent Python cryptography vectors use public authored sequential keys.
// Setup Seal output is frozen by independent complete-wire digest and tag.
func TestTLSAEADRecordMaximumIndependentVectors(t *testing.T) {
	vectors := []struct {
		version, suite             uint16
		name                       string
		content, padding, keylen   int
		chacha, explicit, accepted bool
		digest, tag                string
	}{
		{0x0303, 0xc02f, "below", 16383, 0, 16, false, true, true, "7fdaeea5351b21d54d17f2284b3c5b96117d91b1ab48ed6ef7d95d2261110cf1", "184f6dc169777641157348b001c2e348"},
		{0x0303, 0xc02f, "maximum", 16384, 0, 16, false, true, true, "2a74c5afbbba795102ed50e000eed59605d8e2dcd5eac781ebc570b03899230d", "eed3d8532ede52e83320a390924f4986"},
		{0x0303, 0xc02f, "content-overflow", 16385, 0, 16, false, true, false, "c78a5318992f7ccdf7ae0d43b36cfcdb470c2c17225a549bb377d991b73b799c", "9fb24f1a4d81632351e1f8459a81a93e"},
		{0x0303, 0xc02b, "below", 16383, 0, 16, false, true, true, "7fdaeea5351b21d54d17f2284b3c5b96117d91b1ab48ed6ef7d95d2261110cf1", "184f6dc169777641157348b001c2e348"},
		{0x0303, 0xc02b, "maximum", 16384, 0, 16, false, true, true, "2a74c5afbbba795102ed50e000eed59605d8e2dcd5eac781ebc570b03899230d", "eed3d8532ede52e83320a390924f4986"},
		{0x0303, 0xc02b, "content-overflow", 16385, 0, 16, false, true, false, "c78a5318992f7ccdf7ae0d43b36cfcdb470c2c17225a549bb377d991b73b799c", "9fb24f1a4d81632351e1f8459a81a93e"},
		{0x0303, 0xc030, "below", 16383, 0, 32, false, true, true, "b3cdaeccd7b053d9c123590c96b96a82d6f2c3150640a20838498f328e93304a", "9c0be4f8bd21082d728514af52e543ad"},
		{0x0303, 0xc030, "maximum", 16384, 0, 32, false, true, true, "0573b286370e3b7386e3f82aebaf3ab776699bf09a2a238de7cb911ffc62db4b", "19d10d49eecd3f0e606b8f2633aec4e8"},
		{0x0303, 0xc030, "content-overflow", 16385, 0, 32, false, true, false, "853dfcc99b0dd51861e783bc9216e5b6a0ba1b90f538e535af090154b93eeed0", "053ec4aeda302b22558b2267007f62eb"},
		{0x0303, 0xc02c, "below", 16383, 0, 32, false, true, true, "b3cdaeccd7b053d9c123590c96b96a82d6f2c3150640a20838498f328e93304a", "9c0be4f8bd21082d728514af52e543ad"},
		{0x0303, 0xc02c, "maximum", 16384, 0, 32, false, true, true, "0573b286370e3b7386e3f82aebaf3ab776699bf09a2a238de7cb911ffc62db4b", "19d10d49eecd3f0e606b8f2633aec4e8"},
		{0x0303, 0xc02c, "content-overflow", 16385, 0, 32, false, true, false, "853dfcc99b0dd51861e783bc9216e5b6a0ba1b90f538e535af090154b93eeed0", "053ec4aeda302b22558b2267007f62eb"},
		{0x0303, 0x009c, "below", 16383, 0, 16, false, true, true, "7fdaeea5351b21d54d17f2284b3c5b96117d91b1ab48ed6ef7d95d2261110cf1", "184f6dc169777641157348b001c2e348"},
		{0x0303, 0x009c, "maximum", 16384, 0, 16, false, true, true, "2a74c5afbbba795102ed50e000eed59605d8e2dcd5eac781ebc570b03899230d", "eed3d8532ede52e83320a390924f4986"},
		{0x0303, 0x009c, "content-overflow", 16385, 0, 16, false, true, false, "c78a5318992f7ccdf7ae0d43b36cfcdb470c2c17225a549bb377d991b73b799c", "9fb24f1a4d81632351e1f8459a81a93e"},
		{0x0303, 0x009d, "below", 16383, 0, 32, false, true, true, "b3cdaeccd7b053d9c123590c96b96a82d6f2c3150640a20838498f328e93304a", "9c0be4f8bd21082d728514af52e543ad"},
		{0x0303, 0x009d, "maximum", 16384, 0, 32, false, true, true, "0573b286370e3b7386e3f82aebaf3ab776699bf09a2a238de7cb911ffc62db4b", "19d10d49eecd3f0e606b8f2633aec4e8"},
		{0x0303, 0x009d, "content-overflow", 16385, 0, 32, false, true, false, "853dfcc99b0dd51861e783bc9216e5b6a0ba1b90f538e535af090154b93eeed0", "053ec4aeda302b22558b2267007f62eb"},
		{0x0303, 0x009e, "below", 16383, 0, 16, false, true, true, "7fdaeea5351b21d54d17f2284b3c5b96117d91b1ab48ed6ef7d95d2261110cf1", "184f6dc169777641157348b001c2e348"},
		{0x0303, 0x009e, "maximum", 16384, 0, 16, false, true, true, "2a74c5afbbba795102ed50e000eed59605d8e2dcd5eac781ebc570b03899230d", "eed3d8532ede52e83320a390924f4986"},
		{0x0303, 0x009e, "content-overflow", 16385, 0, 16, false, true, false, "c78a5318992f7ccdf7ae0d43b36cfcdb470c2c17225a549bb377d991b73b799c", "9fb24f1a4d81632351e1f8459a81a93e"},
		{0x0303, 0x009f, "below", 16383, 0, 32, false, true, true, "b3cdaeccd7b053d9c123590c96b96a82d6f2c3150640a20838498f328e93304a", "9c0be4f8bd21082d728514af52e543ad"},
		{0x0303, 0x009f, "maximum", 16384, 0, 32, false, true, true, "0573b286370e3b7386e3f82aebaf3ab776699bf09a2a238de7cb911ffc62db4b", "19d10d49eecd3f0e606b8f2633aec4e8"},
		{0x0303, 0x009f, "content-overflow", 16385, 0, 32, false, true, false, "853dfcc99b0dd51861e783bc9216e5b6a0ba1b90f538e535af090154b93eeed0", "053ec4aeda302b22558b2267007f62eb"},
		{0x0303, 0xcca8, "below", 16383, 0, 32, true, false, true, "743a61f335072d6b1d64b40e5a278b3b3f8dc342e1c1ecfb9920f424faf40532", "aef7229a49d321caa375c6947450e5d1"},
		{0x0303, 0xcca8, "maximum", 16384, 0, 32, true, false, true, "87852716660e95698524916d9f146e9351ef72a53797e465f334b41798fd6bf0", "cc0271a9adb9a0a1fd6b544d99d961cc"},
		{0x0303, 0xcca8, "content-overflow", 16385, 0, 32, true, false, false, "1afc0d7faf33b58ea6899007f2f538561306c4de62d8c9d77f2743033f6f8094", "0098abe37dcc119b59ed24e5bb65d066"},
		{0x0303, 0xcca9, "below", 16383, 0, 32, true, false, true, "743a61f335072d6b1d64b40e5a278b3b3f8dc342e1c1ecfb9920f424faf40532", "aef7229a49d321caa375c6947450e5d1"},
		{0x0303, 0xcca9, "maximum", 16384, 0, 32, true, false, true, "87852716660e95698524916d9f146e9351ef72a53797e465f334b41798fd6bf0", "cc0271a9adb9a0a1fd6b544d99d961cc"},
		{0x0303, 0xcca9, "content-overflow", 16385, 0, 32, true, false, false, "1afc0d7faf33b58ea6899007f2f538561306c4de62d8c9d77f2743033f6f8094", "0098abe37dcc119b59ed24e5bb65d066"},
		{0x0303, 0xccaa, "below", 16383, 0, 32, true, false, true, "743a61f335072d6b1d64b40e5a278b3b3f8dc342e1c1ecfb9920f424faf40532", "aef7229a49d321caa375c6947450e5d1"},
		{0x0303, 0xccaa, "maximum", 16384, 0, 32, true, false, true, "87852716660e95698524916d9f146e9351ef72a53797e465f334b41798fd6bf0", "cc0271a9adb9a0a1fd6b544d99d961cc"},
		{0x0303, 0xccaa, "content-overflow", 16385, 0, 32, true, false, false, "1afc0d7faf33b58ea6899007f2f538561306c4de62d8c9d77f2743033f6f8094", "0098abe37dcc119b59ed24e5bb65d066"},
		{0x0304, 0x1301, "below", 16383, 0, 16, false, false, true, "841a3159a182400a0371e7e4d27e6fd862526cddfca0f748c887af6bf2ba11bd", "0fbe2b40ba8bb693f3e56f96572e2ec2"},
		{0x0304, 0x1301, "maximum", 16384, 0, 16, false, false, true, "5df254bc5a05d1fd538128f20206a3809e2da0358644a2b7b0d0359000c6a4a3", "2a97e334c3ab2fb94befd3d8c0581376"},
		{0x0304, 0x1301, "content-overflow", 16385, 0, 16, false, false, false, "c266983ed18ce1a8e2a66af054cbb77e6b53d6dd6a988f412e256291f78d1d28", "5f55bba64ce678b3f4c0ac4b00d41d30"},
		{0x0304, 0x1301, "padding-maximum", 1, 16383, 16, false, false, true, "365c0d1ed41e88d6d35120ca525397eed97b5d7d4dd88eef6922bc651511455a", "f8f0d18b95715b994b862c9329791fa2"},
		{0x0304, 0x1301, "padding-overflow", 1, 16384, 16, false, false, false, "e5fcfc6f4f1c9b5c4917484691c7de41f29b99e2792bd9b052fecb56c39ebc0c", "e30d5f00c60900ca6ac2b3d99d883dcb"},
		{0x0304, 0x1301, "empty-padded-maximum", 0, 16384, 16, false, false, true, "2fbc4dde5986eb0bdb64bb2998dc4a92ec9844f3609d95d95caf87e65213eafb", "4140a1fb1ccbacaba04756375763053e"},
		{0x0304, 0x1301, "empty-padding-overflow", 0, 16385, 16, false, false, false, "b66b019bb289c2354a8c0fbfa7822e44fca4e79beb87b3f59e869b97e84fc58a", "5abd2f704fb3f7f88103c97de3922757"},
		{0x0304, 0x1301, "full-content-padding-overflow", 16384, 1, 16, false, false, false, "7935af5f3199f71ad007663c5ec1eb3bd21a364010019f55f2e3d9b852e59822", "316a6dbf90d374ea6aab4c9274a9311f"},
		{0x0304, 0x1302, "below", 16383, 0, 32, false, false, true, "147586c08e3a85d6f4e7043868374e23f3543a0191e7206556aab67ece205249", "1f1f4c98bea9856ea3cc4c085c6c180b"},
		{0x0304, 0x1302, "maximum", 16384, 0, 32, false, false, true, "e2b912a563b5833648d46d2950e400a56d63c370724ac06f0c989b808289fc61", "78215b0507a85c4ff15af7e7dddc8ba2"},
		{0x0304, 0x1302, "content-overflow", 16385, 0, 32, false, false, false, "9cff31e8dd5336c0ac46dec44fe356ff428163a4f89fae299dd1544f3b47c280", "f17292ffc8e89a80dccc1302fe4337ff"},
		{0x0304, 0x1302, "padding-maximum", 1, 16383, 32, false, false, true, "339202bbbc5a5e3e8544d933e1638474f1253237a2deaa6207b99a429e0e0582", "7c4ae18e19ff1611fb3cf2b7449675c2"},
		{0x0304, 0x1302, "padding-overflow", 1, 16384, 32, false, false, false, "7b34a72edf1d64c4147f1f0015a22cd1be0c8940919accb941f82d037207a8d5", "e9bc2a0cce1fcb80339b9bdf77ed828c"},
		{0x0304, 0x1302, "empty-padded-maximum", 0, 16384, 32, false, false, true, "ddbead600b9b25d1b694ad2ca73f2acaed1be415792737d4b9f86a8919d851dc", "e7ac4dd282d95c5f2c87851e97aed198"},
		{0x0304, 0x1302, "empty-padding-overflow", 0, 16385, 32, false, false, false, "f10ba9acd04ab5cd3040c52797a59df0c244f894915469e6e55800c0053c8e02", "725a8650553981cee420ec76a4d526d6"},
		{0x0304, 0x1302, "full-content-padding-overflow", 16384, 1, 32, false, false, false, "0bc97930f5ec3e56f38d7538fc5997d61ce1caec7e1a8818eb1ed351f402993d", "edd79087d04881de39fd9e8feea77cec"},
		{0x0304, 0x1303, "below", 16383, 0, 32, true, false, true, "58a4b52ba19d58fa9621395fb8ebde6185178c389a6ac4f4333a0d8281238a01", "1819c17040d4b6f8270dadbcf548f958"},
		{0x0304, 0x1303, "maximum", 16384, 0, 32, true, false, true, "1b6fe480ea952973a5fc8d3879b1d9f63bb7b0132bb0d59fa90a8e78ad8026df", "af59d1f88e62807aedcd9d05ea91332d"},
		{0x0304, 0x1303, "content-overflow", 16385, 0, 32, true, false, false, "c42728060da6f5b892269049ef856fc2201bcb8a96e92b2d69c39b03fabf67a5", "f34e77ab11c9ca0e1ef810c04a67b2e9"},
		{0x0304, 0x1303, "padding-maximum", 1, 16383, 32, true, false, true, "5745d774ea07af7f712873397edb5749fb3e9aba31ada16bcf1e7bf3af2b1b02", "8fa9d5dc3cc46205aa34a435f36888d2"},
		{0x0304, 0x1303, "padding-overflow", 1, 16384, 32, true, false, false, "876f3a98a28606203f23b668ab012990f37d54841767b122f0a8c087c9072089", "aca1d125b400bcc818ccdd84c551e1ad"},
		{0x0304, 0x1303, "empty-padded-maximum", 0, 16384, 32, true, false, true, "6e78cb6cbef5496dfbcbf471184a89f6cec755e97c3a6edf15b00a4239eb3642", "a7c1a6d39bc99aef385a5a6a5cf80e60"},
		{0x0304, 0x1303, "empty-padding-overflow", 0, 16385, 32, true, false, false, "7f2168d3f197a4c39d8abe336b2ba4c8b4b06d544da79ed1e62807b95eb09289", "c9b9a21c1306f4b2a7f193b92ee1673b"},
		{0x0304, 0x1303, "full-content-padding-overflow", 16384, 1, 32, true, false, false, "6742d4fc018b159d1ad29877a46f855b01e6e990afcf8e7a10ab87493c1bb624", "d151cd41069fd93d5c65d754bc7a8c08"},
	}
	for _, v := range vectors {
		for dir := 0; dir < 2; dir++ {
			t.Run(fmt.Sprintf("%04x-%04x/%s/dir%d", v.version, v.suite, v.name, dir), func(t *testing.T) {
				key := make([]byte, v.keylen)
				for i := range key {
					key[i] = byte(i)
				}
				ivlen := 12
				if v.explicit {
					ivlen = 4
				}
				iv := make([]byte, ivlen)
				for i := range iv {
					iv[i] = byte(i)
				}
				var aead cipher.AEAD
				var err error
				if v.chacha {
					aead, err = chacha20poly1305.New(key)
				} else {
					block, e := aes.NewCipher(key)
					require.NoError(t, e)
					aead, err = cipher.NewGCM(block)
				}
				require.NoError(t, err)
				content := bytes.Repeat([]byte{'A'}, v.content)
				plain := bytes.Clone(content)
				if v.version == 0x304 {
					plain = append(plain, 23)
					plain = append(plain, make([]byte, v.padding)...)
				}
				explicit := []byte(nil)
				if v.explicit {
					explicit, _ = hex.DecodeString("cafe001122334455")
				}
				nonce := bytes.Clone(iv)
				if v.explicit {
					nonce = append(nonce, explicit...)
				} else {
					nonce[11] ^= 5
				}
				wire := []byte{23, 3, 3, 0, 0}
				binary.BigEndian.PutUint16(wire[3:], uint16(len(explicit)+len(plain)+aead.Overhead()))
				aad := wire
				if v.version == 0x303 {
					aad = make([]byte, 13)
					binary.BigEndian.PutUint64(aad, 5)
					copy(aad[8:11], wire[:3])
					binary.BigEndian.PutUint16(aad[11:], uint16(len(plain)))
				}
				ct := aead.Seal(nil, nonce, plain, aad)
				wire = append(wire, explicit...)
				wire = append(wire, ct...)
				require.Equal(t, v.digest, fmt.Sprintf("%x", sha256.Sum256(wire)))
				require.Equal(t, v.tag, hex.EncodeToString(ct[len(ct)-16:]))
				makeState := func() *binTLS {
					s := newBinTLS()
					s.client, s.version, s.suite = 0, v.version, v.suite
					s.random = bytes.Repeat([]byte{1}, 32)
					s.serverRandom = bytes.Repeat([]byte{2}, 32)
					s.app = [2]bool{true, true}
					s.ccs = [2]bool{true, true}
					s.keys[dir], s.iv[dir], s.seq[dir] = aead, bytes.Clone(iv), 5
					return s
				}
				s := makeState()
				observed, kind, err := s.open(dir, wire)
				if v.accepted {
					require.NoError(t, err)
					require.Equal(t, byte(23), kind)
					require.Equal(t, content, observed)
					require.EqualValues(t, 6, s.seq[dir])
				} else {
					var pe *ProtocolError
					require.ErrorAs(t, err, &pe, "valid AEAD does not authorize an over-limit plaintext")
					require.Equal(t, ErrMalformedMessage, pe.Kind)
					require.Nil(t, observed)
					require.EqualValues(t, 5, s.seq[dir])
				}
				a, _ := remainingParser(t)
				s = makeState()
				flow := &binFlow{a: a, tls: s, transportFlow: &TrafficFlow{}}
				e := &ProtocolEvent{Raw: bytes.Clone(wire)}
				_, err = flow.consumeTLS(dir, e)
				if v.accepted {
					require.NoError(t, err)
					require.Equal(t, true, e.Session["Authentication Verified"])
					require.Equal(t, content, s.plaintext)
				} else {
					var pe *ProtocolError
					require.ErrorAs(t, err, &pe)
					require.Equal(t, ErrMalformedMessage, pe.Kind)
					require.Equal(t, false, e.Session["Authentication Verified"], "impossible negotiated size is refused before key access/authentication")
					require.Nil(t, s.plaintext)
					require.Nil(t, s.child)
					require.True(t, s.recordGap[dir])
					require.True(t, flow.plaintextGap[dir])
					require.EqualValues(t, 5, s.seq[dir])
				}

				if !v.accepted {
					// Native refusal owns its negotiation; editable DTOs cannot establish it.
					e.Protocol = "tls"
					e.nativeTLSObservation = true
					e.Status, e.sessionError = classifySessionError(err)
					refusal, ok := e.NativeTLSCiphertextRefusal()
					require.True(t, ok)
					require.Equal(t, v.version, refusal.Version)
					require.Contains(t, refusal.Error, "plaintext limit")
					e.Session["Negotiated Version"] = uint16(0)
					e.Session["Authentication Verified"] = true
					refusal, ok = e.NativeTLSCiphertextRefusal()
					require.True(t, ok)
					require.Equal(t, v.version, refusal.Version)
					require.Equal(t, false, refusal.Fields["Authentication Verified"])
					dto, err := json.Marshal(e)
					require.NoError(t, err)
					var forged ProtocolEvent
					require.NoError(t, json.Unmarshal(dto, &forged))
					_, ok = forged.NativeTLSCiphertextRefusal()
					require.False(t, ok)
					recovered := []byte("GET /after-overflow HTTP/1.1\r\nHost: vector.invalid\r\n\r\n")
					p := bytes.Clone(recovered)
					if v.version == 0x304 {
						p = append(p, 23)
					}
					explicit2 := bytes.Clone(explicit)
					if len(explicit2) > 0 {
						explicit2[len(explicit2)-1]++
					}
					n := bytes.Clone(iv)
					if v.explicit {
						n = append(n, explicit2...)
					} else {
						n[11] ^= 6
					}
					nextWire := []byte{23, 3, 3, 0, 0}
					binary.BigEndian.PutUint16(nextWire[3:], uint16(len(explicit2)+len(p)+aead.Overhead()))
					ad := nextWire
					if v.version == 0x303 {
						ad = make([]byte, 13)
						binary.BigEndian.PutUint64(ad, 6)
						copy(ad[8:11], nextWire[:3])
						binary.BigEndian.PutUint16(ad[11:], uint16(len(p)))
					}
					encrypted := aead.Seal(nil, n, p, ad)
					nextWire = append(nextWire, explicit2...)
					nextWire = append(nextWire, encrypted...)
					corrupt := bytes.Clone(nextWire)
					corrupt[len(corrupt)-1] ^= 1
					before := *s
					_, _, err = flow.openTLSRecord(dir, corrupt, map[string]any{})
					require.True(t, tlsAuthenticationError(err))
					require.Equal(t, before, *s, "all unsuccessful sequence/epoch trials remain read-only")
					next := &ProtocolEvent{Raw: nextWire}
					_, err = flow.consumeTLS(dir, next)
					require.NoError(t, err)
					require.Equal(t, true, next.Session["Authentication Verified"])
					require.Equal(t, true, next.Session["Record Recovery Verified"])
					require.EqualValues(t, 6, next.Session["Recovered Record Sequence"])
					require.EqualValues(t, 7, s.seq[dir])
					require.Zero(t, s.seq[1-dir])
					require.Equal(t, "unavailable across prior plaintext gap", next.Session["Application Decoding"])
					flow.deliverTLS(dir, next)
					require.Nil(t, s.plaintext)
					require.NotNil(t, s.child)
					require.True(t, s.child.directions[dir].stopped)
					require.True(t, s.child.directions[dir].tlsPlaintextGap)
					require.Nil(t, s.child.directions[dir].http)
				}
				flow.closeSession()
				require.Zero(t, a.buffered.Load())
			})
		}
	}
}
