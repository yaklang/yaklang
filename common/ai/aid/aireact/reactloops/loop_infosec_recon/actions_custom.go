package loop_infosec_recon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

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
			wd := loop.Get(keyWorkDir)
			if wd == "" {
				wd = workDirFromInvoker(r)
			}
			seed := strings.TrimSpace(action.GetString("seed_url"))
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
			added, mergeErrs := MergeFindings(pool, seed, merged)
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
			op.Continue()
		},
	)
}

func resolveJsStaticScriptPath(workDir string) string {
	if p := strings.TrimSpace(os.Getenv("YAKLANG_JS_STATIC_SCRIPT")); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	dir := workDir
	for i := 0; i < 14 && dir != "" && dir != "/" && dir != "."; i++ {
		cand := filepath.Join(dir, "scripts", "js-static-extract-ai")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func resolveCrawlJsCollectorScriptPath(workDir string) string {
	if p := strings.TrimSpace(os.Getenv("YAKLANG_crawl-js-collector")); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	dir := workDir
	for i := 0; i < 14 && dir != "" && dir != "/" && dir != "."; i++ {
		cand := filepath.Join(dir, "scripts", "crawl-js-collector.yak")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func crawlJsCollectorAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"crawl-js-collector",
		"Run crawl-js-collector: lightweight crawl from seed URL, collect and verify JS URLs, write crawl-js-collector-result.json under workdir and save downloaded scripts in a timestamped folder. "+
			"Then run js-static-extract-ai once with paths= the absolute verified_js_dir path from that JSON. "+
			"Requires `yak` on PATH. Set YAKLANG_crawl-js-collector to the .yak file if not found beside the repo.",
		[]aitool.ToolOption{
			aitool.WithStringParam("start_url", aitool.WithParam_Description("Crawl entry URL; defaults to recon_register_seed seed_url.")),
			aitool.WithBoolParam("deep_js", aitool.WithParam_Default(false)),
			aitool.WithBoolParam("skip_crawl_ai", aitool.WithParam_Default(false), aitool.WithParam_Description("If true, passes --skip-ai to the collector (HTML regex only).")),
			aitool.WithIntegerParam("max_depth", aitool.WithParam_Default(2)),
			aitool.WithIntegerParam("urls_max", aitool.WithParam_Default(80)),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			wd := loop.Get(keyWorkDir)
			if wd == "" {
				wd = workDirFromInvoker(r)
			}
			seed := strings.TrimSpace(action.GetString("start_url"))
			if seed == "" {
				seed = loop.Get(keySeedURL)
			}
			if seed == "" {
				op.Feedback("crawl-js-collector: set start_url or run recon_register_seed first.")
				op.Continue()
				return
			}
			yakBin, err := exec.LookPath("yak")
			if err != nil {
				op.Feedback("yak binary not found on PATH; cannot run crawl-js-collector.yak")
				op.Continue()
				return
			}
			script := resolveCrawlJsCollectorScriptPath(wd)
			if script == "" {
				op.Feedback("crawl-js-collector.yak not found (set YAKLANG_crawl-js-collector).")
				op.Continue()
				return
			}
			jobRoot := filepath.Join(wd, "crawl-js-collector", fmt.Sprintf("job_%d", time.Now().Unix()))
			if err := os.MkdirAll(jobRoot, 0755); err != nil {
				op.Feedback(fmt.Sprintf("mkdir crawl job: %v", err))
				op.Continue()
				return
			}
			args := []string{
				script,
				"--url", seed,
				"--workdir", jobRoot,
				"--max-depth", fmt.Sprintf("%d", action.GetInt("max_depth")),
				"--urls-max", fmt.Sprintf("%d", action.GetInt("urls_max")),
			}
			if action.GetBool("deep_js") {
				args = append(args, "--deep-js")
			}
			if action.GetBool("skip_crawl_ai") {
				args = append(args, "--skip-ai")
			}
			ctx := context.Background()
			if task := loop.GetCurrentTask(); task != nil && task.GetContext() != nil {
				ctx = task.GetContext()
			}
			cmd := exec.CommandContext(ctx, yakBin, args...)
			cmd.Dir = wd
			out, runErr := cmd.CombinedOutput()
			outTrim := utils.ShrinkString(string(out), 4000)
			var b strings.Builder
			reportPath := filepath.Join(jobRoot, "crawl-js-collector-result.json")
			b.WriteString(fmt.Sprintf("crawl-js-collector job dir: %s\n", jobRoot))
			b.WriteString(fmt.Sprintf("JSON report: %s\n", reportPath))
			if runErr != nil {
				log.Warnf("crawl-js-collector: %v: %s", runErr, utils.ShrinkString(string(out), 600))
				r.AddToTimeline("crawl-js-collector_err", runErr.Error())
				b.WriteString(fmt.Sprintf("ERROR: %v\n", runErr))
				b.WriteString(outTrim)
				op.Feedback(b.String())
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
				if json.Unmarshal(data, &rep) == nil && rep.Artifacts != nil {
					b.WriteString(fmt.Sprintf("Pass this directory to js-static-extract-ai paths: %s\n", rep.Artifacts.VerifiedJsDir))
				}
				b.WriteString(fmt.Sprintf("Verified JS URLs in report: %d\n", len(rep.Verified)))
			}
			b.WriteString(outTrim)
			summary := b.String()
			r.AddToTimeline("crawl-js-collector_done", utils.ShrinkString(summary, 4096))
			appendInfosecReconLog(loop, "=== crawl-js-collector ===\n"+summary)
			op.Feedback(summary)
			op.Continue()
		},
	)
}

func runJsStaticAnalysisAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"js-static-extract-ai",
		"Run js-static-extract-ai once per invocation: merges small JS, splits huge files, recurses directories. "+
			"If paths contains exactly one local directory, passes --dir (all .js/.mjs/.cjs under it); otherwise passes --files as a comma-separated list (files and/or http(s) URLs). "+
			"Set YAKLANG_JS_STATIC_SCRIPT to override script path. Requires `yak` on PATH. Default skip_phase2=true.",
		[]aitool.ToolOption{
			aitool.WithStringParam("paths", aitool.WithParam_Required(true), aitool.WithParam_Description("Comma-separated: one local directory (recursive .js/.mjs/.cjs) and/or files and/or http(s) JS URLs. After crawl-js-collector, pass artifacts.verified_js_dir from crawl-js-collector-result.json.")),
			aitool.WithIntegerParam("concurrent", aitool.WithParam_Default(2)),
			aitool.WithBoolParam("skip_phase2", aitool.WithParam_Default(true)),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			wd := loop.Get(keyWorkDir)
			if wd == "" {
				wd = workDirFromInvoker(r)
			}
			yakBin, err := exec.LookPath("yak")
			if err != nil {
				op.Feedback("yak binary not found on PATH; cannot run js-static-extract-ai.yak")
				op.Continue()
				return
			}
			script := resolveJsStaticScriptPath(wd)
			if script == "" {
				op.Feedback("js-static-extract-ai.yak not found (set YAKLANG_JS_STATIC_SCRIPT or run from a repo checkout).")
				op.Continue()
				return
			}
			pathsStr := action.GetString("paths")
			parts := strings.Split(pathsStr, ",")
			var paths []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					paths = append(paths, p)
				}
			}
			if len(paths) == 0 {
				op.Feedback("no paths in paths= parameter")
				op.Continue()
				return
			}
			conc := action.GetInt("concurrent")
			if conc < 1 {
				conc = 2
			}
			skipP2 := action.GetBool("skip_phase2", true)
			seed := loop.Get(keySeedURL)
			pool, lerr := LoadAPIPool(wd)
			if lerr != nil {
				op.Feedback(fmt.Sprintf("load pool: %v", lerr))
				op.Continue()
				return
			}
			outPath := filepath.Join(wd, fmt.Sprintf("js_static_report_%d.json", time.Now().UnixNano()))
			var args []string
			if len(paths) == 1 && utils.IsDir(paths[0]) {
				args = []string{script, "--dir", paths[0], "--output", outPath, "--concurrent", strconv.Itoa(conc)}
			} else {
				args = []string{script, "--files", strings.Join(paths, ","), "--output", outPath, "--concurrent", strconv.Itoa(conc)}
			}
			if skipP2 {
				args = append(args, "--skip-phase2")
			}
			ctx := context.Background()
			if task := loop.GetCurrentTask(); task != nil && task.GetContext() != nil {
				ctx = task.GetContext()
			}
			cmd := exec.CommandContext(ctx, yakBin, args...)
			cmd.Dir = wd
			out, err := cmd.CombinedOutput()
			totalAdded := 0
			if err != nil {
				log.Warnf("js-static-extract-ai: %v: %s", err, utils.ShrinkString(string(out), 800))
				r.AddToTimeline("js-static-extract-ai_err", fmt.Sprintf("%v", err))
				op.Feedback(fmt.Sprintf("js-static-extract-ai failed: %v\n%s", err, utils.ShrinkString(string(out), 2000)))
			} else {
				data, rerr := os.ReadFile(outPath)
				if rerr != nil {
					r.AddToTimeline("js-static-extract-ai_read", rerr.Error())
					op.Feedback(fmt.Sprintf("js-static output read failed: %v", rerr))
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
					add, _ = MergeFindings(pool, seed, merged)
					totalAdded = add
				}
			}
			if err := SaveAPIPool(wd, pool); err != nil {
				op.Feedback(fmt.Sprintf("save pool: %v", err))
				op.Continue()
				return
			}
			r.AddToTimeline("js-static-extract-ai_done", fmt.Sprintf("added %d from js static", totalAdded))
			op.Feedback(fmt.Sprintf("JS static pass done: +%d pool entries (total %d).", totalAdded, len(pool.Entries)))
			op.Feedback("[Next] js-static-extract-ai 已完成。请根据 Reactive Data 中的 API 池摘要（总量、按 source 分布）、ReconLog 与本轮反馈，简要解读新增的接口线索与来源，并决定下一步（例如 probe_api_candidates、继续爬取或合并发现）；勿对已成功分析的 paths 无意义重复调用。")
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
			n := ProbePoolHTTP(pool, limit, conc, useHead, to)
			if err := SaveAPIPool(wd, pool); err != nil {
				op.Feedback(fmt.Sprintf("save pool: %v", err))
				op.Continue()
				return
			}
			_, verified, _, _ := PoolStats(pool)
			r.AddToTimeline("probe_api", fmt.Sprintf("probed %d entries; verified count=%d", n, verified))
			op.Feedback(fmt.Sprintf("Probed %d URLs this batch. Verified entries in pool: %d / %d", n, verified, len(pool.Entries)))
			op.Continue()
		},
	)
}
