package aicache

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
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
	// sectionTotalUses section -> 累计使用次数 (含重复, 用来配合 distinct 算
	// reuse_rate = 1 - distinct/total). distinct 大不一定 = "污染", 跨多个
	// 不同 forge 入口本来就会出现多个 high-static hash; 但每个入口内部稳定
	// 时, total >> distinct, reuse_rate 会很高, advice 不再误报.
	// 关键词: aicache, sectionTotalUses, reuse_rate
	sectionTotalUses map[string]int
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

	// dynamicSubtagSightings 跟踪 dynamic 段内嵌套子 AITag 的"body 是否被
	// 重复用到, 但 nonce 漂移 (无法被 prefix cache 命中)" 反模式。
	//
	// key 是 sha256(tag-name + "|" + body) 的 hex 表示; value 是一份 sighting
	// 记录, 含累计出现次数与该 body 见过的所有 distinct nonces。
	//
	// 注意只跟踪 dynamic 段内的子标签 (其他段的子标签在主 chunk hash 层面已经
	// 被覆盖), 防止数据爆炸。
	//
	// 关键词: dynamicSubtagSightings, AITag drift, dynamic 段反模式,
	//        prefix cache 诊断
	dynamicSubtagSightings map[string]*dynamicSubtagSighting
}

// dynamicSubtagSighting 是 dynamic 段内子 AITag (tag-name + body) 的全局观测.
// 关键词: dynamicSubtagSighting, AITag drift
type dynamicSubtagSighting struct {
	// TagName 是 AITag 名 (例如 "PARENT_TASK", "FACTS")
	TagName string
	// BodyBytes 是 body 字节长度
	BodyBytes int
	// Occurrences 是该 body 累计被用到的次数
	Occurrences int
	// Nonces 是该 body 见过的所有 distinct nonces 集合
	Nonces map[string]struct{}
}

// DynamicSubtagDrift 是给 advice / cli report 看的 dynamic 段子 AITag 漂移
// 项: body 重复出现 ≥ 2 次, 但 nonce 也 ≥ 2 个 (说明 body 字节稳定但 nonce
// 每次都换, 是典型 RandStringBytes 反模式).
//
// 关键词: DynamicSubtagDrift, AITag 漂移, 反模式
type DynamicSubtagDrift struct {
	TagName       string
	BodyBytes     int
	Occurrences   int
	DistinctNonce int
	BodyHash      string
}

// newGlobalCache 构造一个 globalCache 实例
// 关键词: aicache, newGlobalCache
func newGlobalCache(maxRequests int) *globalCache {
	if maxRequests <= 0 {
		maxRequests = defaultMaxRequests
	}
	return &globalCache{
		chunks:                 make(map[string]*ChunkInfo),
		sectionHashes:          make(map[string]map[string]struct{}),
		sectionTotalUses:       make(map[string]int),
		requests:               make([]*requestRecord, 0, maxRequests),
		maxRequests:            maxRequests,
		dynamicSubtagSightings: make(map[string]*dynamicSubtagSighting),
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
		// dynamic 段做"二级 AITag 跟踪", 用于 reusable_aitag_in_dynamic 告警.
		// 关键词: dynamic 段二级解析, AITag 漂移诊断
		if ch.Section == SectionDynamic {
			g.upsertDynamicSubtagsLocked(ch.Content)
		}
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
		SectionTotalUses:   g.snapshotSectionTotalUses(),
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
	// 累计 section 使用次数 (含重复) - 用来配合 distinct hash 算 reuse_rate
	// 关键词: upsertChunk sectionTotalUses, reuse_rate 数据源
	g.sectionTotalUses[ch.Section]++

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

// snapshotSectionTotalUses 拷贝当前 section -> 累计使用次数映射
// 与 snapshotSectionHashCount 配合, advice 用 distinct/total 判定真稳定性,
// 避免把"跨多个不同 forge 入口"误判为 high-static 污染.
// 关键词: aicache, snapshotSectionTotalUses
func (g *globalCache) snapshotSectionTotalUses() map[string]int {
	out := make(map[string]int, len(g.sectionTotalUses))
	for section, total := range g.sectionTotalUses {
		out[section] = total
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

// upsertDynamicSubtagsLocked 扫描 dynamic chunk 内容里的所有顶层 AITag 子块,
// 把 (tag-name, body, nonce) 三元组登记到 dynamicSubtagSightings 表。
//
// 调用方必须已持有 g.mu。
//
// 用一个自手写的扫描器跳过嵌套(只取顶层): 找到 <|TAG_<nonce>|> 起始, 寻
// 与之配对的 <|TAG_END_<nonce>|>, 抽取中间 body 为顶层块, 然后从 END 之后
// 继续。内层嵌套块被外层块"吞下", 不再单独登记 (顶层稳定即够诊断)。
//
// 跳过 tag-name 含有特殊字符或者 nonce 不是 word 字符的标签, 防止 raw
// markup 误识别。
//
// 关键词: upsertDynamicSubtagsLocked, dynamic 顶层子标签解析,
//        AITag 漂移检测, prefix cache 诊断
func (g *globalCache) upsertDynamicSubtagsLocked(dynamicContent string) {
	dynamicContent = strings.TrimSpace(dynamicContent)
	if dynamicContent == "" {
		return
	}
	for offset := 0; offset < len(dynamicContent); {
		startOffset := strings.Index(dynamicContent[offset:], "<|")
		if startOffset < 0 {
			break
		}
		start := offset + startOffset
		tagCloseOffset := strings.Index(dynamicContent[start:], "|>")
		if tagCloseOffset < 0 {
			break
		}
		tagClose := start + tagCloseOffset + 2
		tagName, nonce, ok := parseDynamicSubtagStartToken(dynamicContent[start+2 : tagClose-2])
		if !ok {
			offset = tagClose
			continue
		}
		endTag := "<|" + tagName + "_END_" + nonce + "|>"
		endOffset := strings.Index(dynamicContent[tagClose:], endTag)
		if endOffset < 0 {
			offset = tagClose
			continue
		}
		end := tagClose + endOffset
		body := strings.TrimSpace(dynamicContent[tagClose:end])
		if body != "" {
			key := dynamicSubtagBodyKey(tagName, body)
			s, has := g.dynamicSubtagSightings[key]
			if !has {
				s = &dynamicSubtagSighting{
					TagName:   tagName,
					BodyBytes: len(body),
					Nonces:    make(map[string]struct{}),
				}
				g.dynamicSubtagSightings[key] = s
			}
			s.Occurrences++
			if nonce != "" {
				s.Nonces[nonce] = struct{}{}
			}
		}
		offset = end + len(endTag)
	}
}

// parseDynamicSubtagStartToken 解析 "<|<token>|>" 中的 token (不含两侧 <| |>).
// 期望形式: TAG_NAME_<nonce> (TAG_NAME 可包含下划线, nonce 是 [A-Za-z0-9]+).
// 算法: 找最右侧的 "_", 之后部分若全是 word 字符 → 视为 nonce, 之前部分为
// tagName。
//
// 拒绝形式:
//   - 起始为 "TAG_END_..." (这是结束标签, 不应该误识别为开始)
//   - tagName / nonce 任一为空
//   - tagName 含非 [A-Z0-9_] 字符
//   - nonce 含非 [A-Za-z0-9] 字符
//
// 关键词: parseDynamicSubtagStartToken, AITag 起始解析, 安全判定
func parseDynamicSubtagStartToken(token string) (tagName string, nonce string, ok bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", "", false
	}
	idx := strings.LastIndex(token, "_")
	if idx <= 0 || idx == len(token)-1 {
		return "", "", false
	}
	tagName = token[:idx]
	nonce = token[idx+1:]
	if tagName == "" || nonce == "" {
		return "", "", false
	}
	if strings.HasSuffix(tagName, "_END") {
		return "", "", false
	}
	for _, r := range tagName {
		if !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '_' {
			return "", "", false
		}
	}
	for _, r := range nonce {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return "", "", false
		}
	}
	return tagName, nonce, true
}

// dynamicSubtagBodyKey 计算 (tag-name, body) 的稳定 key。
// 关键词: dynamicSubtagBodyKey, body 哈希
func dynamicSubtagBodyKey(tagName, body string) string {
	h := sha256.New()
	h.Write([]byte(tagName))
	h.Write([]byte("|"))
	h.Write([]byte(body))
	return hex.EncodeToString(h.Sum(nil))
}

// GetReusableDynamicSubtagDrifts 返回所有"body 多次出现但 nonce 漂移"的
// dynamic 段子 AITag, 按 (occurrences * bodyBytes) 降序排列, 优先暴露浪费
// 最多 token 的反模式。
//
// 触发条件:
//   - Occurrences >= minOccurrences (建议 ≥ 3, 避免初期误报)
//   - len(Nonces) >= 2 (body 复用但 nonce 每次都换)
//
// 关键词: GetReusableDynamicSubtagDrifts, AITag 漂移, advice 输入
func (g *globalCache) GetReusableDynamicSubtagDrifts(minOccurrences int) []DynamicSubtagDrift {
	if g == nil {
		return nil
	}
	if minOccurrences < 2 {
		minOccurrences = 2
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	var out []DynamicSubtagDrift
	for hash, s := range g.dynamicSubtagSightings {
		if s == nil || s.Occurrences < minOccurrences || len(s.Nonces) < 2 {
			continue
		}
		out = append(out, DynamicSubtagDrift{
			TagName:       s.TagName,
			BodyBytes:     s.BodyBytes,
			Occurrences:   s.Occurrences,
			DistinctNonce: len(s.Nonces),
			BodyHash:      hash,
		})
	}
	// 简单按 BodyBytes * Occurrences 降序排
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			ai := out[j-1].BodyBytes * out[j-1].Occurrences
			bj := out[j].BodyBytes * out[j].Occurrences
			if bj > ai {
				out[j-1], out[j] = out[j], out[j-1]
				continue
			}
			break
		}
	}
	return out
}
