package loop_infosec_recon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	maxInlineReconAssetPayloadBytes = 60 * 1024
	maxInlineReconAssetURLBytes     = 16 * 1024
	maxInlineReconAssetSourceBytes  = 4 * 1024
	maxInlineReconAssetErrorBytes   = 4 * 1024
)

// infosecRejectUnsafeArgv blocks control characters that must never appear in tool parameters.
func infosecRejectUnsafeArgv(s string) error {
	if strings.ContainsAny(s, "\x00\n\r") {
		return utils.Error("argument contains NUL or newline characters")
	}
	return nil
}

func infosecValidateHTTPURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if err := infosecRejectUnsafeArgv(raw); err != nil {
		return err
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return utils.Errorf("URL must use http or https, got scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return utils.Error("URL missing host")
	}
	return nil
}

// installBoundedSaaSReconSeed materializes the server authorization into the
// inner engine loop before the model sees an action schema. This keeps the
// release deterministic: the model can probe the exact target, but cannot
// choose or widen the seed-registration step.
func installBoundedSaaSReconSeed(
	loop *reactloops.ReActLoop,
	workDir string,
	proposedTarget string,
	requestedScope string,
) (string, error) {
	if loop == nil {
		return "", utils.Error("bounded SaaS recon loop is missing")
	}
	seed, err := validateSaaSReconActionTarget(loop.GetConfig(), proposedTarget)
	if err != nil {
		return "", err
	}
	scopeHost, err := normalizeSaaSReconScope(seed, requestedScope)
	if err != nil {
		return "", err
	}
	pool, err := LoadAPIPool(workDir)
	if err != nil {
		return "", utils.Wrap(err, "load bounded SaaS recon pool")
	}
	pool.SeedURL = seed
	pool.Entries = []APIPoolEntry{}
	if _, mergeErrs := addSaaSReconSeedCandidate(pool, seed, scopeHost); len(mergeErrs) > 0 {
		return "", utils.Errorf("register SaaS seed candidate: %s", strings.Join(mergeErrs, "; "))
	}
	if err := SaveAPIPool(workDir, pool); err != nil {
		return "", utils.Wrap(err, "save bounded SaaS recon pool")
	}
	loop.Set(keySeedURL, seed)
	loop.Set(keyScopeHosts, scopeHost)
	loop.Set(keyMaxCrawlDepth, defaultCrawlDepth)
	loop.Set(keyProbeConcurrency, defaultProbeConc)
	loop.Set(keySaaSSeedRegistered, "true")
	return seed, nil
}

// infosecResolveLocalPathForExec returns an absolute, existing path for use as a yak CLI argument.
func infosecResolveLocalPathForExec(p, baseWd string) (abs string, err error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", utils.Error("empty path")
	}
	if err := infosecRejectUnsafeArgv(p); err != nil {
		return "", err
	}
	clean := filepath.Clean(p)
	if !filepath.IsAbs(clean) {
		clean = filepath.Join(baseWd, clean)
	}
	abs, err = filepath.Abs(clean)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(abs); err != nil {
		return "", utils.Errorf("path not accessible: %v", err)
	}
	return abs, nil
}

func registerSeedAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"recon_register_seed",
		"Register authorized target seed URL and optional scope for this recon session. "+
			"Updates the on-disk API pool metadata. Use only for explicitly authorized assessments.",
		[]aitool.ToolOption{
			aitool.WithStringParam("seed_url", aitool.WithParam_Required(true), aitool.WithParam_Description("Primary https?:// URL or root to scope recon.")),
			aitool.WithStringParam("scope_hosts", aitool.WithParam_Description("Optional comma-separated hostnames allowed for crawling/probing.")),
			aitool.WithIntegerParam("max_crawl_depth", aitool.WithParam_Default(2), aitool.WithParam_Description("Suggested crawl depth for simple_crawler.")),
			aitool.WithIntegerParam("probe_concurrency", aitool.WithParam_Default(6), aitool.WithParam_Description("Default max parallelism for probe_api_candidates.")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			seed := strings.TrimSpace(action.GetString("seed_url"))
			if seed == "" {
				return utils.Error("recon_register_seed requires seed_url")
			}
			if isBoundedSaaSRecon(loop.GetConfig()) {
				if loop.Get(keySaaSSeedRegistered) == "true" {
					return utils.Error("bounded SaaS recon seed is already registered")
				}
				authorized, err := validateSaaSReconActionTarget(loop.GetConfig(), seed)
				if err != nil {
					return err
				}
				_, err = normalizeSaaSReconScope(
					authorized,
					action.GetString("scope_hosts"),
				)
				return err
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			reactloops.EmitActionLog(loop, infosecAPIPoolNodeID, "开始: recon_register_seed / Start: recon_register_seed")
			reactloops.EmitStatus(loop, "注册侦察种子中 / Registering recon seed...")

			wd := loop.Get(keyWorkDir)
			if wd == "" {
				wd = workDirFromInvoker(r)
			}
			seed := strings.TrimSpace(action.GetString("seed_url"))
			saasMode := isBoundedSaaSRecon(loop.GetConfig())
			if saasMode {
				var err error
				seed, err = installBoundedSaaSReconSeed(loop, wd, seed, action.GetString("scope_hosts"))
				if err != nil {
					op.Fail(err)
					return
				}
				r.AddToTimeline("infosec_seed", fmt.Sprintf("seed=%s workdir=%s", seed, wd))
				op.Feedback(fmt.Sprintf("Registered server-authorized seed URL. Pool file: %s", filepath.Join(wd, poolFileName)))
				reactloops.EmitStatus(loop, "完成 / Complete")
				reactloops.EmitActionLog(loop, infosecAPIPoolNodeID, fmt.Sprintf("完成: recon_register_seed / Done: recon_register_seed (seed=%s)", seed))
				op.Continue()
				return
			} else if norm, coerced, note := infosecPickFirstHTTPURL(seed); coerced {
				seed = norm
				r.AddToTimeline("infosec_seed_url_coerced", note)
			}
			if err := infosecValidateHTTPURL(seed); err != nil {
				op.Feedback(fmt.Sprintf("recon_register_seed: invalid seed_url: %v", err))
				op.Continue()
				return
			}
			loop.Set(keySeedURL, seed)
			if sh := strings.TrimSpace(action.GetString("scope_hosts")); sh != "" {
				loop.Set(keyScopeHosts, sh)
			}
			loop.Set(keyMaxCrawlDepth, fmt.Sprintf("%d", action.GetInt("max_crawl_depth")))
			loop.Set(keyProbeConcurrency, fmt.Sprintf("%d", action.GetInt("probe_concurrency")))

			pool, err := LoadAPIPool(wd)
			if err != nil {
				op.Feedback(fmt.Sprintf("load pool failed: %v", err))
				op.Continue()
				return
			}
			pool.SeedURL = seed
			if err := SaveAPIPool(wd, pool); err != nil {
				op.Feedback(fmt.Sprintf("save pool failed: %v", err))
				op.Continue()
				return
			}
			r.AddToTimeline("infosec_seed", fmt.Sprintf("seed=%s workdir=%s", seed, wd))
			op.Feedback(fmt.Sprintf("Registered seed URL. Pool file: %s", filepath.Join(wd, poolFileName)))
			reactloops.EmitStatus(loop, "完成 / Complete")
			reactloops.EmitActionLog(loop, infosecAPIPoolNodeID, fmt.Sprintf("完成: recon_register_seed / Done: recon_register_seed (seed=%s)", seed))
			op.Continue()
		},
	)
}

func apiPoolMergeAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"api_pool_merge",
		"Merge API/URL findings into the shared deduplicated pool. "+
			"Pass findings_json as a JSON array of objects: {\"url\":\"...\",\"method\":\"GET\",\"source\":\"crawler|manual|...\",\"evidence\":\"...\"}.",
		[]aitool.ToolOption{
			aitool.WithStringParam("findings_json", aitool.WithParam_Required(true), aitool.WithParam_Description("JSON array of finding objects.")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			reactloops.EmitActionLog(loop, infosecAPIPoolNodeID, "开始: api_pool_merge / Start: api_pool_merge")
			reactloops.EmitStatus(loop, "整理 API 池中 / Merging API pool...")

			wd := loop.Get(keyWorkDir)
			if wd == "" {
				wd = workDirFromInvoker(r)
			}
			seed := loop.Get(keySeedURL)
			raw := action.GetString("findings_json")
			var rows []map[string]interface{}
			if err := json.Unmarshal([]byte(raw), &rows); err != nil {
				op.Feedback(fmt.Sprintf("invalid findings_json: %v", err))
				op.Continue()
				return
			}
			var findings []struct {
				URL, Method, Source, Evidence string
				Confidence                    float64
			}
			for _, row := range rows {
				findings = append(findings, struct {
					URL, Method, Source, Evidence string
					Confidence                    float64
				}{
					URL:        utils.InterfaceToString(row["url"]),
					Method:     utils.InterfaceToString(row["method"]),
					Source:     utils.InterfaceToString(row["source"]),
					Evidence:   utils.InterfaceToString(row["evidence"]),
					Confidence: utils.InterfaceToFloat64(row["confidence"]),
				})
			}
			pool, err := LoadAPIPool(wd)
			if err != nil {
				op.Feedback(fmt.Sprintf("load pool: %v", err))
				op.Continue()
				return
			}
			var merged []struct {
				URL, Method, Source, Evidence string
				Confidence                    float64
			}
			for _, f := range findings {
				merged = append(merged, f)
			}
			scopeHosts := loop.Get(keyScopeHosts)
			added, mergeErrs := MergeFindings(pool, seed, merged, scopeHosts)
			if len(mergeErrs) > 0 {
				log.Warnf("api_pool_merge partial errors: %v", mergeErrs)
			}
			if err := SaveAPIPool(wd, pool); err != nil {
				op.Feedback(fmt.Sprintf("save pool: %v", err))
				op.Continue()
				return
			}
			r.AddToTimeline("api_pool_merge", fmt.Sprintf("added %d endpoints (errors: %d)", added, len(mergeErrs)))
			op.Feedback(fmt.Sprintf("Merged into pool: +%d new entries. Total entries: %d. Parse errors: %d", added, len(pool.Entries), len(mergeErrs)))
			reactloops.EmitStatus(loop, "完成 / Complete")
			reactloops.EmitActionLog(loop, infosecAPIPoolNodeID, fmt.Sprintf("完成: api_pool_merge (+%d) / Done: api_pool_merge (+%d)", added, added))
			op.Continue()
		},
	)
}

func infosecInvokerContext(loop *reactloops.ReActLoop) (aicommon.AIInvokeRuntime, context.Context) {
	invoker := loop.GetInvoker()
	ctx := invoker.GetConfig().GetContext()
	task := loop.GetCurrentTask()
	if task != nil && !utils.IsNil(task.GetContext()) {
		ctx = task.GetContext()
	}
	return invoker, ctx
}

func crawlJsCollectorAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		ToolCrawlJsCollector,
		"Run registered tool "+ToolCrawlJsCollector+": crawl from seed URL, verify JS URLs, write crawl-js-collector-result.json under the job workdir. "+
			"Then pass artifacts.verified_js_dir from that JSON to "+ToolJsStaticExtractAI+" (paths).",
		[]aitool.ToolOption{
			aitool.WithStringParam("start_url", aitool.WithParam_Description("Crawl entry URL; defaults to recon_register_seed seed_url.")),
			aitool.WithBoolParam("deep_js", aitool.WithParam_Default(false)),
			aitool.WithBoolParam("skip_crawl_ai", aitool.WithParam_Default(false), aitool.WithParam_Description("If true, skip AI in the collector (HTML regex only).")),
			aitool.WithIntegerParam("max_depth", aitool.WithParam_Default(2)),
			aitool.WithIntegerParam("urls_max", aitool.WithParam_Default(80)),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if !isBoundedSaaSRecon(loop.GetConfig()) {
				return nil
			}
			if loop.Get(keySaaSCrawlAttempted) == "true" {
				return utils.Error("SaaS recon crawl is already attempted")
			}
			if loop.Get(keySaaSSeedRegistered) != "true" {
				return utils.Error("SaaS recon seed is not initialized")
			}
			startURL := strings.TrimSpace(action.GetString("start_url"))
			if startURL == "" {
				return nil
			}
			_, err := validateSaaSReconActionTarget(loop.GetConfig(), startURL)
			return err
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			reactloops.EmitActionLog(loop, infosecJsCrawlNodeID, fmt.Sprintf("开始: %s / Start: %s", ToolCrawlJsCollector, ToolCrawlJsCollector))
			reactloops.EmitStatus(loop, "JS 爬取分析中 / Running JS crawl analysis...")

			wd := loop.Get(keyWorkDir)
			if wd == "" {
				wd = workDirFromInvoker(r)
			}
			seed := strings.TrimSpace(action.GetString("start_url"))
			if seed == "" {
				seed = loop.Get(keySeedURL)
			}
			if seed == "" {
				op.Feedback(ToolCrawlJsCollector + ": set start_url or run recon_register_seed first.")
				op.Continue()
				return
			}
			if isBoundedSaaSRecon(loop.GetConfig()) {
				loop.Set(keySaaSCrawlAttempted, "true")
				_, ctx := infosecInvokerContext(loop)
				stats, findings, crawlErr := crawlBoundedSaaSRecon(ctx, nil, seed, wd)
				if crawlErr != nil {
					op.Fail(fmt.Errorf("SaaS recon crawl failed: %w", crawlErr))
					return
				}
				pool, loadErr := LoadAPIPool(wd)
				if loadErr != nil {
					op.Fail(fmt.Errorf("load SaaS recon pool: %w", loadErr))
					return
				}
				added, mergeErrs := mergeBoundedSaaSReconFindings(
					pool,
					seed,
					loop.Get(keyScopeHosts),
					findings,
				)
				filterBoundedSaaSReconPool(pool, seed)
				if saveErr := SaveAPIPool(wd, pool); saveErr != nil {
					op.Fail(fmt.Errorf("save SaaS recon pool: %w", saveErr))
					return
				}
				loop.Set(keyVerifiedJsDir, stats.VerifiedJSDir)
				loop.Set(keySaaSCrawlCompleted, "true")
				summary := fmt.Sprintf(
					"SaaS crawl completed: pages=%d scripts=%d requests=%d candidates=%d added=%d parse_errors=%d",
					stats.Pages,
					stats.Scripts,
					stats.Requests,
					stats.Candidates,
					added,
					len(mergeErrs),
				)
				r.AddToTimeline(ToolCrawlJsCollector+"_done", summary)
				appendInfosecReconLog(loop, summary)
				op.Feedback(summary + ". Next: call " + ToolJsStaticExtractAI + ".")
				reactloops.EmitStatus(loop, "站点与脚本采集完成 / Site and script collection complete")
				reactloops.EmitActionLog(loop, infosecJsCrawlNodeID, "完成: SaaS 同源站点与脚本采集 / Done: SaaS same-origin site and script collection")
				op.Continue()
				return
			}
			if norm, coerced, note := infosecPickFirstHTTPURL(seed); coerced {
				seed = norm
				r.AddToTimeline("infosec_crawl_url_coerced", note)
			}
			if err := infosecValidateHTTPURL(seed); err != nil {
				op.Feedback(fmt.Sprintf("%s: invalid start_url / seed (require http/https): %v", ToolCrawlJsCollector, err))
				op.Continue()
				return
			}
			jobRoot := filepath.Join(wd, ToolCrawlJsCollector, fmt.Sprintf("job_%d", time.Now().Unix()))
			if err := os.MkdirAll(jobRoot, 0755); err != nil {
				op.Feedback(fmt.Sprintf("mkdir crawl job: %v", err))
				op.Continue()
				return
			}
			params := aitool.InvokeParams{
				"url":       seed,
				"workdir":   jobRoot,
				"max-depth": action.GetInt("max_depth"),
				"urls-max":  action.GetInt("urls_max"),
			}
			if action.GetBool("deep_js") {
				params["deep-js"] = true
			}
			if action.GetBool("skip_crawl_ai") {
				params["skip-ai"] = true
			}
			invoker, ctx := infosecInvokerContext(loop)
			_, _, runErr := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, ToolCrawlJsCollector, params)
			var b strings.Builder
			reportPath := filepath.Join(jobRoot, "crawl-js-collector-result.json")
			b.WriteString(fmt.Sprintf("%s job dir: %s\n", ToolCrawlJsCollector, jobRoot))
			b.WriteString(fmt.Sprintf("JSON report: %s\n", reportPath))
			if runErr != nil {
				log.Warnf("%s: %v", ToolCrawlJsCollector, runErr)
				r.AddToTimeline(ToolCrawlJsCollector+"_err", runErr.Error())
				b.WriteString(fmt.Sprintf("ERROR: %v\n", runErr))
				feedback, _ := reactloops.SpillLongContent(loop, ToolCrawlJsCollector, b.String())
				op.Feedback(feedback)
				op.Continue()
				return
			}
			if data, rerr := os.ReadFile(reportPath); rerr == nil {
				var rep struct {
					Artifacts *struct {
						VerifiedJsDir string `json:"verified_js_dir"`
					} `json:"artifacts"`
					Verified []any `json:"verified_js_urls"`
				}
				if json.Unmarshal(data, &rep) == nil && rep.Artifacts != nil && strings.TrimSpace(rep.Artifacts.VerifiedJsDir) != "" {
					vdir := strings.TrimSpace(rep.Artifacts.VerifiedJsDir)
					loop.Set(keyVerifiedJsDir, vdir)
					b.WriteString(fmt.Sprintf("Pass this directory to %s dir (preferred) or paths: %s\n", ToolJsStaticExtractAI, vdir))
					b.WriteString("If the directory name contains commas, you MUST use dir=, not paths=.\n")
				}
				b.WriteString(fmt.Sprintf("Verified JS URLs in report: %d\n", len(rep.Verified)))
			}
			summary := b.String()
			feedback, reference := reactloops.SpillLongContent(loop, ToolCrawlJsCollector, summary)
			timelineEntry := utils.ShrinkString(summary, 4096)
			if reference != summary {
				timelineEntry = timelineEntry + "\n\n[spill] " + reference
			}
			r.AddToTimeline(ToolCrawlJsCollector+"_done", timelineEntry)
			appendInfosecReconLog(loop, "=== "+ToolCrawlJsCollector+" ===\n"+summary)
			op.Feedback(feedback)
			reactloops.EmitStatus(loop, "完成 / Complete")
			reactloops.EmitActionLog(loop, infosecJsCrawlNodeID, fmt.Sprintf("完成: %s / Done: %s", ToolCrawlJsCollector, ToolCrawlJsCollector))
			op.Continue()
		},
	)
}

func runJsStaticAnalysisAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		ToolJsStaticExtractAI,
		"Run registered tool "+ToolJsStaticExtractAI+" once: static JS API extraction; output JSON is merged into the API pool. "+
			"Prefer dir= for a single local directory (especially verified_js_dir). paths= is comma-separated for multiple entries. Default skip_phase2=true.",
		[]aitool.ToolOption{
			aitool.WithStringParam("dir", aitool.WithParam_Description("Single local directory (preferred after "+ToolCrawlJsCollector+"; safe when directory names contain commas).")),
			aitool.WithStringParam("paths", aitool.WithParam_Description("Optional comma-separated files/dirs/http(s) URLs. Omit when dir= is set; auto-fills from crawl verified_js_dir if empty.")),
			aitool.WithIntegerParam("concurrent", aitool.WithParam_Default(2)),
			aitool.WithBoolParam("skip_phase2", aitool.WithParam_Default(true)),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if isBoundedSaaSRecon(loop.GetConfig()) {
				if loop.Get(keySaaSCrawlCompleted) != "true" {
					return utils.Error("complete SaaS recon crawl before static endpoint extraction")
				}
				if loop.Get(keySaaSStaticAttempted) == "true" {
					return utils.Error("SaaS recon static extraction is already attempted")
				}
				return nil
			}
			dir := strings.TrimSpace(action.GetString("dir"))
			paths := strings.TrimSpace(action.GetString("paths"))
			if dir == "" && paths == "" && strings.TrimSpace(loop.Get(keyVerifiedJsDir)) == "" {
				return utils.Error("js_static_extract_ai requires dir= or paths= (or run crawl_js_collector first for auto dir)")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			reactloops.EmitActionLog(loop, infosecJsCrawlNodeID, fmt.Sprintf("开始: %s / Start: %s", ToolJsStaticExtractAI, ToolJsStaticExtractAI))
			reactloops.EmitStatus(loop, "JS 静态分析中 / Running JS static analysis...")

			wd := loop.Get(keyWorkDir)
			if wd == "" {
				wd = workDirFromInvoker(r)
			}
			if isBoundedSaaSRecon(loop.GetConfig()) {
				loop.Set(keySaaSStaticAttempted, "true")
				seed := loop.Get(keySeedURL)
				verifiedDir := loop.Get(keyVerifiedJsDir)
				findings, filesRead, extractErr := extractBoundedSaaSReconStaticFindings(seed, verifiedDir)
				if extractErr != nil {
					op.Fail(fmt.Errorf("SaaS static endpoint extraction failed: %w", extractErr))
					return
				}
				pool, loadErr := LoadAPIPool(wd)
				if loadErr != nil {
					op.Fail(fmt.Errorf("load SaaS recon pool: %w", loadErr))
					return
				}
				added, mergeErrs := mergeBoundedSaaSReconFindings(
					pool,
					seed,
					loop.Get(keyScopeHosts),
					findings,
				)
				filterBoundedSaaSReconPool(pool, seed)
				if saveErr := SaveAPIPool(wd, pool); saveErr != nil {
					op.Fail(fmt.Errorf("save SaaS recon pool: %w", saveErr))
					return
				}
				loop.Set(keySaaSStaticCompleted, "true")
				summary := fmt.Sprintf(
					"SaaS static endpoint extraction completed: files=%d candidates=%d added=%d parse_errors=%d",
					filesRead,
					len(findings),
					added,
					len(mergeErrs),
				)
				r.AddToTimeline(ToolJsStaticExtractAI+"_done", summary)
				appendInfosecReconLog(loop, summary)
				op.Feedback(summary + ". Next: call probe_api_candidates.")
				reactloops.EmitStatus(loop, "API 候选提取完成 / API candidate extraction complete")
				reactloops.EmitActionLog(loop, infosecJsCrawlNodeID, "完成: SaaS JS API 候选提取 / Done: SaaS JS API candidate extraction")
				op.Continue()
				return
			}
			pathsStr := action.GetString("paths")
			dirStr := action.GetString("dir")
			verifiedDir := loop.Get(keyVerifiedJsDir)
			paths, pathSource, resolveErr := infosecResolveJsStaticPaths(pathsStr, dirStr, verifiedDir, wd)
			if resolveErr != nil {
				fb := resolveErr.Error()
				infosecRecordJsStaticPathFailure(loop, fb)
				if hint := strings.TrimSpace(loop.Get(keySpinRecoveryHint)); hint != "" {
					fb += "\n\n" + hint
				}
				op.Feedback(fb)
				op.Continue()
				return
			}
			if pathSource != "comma-separated paths" {
				log.Infof("infosec_recon: js_static_extract_ai input resolved via %s: %v", pathSource, paths)
			}
			conc := action.GetInt("concurrent")
			if conc < 1 {
				conc = 2
			}
			skipP2 := action.GetBool("skip_phase2", true)
			seed := loop.Get(keySeedURL)
			scopeHosts := loop.Get(keyScopeHosts)
			pool, lerr := LoadAPIPool(wd)
			if lerr != nil {
				op.Feedback(fmt.Sprintf("load pool: %v", lerr))
				op.Continue()
				return
			}
			outPath := filepath.Join(wd, fmt.Sprintf("js_static_report_%d.json", time.Now().UnixNano()))
			params := aitool.InvokeParams{
				"output":     outPath,
				"concurrent": conc,
			}
			if skipP2 {
				params["skip-phase2"] = true
			}
			if len(paths) == 1 && utils.IsDir(paths[0]) {
				params["dir"] = paths[0]
			} else {
				params["files"] = strings.Join(paths, ",")
			}
			invoker, ctx := infosecInvokerContext(loop)
			_, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, ToolJsStaticExtractAI, params)
			totalAdded := 0
			if err != nil {
				log.Warnf("%s: %v", ToolJsStaticExtractAI, err)
				r.AddToTimeline(ToolJsStaticExtractAI+"_err", fmt.Sprintf("%v", err))
				op.Feedback(fmt.Sprintf("%s failed: %v", ToolJsStaticExtractAI, err))
				infosecRecordJsStaticPathFailure(loop, err.Error())
			} else {
				data, rerr := os.ReadFile(outPath)
				if rerr != nil {
					r.AddToTimeline(ToolJsStaticExtractAI+"_read", rerr.Error())
					op.Feedback(fmt.Sprintf("js static output read failed: %v", rerr))
				} else {
					extracted := ExtractFromJSReport(data)
					var merged []struct {
						URL, Method, Source, Evidence string
						Confidence                    float64
					}
					tag := "batch"
					if len(paths) == 1 {
						tag = filepath.Base(paths[0])
					}
					for _, e := range extracted {
						merged = append(merged, struct {
							URL, Method, Source, Evidence string
							Confidence                    float64
						}{URL: e.URL, Method: e.Method, Source: e.Source + ":" + tag, Evidence: e.Evidence, Confidence: e.Confidence})
					}
					var add int
					add, _ = MergeFindings(pool, seed, merged, scopeHosts)
					totalAdded = add
				}
			}
			if err := SaveAPIPool(wd, pool); err != nil {
				op.Feedback(fmt.Sprintf("save pool: %v", err))
				op.Continue()
				return
			}
			infosecClearJsStaticPathFailures(loop)
			r.AddToTimeline(ToolJsStaticExtractAI+"_done", fmt.Sprintf("added %d from js static", totalAdded))
			op.Feedback(fmt.Sprintf("JS static pass done: +%d pool entries (total %d). Resolved via %s.", totalAdded, len(pool.Entries), pathSource))
			op.Feedback("[Next] " + ToolJsStaticExtractAI + " 已完成。请根据 API 池摘要、ReconLog 与本轮反馈决定下一步（如 probe_api_candidates）；勿对已成功分析的 paths 无意义重复调用。")
			reactloops.EmitStatus(loop, "完成 / Complete")
			reactloops.EmitActionLog(loop, infosecJsCrawlNodeID, fmt.Sprintf("完成: %s (+%d) / Done: %s (+%d)", ToolJsStaticExtractAI, totalAdded, ToolJsStaticExtractAI, totalAdded))
			op.Continue()
		},
	)
}

func probeAPICandidatesAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"probe_api_candidates",
		"HTTP probe unverified https? URLs in the pool (HEAD or GET), low concurrency. Authorized targets only.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("limit", aitool.WithParam_Default(40)),
			aitool.WithIntegerParam("concurrency", aitool.WithParam_Default(6)),
			aitool.WithBoolParam("use_head", aitool.WithParam_Default(true)),
			aitool.WithIntegerParam("timeout_seconds", aitool.WithParam_Default(12)),
		},
		func(loop *reactloops.ReActLoop, _ *aicommon.Action) error {
			if isBoundedSaaSRecon(loop.GetConfig()) {
				if loop.Get(keySaaSProbeAttempted) == "true" {
					return utils.Error("bounded SaaS recon probe is already attempted")
				}
				if loop.Get(keySaaSStaticCompleted) != "true" {
					return utils.Error("complete SaaS crawl and static extraction before probing API candidates")
				}
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			reactloops.EmitActionLog(loop, infosecAPIPoolNodeID, "开始: probe_api_candidates / Start: probe_api_candidates")
			reactloops.EmitStatus(loop, "探测 API 候选中 / Probing API candidates...")

			wd := loop.Get(keyWorkDir)
			if wd == "" {
				wd = workDirFromInvoker(r)
			}
			saasMode := isBoundedSaaSRecon(loop.GetConfig())
			if saasMode {
				loop.Set(keySaaSProbeAttempted, "true")
			}
			settings := reconProbeSettings{
				limit:           action.GetInt("limit"),
				concurrency:     action.GetInt("concurrency"),
				useHead:         action.GetBool("use_head"),
				timeout:         time.Duration(action.GetInt("timeout_seconds")) * time.Second,
				followRedirects: true,
			}
			if settings.limit < 1 {
				settings.limit = 40
			}
			if settings.concurrency < 1 {
				settings.concurrency = 6
			}
			if settings.timeout <= 0 {
				settings.timeout = 12 * time.Second
			}
			if saasMode {
				settings = boundedSaaSReconProbeSettings(
					settings.limit,
					settings.concurrency,
					settings.useHead,
					settings.timeout,
				)
			}
			pool, err := LoadAPIPool(wd)
			if err != nil {
				op.Feedback(fmt.Sprintf("load pool: %v", err))
				op.Continue()
				return
			}
			if saasMode {
				filterBoundedSaaSReconPool(pool, loop.Get(keySeedURL))
			}
			allowed := ParseScopeHostSet(loop.Get(keyScopeHosts))
			if saasMode && len(allowed) == 0 {
				op.Fail("bounded SaaS recon requires the exact seed host scope")
				return
			}
			n := probePoolHTTP(
				pool,
				settings.limit,
				settings.concurrency,
				settings.useHead,
				settings.timeout,
				allowed,
				settings.followRedirects,
			)
			if err := SaveAPIPool(wd, pool); err != nil {
				op.Feedback(fmt.Sprintf("save pool: %v", err))
				op.Continue()
				return
			}
			submitted, err := submitVerifiedAPIAssets(
				reconResultContext(loop),
				aicommon.AssetResultSinkFromConfig(loop.GetConfig()),
				pool,
			)
			if err != nil {
				op.Fail(fmt.Errorf("publish verified API assets: %w", err))
				return
			}
			_, verified, _, _ := PoolStats(pool)
			r.AddToTimeline(
				"probe_api",
				fmt.Sprintf(
					"probed %d entries; verified count=%d; submitted assets=%d",
					n,
					verified,
					submitted,
				),
			)
			op.Feedback(fmt.Sprintf(
				"Probed %d URLs this batch. Verified entries in pool: %d / %d. Submitted assets: %d",
				n,
				verified,
				len(pool.Entries),
				submitted,
			))
			reactloops.EmitStatus(loop, "完成 / Complete")
			reactloops.EmitActionLog(loop, infosecAPIPoolNodeID, fmt.Sprintf("完成: probe_api_candidates (%d probed) / Done: probe_api_candidates (%d probed)", n, n))
			if saasMode {
				if submitted == 0 {
					op.Feedback("The bounded discovery pipeline completed, but no endpoint passed HEAD verification in this run.")
				}
				op.Exit()
				return
			}
			op.Continue()
		},
	)
}

func submitVerifiedAPIAssets(
	ctx context.Context,
	sink aicommon.AssetResultSink,
	pool *APIPool,
) (int, error) {
	if sink == nil || pool == nil {
		return 0, nil
	}

	submitted := 0
	var submitErr error
	for _, entry := range pool.Entries {
		if !entry.Verified {
			continue
		}
		method := strings.ToUpper(strings.TrimSpace(entry.Method))
		if method == "" {
			method = http.MethodGet
		}
		target := strings.TrimSpace(entry.NormalizedURL)
		if target == "" {
			continue
		}
		payload, err := marshalVerifiedAPIAssetPayload(entry)
		if err != nil {
			submitErr = errors.Join(
				submitErr,
				fmt.Errorf("marshal verified API endpoint %s %s: %w", method, target, err),
			)
			continue
		}
		identityKey := "http_endpoint:" + method + ":" + target
		if _, err := sink.SubmitAsset(ctx, aicommon.AssetResult{
			Kind:        "http_endpoint",
			Title:       method + " " + target,
			Target:      target,
			IdentityKey: identityKey,
			Payload:     payload,
		}); err != nil {
			submitErr = errors.Join(
				submitErr,
				fmt.Errorf("submit verified API endpoint %s %s: %w", method, target, err),
			)
			continue
		}
		submitted++
	}
	return submitted, submitErr
}

type verifiedAPIAssetPayload struct {
	APIPoolEntry
	SchemaVersion          string `json:"schema_version"`
	VerificationState      string `json:"verification_state"`
	NetworkAccessPerformed bool   `json:"network_access_performed"`
	URL                    string `json:"url"`
	Scheme                 string `json:"scheme"`
	Host                   string `json:"host"`
	Port                   string `json:"port"`
	HTTPURL                string `json:"http_url"`
	HTTPStatusCode         int    `json:"http_status_code,omitempty"`
}

func marshalVerifiedAPIAssetPayload(entry APIPoolEntry) ([]byte, error) {
	payload, err := newVerifiedAPIAssetPayload(entry)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if len(raw) <= maxInlineReconAssetPayloadBytes {
		return raw, nil
	}

	originalEvidence := payload.Evidence
	payload.Evidence = ""
	payload.NormalizedURL = truncateReconAssetText(
		payload.NormalizedURL,
		maxInlineReconAssetURLBytes,
	)
	payload.URL = truncateReconAssetText(payload.URL, maxInlineReconAssetURLBytes)
	payload.HTTPURL = payload.URL
	payload.Source = truncateReconAssetText(payload.Source, maxInlineReconAssetSourceBytes)
	payload.ProbeError = truncateReconAssetText(payload.ProbeError, maxInlineReconAssetErrorBytes)
	base, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if len(base) > maxInlineReconAssetPayloadBytes {
		return nil, fmt.Errorf(
			"verified API endpoint metadata exceeds %d bytes",
			maxInlineReconAssetPayloadBytes,
		)
	}

	// JSON may expand control characters to six bytes. Use that worst-case
	// ratio so the reconstructed payload always remains below the Scan Node
	// inline-result contract while preserving as much evidence as possible.
	evidenceBudget := (maxInlineReconAssetPayloadBytes - len(base)) / 6
	payload.Evidence = truncateReconAssetText(originalEvidence, evidenceBudget)
	raw, err = json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	for len(raw) > maxInlineReconAssetPayloadBytes && payload.Evidence != "" {
		payload.Evidence = truncateReconAssetText(payload.Evidence, len(payload.Evidence)/2)
		raw, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	if len(raw) > maxInlineReconAssetPayloadBytes {
		return nil, fmt.Errorf(
			"verified API endpoint payload exceeds %d bytes",
			maxInlineReconAssetPayloadBytes,
		)
	}
	return raw, nil
}

func newVerifiedAPIAssetPayload(entry APIPoolEntry) (verifiedAPIAssetPayload, error) {
	target := strings.TrimSpace(entry.NormalizedURL)
	parsed, err := url.Parse(target)
	if err != nil {
		return verifiedAPIAssetPayload{}, err
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if (scheme != "http" && scheme != "https") || host == "" {
		return verifiedAPIAssetPayload{}, fmt.Errorf(
			"verified API endpoint must be an absolute HTTP(S) URL: %q",
			target,
		)
	}
	port := parsed.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return verifiedAPIAssetPayload{
		APIPoolEntry:           entry,
		SchemaVersion:          "1",
		VerificationState:      "verified",
		NetworkAccessPerformed: true,
		URL:                    target,
		Scheme:                 scheme,
		Host:                   host,
		Port:                   port,
		HTTPURL:                target,
		HTTPStatusCode:         entry.StatusCode,
	}, nil
}

func truncateReconAssetText(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	truncated := value[:maxBytes]
	for !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated
}

func reconResultContext(loop *reactloops.ReActLoop) context.Context {
	if loop != nil {
		if config := loop.GetConfig(); config != nil && config.GetContext() != nil {
			return config.GetContext()
		}
	}
	return context.Background()
}
