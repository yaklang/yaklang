package payloads

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCshrapPayloadRestoredFromEncryptedResource(t *testing.T) {
	sum := sha256.Sum256(CshrapPayload)
	assert.Equal(t, "e114f5318fa035c3233a0029b8fbdef129dc64a5d89f389143c8098c43369def", hex.EncodeToString(sum[:]))
	assert.Len(t, CshrapPayload, 25600)
}

// TestUnifiedPayloadFS 检查四个明文目录都被打进同一个压缩包，并且按源码路径可读。
func TestUnifiedPayloadFS(t *testing.T) {
	for _, dir := range []string{behinderStaticDir, yakshellStaticDir, yakshellEncryptDir, godzillaStaticDir} {
		entries, err := FS.ReadDir(dir)
		assert.NoError(t, err, dir)
		assert.NotEmpty(t, entries, dir)
	}

	assert.NotEmpty(t, HexPayload)
	assert.NotEmpty(t, EncryptPayload)
}

func TestGetYakShellPayloads(t *testing.T) {
	payload, err := GetHexYakPayload("AllPayloadGo.php")
	if err != nil {
		panic(err)
	}
	decodeString, err := hex.DecodeString(string(payload))
	if err != nil {
		panic(err)
	}
	assert.True(t, strings.Contains(string(decodeString), "AllPayloadGo"))
}
