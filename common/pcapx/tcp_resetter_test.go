package pcapx

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"testing"
)

func TestTCPReSetter(t *testing.T) {
	packet := `f02f4b09df5994d9b31db46a0800450000867fbd40002e0607309d9466c2c0a800862ee3d97b7c75fe74e7165bc68018002aca7b00000101080a6bb8aa7eeaee7e474f26a250c14ffa176111b4c149bfc80f3436038f849392c6463f4ac86ab3e345b60502fb5becd1d30a40925473657a9f40ef123d7d86e57ffb5691e6873d04b17b098dbd0f40f18c013350847c586d0b189b`
	raw, _ := codec.DecodeHex(packet)
	results, err := GenerateTCPRST(raw)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 2, len(results))
}
