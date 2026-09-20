package oracleprobe

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

// Oracle 26ai AES256/SHA1 sends a marker reset before ORA-01017. The
// generator consumes 16 bytes per reset, even though only 15 form the key.
func TestModernSHA1MarkerReset(t *testing.T) {
	key := bytes.Repeat([]byte{0x31}, 64)
	iv := bytes.Repeat([]byte{0x72}, 32)
	h, err := NewOracleNetworkHash(sha1.New(), key, iv, true)
	if err != nil {
		t.Fatal(err)
	}
	seed := append(append(bytes.Clone(key[:15]), 0xff), iv...)
	g, _ := rc4.NewCipher(seed)
	blocks := make([]byte, 48)
	g.XORKeyStream(blocks, blocks)
	for epoch := 0; epoch < 3; epoch++ {
		if epoch > 0 {
			if err := h.Init(); err != nil {
				t.Fatal(err)
			}
		}
		serverKey := append(bytes.Clone(blocks[epoch*16:epoch*16+15]), 180)
		server, _ := rc4.NewCipher(serverKey)
		for _, message := range []string{"authentication response", "ORA-01017"} {
			salt := make([]byte, sha1.Size)
			server.XORKeyStream(salt, salt)
			digest := sha1.Sum(append([]byte(message), salt...))
			packet := append([]byte(message), digest[:]...)
			clear, err := h.Validate(packet)
			if err != nil || string(clear) != message {
				t.Fatalf("epoch %d: %q %v", epoch, clear, err)
			}
		}
	}
	if _, err := h.Validate(bytes.Repeat([]byte{0xff}, 64)); err == nil {
		t.Fatal("accepted invalid checksum")
	}
}

func TestLegacyOraclePasswordVector(t *testing.T) {
	for _, test := range []struct{ user, password, key string }{
		{"scott", "tiger", "f894844c34402b670000000000000000"},
	} {
		key, e := getKeyFromUserNameAndPassword(test.user, test.password)
		if e != nil {
			t.Fatal(e)
		}
		if got := hex.EncodeToString(key); got != test.key {
			t.Fatalf("%s key = %s", test.user, got)
		}
	}
}

func TestLegacyUnsupportedEncodingIsExplicit(t *testing.T) {
	obj := &AuthObject{conn: &Connection{connOption: &loginOptions{UserID: "probe", Password: "Café"}}, VerifierType: 2361, EServerSessKey: strings.Repeat("0", 64)}
	if e := obj.deriveResponse(); !errors.Is(e, ErrUnsupportedCredentialEncoding) {
		t.Fatalf("expected explicit encoding limitation: %v", e)
	}
}
func TestPasswordVerifiers(t *testing.T) {
	for _, test := range []struct {
		verifier int
		modern   bool
	}{{2361, false}, {2361, true}, {6949, false}, {6949, true}, {18453, true}} {
		t.Run(fmt.Sprintf("%d/modern=%v", test.verifier, test.modern), func(t *testing.T) {
			password := "Probe_password!"
			salt := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
			checkSalt := bytes.Repeat([]byte{0x62}, 16)
			var passwordKey, speedy []byte
			switch test.verifier {
			case 2361:
				var e error
				passwordKey, e = getKeyFromUserNameAndPassword("PROBE", password)
				if e != nil {
					t.Fatal(e)
				}
			case 6949:
				sum := sha1.Sum(append([]byte(password), salt...))
				passwordKey = append(sum[:], 0, 0, 0, 0)
			case 18453:
				speedy = pbkdf2.Key([]byte(password), append(bytes.Clone(salt), []byte("AUTH_PBKDF2_SPEEDY_KEY")...), 4096, 64, sha512.New)
				sum := sha512.Sum512(append(bytes.Clone(speedy), salt...))
				passwordKey = sum[:32]
			}
			serverKey := bytes.Repeat([]byte{0x53}, 48)
			if test.verifier == 2361 {
				serverKey = serverKey[:32]
			}
			blk, _ := aes.NewCipher(passwordKey)
			encrypted := make([]byte, len(serverKey))
			cipher.NewCBCEncrypter(blk, make([]byte, 16)).CryptBlocks(encrypted, serverKey)
			caps := make([]byte, 8)
			caps[4] = 2
			if test.modern {
				caps[4] |= 32
			}
			obj := &AuthObject{conn: &Connection{ctx: context.Background(), connOption: &loginOptions{UserID: "PROBE", Password: password}}, tcpNego: &TCPNego{ServerCompileTimeCaps: caps}, EServerSessKey: hex.EncodeToString(encrypted), Salt: hex.EncodeToString(salt), VerifierType: test.verifier, pbkdf2ChkSalt: hex.EncodeToString(checkSalt), pbkdf2VgenCount: 4096, pbkdf2SderCount: 3}
			if e := obj.deriveResponse(); e != nil {
				t.Fatal(e)
			}
			client := decryptTest(t, passwordKey, obj.EClientSessKey, false)
			if bytes.Equal(client, serverKey) {
				t.Fatal("client must generate a fresh nonce")
			}
			keySize := 16
			if test.verifier == 6949 {
				keySize = 24
			}
			if test.verifier == 18453 {
				keySize = 32
			}
			var combined []byte
			if test.modern {
				n := len(client)
				if test.verifier == 2361 {
					n /= 2
				}
				if test.verifier == 6949 {
					n = 24
				}
				material := append(bytes.Clone(client[:n]), serverKey[:n]...)
				combined = pbkdf2.Key([]byte(fmt.Sprintf("%X", material)), checkSalt, 3, keySize, sha512.New)
			} else {
				n := keySize
				x := make([]byte, n)
				for i := range x {
					x[i] = client[16+i] ^ serverKey[16+i]
				}
				first := md5.Sum(x[:16])
				combined = append([]byte{}, first[:]...)
				if n > 16 {
					last := md5.Sum(x[16:])
					combined = append(combined, last[:]...)
				}
				combined = combined[:n]
			}
			clear := decryptTest(t, combined, obj.EPassword, true)
			if string(clear[16:]) != password {
				t.Fatal("server could not recover the candidate password")
			}
			if test.verifier == 18453 {
				clear = decryptTest(t, combined, obj.ESpeedyKey, false)
				if !bytes.Equal(clear[16:], speedy) {
					t.Fatal("12c+ speedy key mismatch")
				}
			}
		})
	}
}
func decryptTest(t *testing.T, key []byte, text string, padding bool) []byte {
	t.Helper()
	b, e := hex.DecodeString(text)
	if e != nil {
		t.Fatal(e)
	}
	blk, e := aes.NewCipher(key)
	if e != nil {
		t.Fatal(e)
	}
	out := make([]byte, len(b))
	cipher.NewCBCDecrypter(blk, make([]byte, 16)).CryptBlocks(out, b)
	if padding {
		n := int(out[len(out)-1])
		if n < 1 || n > 16 || !bytes.Equal(out[len(out)-n:], bytes.Repeat([]byte{byte(n)}, n)) {
			t.Fatal("invalid password padding")
		}
		out = out[:len(out)-n]
	}
	return out
}
func TestPBKDF2LimitsAndCancellation(t *testing.T) {
	for _, n := range []int{-1, 0, 1000001} {
		if _, e := generateSpeedyKey(context.Background(), []byte("salt"), []byte("pw"), n); e == nil {
			t.Fatal("accepted unbounded iteration count", n)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := generateSpeedyKey(ctx, []byte("salt"), []byte("pw"), 4096); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestMalformedCryptoInputs(t *testing.T) {
	for _, text := range []string{"", "00", "not-hex"} {
		if _, e := decryptSessionKey(true, make([]byte, 16), text); e == nil {
			t.Fatal("accepted malformed encrypted key")
		}
	}
	aesCipher, _ := NewOracleNetworkCBCEncrypter(make([]byte, 16), nil)
	for _, c := range []OracleNetworkEncryption{aesCipher} {
		for _, b := range [][]byte{nil, {0}, {17}, make([]byte, 17)} {
			if _, e := c.Decrypt(b); e == nil {
				t.Fatal("accepted malformed cipher padding")
			}
		}
	}
}
