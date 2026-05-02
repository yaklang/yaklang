package aitag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 关键词: aitag, SplitViaTAG, variadic 多tag名
func TestSplitViaTAG_VariadicMultiTag(t *testing.T) {
	prompt := `head text
<|AI_CACHE_STATIC_s1|>
static one
<|AI_CACHE_STATIC_END_s1|>
mid text
<|AI_CACHE_USER_u1|>
user one
<|AI_CACHE_USER_END_u1|>
between
<|AI_CACHE_STATIC_s2|>
static two
<|AI_CACHE_STATIC_END_s2|>
tail text
`

	res, err := SplitViaTAG(prompt, "AI_CACHE_STATIC", "AI_CACHE_USER")
	require.NoError(t, err)

	blocks := res.GetOrderedBlocks()
	require.Equal(t, 7, len(blocks))

	assert.Equal(t, BlockTypeText, blocks[0].Type)
	assert.Contains(t, blocks[0].Content, "head text")

	assert.Equal(t, BlockTypeTagged, blocks[1].Type)
	assert.Equal(t, "AI_CACHE_STATIC", blocks[1].TagName)
	assert.Equal(t, "s1", blocks[1].Nonce)
	assert.Equal(t, "static one", blocks[1].Content)

	assert.Equal(t, BlockTypeText, blocks[2].Type)
	assert.Contains(t, blocks[2].Content, "mid text")

	assert.Equal(t, BlockTypeTagged, blocks[3].Type)
	assert.Equal(t, "AI_CACHE_USER", blocks[3].TagName)
	assert.Equal(t, "u1", blocks[3].Nonce)
	assert.Equal(t, "user one", blocks[3].Content)

	assert.Equal(t, BlockTypeText, blocks[4].Type)
	assert.Contains(t, blocks[4].Content, "between")

	assert.Equal(t, BlockTypeTagged, blocks[5].Type)
	assert.Equal(t, "AI_CACHE_STATIC", blocks[5].TagName)
	assert.Equal(t, "s2", blocks[5].Nonce)
	assert.Equal(t, "static two", blocks[5].Content)

	assert.Equal(t, BlockTypeText, blocks[6].Type)
	assert.Contains(t, blocks[6].Content, "tail text")

	for i, b := range blocks {
		assert.Equal(t, i, b.Index, "block index should equal its position")
	}
}

// 关键词: aitag, SplitViaTAG, round-trip 还原
func TestSplitViaTAG_RoundTrip(t *testing.T) {
	prompt := `prefix
<|AI_CACHE_STATIC_x|>
inner content
<|AI_CACHE_STATIC_END_x|>
between
<|AI_CACHE_USER_y|>
inner2
<|AI_CACHE_USER_END_y|>
suffix
`

	res, err := SplitViaTAG(prompt, "AI_CACHE_STATIC", "AI_CACHE_USER")
	require.NoError(t, err)

	assert.Equal(t, prompt, res.String(), "concatenating Raw of all blocks should reproduce input")
}

// 关键词: aitag, SplitViaTAG, 仅文本无标签
func TestSplitViaTAG_OnlyText(t *testing.T) {
	prompt := "this prompt has no tag at all\nmultiple lines\n"

	res, err := SplitViaTAG(prompt, "AI_CACHE_STATIC")
	require.NoError(t, err)

	blocks := res.GetOrderedBlocks()
	require.Equal(t, 1, len(blocks))
	assert.Equal(t, BlockTypeText, blocks[0].Type)
	assert.Equal(t, prompt, blocks[0].Content)
	assert.Equal(t, prompt, res.String())
}

// 关键词: aitag, SplitViaTAG, 单一标签紧贴首尾
func TestSplitViaTAG_TagOnly(t *testing.T) {
	prompt := `<|AI_CACHE_STATIC_only|>
only one block
<|AI_CACHE_STATIC_END_only|>`

	res, err := SplitViaTAG(prompt, "AI_CACHE_STATIC")
	require.NoError(t, err)

	blocks := res.GetOrderedBlocks()
	require.Equal(t, 1, len(blocks))
	assert.Equal(t, BlockTypeTagged, blocks[0].Type)
	assert.Equal(t, "only one block", blocks[0].Content)
	assert.Equal(t, prompt, res.String())
}

// 关键词: aitag, SplitViaTAG, 缺失结束 tag 报错
func TestSplitViaTAG_MissingEndTag(t *testing.T) {
	prompt := `head
<|AI_CACHE_STATIC_z|>
no closing here
`

	res, err := SplitViaTAG(prompt, "AI_CACHE_STATIC")
	assert.Error(t, err)
	assert.Nil(t, res)
}

// 关键词: aitag, SplitViaTAG, 空 tag 名报错
func TestSplitViaTAG_EmptyTagNames(t *testing.T) {
	_, err := SplitViaTAG("anything")
	assert.Error(t, err)

	_, err = SplitViaTAG("anything", "")
	assert.Error(t, err)
}

// 关键词: aitag, SplitViaTAG, 未匹配标签视为文本
func TestSplitViaTAG_UnacceptedTagStaysText(t *testing.T) {
	prompt := `before
<|OTHER_TAG_xx|>
should be treated as part of text
<|OTHER_TAG_END_xx|>
after
<|AI_CACHE_STATIC_kept|>
kept content
<|AI_CACHE_STATIC_END_kept|>
end
`

	res, err := SplitViaTAG(prompt, "AI_CACHE_STATIC")
	require.NoError(t, err)

	blocks := res.GetOrderedBlocks()
	// 期望: 文本块(包含 OTHER_TAG 整段) + tagged + 文本块
	require.Equal(t, 3, len(blocks))
	assert.Equal(t, BlockTypeText, blocks[0].Type)
	assert.Contains(t, blocks[0].Content, "<|OTHER_TAG_xx|>")
	assert.Contains(t, blocks[0].Content, "<|OTHER_TAG_END_xx|>")
	assert.Equal(t, BlockTypeTagged, blocks[1].Type)
	assert.Equal(t, "kept content", blocks[1].Content)
	assert.Equal(t, BlockTypeText, blocks[2].Type)
	assert.Contains(t, blocks[2].Content, "end")

	assert.Equal(t, prompt, res.String(), "round-trip must hold even when other tags exist")
}

// 关键词: aitag, SplitResult, 过滤器访问器
func TestSplitViaTAG_Accessors(t *testing.T) {
	prompt := `t1
<|AI_CACHE_STATIC_a|>
A1
<|AI_CACHE_STATIC_END_a|>
t2
<|AI_CACHE_USER_b|>
B1
<|AI_CACHE_USER_END_b|>
t3
<|AI_CACHE_STATIC_a|>
A2
<|AI_CACHE_STATIC_END_a|>
t4
`

	res, err := SplitViaTAG(prompt, "AI_CACHE_STATIC", "AI_CACHE_USER")
	require.NoError(t, err)

	textBlocks := res.GetTextBlocks()
	taggedBlocks := res.GetTaggedBlocks()
	assert.Equal(t, 4, len(textBlocks))
	assert.Equal(t, 3, len(taggedBlocks))

	staticBlocks := res.GetBlocksByTagName("AI_CACHE_STATIC")
	require.Equal(t, 2, len(staticBlocks))
	assert.Equal(t, "A1", staticBlocks[0].Content)
	assert.Equal(t, "A2", staticBlocks[1].Content)

	userBlocks := res.GetBlocksByTagName("AI_CACHE_USER")
	require.Equal(t, 1, len(userBlocks))
	assert.Equal(t, "B1", userBlocks[0].Content)

	nonceA := res.GetBlocksByNonce("a")
	require.Equal(t, 2, len(nonceA))
	assert.Equal(t, "A1", nonceA[0].Content)
	assert.Equal(t, "A2", nonceA[1].Content)

	nonceB := res.GetBlocksByNonce("b")
	require.Equal(t, 1, len(nonceB))
	assert.Equal(t, "B1", nonceB[0].Content)

	nonceMissing := res.GetBlocksByNonce("does-not-exist")
	assert.Equal(t, 0, len(nonceMissing))

	assert.Equal(t, 7, res.Len())
}

// 关键词: aitag, Block, Render 重新包裹
func TestBlock_Render(t *testing.T) {
	textBlk := &Block{Type: BlockTypeText, Content: "hello text", Raw: "hello text"}
	assert.Equal(t, "hello text", textBlk.Render())

	taggedBlk := &Block{
		Type:    BlockTypeTagged,
		Content: "wrapped content",
		TagName: "AI_CACHE_STATIC",
		Nonce:   "n1",
	}
	expected := "<|AI_CACHE_STATIC_n1|>\nwrapped content\n<|AI_CACHE_STATIC_END_n1|>"
	assert.Equal(t, expected, taggedBlk.Render())
}

// 关键词: aitag, Block, IsText IsTagged
func TestBlock_TypeHelpers(t *testing.T) {
	text := &Block{Type: BlockTypeText}
	tagged := &Block{Type: BlockTypeTagged}

	assert.True(t, text.IsText())
	assert.False(t, text.IsTagged())
	assert.True(t, tagged.IsTagged())
	assert.False(t, tagged.IsText())

	var nilBlk *Block
	assert.False(t, nilBlk.IsText())
	assert.False(t, nilBlk.IsTagged())
}

// 关键词: aitag, BlockType, String
func TestBlockType_String(t *testing.T) {
	assert.Equal(t, "text", BlockTypeText.String())
	assert.Equal(t, "tagged", BlockTypeTagged.String())
	assert.Equal(t, "unknown", BlockType(99).String())
}

// 关键词: aitag, SplitResult, nil 安全
func TestSplitResult_NilSafe(t *testing.T) {
	var nilRes *SplitResult
	assert.Nil(t, nilRes.GetOrderedBlocks())
	assert.Nil(t, nilRes.GetTextBlocks())
	assert.Nil(t, nilRes.GetTaggedBlocks())
	assert.Nil(t, nilRes.GetBlocksByTagName("x"))
	assert.Nil(t, nilRes.GetBlocksByNonce("x"))
	assert.Equal(t, "", nilRes.String())
	assert.Equal(t, 0, nilRes.Len())
}

// 关键词: aitag, SplitViaTAG, 行内格式
func TestSplitViaTAG_InlineFormat(t *testing.T) {
	prompt := `prefix <|AI_CACHE_STATIC_inline|>inline body<|AI_CACHE_STATIC_END_inline|> suffix`

	res, err := SplitViaTAG(prompt, "AI_CACHE_STATIC")
	require.NoError(t, err)

	blocks := res.GetOrderedBlocks()
	require.Equal(t, 3, len(blocks))
	assert.Equal(t, "prefix ", blocks[0].Content)
	assert.Equal(t, BlockTypeTagged, blocks[1].Type)
	assert.Equal(t, "inline body", blocks[1].Content)
	assert.Equal(t, " suffix", blocks[2].Content)
	assert.Equal(t, prompt, res.String())
}
