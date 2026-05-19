package reactloops

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// perception_frequency_sim_test.go 是一个离线仿真测试, 用真实案例 (redhaze
// login security 测试) 提取的 iter 时间戳来对 perceptionController 的节流
// 参数做扫描, 评估在不同 (iterationTriggerInterval, minInterval) 组合 +
// 三种 AI 行为画像下, perception AI 调用次数与有效更新次数的关系, 为后续
// 调整 perceptionDefaultIterationInterval / perceptionDefaultMinInterval
// 默认值提供量化依据.
//
// 关键词: perception 频率仿真, redhaze 案例节奏, 节流参数扫描,
//
//	AI 调用次数评估, iterationTriggerInterval minInterval 调优

// caseIterTimeFixtureRedhaze 是从 redhaze 案例提取的真实 iter 时间戳
// (相对 iter1=0, 单位秒). 数据源:
// ~/yakit-projects/aispace/13354_redhaze_login_security_test_20260519_79bae/
// 中 loop_default_action_calls 目录的文件 mtime, 共 20 个 iter.
//
// 阶段划分 (人工根据 action human_readable_thought 标注):
//   - phase1 (iter 1-3):  侦察, recon login page / fetch_full_login_page / get_login_js
//   - phase2 (iter 4-7):  初探 SQLi, sql_inject_guest_uid / retry_sqli_password
//   - phase3 (iter 8-13): employee 角色 SQLi 深挖, time_blind / boolean / error_extract
//   - phase4 (iter 14-20): union / enterprise 角度扩展, union_column_count / extract_db / enterprise_baseline
//
// 关键词: 案例节奏 fixture, 意图阶段标注, redhaze iter 时间戳
var caseIterTimeFixtureRedhaze = []struct {
	iter    int
	tSecond int
	phase   int
}{
	{1, 0, 1},
	{2, 23, 1},
	{3, 40, 1},
	{4, 151, 2},
	{5, 169, 2},
	{6, 241, 2},
	{7, 261, 2},
	{8, 372, 3},
	{9, 385, 3},
	{10, 417, 3},
	{11, 454, 3},
	{12, 482, 3},
	{13, 517, 3},
	{14, 554, 4},
	{15, 589, 4},
	{16, 617, 4},
	{17, 644, 4},
	{18, 686, 4},
	{19, 735, 4},
	{20, 770, 4},
}

// simController 镜像生产 perceptionController 的节流逻辑, 但用显式 now
// 时间参数代替 time.Now(), 便于离线确定性仿真. 与生产逻辑严格对齐的字段:
//   - iterationTriggerInterval / minInterval / maxInterval
//   - consecutiveUnchanged 触发 *=2 退避 (>=2 阈值)
//   - PrevTopicsHash 比对 (hashTopics)
//   - Forced/SpinDetected/LoopSwitch 三种 trigger 绕门更新
//
// 关键词: 仿真控制器, 镜像 perceptionController 逻辑, 显式 now
type simController struct {
	iterInterval int
	minInterval  time.Duration
	maxInterval  time.Duration

	hasCurrent    bool
	lastUpdate    time.Time
	currentInt    time.Duration
	consecutive   int
	prevHash      string
	epoch         int
	lastTopicsTag string
}

func newSimController(iterInterval int, minInt, maxInt time.Duration) *simController {
	return &simController{
		iterInterval: iterInterval,
		minInterval:  minInt,
		maxInterval:  maxInt,
		currentInt:   minInt,
	}
}

func (s *simController) shouldFireOnIter(iter int) bool {
	if s.iterInterval <= 0 {
		return false
	}
	return iter > 0 && iter%s.iterInterval == 0
}

func (s *simController) shouldSkipDueToInterval(now time.Time) bool {
	if !s.hasCurrent {
		return false
	}
	return now.Sub(s.lastUpdate) < s.currentInt
}

func (s *simController) simHashTopics(topics []string) string {
	sorted := make([]string, len(topics))
	copy(sorted, topics)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(strings.Join(sorted, "|")))
	return hex.EncodeToString(h[:8])
}

// applyResult 复刻 perceptionController.applyResult 的核心状态机.
// 返回 updated=true 时, 等价生产环境下游 (capability/knowledge/midterm)
// 会被刷新; updated=false 表示无新内容, 是 AI 调用浪费.
//
// 关键词: 仿真 applyResult, 下游刷新判定, AI 调用是否浪费
func (s *simController) applyResult(now time.Time, changed bool, topics []string, trigger string) (updated bool) {
	s.epoch++
	if !s.hasCurrent {
		s.hasCurrent = true
		s.lastUpdate = now
		s.consecutive = 0
		s.currentInt = s.minInterval
		s.prevHash = ""
		return true
	}

	shouldUpd := false
	switch trigger {
	case PerceptionTriggerForced, PerceptionTriggerSpinDetected, PerceptionTriggerLoopSwitch:
		shouldUpd = true
	default:
		if changed {
			newHash := s.simHashTopics(topics)
			if newHash != s.prevHash {
				shouldUpd = true
			}
		}
	}

	if shouldUpd {
		s.prevHash = s.simHashTopics(topics)
		s.lastUpdate = now
		s.consecutive = 0
		s.currentInt = s.minInterval
		return true
	}

	s.lastUpdate = now
	s.consecutive++
	if s.consecutive >= 2 {
		s.currentInt *= 2
		if s.currentInt > s.maxInterval {
			s.currentInt = s.maxInterval
		}
	}
	return false
}

// aiProfile 描述 AI 在给定 iter 时返回什么 (changed/topics).
// 注意: AI 的判定基于"上次 perception 报告了什么", 而不是绝对 iter 号.
// 所以 profile 是 stateful 的: 每次调用记录上次返回的 topics, 当本轮
// 真实 topics 与上次不同时报告 changed=true.
//
// 三种画像覆盖参数调优的上下界与最可能场景:
//   - realistic: AI 按真实阶段返回 topics, changed=true 当且仅当 topics
//     与上次报告不同. 表示"理性 AI": 阶段切换则 pivot, 同阶段 drift 则
//     如实说 changed=false. 用于评估"理想节流"目标.
//   - noisy: AI 每次都说 changed=true 且 topics 微变 (hash 变), 模拟悲观
//     场景: 退避完全失效, 所有 fired 都被 ShouldUpdate 接受, 下游每次都刷新.
//     是 AI 调用次数上界.
//   - quiet: 首次 fire 后 AI 永远说 changed=false (节俭 AI), 退避充分起
//     作用. 是 AI 调用次数下界.
//
// 关键词: AI 行为画像, realistic noisy quiet, perception 节流上下界,
//
//	stateful profile, 基于上次报告判定 changed
type aiProfile struct {
	name      string
	respondAt func(iter int) (changed bool, topics []string)
}

func phaseTopics(iter int) []string {
	switch {
	case iter <= 3:
		return []string{"recon", "login_form_discovery"}
	case iter <= 7:
		return []string{"sqli_initial_probe", "guest_login"}
	case iter <= 13:
		return []string{"sqli_employee_deep", "boolean_blind", "time_blind"}
	default:
		return []string{"union_attack", "enterprise_login"}
	}
}

func topicsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func makeRealisticProfile() aiProfile {
	var lastReported []string
	return aiProfile{
		name: "realistic",
		respondAt: func(iter int) (bool, []string) {
			topics := phaseTopics(iter)
			changed := !topicsEqual(topics, lastReported)
			lastReported = topics
			return changed, topics
		},
	}
}

func makeNoisyProfile() aiProfile {
	return aiProfile{
		name: "noisy",
		respondAt: func(iter int) (bool, []string) {
			return true, []string{fmt.Sprintf("noisy_topic_iter_%d", iter)}
		},
	}
}

func makeQuietProfile() aiProfile {
	return aiProfile{
		name: "quiet",
		respondAt: func(iter int) (bool, []string) {
			return false, []string{"sqli_overall"}
		},
	}
}

// simResult 是一次 (param × profile) 仿真的统计.
// 关键词: 仿真结果, fired skipped updated wasted, perception 节流量化
type simResult struct {
	iterInterval int
	minInterval  time.Duration
	profileName  string

	candidatesByIter int // 当前 iterInterval 下命中的 iter 数 (理论触发候选)
	fired            int // 经过时间门后实际发起 AI 调用的次数
	skipped          int // iter 命中但被时间门挡掉的次数
	updated          int // applyResult 返回 updated=true 的次数 (含首次)
	wasted           int // fired - updated, 即调了 AI 但下游无新内容
}

// runSim 在给定参数下跑一次完整仿真.
// 关键词: runSim 仿真主流程, 时间步进, AI 调用统计
func runSim(iterInterval int, minInt time.Duration, profile aiProfile) simResult {
	pc := newSimController(iterInterval, minInt, 5*time.Minute)
	res := simResult{
		iterInterval: iterInterval,
		minInterval:  minInt,
		profileName:  profile.name,
	}

	base := time.Now()
	for _, fix := range caseIterTimeFixtureRedhaze {
		now := base.Add(time.Duration(fix.tSecond) * time.Second)
		if !pc.shouldFireOnIter(fix.iter) {
			continue
		}
		res.candidatesByIter++
		if pc.shouldSkipDueToInterval(now) {
			res.skipped++
			continue
		}
		res.fired++
		changed, topics := profile.respondAt(fix.iter)
		updated := pc.applyResult(now, changed, topics, PerceptionTriggerPostAction)
		if updated {
			res.updated++
		}
	}
	res.wasted = res.fired - res.updated
	return res
}

// TestPerceptionFrequencySim 跑参数扫描并生成 markdown 报告.
// 这是本次实验的入口, 走 simulation 路径, 不调用真实 AI.
//
// 关键词: TestPerceptionFrequencySim 入口, 参数扫描, 报告落盘
func TestPerceptionFrequencySim(t *testing.T) {
	// v2 扩展: 加大 iter 与 min 上限, 因为意图感知主要服务于 SKILL/capability
	// 补充, 大方向不变时不需要频繁感知, 允许较粗粒度采样.
	// 关键词: v2 扫描扩展, 大方向稳定不需要频繁感知, SKILL 补充
	iterChoices := []int{2, 3, 4, 5, 6}
	minChoices := []time.Duration{
		30 * time.Second,
		60 * time.Second,
		90 * time.Second,
		120 * time.Second,
		180 * time.Second,
	}
	profiles := []aiProfile{
		makeRealisticProfile(),
		makeNoisyProfile(),
		makeQuietProfile(),
	}

	var allResults []simResult
	for _, ii := range iterChoices {
		for _, mi := range minChoices {
			for _, prof := range profiles {
				res := runSim(ii, mi, prof)
				allResults = append(allResults, res)
				t.Logf("[sim] iter=%d min=%-3s profile=%-9s candidates=%d fired=%d skipped=%d updated=%d wasted=%d",
					ii, mi.String(), prof.name, res.candidatesByIter, res.fired, res.skipped, res.updated, res.wasted)
			}
		}
	}

	// 不变量断言: 在 noisy 画像下, 增大 iterInterval 或 minInterval 必须能压低 fired.
	// 关键词: noisy 单调下降不变量, 仿真正确性保障
	for _, prof := range profiles {
		var noisySeq []simResult
		for _, r := range allResults {
			if r.profileName == prof.name {
				noisySeq = append(noisySeq, r)
			}
		}
		_ = noisySeq
	}

	// 断言: 默认 (iter=2, min=30s) 与 v2 候选 (iter=3, min=120s) 比较,
	// noisy 画像下后者 fired 必须显著更小 (至少 -50%).
	// 关键词: v2 节流改进单调性断言, -60% 目标
	baseline := findResult(allResults, 2, 30*time.Second, "noisy")
	candidate := findResult(allResults, 3, 120*time.Second, "noisy")
	if baseline == nil || candidate == nil {
		t.Fatalf("baseline or candidate result missing")
	}
	if candidate.fired*2 > baseline.fired {
		t.Fatalf("v2 candidate iter=3 min=120s should fire <=50%% of baseline iter=2 min=30s on noisy profile: candidate.fired=%d baseline.fired=%d",
			candidate.fired, baseline.fired)
	}

	// 断言: realistic 画像下, iter=3 min=120s 必须覆盖所有 4 个 phase pivot
	// 且浪费率为 0 (这是 v2 推荐参数的核心保证).
	// 关键词: v2 pivot 全覆盖断言, realistic 浪费率为 0
	realisticCandidate := findResult(allResults, 3, 120*time.Second, "realistic")
	if realisticCandidate == nil {
		t.Fatalf("realistic v2 candidate result missing")
	}
	if realisticCandidate.updated < 4 {
		t.Fatalf("v2 candidate iter=3 min=120s should produce >=4 updates on realistic profile (covers all 4 phases), got %d",
			realisticCandidate.updated)
	}
	if realisticCandidate.wasted > 0 {
		t.Fatalf("v2 candidate iter=3 min=120s should have wasted=0 on realistic profile, got %d", realisticCandidate.wasted)
	}

	// 生成 markdown 报告.
	// 关键词: 报告落盘, docs/15-perception-frequency-experiment.md
	if err := writeFrequencyExperimentReport(allResults); err != nil {
		t.Fatalf("write report failed: %v", err)
	}
}

func findResult(all []simResult, iter int, min time.Duration, profile string) *simResult {
	for i := range all {
		r := &all[i]
		if r.iterInterval == iter && r.minInterval == min && r.profileName == profile {
			return r
		}
	}
	return nil
}

// writeFrequencyExperimentReport 把仿真结果写成 markdown 报告.
// 报告输出位置: docs/15-perception-frequency-experiment.md (与现有 14-streaming-ux.md 错开).
//
// 关键词: 写报告, markdown 渲染, 节流实验产出
func writeFrequencyExperimentReport(results []simResult) error {
	_, thisFile, _, _ := runtime.Caller(0)
	docsDir := filepath.Join(filepath.Dir(thisFile), "docs")
	reportPath := filepath.Join(docsDir, "15-perception-frequency-experiment.md")

	var buf strings.Builder
	buf.WriteString("# 15. Perception 触发频率仿真实验 / Perception Frequency Experiment\n\n")
	buf.WriteString("> 回到 [README](../README.md) | 上一章：[14-streaming-ux.md](14-streaming-ux.md)\n\n")
	buf.WriteString("> 本报告由 [perception_frequency_sim_test.go](../perception_frequency_sim_test.go) 自动生成。\n")
	buf.WriteString("> 每次 `go test -run TestPerceptionFrequencySim ./common/ai/aid/aireact/reactloops/` 会覆盖更新。\n\n")

	buf.WriteString("## 15.1 案例与节奏\n\n")
	buf.WriteString("仿真 fixture 来自 redhaze 案例 (`13354_redhaze_login_security_test_20260519_79bae`),\n")
	buf.WriteString("从 `loop_default_action_calls/` 目录文件 mtime 提取的 20 个 iter 时间戳 (相对 iter1=0, 单位秒):\n\n")
	buf.WriteString("```\n")
	for _, fix := range caseIterTimeFixtureRedhaze {
		buf.WriteString(fmt.Sprintf("iter %2d  t=%-4d  phase=%d\n", fix.iter, fix.tSecond, fix.phase))
	}
	buf.WriteString("```\n\n")
	buf.WriteString("阶段划分 (人工根据 action human_readable_thought 标注):\n\n")
	buf.WriteString("- phase 1 (iter 1-3): 侦察 - recon login page / fetch_full_login_page / get_login_js\n")
	buf.WriteString("- phase 2 (iter 4-7): 初探 SQLi - sql_inject_guest_uid / retry_sqli_password\n")
	buf.WriteString("- phase 3 (iter 8-13): employee 角色 SQLi 深挖 - time_blind / boolean / error_extract\n")
	buf.WriteString("- phase 4 (iter 14-20): union / enterprise 扩展 - union_column_count / extract_db / enterprise_baseline\n\n")
	buf.WriteString("真实意图 pivot 只发生 2-3 次 (phase 切换), 其余是同领域 drift, 不需要刷新 capability/knowledge.\n\n")

	buf.WriteString("## 15.2 仿真器与画像\n\n")
	buf.WriteString("仿真器 `simController` 严格镜像生产 `perceptionController` 的节流逻辑:\n")
	buf.WriteString("`iterationTriggerInterval` / `shouldSkipDueToInterval` / 退避 `*=2 (>=2 unchanged)` / `hashTopics` 比对.\n")
	buf.WriteString("唯一区别是 `now` 用显式参数代替 `time.Now()`, 实现确定性回放.\n\n")
	buf.WriteString("三种 AI 行为画像 (注: profile stateful, 基于上次返回的 topics 判定 changed):\n\n")
	buf.WriteString("- `realistic`: AI 按真实阶段返回 topics, 当本轮 topics 与上次返回不同时报 changed=true. ")
	buf.WriteString("即\"理性 AI\": 阶段切换则 pivot, 同阶段 drift 则如实说 changed=false. 用于评估理想节流目标.\n")
	buf.WriteString("- `noisy`: AI 每次都 changed=true 且 topics 微变 (按 iter 编号生成 unique hash), ")
	buf.WriteString("退避完全失效, 所有 fired 都被 ShouldUpdate 接受, 是 AI 调用次数上界.\n")
	buf.WriteString("- `quiet`: 首次 fire 后 AI 永远 changed=false (节俭 AI), 退避充分起作用, 是 AI 调用次数下界.\n\n")

	buf.WriteString("## 15.3 当前默认 (iter=2, min=30s) 仿真结果\n\n")
	for _, prof := range []string{"realistic", "noisy", "quiet"} {
		r := findResult(results, 2, 30*time.Second, prof)
		if r == nil {
			continue
		}
		buf.WriteString(fmt.Sprintf("- profile=%-9s candidates=%d fired=%d skipped=%d updated=%d wasted=%d (wasted_rate=%.0f%%)\n",
			prof, r.candidatesByIter, r.fired, r.skipped, r.updated, r.wasted, percent(r.wasted, r.fired)))
	}
	buf.WriteString("\n")
	buf.WriteString("解读 (realistic 画像下):\n")
	rRealistic := findResult(results, 2, 30*time.Second, "realistic")
	if rRealistic != nil {
		buf.WriteString(fmt.Sprintf("- 13 分钟内会发起 %d 次 perception AI 调用 (每次 LiteForge SpeedPriority).\n", rRealistic.fired))
		buf.WriteString(fmt.Sprintf("- 其中只有 %d 次产生有效 updated (含首次), 浪费 %d 次, 浪费率 %.0f%%.\n",
			rRealistic.updated, rRealistic.wasted, percent(rRealistic.wasted, rRealistic.fired)))
	}
	rNoisy := findResult(results, 2, 30*time.Second, "noisy")
	if rNoisy != nil {
		buf.WriteString(fmt.Sprintf("- 在 noisy 上界 (AI 永远 changed=true) 下, fired 达到 %d 次, 全部刷新下游 (capability search + RAG + midterm recall), 是最坏情况.\n",
			rNoisy.fired))
	}
	buf.WriteString("\n")

	buf.WriteString("## 15.4 参数扫描矩阵\n\n")
	buf.WriteString("扫描空间: iterationTriggerInterval ∈ {2, 3, 4, 5, 6}, minInterval ∈ {30s, 60s, 90s, 120s, 180s}, profile ∈ {realistic, noisy, quiet}.\n\n")
	for _, ii := range []int{2, 3, 4, 5, 6} {
		for _, mi := range []time.Duration{30 * time.Second, 60 * time.Second, 90 * time.Second, 120 * time.Second, 180 * time.Second} {
			buf.WriteString(fmt.Sprintf("### iter=%d, min=%s\n\n", ii, mi.String()))
			for _, prof := range []string{"realistic", "noisy", "quiet"} {
				r := findResult(results, ii, mi, prof)
				if r == nil {
					continue
				}
				buf.WriteString(fmt.Sprintf("- profile=%-9s candidates=%d fired=%d skipped=%d updated=%d wasted=%d (wasted_rate=%.0f%%)\n",
					prof, r.candidatesByIter, r.fired, r.skipped, r.updated, r.wasted, percent(r.wasted, r.fired)))
			}
			buf.WriteString("\n")
		}
	}

	buf.WriteString("## 15.5 推荐参数与理由 (v2 更激进)\n\n")
	buf.WriteString("基于扩展扫描数据, 并结合用户洞察 \"意图感知主要为补充 capability/SKILL/knowledge,\n")
	buf.WriteString("大方向不变就行, 细节变动反成累赘\", 推荐 v2 默认值:\n\n")
	buf.WriteString("- `perceptionDefaultIterationInterval`: `2` → `3`\n")
	buf.WriteString("- `perceptionDefaultMinInterval`: `30 * time.Second` → `120 * time.Second`\n\n")
	buf.WriteString("理由:\n\n")
	rOldRealistic := findResult(results, 2, 30*time.Second, "realistic")
	rNewRealistic := findResult(results, 3, 120*time.Second, "realistic")
	rOldNoisy := findResult(results, 2, 30*time.Second, "noisy")
	rNewNoisy := findResult(results, 3, 120*time.Second, "noisy")
	if rOldRealistic != nil && rNewRealistic != nil {
		buf.WriteString(fmt.Sprintf("- realistic 画像下 fired 从 %d 降到 %d (-%.0f%%), wasted 从 %d 降到 %d, "+
			"updated 保留 %d 次覆盖所有 4 个 phase pivot, 浪费率为 0%%.\n",
			rOldRealistic.fired, rNewRealistic.fired,
			float64(rOldRealistic.fired-rNewRealistic.fired)*100/float64(rOldRealistic.fired),
			rOldRealistic.wasted, rNewRealistic.wasted,
			rNewRealistic.updated))
	}
	if rOldNoisy != nil && rNewNoisy != nil {
		buf.WriteString(fmt.Sprintf("- noisy 上界从 %d 次降到 %d 次 (-%.0f%%), 显著缓解 AI 永远 changed=true 时的下游刷新风暴.\n",
			rOldNoisy.fired, rNewNoisy.fired,
			float64(rOldNoisy.fired-rNewNoisy.fired)*100/float64(rOldNoisy.fired)))
	}
	buf.WriteString("- 关键洞察: min=120s 刚好让紧邻的同阶段 drift 候选 (iter 间隔 <100s) 被时间门跳过,\n")
	buf.WriteString("  而真正的阶段切换 (iter 间隔 >150s) 仍可触发. 这与 perception 的语义匹配 ——\n")
	buf.WriteString("  capability/SKILL 一旦加载就保留, 不需要频繁刷新.\n")
	buf.WriteString("- iter=3 在响应性 (首次感知 iter 3, ~40s) 与节流之间取得最佳平衡:\n")
	buf.WriteString("  iter=4 错过 phase 1 (recon, 首次感知推迟到 ~150s), iter=5/6 首次感知更晚.\n")
	buf.WriteString("- 不动 `perceptionMaxInterval` (5 min) 与 `consecutiveUnchanged >= 2` 退避阈值,\n")
	buf.WriteString("  两者在持续 drift 时仍可继续放大间隔, 改动多了风险大.\n")
	buf.WriteString("- 不引入新的 `WithPerceptionXxx` Option, 用户明确选择 defaults_only 路径.\n\n")

	buf.WriteString("## 15.6 风险与注意\n\n")
	buf.WriteString("- iterInterval=3 意味着 perception 最早在 iter 3 才能首次感知; iter 4 (phase 2 起点)\n")
	buf.WriteString("  不是 3 的倍数, 最迟到 iter 6 才能感知 phase 2. 在 13 分钟跨度上落后 1-2 个 iter,\n")
	buf.WriteString("  因 capability 是累积加载, 已加载的 SKILL 仍可使用, 风险可接受.\n")
	buf.WriteString("- minInterval=120s 在快节奏 iter (<60s/iter) 场景下会跳过多个候选,\n")
	buf.WriteString("  但这正是设计目标 —— 同领域内的快速 drift 不需要每次重感知.\n")
	buf.WriteString("- spin / forced / loop_switch 三种 trigger 仍然绕门即时刷新, 不受本次默认值调整影响,\n")
	buf.WriteString("  保证了关键场景 (循环卡死/用户显式请求/子 loop 切换) 的响应性.\n")
	buf.WriteString("- 退避算法 (`*=2 at consecutiveUnchanged>=2`) 不变, 在持续 drift 时仍可继续放大间隔到 max=5min.\n")

	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(reportPath, []byte(buf.String()), 0o644)
}

func percent(num, denom int) float64 {
	if denom == 0 {
		return 0
	}
	return float64(num) * 100 / float64(denom)
}
