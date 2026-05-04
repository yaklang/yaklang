package aicache

import "fmt"

// buildAdvices 根据 HitReport 与切片结构推断缓存优化建议
// 这些建议是给开发者看的诊断字符串，先写死，后续按真实数据再调
// 关键词: aicache, buildAdvices, 测算建议
func buildAdvices(rep *HitReport, split *PromptSplit) []string {
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
	for section, count := range rep.SectionHashCount {
		switch section {
		case SectionHighStatic:
			if count > 1 {
				advices = append(advices, fmt.Sprintf("high-static section unstable: %d distinct hashes seen, check template variables polluting it", count))
			}
		case SectionSemiDynamic:
			if count > 3 {
				advices = append(advices, fmt.Sprintf("semi-dynamic section drifts more than expected: %d distinct hashes, tool/forge/schema list may be churning", count))
			}
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
