package aitag

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAITAG_SPLIT(t *testing.T) {
	prompt := `
text block
<|AI_CACHE_STATIC_ccc1|>
static block
<|AI_CACHE_STATIC_END_ccc1|>
text block2
<|AI_CACHE_STATIC_ccc1|>
static3 block
<|AI_CACHE_STATIC_END_ccc1|>
text block3
text block4
`

	blocksResult, err := SplitViaTAG(prompt, "AI_CACHE_STATIC")
	if err != nil {
		t.Error(err)
		t.Fail()
	}
	blocks := blocksResult.GetOrderedBlocks()
	assert.Contains(t, blocks[0].Content, "text block")
	assert.Contains(t, blocks[1].Content, "static block")
	assert.Contains(t, blocks[2].Content, "text block2")
	assert.Contains(t, blocks[3].Content, "static3 block")
	assert.Contains(t, blocks[4].Content, "text block3")
	assert.Contains(t, blocks[4].Content, "text block4")
}
