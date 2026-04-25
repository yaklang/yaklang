package loop_infosec_recon

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

//go:embed embedded/js-static-extract-ai.yak
var embeddedJsStaticExtractAiScript string

//go:embed embedded/crawl-js-collector.yak
var embeddedCrawlJsCollectorScript string

const (
	keyWorkDir          = "infosec_workdir"
	keyPoolPath         = "infosec_pool_path"
	keySeedURL          = "infosec_seed_url"
	keyScopeHosts       = "infosec_scope_hosts"
	keyMaxCrawlDepth    = "infosec_max_crawl_depth"
	keyProbeConcurrency = "infosec_probe_concurrency"
	keyLastReconSnippet = "infosec_recon_log_tail"
	defaultCrawlDepth   = "2"
	defaultProbeConc    = "6"
)

// Markers for idempotent merge of infosec_recon-specific interval-review (progress audit) extra prompts.
// If already present in ToolCallIntervalReviewExtraPrompt, we do not append again.
const (
	markerIntervalReviewCrawlJs  = "<!-- infosec_recon_interval_review:crawl_js_collector -->"
	markerIntervalReviewJsStaticAI = "<!-- infosec_recon_interval_review:js_static_extract_ai -->"
)

const intervalReviewBlockCrawlJs = `### ` + ToolCrawlJsCollector + `（长耗时 / 任务强依赖）
- 该工具通常持续 **数分钟到数十分钟**；输出稀疏、长时间无新日志在多数情况下仍属正常，**不要**仅因「无输出」「耗时长」或「看似卡住」就选择 cancel。
- **除非**在 stdout/stderr 中出现 **明确错误、异常、fatal、panic、失败栈** 等表明已失败的内容，否则必须 **continue**，**不要**终止工具调用。
- 本工具为信息搜集/API 发现流程的强依赖步骤；被 interval review 提前取消会导致本轮 recon **任务失败**。仅在日志出现明显报错或确认不可恢复时再考虑 cancel。`

const intervalReviewBlockJsStaticAI = `### ` + ToolJsStaticExtractAI + `（长耗时 / 任务强依赖）
- 该工具通常持续 **数分钟到数十分钟**；静态分析与批量处理阶段输出可能较少，**不要**仅因「无新输出」就判定卡死并 cancel。
- **除非**在 stdout/stderr 中出现 **明确错误、异常、fatal、panic、失败信息** 等，否则必须 **continue**，**不要**终止工具调用。
- 本工具为 API 抽取与池合并的强依赖步骤；被提前取消会导致本轮 recon **任务失败**。仅在日志出现明显报错或确认不可恢复时再考虑 cancel。`

// mergeInfosecLongRunningToolIntervalReviewExtraPrompt appends tool-specific interval-review guidance for
// crawl_js_collector and js_static_extract_ai when missing. It preserves any existing extra prompt text.
func mergeInfosecLongRunningToolIntervalReviewExtraPrompt(cfg *aicommon.Config) {
	if cfg == nil {
		return
	}
	current := strings.TrimSpace(cfg.GetToolCallIntervalReviewExtraPrompt())
	if current == "" {
		current = strings.TrimSpace(cfg.GetConfigString(aicommon.ConfigKeyToolCallIntervalReviewExtraPrompt))
	}
	out := current
	if !strings.Contains(out, markerIntervalReviewCrawlJs) {
		out = appendIntervalReviewExtraBlock(out, markerIntervalReviewCrawlJs, intervalReviewBlockCrawlJs)
	}
	if !strings.Contains(out, markerIntervalReviewJsStaticAI) {
		out = appendIntervalReviewExtraBlock(out, markerIntervalReviewJsStaticAI, intervalReviewBlockJsStaticAI)
	}
	if out == current {
		return
	}
	if err := aicommon.WithToolCallIntervalReviewExtraPrompt(out)(cfg); err != nil {
		log.Warnf("infosec_recon: apply ToolCallIntervalReviewExtraPrompt: %v", err)
	}
}

func appendIntervalReviewExtraBlock(base, marker, body string) string {
	block := strings.TrimSpace(marker) + "\n" + strings.TrimSpace(body)
	if strings.TrimSpace(base) == "" {
		return block
	}
	return strings.TrimSpace(base) + "\n\n" + block
}

func workDirFromInvoker(r aicommon.AIInvokeRuntime) string {
	cfg := r.GetConfig()
	if c, ok := cfg.(interface{ GetOrCreateWorkDir() string }); ok {
		return c.GetOrCreateWorkDir()
	}
	return ""
}

func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		wd := workDirFromInvoker(r)
		if wd == "" {
			log.Warnf("infosec_recon: workdir empty, pool path may be invalid")
		}
		loop.Set(keyWorkDir, wd)
		poolPath := filepath.Join(wd, poolFileName)
		loop.Set(keyPoolPath, poolPath)
		loop.Set(keyMaxCrawlDepth, defaultCrawlDepth)
		loop.Set(keyProbeConcurrency, defaultProbeConc)

		if _, err := LoadAPIPool(wd); err != nil {
			log.Warnf("infosec_recon: load pool: %v", err)
		}
		if err := ensurePoolFile(wd); err != nil {
			log.Warnf("infosec_recon: init pool file: %v", err)
		}

		embeddedInfosecYakTools()

		if c, ok := r.GetConfig().(*aicommon.Config); ok {
			mergeInfosecLongRunningToolIntervalReviewExtraPrompt(c)
		}

		r.AddToTimeline("infosec_recon_init", "API surface recon loop ready. recon_register_seed → "+ToolCrawlJsCollector+" (optional deep_js) → "+ToolJsStaticExtractAI+"(paths / verified JS dir) → api_pool_merge / probe_api_candidates as needed.")
		operator.Continue()
	}
}

func ensureEmbeddedAIYakTool(name, script string) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		log.Warnf("infosec_recon: cannot get profile database, skip %s tool registration", name)
		return
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(name, script)
	if aiTool == nil {
		log.Warnf("infosec_recon: parse embedded %s script metadata failed", name)
		return
	}
	aiTool.Author = schema.AIResourceAuthorBuiltin
	aiTool.IsBuiltin = true
	if _, err := yakit.SaveAIYakTool(db, aiTool); err != nil {
		log.Warnf("infosec_recon: register/update %s tool failed: %v", name, err)
		return
	}
	log.Infof("infosec_recon: synced AI tool %s from embedded script source", name)
}

func embeddedInfosecYakTools() {
	ensureEmbeddedAIYakTool(ToolJsStaticExtractAI, embeddedJsStaticExtractAiScript)
	ensureEmbeddedAIYakTool(ToolCrawlJsCollector, embeddedCrawlJsCollectorScript)
}

func ensurePoolFile(workDir string) error {
	if workDir == "" {
		return nil
	}
	path := filepath.Join(workDir, poolFileName)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	p := &APIPool{Version: poolFormatVersion, Entries: []APIPoolEntry{}}
	return SaveAPIPool(workDir, p)
}
