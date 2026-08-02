package loop_infosec_recon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
			if strings.TrimSpace(action.GetString("seed_url")) == "" {
				return utils.Error("recon_register_seed requires seed_url")
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
			if norm, coerced, note := infosecPickFirstHTTPURL(seed); coerced {
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
		nil,
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
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			reactloops.EmitActionLog(loop, infosecAPIPoolNodeID, "开始: probe_api_candidates / Start: probe_api_candidates")
			reactloops.EmitStatus(loop, "探测 API 候选中 / Probing API candidates...")

			wd := loop.Get(keyWorkDir)
			if wd == "" {
				wd = workDirFromInvoker(r)
			}
			limit := action.GetInt("limit")
			if limit < 1 {
				limit = 40
			}
			conc := action.GetInt("concurrency")
			if conc < 1 {
				conc = 6
			}
			useHead := action.GetBool("use_head")
			to := time.Duration(action.GetInt("timeout_seconds")) * time.Second
			if to <= 0 {
				to = 12 * time.Second
			}
			pool, err := LoadAPIPool(wd)
			if err != nil {
				op.Feedback(fmt.Sprintf("load pool: %v", err))
				op.Continue()
				return
			}
			allowed := ParseScopeHostSet(loop.Get(keyScopeHosts))
			n := ProbePoolHTTP(pool, limit, conc, useHead, to, allowed)
			if err := SaveAPIPool(wd, pool); err != nil {
				op.Feedback(fmt.Sprintf("save pool: %v", err))
				op.Continue()
				return
			}
			_, verified, _, _ := PoolStats(pool)
			r.AddToTimeline("probe_api", fmt.Sprintf("probed %d entries; verified count=%d", n, verified))
			op.Feedback(fmt.Sprintf("Probed %d URLs this batch. Verified entries in pool: %d / %d", n, verified, len(pool.Entries)))
			reactloops.EmitStatus(loop, "完成 / Complete")
			reactloops.EmitActionLog(loop, infosecAPIPoolNodeID, fmt.Sprintf("完成: probe_api_candidates (%d probed) / Done: probe_api_candidates (%d probed)", n, n))
			op.Continue()
		},
	)
}
