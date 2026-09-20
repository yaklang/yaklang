package vncprobe

import "crypto/des"

// vncAuthResponse is RFC 6143 §7.2.2: DES-ECB of the 16-byte challenge
// with an 8-byte key made from the password, each key byte bit-reversed.
func vncAuthResponse(password string, challenge []byte) ([]byte, error) {
	if len(challenge) < 16 {
		return nil, ErrProtocolMismatch
	}
	key := make([]byte, 8)
	n := len(password)
	if n > 8 {
		n = 8
	}
	copy(key, password[:n])
	for i := range key {
		key[i] = reverseBits(key[i])
	}
	block, err := des.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 16)
	block.Encrypt(out[:8], challenge[:8])
	block.Encrypt(out[8:], challenge[8:])
	return out, nil
}

func reverseBits(b byte) byte {
	var out byte
	for i := 0; i < 8; i++ {
		out = (out << 1) | (b & 1)
		b >>= 1
	}
	return out
}
