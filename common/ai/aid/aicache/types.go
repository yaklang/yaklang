package aicache

import "time"

// SectionName 列举切片识别到的 section 类型
//
// SectionTimeline 与 SectionTimelineOpen 同时存在:
//   - SectionTimeline ("timeline"): 老路径 (liteforge / 部分老 caller) 仍使用
//     的合并 timeline 段, 同时承载 frozen reducer + interval + open last bucket。
//   - SectionTimelineOpen ("timeline-open"): aireact 新路径 "按稳定性分层" 拆分
//     后的 timeline 易变尾段 (仅含最末 interval 桶 + Current Time + Workspace +
//     midterm prefix)。frozen 部分被迁到 AI_CACHE_FROZEN 块中, 不再走 timeline
//     section 包装。
//
// SectionSemiDynamic / SectionSemiDynamic1 / SectionSemiDynamic2 同时存在:
//   - SectionSemiDynamic ("semi-dynamic"): 老路径 (liteforge / aireduce) 仍使用
//     的单一 semi-dynamic 段。
//   - SectionSemiDynamic1 ("semi-dynamic-1") / SectionSemiDynamic2 ("semi-dynamic-2"):
//     aireact 新路径 "按稳定性分层 + UI 信息密度" 拆分后的两块 semi 段, 分别
//     承载 SkillsContext + RecentToolsCache 与 TaskInstruction + Schema +
//     OutputExample, 由 hijacker 切成两条 user message (semi-1 不打 cc /
//     semi-2 打 cc, 合并算 prefix cache).
//
// 三种 semi 段都被 splitter 识别, advice 把它们等价计入 semi-dynamic 类别;
// SectionHashCount 分别独立计数便于命中率诊断。
//
// 两段都被 splitter 与 hijacker 识别为 "timeline 类" section, 从而 SectionHashCount
// 分别独立计数, 命中率分析能区分两条路径的稳定性。
//
// 关键词: aicache, section, 切片类型, timeline / timeline-open 双段,
//        semi-dynamic / semi-dynamic-1 / semi-dynamic-2 三段
const (
	SectionHighStatic    = "high-static"
	SectionSemiDynamic   = "semi-dynamic"
	SectionSemiDynamic1  = "semi-dynamic-1"
	SectionSemiDynamic2  = "semi-dynamic-2"
	SectionTimeline      = "timeline"
	SectionTimelineOpen  = "timeline-open"
	SectionDynamic       = "dynamic"
	SectionRaw           = "raw"
)

// Chunk 表示 prompt 切片后的一个最小单元
// 关键词: aicache, Chunk, 切片单元
type Chunk struct {
	// Section 是 section 类别，对应 PROMPT_SECTION_<section>
	Section string
	// Nonce 是该 chunk 的标识；动态段 == "dynamic_<inner-nonce>"，其余 == Section，raw == "raw"
	Nonce string
	// Bytes 是 Content 的字节长度
	Bytes int
	// Hash 是稳定哈希源 sha256(Section + "|" + Content) 的 hex 表示
	// 关键词: aicache, Hash, 稳定哈希
	Hash string
	// Content 是该 chunk 的语义内容
	// 仅在 DEBUG 落盘时使用，命中率统计只看 Hash
	Content string
}

// PromptSplit 是 Split 函数的返回结果
// 关键词: aicache, PromptSplit, 切片结果
type PromptSplit struct {
	// Original 是原始 prompt 字符串，仅 DEBUG 落盘时回写
	Original string
	// Chunks 按出现顺序排列
	Chunks []*Chunk
	// Bytes 是 Original 的字节长度
	Bytes int
}

// ChunkInfo 记录全局缓存表中单个 chunk 的统计数据
// 关键词: aicache, ChunkInfo, chunk 统计
type ChunkInfo struct {
	Hash      string
	Section   string
	Bytes     int
	FirstSeen time.Time
	LastSeen  time.Time
	HitCount  int64
}

// HitReport 是 Record 一次后返回给业务方的命中率报告
// 关键词: aicache, HitReport, 命中率报告
type HitReport struct {
	SeqId              int64
	Model              string
	GeneratedAt        time.Time
	RequestChunks      int
	RequestBytes       int
	PrefixHitChunks    int
	PrefixHitBytes     int
	PrefixHitRatio     float64
	GlobalUniqueChunks int
	GlobalCacheBytes   int
	TotalRequests      int64
	// SectionHashCount 描述当前全局每个 section 已经出现过几个不同的 hash 值
	// 例如 high-static -> 3 表示 high-static 段已经漂移 3 次
	// 关键词: aicache, SectionHashCount, 段稳定性
	SectionHashCount map[string]int
	// SectionTotalUses 描述当前全局每个 section 累计被 prompt 使用的次数 (含重复)
	// 与 SectionHashCount 配合, advice 用 reuse_rate = 1 - distinct/total 判定真稳定性,
	// 避免把"跨多个不同 forge 入口"误判为 high-static 污染.
	// 关键词: aicache, SectionTotalUses, reuse_rate
	SectionTotalUses map[string]int
	// Advices 由 advice.go 生成
	Advices []string
}

// Equal 比较两个 HitReport 关键字段是否一致，用于节流打印的去重判定
// SeqId / GeneratedAt 不参与比较（每次请求都不一样）
// 关键词: aicache, HitReport.Equal, 节流去重
func (r *HitReport) Equal(other *HitReport) bool {
	if r == nil || other == nil {
		return r == other
	}
	if r.Model != other.Model ||
		r.RequestChunks != other.RequestChunks ||
		r.RequestBytes != other.RequestBytes ||
		r.PrefixHitChunks != other.PrefixHitChunks ||
		r.PrefixHitBytes != other.PrefixHitBytes ||
		r.GlobalUniqueChunks != other.GlobalUniqueChunks ||
		r.GlobalCacheBytes != other.GlobalCacheBytes {
		return false
	}
	if len(r.SectionHashCount) != len(other.SectionHashCount) {
		return false
	}
	for k, v := range r.SectionHashCount {
		if other.SectionHashCount[k] != v {
			return false
		}
	}
	if len(r.SectionTotalUses) != len(other.SectionTotalUses) {
		return false
	}
	for k, v := range r.SectionTotalUses {
		if other.SectionTotalUses[k] != v {
			return false
		}
	}
	if len(r.Advices) != len(other.Advices) {
		return false
	}
	for i := range r.Advices {
		if r.Advices[i] != other.Advices[i] {
			return false
		}
	}
	return true
}
