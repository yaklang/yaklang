package loop_infosec_recon

import (
	"bytes"
	_ "embed"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/persistent_instruction.txt
var persistentInstruction string

//go:embed prompts/reflection_output_example.txt
var reflectionOutputExample string

//go:embed prompts/reactive_data.txt
var reactiveDataTemplate string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_INFOSEC_RECON,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			allowed := []string{
				schema.AI_REACT_LOOP_ACTION_DIRECTLY_ANSWER,
				"finish",
				schema.AI_REACT_LOOP_ACTION_KNOWLEDGE_ENHANCE,
				schema.AI_REACT_LOOP_ACTION_SEARCH_CAPABILITIES,
				schema.AI_REACT_LOOP_ACTION_LOAD_CAPABILITY,
				schema.AI_REACT_LOOP_ACTION_LOADING_SKILLS,
				schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES,
				schema.AI_REACT_LOOP_ACTION_CHANGE_SKILL_VIEW_OFFSET,
				"recon_register_seed",
				"api_pool_merge",
				"crawl-js-collector",
				"js-static-extract-ai",
				"probe_api_candidates",
				"web_search",
				"scan_port",
				"simple_crawler",
				"banner_grab",
				"dig",
				"do_http_request",
				"batch_do_http_request",
				"read_file",
				"find_files",
				"grep_text",
				"url_content_summary",
				"subdomain_scan",
				"network_space_search",
				"search_knowledge",
			}
			if r.GetConfig().GetAllowUserInteraction() {
				allowed = append(allowed, schema.AI_REACT_LOOP_ACTION_ASK_FOR_CLARIFICATION)
			}

			maxIter := int(r.GetConfig().GetMaxIterationCount())
			if maxIter < 16 {
				maxIter = 16
			}

			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(false),
				reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(maxIter),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
					for _, name := range allowed {
						if action.ActionType == name {
							return true
						}
					}
					return false
				}),
				reactloops.WithPersistentInstruction(persistentInstruction),
				reactloops.WithReflectionOutputExample(reflectionOutputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					wd := loop.Get(keyWorkDir)
					if wd == "" {
						wd = workDirFromInvoker(r)
					}
					pool, _ := LoadAPIPool(wd)
					tot, ver, unver, bySrc := PoolStats(pool)
					var srcParts []string
					for k, v := range bySrc {
						srcParts = append(srcParts, k+":"+utils.InterfaceToString(v))
					}
					reconLog := loop.Get(keyReconLog)
					if len(reconLog) > 6000 {
						reconLog = reconLog[len(reconLog)-6000:]
					}
					renderMap := map[string]any{
						"Nonce":            nonce,
						"SeedURL":          loop.Get(keySeedURL),
						"ScopeHosts":       loop.Get(keyScopeHosts),
						"WorkDir":          wd,
						"PoolTotal":        tot,
						"PoolVerified":     ver,
						"PoolUnverified":   unver,
						"PoolBySource":     strings.Join(srcParts, ", "),
						"EnhanceData":      utils.ShrinkString(loop.Get(keyInfosecEnhanceData), 4000),
						"ReconLogTail":     reconLog,
						"FeedbackMessages": strings.TrimSpace(feedbacker.String()),
					}
					return utils.RenderTemplate(reactiveDataTemplate, renderMap)
				}),
				registerSeedAction(r),
				apiPoolMergeAction(r),
				crawlJsCollectorAction(r),
				runJsStaticAnalysisAction(r),
				probeAPICandidatesAction(r),
				searchKnowledgeInfosec(r),
				webSearchAction(r),
				scanPortAction(r),
				simpleCrawlerAction(r),
				bannerGrabAction(r),
				digAction(r),
				subdomainScanAction(r),
				networkSpaceAction(r),
				readFileAction(r),
				findFilesAction(r),
				grepTextAction(r),
				doHTTPAction(r),
				batchHTTPAction(r),
				urlSummaryAction(r),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_INFOSEC_RECON, r, preset...)
		},
		reactloops.WithLoopDescription("Focused information gathering and API endpoint discovery for authorized penetration tests. "+
			"Merges candidates into a shared on-disk pool, supports JS static extraction via yak script, and optional HTTP probing."),
		reactloops.WithLoopUsagePrompt("Use when the user needs structured web/API recon on an authorized target: crawl-js-collector (save verified JS), "+
			"js-static-extract-ai with the downloaded JS directory or URLs, api_pool_merge, probe_api_candidates, plus DNS/ports/crawl as needed."),
		reactloops.WithLoopOutputExample(reflectionOutputExample),
		reactloops.WithVerboseName("Infosec/API Surface Recon"),
		reactloops.WithVerboseNameZh("信息搜集与 API 发现"),
	)
	if err != nil {
		log.Errorf("register reactloop %s failed: %v", schema.AI_REACT_LOOP_NAME_INFOSEC_RECON, err)
	}
}
