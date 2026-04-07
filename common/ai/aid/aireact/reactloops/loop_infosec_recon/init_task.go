package loop_infosec_recon

import (
	_ "embed"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

//go:embed embedded/js-static-extract-ai.yak
var embeddedJsStaticExtractAiScript string

//go:embed embedded/crawl-js-collector.yak
var embeddedCrawlJsCollectorScript string

const (
	keyWorkDir            = "infosec_workdir"
	keyPoolPath           = "infosec_pool_path"
	keySeedURL            = "infosec_seed_url"
	keyScopeHosts         = "infosec_scope_hosts"
	keyMaxCrawlDepth      = "infosec_max_crawl_depth"
	keyProbeConcurrency   = "infosec_probe_concurrency"
	keyLastReconSnippet   = "infosec_recon_log_tail"
	defaultCrawlDepth     = "2"
	defaultProbeConc      = "6"
)

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
	if _, err := yakit.GetAIYakTool(db, name); err == nil {
		return
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(name, script)
	if aiTool == nil {
		log.Warnf("infosec_recon: parse embedded %s script metadata failed", name)
		return
	}
	if _, err := yakit.SaveAIYakTool(db, aiTool); err != nil {
		log.Warnf("infosec_recon: register %s tool failed: %v", name, err)
		return
	}
	log.Infof("infosec_recon: auto-registered AI tool %s from embedded script source", name)
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
