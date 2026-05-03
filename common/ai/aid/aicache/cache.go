package aicache

import (
	"sync"
	"sync/atomic"
	"time"
)

// defaultMaxRequests 是环形历史保留的最大请求条数
// 关键词: aicache, defaultMaxRequests, 环形历史长度
const defaultMaxRequests = 256

// requestRecord 是一次镜像观测的快照
// 关键词: aicache, requestRecord, 历史请求
type requestRecord struct {
	seq    int64
	hashes []string
	bytes  []int
	model  string
	at     time.Time
}

// globalCache 是 aicache 的全局缓存表与历史窗口
// 关键词: aicache, globalCache, 全局缓存
type globalCache struct {
	mu sync.Mutex
	// chunks 按 Hash 索引所有出现过的 chunk
	chunks map[string]*ChunkInfo
	// sectionHashes section -> set(hash) 用于统计每个 section 的稳定性
	sectionHashes map[string]map[string]struct{}
	// requests 是环形历史
	requests   []*requestRecord
	requestPos int
	// totalRequests 累计请求数（不受窗口限制）
	totalRequests int64
	// seqCounter 单调递增的请求序号
	seqCounter atomic.Int64
	// maxRequests 历史窗口大小
	maxRequests int
	// totalCacheBytes 全局所有不同 chunk 的字节总和
	totalCacheBytes int
}

// newGlobalCache 构造一个 globalCache 实例
// 关键词: aicache, newGlobalCache
func newGlobalCache(maxRequests int) *globalCache {
	if maxRequests <= 0 {
		maxRequests = defaultMaxRequests
	}
	return &globalCache{
		chunks:        make(map[string]*ChunkInfo),
		sectionHashes: make(map[string]map[string]struct{}),
		requests:      make([]*requestRecord, 0, maxRequests),
		maxRequests:   maxRequests,
	}
}

// Record 把一次切片结果登记到全局缓存表，并计算 LCP 命中信息
// 关键词: aicache, Record, 命中率统计
func (g *globalCache) Record(split *PromptSplit, model string) *HitReport {
	if g == nil || split == nil {
		return &HitReport{GeneratedAt: time.Now()}
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	seq := g.seqCounter.Add(1)
	g.totalRequests++

	curHashes := make([]string, len(split.Chunks))
	curBytes := make([]int, len(split.Chunks))
	requestBytes := 0
	for i, ch := range split.Chunks {
		curHashes[i] = ch.Hash
		curBytes[i] = ch.Bytes
		requestBytes += ch.Bytes
		g.upsertChunk(ch, now)
	}

	// LCP: 与所有历史请求做最长公共前缀比对
	bestLcp := 0
	for _, prev := range g.requests {
		if prev == nil {
			continue
		}
		lcp := commonPrefixLen(curHashes, prev.hashes)
		if lcp > bestLcp {
			bestLcp = lcp
		}
	}

	hitBytes := 0
	for i := 0; i < bestLcp; i++ {
		hitBytes += curBytes[i]
	}

	rep := &HitReport{
		SeqId:              seq,
		Model:              model,
		GeneratedAt:        now,
		RequestChunks:      len(split.Chunks),
		RequestBytes:       requestBytes,
		PrefixHitChunks:    bestLcp,
		PrefixHitBytes:     hitBytes,
		PrefixHitRatio:     ratio(hitBytes, requestBytes),
		GlobalUniqueChunks: len(g.chunks),
		GlobalCacheBytes:   g.totalCacheBytes,
		TotalRequests:      g.totalRequests,
		SectionHashCount:   g.snapshotSectionHashCount(),
	}

	// 写入新 record，环形覆盖
	rec := &requestRecord{
		seq:    seq,
		hashes: curHashes,
		bytes:  curBytes,
		model:  model,
		at:     now,
	}
	if len(g.requests) < g.maxRequests {
		g.requests = append(g.requests, rec)
	} else {
		g.requests[g.requestPos] = rec
		g.requestPos = (g.requestPos + 1) % g.maxRequests
	}

	return rep
}

// upsertChunk 在 chunks 表中登记或更新一个 chunk
// 关键词: aicache, upsertChunk
func (g *globalCache) upsertChunk(ch *Chunk, now time.Time) {
	if ch == nil {
		return
	}
	info, ok := g.chunks[ch.Hash]
	if !ok {
		info = &ChunkInfo{
			Hash:      ch.Hash,
			Section:   ch.Section,
			Bytes:     ch.Bytes,
			FirstSeen: now,
			LastSeen:  now,
			HitCount:  1,
		}
		g.chunks[ch.Hash] = info
		g.totalCacheBytes += ch.Bytes
		set, ok := g.sectionHashes[ch.Section]
		if !ok {
			set = make(map[string]struct{})
			g.sectionHashes[ch.Section] = set
		}
		set[ch.Hash] = struct{}{}
		return
	}
	info.LastSeen = now
	info.HitCount++
}

// snapshotSectionHashCount 拷贝当前 section -> hash 数量映射
// 关键词: aicache, snapshotSectionHashCount, 段稳定性快照
func (g *globalCache) snapshotSectionHashCount() map[string]int {
	out := make(map[string]int, len(g.sectionHashes))
	for section, set := range g.sectionHashes {
		out[section] = len(set)
	}
	return out
}

// ChunkInfoByHash 返回某个 hash 对应的 chunk 统计信息（拷贝）
// 关键词: aicache, ChunkInfoByHash, 查询接口
func (g *globalCache) ChunkInfoByHash(hash string) *ChunkInfo {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	info, ok := g.chunks[hash]
	if !ok {
		return nil
	}
	cp := *info
	return &cp
}

// commonPrefixLen 返回两个 hash 序列的最长公共前缀长度
// 关键词: aicache, commonPrefixLen, LCP
func commonPrefixLen(a, b []string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// ratio 安全计算 hit / total，避免除零
// 关键词: aicache, ratio
func ratio(hit, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(hit) / float64(total)
}
