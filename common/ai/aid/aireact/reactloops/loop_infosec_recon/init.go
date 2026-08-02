package loop_infosec_recon

import (
	"bytes"
	_ "embed"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/ytoken"
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
			saasMode := isBoundedSaaSRecon(r.GetConfig())
			allowed := infosecReconAllowedActions(
				saasMode,
				r.GetConfig().GetAllowUserInteraction(),
			)

			maxIter := int(r.GetConfig().GetMaxIterationCount())
			if saasMode {
				maxIter = 6
			} else if maxIter < 16 {
				maxIter = 16
			}
			instruction := persistentInstruction
			if saasMode {
				instruction = boundedSaaSReconInstruction
			}

			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(!saasMode),
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
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(reflectionOutputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					wd := loop.Get(keyWorkDir)
					if wd == "" {
						wd = workDirFromInvoker(r)
					}
					pool, _ := LoadAPIPool(wd)
					tot, ver, unver, bySrc := PoolStats(pool)
					srcKeys := make([]string, 0, len(bySrc))
					for k := range bySrc {
						srcKeys = append(srcKeys, k)
					}
					sort.Strings(srcKeys)
					var srcParts []string
					for _, k := range srcKeys {
						srcParts = append(srcParts, k+":"+utils.InterfaceToString(bySrc[k]))
					}
					reconLog := loop.Get(keyReconLog)
					if ytoken.CalcTokenCount(reconLog) > 3500 {
						reconLog = reconLog[len(reconLog)-3500:]
					}
					spinHint := strings.TrimSpace(loop.Get(keySpinRecoveryHint))
					renderMap := map[string]any{
						"Nonce":            nonce,
						"SeedURL":          loop.Get(keySeedURL),
						"ScopeHosts":       loop.Get(keyScopeHosts),
						"WorkDir":          wd,
						"VerifiedJsDir":    loop.Get(keyVerifiedJsDir),
						"SpinRecoveryHint": spinHint,
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
				buildInfosecPostIterationHook(r),
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
			"Merges candidates into a shared on-disk pool; JS pipeline uses registered tools "+ToolCrawlJsCollector+" and "+ToolJsStaticExtractAI+"; optional HTTP probing."),
		reactloops.WithLoopUsagePrompt("Use when the user needs structured web/API recon on an authorized target: "+ToolCrawlJsCollector+" (save verified JS), "+
			ToolJsStaticExtractAI+" with the downloaded JS directory or URLs, api_pool_merge, probe_api_candidates, plus DNS/ports/crawl as needed."),
		reactloops.WithLoopOutputExample(reflectionOutputExample),
		reactloops.WithVerboseName("Infosec/API Surface Recon"),
		reactloops.WithVerboseNameZh("信息搜集与 API 发现"),
	)
	if err != nil {
		log.Errorf("register reactloop %s failed: %v", schema.AI_REACT_LOOP_NAME_INFOSEC_RECON, err)
	}
}
