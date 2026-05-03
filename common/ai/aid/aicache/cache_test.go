package aicache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 关键词: aicache, cache, 首次请求无命中
func TestCache_FirstRequestNoHit(t *testing.T) {
	gc := newGlobalCache(8)
	prompt := buildFourSectionPrompt("n1", "q1", "tools-1", "static-1", "timeline-1", "memory-1")
	split := Split(prompt)

	rep := gc.Record(split, "test-model")
	require.NotNil(t, rep)

	assert.EqualValues(t, 1, rep.SeqId)
	assert.Equal(t, 4, rep.RequestChunks)
	assert.Equal(t, 0, rep.PrefixHitChunks)
	assert.Equal(t, 0, rep.PrefixHitBytes)
	assert.InDelta(t, 0.0, rep.PrefixHitRatio, 0.0001)
	assert.Equal(t, 4, rep.GlobalUniqueChunks)
	assert.EqualValues(t, 1, rep.TotalRequests)
	assert.Equal(t, "test-model", rep.Model)
}

// 关键词: aicache, cache, 完全相同 prompt 全命中
func TestCache_FullPrefixHit(t *testing.T) {
	gc := newGlobalCache(8)
	prompt := buildFourSectionPrompt("n1", "q1", "tools-1", "static-1", "timeline-1", "memory-1")

	_ = gc.Record(Split(prompt), "m")
	rep := gc.Record(Split(prompt), "m")

	assert.Equal(t, 4, rep.PrefixHitChunks)
	assert.Equal(t, rep.RequestBytes, rep.PrefixHitBytes)
	assert.InDelta(t, 1.0, rep.PrefixHitRatio, 0.0001)
	assert.Equal(t, 4, rep.GlobalUniqueChunks, "no new chunks should be registered")
}

// 关键词: aicache, cache, 部分前缀命中
func TestCache_PartialPrefixHit(t *testing.T) {
	gc := newGlobalCache(8)

	// 第 1 次: A B C D
	p1 := buildFourSectionPrompt("n1", "q1", "tools-1", "static-A", "timeline-A", "memory-1")
	_ = gc.Record(Split(p1), "m")

	// 第 2 次: A B C' D'  (只前 2 段一致)
	p2 := buildFourSectionPrompt("n2", "q2", "tools-1", "static-A", "timeline-DIFF", "memory-2")
	rep := gc.Record(Split(p2), "m")

	assert.Equal(t, 2, rep.PrefixHitChunks, "prefix hit should be 2 (high-static + semi-dynamic)")
	assert.Greater(t, rep.PrefixHitBytes, 0)
	assert.Less(t, rep.PrefixHitBytes, rep.RequestBytes)
}

// 关键词: aicache, cache, section_hash_count
func TestCache_SectionHashCount(t *testing.T) {
	gc := newGlobalCache(8)

	// 同一份 high-static 出现 2 次，但 timeline 漂移 2 次
	p1 := buildFourSectionPrompt("n1", "q1", "tools", "static-A", "timeline-1", "mem")
	p2 := buildFourSectionPrompt("n2", "q2", "tools", "static-A", "timeline-2", "mem")
	_ = gc.Record(Split(p1), "m")
	rep := gc.Record(Split(p2), "m")

	assert.Equal(t, 1, rep.SectionHashCount[SectionHighStatic])
	assert.Equal(t, 2, rep.SectionHashCount[SectionTimeline])
}

// 关键词: aicache, cache, 环形历史
func TestCache_RingBufferEviction(t *testing.T) {
	gc := newGlobalCache(2)

	for i := 0; i < 5; i++ {
		p := buildFourSectionPrompt("n", "q", "tools", "static", "timeline-"+string(rune('A'+i)), "mem")
		_ = gc.Record(Split(p), "m")
	}

	assert.EqualValues(t, 5, gc.totalRequests)
	assert.LessOrEqual(t, len(gc.requests), 2, "ring buffer must respect maxRequests")
}

// 关键词: aicache, cache, raw 单 chunk
func TestCache_RawChunk(t *testing.T) {
	gc := newGlobalCache(4)
	rep := gc.Record(Split("plain text without tag"), "m")
	require.Equal(t, 1, rep.RequestChunks)
	assert.Equal(t, 0, rep.PrefixHitChunks, "first occurrence should not hit")

	rep2 := gc.Record(Split("plain text without tag"), "m")
	assert.Equal(t, 1, rep2.PrefixHitChunks, "second identical raw prompt should fully hit")
}

// 关键词: aicache, ChunkInfoByHash
func TestChunkInfoByHash(t *testing.T) {
	gc := newGlobalCache(4)
	p := buildFourSectionPrompt("n", "q", "tools", "static", "timeline", "mem")
	split := Split(p)
	_ = gc.Record(split, "m")

	for _, ch := range split.Chunks {
		info := gc.ChunkInfoByHash(ch.Hash)
		require.NotNil(t, info, "chunk hash %s should be registered", ch.Section)
		assert.Equal(t, ch.Section, info.Section)
		assert.Equal(t, ch.Bytes, info.Bytes)
		assert.False(t, info.FirstSeen.IsZero())
	}
}
