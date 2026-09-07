package godzilla

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGodzillaPayloadParity 保证统一 payloads.tar.gz 还原出来的 payload 与重构前
// 内联在代码里的常量完全一致（期望值取自 main 分支上原始常量字节的 sha256）。
func TestGodzillaPayloadParity(t *testing.T) {
	cases := []struct {
		name     string
		hexValue string
		sum      string
		size     int
	}{
		{"payload2.class", JavaClassPayload, "c2ce7f673d5e70aed446ce6538746e51dc4bb5218efbda2d6bf346735ff36a32", 34807},
		{"payload.php", PhpCodePayload, "91237f7d538d291d6c09ce1327f8f2bf019d7d5556550e03792e54356925b2d9", 28358},
		{"payload.asp", AspCodePayload, "020a80744ecd9c640d09308c6df310bc7a5714e38e6bd7489eba8604d380de52", 16674},
		{"payload_aspx.bin", CsharpDllPayload, "cfcbb3014ecc560ba36103213b36fc62d6b0ef22c49067ff0d860fd7253a7c94", 20480},
	}
	for _, c := range cases {
		raw, err := hex.DecodeString(c.hexValue)
		assert.NoError(t, err, c.name)
		assert.Equal(t, c.sum, hashOf(raw), "payload %s mismatch", c.name)
		assert.Len(t, raw, c.size, c.name)
	}
}

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
