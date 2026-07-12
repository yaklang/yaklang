package reactloops

import (
	"sort"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// intent_skill_catalog.go 实现「意图识别 → 最相关 SKILL 目录」写入.
//
// 需求 2: 意图识别和意图感知可以加载当前任务感兴趣的 SKILL 到会话目录, 只加载最相关 10 个.
// 默认 SKILL 目录隐藏 (需求 1), 意图识别命中后由 ApplyMatchedSkillsToCatalog 写入
// top-N 并开启目录可见. top-N 选择接入命中数反馈 (UserAIStats): 高命中 SKILL 优先入选,
// 让「重要反馈点」真正落到意图层.
//
// 关键词: ApplyMatchedSkillsToCatalog, 意图识别目录, top-N, 命中数排序

// ApplyMatchedSkillsToCatalog 把意图识别命中的 SKILL 写入 SkillsContextManager 的
// 「最相关目录」(默认隐藏). 内部按命中数反馈排序后截断到 MaxCatalogSkills, 并对每个
// 入选的 skill 记一次 intent_catalog 命中. fast / deep 两条意图路径都调用本函数.
//
// cfgFromRuntime 取 r.GetConfig() (AICallerConfigIf), 用于解析 *aicommon.Config 做命中统计.
// 传入 nil / 空 → 静默 no-op.
func ApplyMatchedSkillsToCatalog(loop *ReActLoop, cfgFromRuntime aicommon.AICallerConfigIf, matchedMetas []*aiskillloader.SkillMeta) {
	if loop == nil || len(matchedMetas) == 0 {
		return
	}
	mgr := loop.GetSkillsContextManager()
	if mgr == nil {
		return
	}

	// 去重.
	seen := make(map[string]bool, len(matchedMetas))
	deduped := make([]*aiskillloader.SkillMeta, 0, len(matchedMetas))
	for _, meta := range matchedMetas {
		if meta == nil || meta.Name == "" || seen[meta.Name] {
			continue
		}
		seen[meta.Name] = true
		deduped = append(deduped, meta)
	}
	if len(deduped) == 0 {
		return
	}

	// 按命中数反馈排序: 高命中 SKILL 优先 (重要反馈点落地). 失败/无 DB 时按原序兜底.
	ranked := rankSkillsByHitCount(deduped)

	// 截断到 MaxCatalogSkills (二次保护: SetCatalogSkills 内部也会截断).
	if max := aiskillloader.MaxCatalogSkills; max > 0 && len(ranked) > max {
		ranked = ranked[:max]
	}

	mgr.SetCatalogSkills(ranked)
	mgr.SetCatalogVisible(true)

	// 每个入选目录的 skill 记一次 intent_catalog 命中.
	var cfgConcrete *aicommon.Config
	if c, ok := cfgFromRuntime.(*aicommon.Config); ok {
		cfgConcrete = c
	}
	for _, meta := range ranked {
		aicommon.SubmitSkillHit(cfgConcrete, meta.Name, aicommon.StatsSourceSkillIntentCatalog)
	}
	log.Infof("intent catalog updated: %d relevant skills surfaced (visible)", len(ranked))
}

// rankSkillsByHitCount 按 UserAIStats 的 per-skill 命中数降序排序 (高命中优先).
// 无 DB / 查询失败时保持原序 (BM25/意图匹配顺序), 不影响功能.
func rankSkillsByHitCount(metas []*aiskillloader.SkillMeta) []*aiskillloader.SkillMeta {
	db := consts.GetGormProfileDatabase()
	if db == nil || len(metas) <= 1 {
		return metas
	}
	// 取 Top-N 命中数排序的 skill 名 (N = 候选数, 保证覆盖所有候选).
	topNames := yakit.TopEntitiesByHits(db, schema.AIStatsEntityTypeSkill, len(metas))
	if len(topNames) == 0 {
		return metas
	}
	rank := make(map[string]int, len(topNames))
	for i, name := range topNames {
		rank[name] = len(topNames) - i // 高命中 → 高 rank
	}
	sorted := append([]*aiskillloader.SkillMeta(nil), metas...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return rank[sorted[i].Name] > rank[sorted[j].Name]
	})
	return sorted
}
