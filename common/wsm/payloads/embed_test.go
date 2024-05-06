package payloads

import (
	"encoding/hex"
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

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
