package ppp

import (
	"crypto/des"
	"crypto/sha1"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/crypto/md4"
	"golang.org/x/text/encoding/unicode"
	"math/bits"
)

func ToUTF16(in []byte) ([]byte, error) {
	encoder := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder()
	pwd, err := encoder.Bytes(in)
	if err != nil {
		return nil, err
	}
	return pwd, nil
}

func GenerateMSChapV2Response(authenticatorChallenge, username, password []byte) ([]byte, error) {
	peerChallenge := []byte(utils.RandSecret(16))
	//peerChallenge, _ := codec.DecodeHex(`5ce11f697d3219af14a96d85f97f72ea`)
	ntResponse, err := GenerateNTResponse(authenticatorChallenge, peerChallenge, username, password)
	if err != nil {
		return nil, err
	}
	response := append(peerChallenge, make([]byte, 8)...)
	response = append(response, ntResponse...)
	response = append(response, byte(0))

	return response, nil
}

func GenerateNTResponse(authenticatorChallenge, peerChallenge, username, password []byte) ([]byte, error) {
	challenge := ChallengeHash(peerChallenge, authenticatorChallenge, username)
	ucs2Password, err := ToUTF16(password)
	if err != nil {
		return nil, err
	}
	passwordHash := NTPasswordHash(ucs2Password)

	return ChallengeResponse(challenge, passwordHash), nil
}

// ChallengeHash - rfc2759, 8.2
func ChallengeHash(peerChallenge, authenticatorChallenge, username []byte) []byte {
	sha := sha1.New()
	sha.Write(peerChallenge)
	sha.Write(authenticatorChallenge)
	sha.Write(username)
	return sha.Sum(nil)[:8]
}

// NTPasswordHash with MD4 - rfc2759, 8.3
func NTPasswordHash(password []byte) []byte {
	h := md4.New()
	h.Write(password)
	return h.Sum(nil)
}

// ChallengeResponse - rfc2759, 8.5
func ChallengeResponse(challenge, passwordHash []byte) []byte {
	zPasswordHash := make([]byte, 21)
	copy(zPasswordHash, passwordHash)

	challengeResponse := make([]byte, 24)
	copy(challengeResponse[0:], DESCrypt(zPasswordHash[0:7], challenge))
	copy(challengeResponse[8:], DESCrypt(zPasswordHash[7:14], challenge))
	copy(challengeResponse[16:], DESCrypt(zPasswordHash[14:21], challenge))

	return challengeResponse
}

// parityPadDESKey transforms a 7-octet key into an 8-octed one by
// adding a parity at every 8th bit position.
// See https://limbenjamin.com/articles/des-key-parity-bit-calculator.html
func parityPadDESKey(inBytes []byte) []byte {
	in := uint64(0)
	outBytes := make([]byte, 8)

	for i := 0; i < len(inBytes); i++ {
		offset := uint64(8 * (len(inBytes) - i - 1))
		in |= uint64(inBytes[i]) << offset
	}

	for i := 0; i < len(outBytes); i++ {
		offset := uint64(7 * (len(outBytes) - i - 1))
		outBytes[i] = byte(in>>offset) << 1

		if bits.OnesCount(uint(outBytes[i]))%2 == 0 {
			outBytes[i] |= 1
		}
	}

	return outBytes
}

// DESCrypt - rfc2759, 8.6
func DESCrypt(key, clear []byte) []byte {
	k := key
	if len(k) == 7 {
		k = parityPadDESKey(key)
	}

	cipher, err := des.NewCipher(k)
	if err != nil {
		panic(err)
	}

	b := make([]byte, 8)
	cipher.Encrypt(b, clear)

	return b
}
