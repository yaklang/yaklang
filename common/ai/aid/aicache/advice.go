package aicache

import (
	"fmt"
	"sort"
)

// dynamicSectionOversizeThreshold 是 dynamic 段告警阈值。
// 单次 prompt 的 dynamic 段超过该阈值时, advice 会提示 "dynamic 段过大", 引导
// 开发者把里面跨 turn 字节稳定的子标签 (PARENT_TASK / FACTS / DOCUMENT 等)
// 迁到 frozen-block 段以提升缓存命中率。
//
// 阈值参考 DashScope explicit cache 最低创建大小 (≈ 1024 tokens ≈ 4 KB),
// 上调到 8 KB 是为了避免对正常 reactive_data + injected_memory 误报。
//
// 关键词: dynamicSectionOversizeThreshold, 缓存阈值, advice
const dynamicSectionOversizeThreshold = 8 * 1024

// reusableAITagMinOccurrences 是 reusable_aitag_in_dynamic 告警的最小出现
// 次数门槛。低于该值时, 跨 turn 的样本量不够, 不报警以避免误报。
// 关键词: reusableAITagMinOccurrences
const reusableAITagMinOccurrences = 3

// buildAdvices 根据 HitReport 与切片结构推断缓存优化建议
// 这些建议是给开发者看的诊断字符串
//
// 关键词: aicache, buildAdvices, 测算建议
func buildAdvices(rep *HitReport, split *PromptSplit) []string {
	return buildAdvicesWithCache(rep, split, nil)
}

// buildAdvicesWithCache 在 buildAdvices 基础上, 当传入 globalCache 时多产出
// 一类跨 turn 状态相关的诊断 (例如 reusable_aitag_in_dynamic), 用于检测
// "body 字节稳定但 nonce 漂移" 的 RandStringBytes 反模式。
//
// gc 为 nil 时只输出与 (rep, split) 相关的无状态 advice。
//
// 关键词: aicache, buildAdvicesWithCache, 跨 turn 诊断, AITag 漂移
func buildAdvicesWithCache(rep *HitReport, split *PromptSplit, gc *globalCache) []string {
	if rep == nil || split == nil {
		return nil
	}
	var advices []string

	// 1. 单 raw chunk: 没有外层标签，无法做 section 级缓存
	if len(split.Chunks) == 1 && split.Chunks[0].Section == SectionRaw {
		advices = append(advices, "prompt has no PROMPT_SECTION wrapper; not eligible for section-level caching")
	}

	// 2. 切片数 < 4 时提示有些 section 缺失
	if len(split.Chunks) > 0 && len(split.Chunks) < 4 && split.Chunks[0].Section != SectionRaw {
		missing := missingSections(split)
		if len(missing) > 0 {
			advices = append(advices, fmt.Sprintf("only %d/4 sections present; missing: %v", len(split.Chunks), missing))
		}
	}

	// 3. 前缀完全未对齐
	if rep.RequestChunks > 1 && rep.PrefixHitChunks == 0 && rep.TotalRequests > 1 {
		advices = append(advices, "prefix not aligned at all; first section hash changed - check if static section template was polluted")
	}

	// 4. 段稳定性诊断
	// 用 reuse_rate = 1 - distinct/total 替代单纯 distinct 判定:
	//   distinct=10 但 total=55 (主 React loop 复用 34 次, 其余 forge 各 1-8 次)
	//   reuse_rate=82% -> 实际很稳定, 不应报 unstable.
	//   只有 distinct ≈ total (每次都换新 hash) 时才是真"污染".
	// 关键词: buildAdvices reuse_rate, high-static distinct 误报修复
	for section, distinct := range rep.SectionHashCount {
		total := rep.SectionTotalUses[section]
		if total <= 0 {
			total = distinct
		}
		// reuse_rate < 30% (大量新 hash 没复用) 才报 unstable; 同时要求 total > 3
		// 否则启动期 prompt 数 < 3 时易触发误报
		switch section {
		case SectionHighStatic:
			if total > 3 && distinct > 1 {
				reuseRate := 1.0 - float64(distinct)/float64(total)
				if reuseRate < 0.3 {
					advices = append(advices,
						fmt.Sprintf("high-static section unstable: %d distinct / %d total uses (reuse_rate=%.0f%%), template variables likely polluting it",
							distinct, total, reuseRate*100))
				}
			}
		case SectionSemiDynamic:
			if total > 5 && distinct > 3 {
				reuseRate := 1.0 - float64(distinct)/float64(total)
				if reuseRate < 0.4 {
					advices = append(advices,
						fmt.Sprintf("semi-dynamic section drifts more than expected: %d distinct / %d total uses (reuse_rate=%.0f%%), tool/forge/schema list may be churning",
							distinct, total, reuseRate*100))
				}
			}
		}
	}

	// 5.0 dynamic 段过大告警: 单次 prompt 的 dynamic chunk 字节超过阈值时, 提示
	// 开发者考虑把内部跨 turn 字节稳定的子 AITag (PARENT_TASK / FACTS /
	// DOCUMENT / CURRENT_TASK / INSTRUCTION 等) 迁到 frozen-block 段以提升
	// 缓存命中率。
	//
	// 关键词: advice, dynamic_section_oversized, frozen-block 迁移建议
	for _, ch := range split.Chunks {
		if ch == nil || ch.Section != SectionDynamic {
			continue
		}
		if ch.Bytes > dynamicSectionOversizeThreshold {
			advices = append(advices, fmt.Sprintf(
				"[dynamic_section_oversized] dynamic section is %d bytes (> %d threshold); "+
					"consider hoisting plan-scoped subtags (PARENT_TASK / FACTS / DOCUMENT / CURRENT_TASK / INSTRUCTION) into frozen-block",
				ch.Bytes, dynamicSectionOversizeThreshold,
			))
		}
	}

	// 5.1 reusable_aitag_in_dynamic: 跨 turn 检测 dynamic 段内"body 字节稳定
	// 但 nonce 漂移"的子 AITag, 是 RandStringBytes 反模式的典型表现。
	//
	// 仅在 gc 非 nil 且观测窗口已积累足够样本时输出, 避免初期误报。
	// 关键词: advice, reusable_aitag_in_dynamic, AITag 漂移, RandStringBytes
	if gc != nil {
		drifts := gc.GetReusableDynamicSubtagDrifts(reusableAITagMinOccurrences)
		// 按 BodyBytes 降序限制最多 5 条, 避免 advice 过于冗长
		if len(drifts) > 5 {
			sort.SliceStable(drifts, func(i, j int) bool {
				wi := drifts[i].BodyBytes * drifts[i].Occurrences
				wj := drifts[j].BodyBytes * drifts[j].Occurrences
				return wi > wj
			})
			drifts = drifts[:5]
		}
		for _, d := range drifts {
			advices = append(advices, fmt.Sprintf(
				"[reusable_aitag_in_dynamic] tag=%s body=%dB seen %dx with %d distinct nonces -> body-stable but nonce drifts (RandStringBytes anti-pattern); use stable nonce or hoist to frozen-block",
				d.TagName, d.BodyBytes, d.Occurrences, d.DistinctNonce,
			))
		}
	}

	// 5. 命中率定性
	if rep.RequestBytes > 0 && rep.TotalRequests > 1 {
		switch {
		case rep.PrefixHitRatio > 0.7:
			advices = append(advices, fmt.Sprintf("hit ratio good (%.1f%%)", rep.PrefixHitRatio*100))
		case rep.PrefixHitRatio >= 0.3:
			advices = append(advices, fmt.Sprintf("hit ratio fair (%.1f%%) - room for improvement", rep.PrefixHitRatio*100))
		default:
			if rep.PrefixHitRatio > 0 {
				advices = append(advices, fmt.Sprintf("hit ratio poor (%.1f%%) - prefix alignment broken early", rep.PrefixHitRatio*100))
			}
		}
	}

	return advices
}

// missingSections 返回当前切片中缺失的标准 section 列表。
//
// timeline 段同时识别 SectionTimeline 与 SectionTimelineOpen: 两者出现任一即认为
// "timeline 段已存在", 不会误报缺失。对应 aireact 新"按稳定性分层"路径下
// 老 timeline 段被拆为 frozen 块 (不算 timeline section) + timeline-open 段。
//
// 关键词: aicache, missingSections, timeline / timeline-open 等价识别
func missingSections(split *PromptSplit) []string {
	have := make(map[string]bool, len(split.Chunks))
	for _, ch := range split.Chunks {
		have[ch.Section] = true
	}
	hasTimeline := have[SectionTimeline] || have[SectionTimelineOpen]
	expected := []string{SectionHighStatic, SectionSemiDynamic, SectionTimeline, SectionDynamic}
	var missing []string
	for _, s := range expected {
		if s == SectionTimeline {
			if !hasTimeline {
				missing = append(missing, s)
			}
			continue
		}
		if !have[s] {
			missing = append(missing, s)
		}
	}
	return missing
}

// FirstAdvice 返回 advices 列表中的首条（用于节流单行打印）
// 关键词: aicache, FirstAdvice
func FirstAdvice(rep *HitReport) string {
	if rep == nil || len(rep.Advices) == 0 {
		return ""
	}
	return rep.Advices[0]
}
