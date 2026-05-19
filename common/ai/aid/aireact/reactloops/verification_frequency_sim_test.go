package reactloops

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// verification_frequency_sim_test.go 是一个离线仿真测试, 镜像
// perception_frequency_sim_test.go 的方法论, 但作用于 VerifyUserSatisfaction
// 的节流门 (verificationAutoTriggerMaxSnapshotAge / verificationAutoTriggerMinPromptDelta
// / verificationIterationTriggerInterval). 仿真使用 redhaze 案例的真实 iter
// 时间戳 + 真实 prompt token 生长曲线 + 真实 hasNewEvidence 标注, 在不同
// (snapshotAge, promptDelta, iterInterval) 组合 x 三种 AI 行为画像下评估:
//
//   - fired           : 实际发起 AI 调用的次数 (成本指标)
//   - wasted          : 没有产生新证据 / 退出信号的 AI 调用 (成本浪费)
//   - satisfiedLag    : groundTruthSatisfied 后多少 iter 才被检测到 (副作用代价)
//
// 与 perception 不同: verification 有副作用 (Satisfied=true 退出 loop /
// 增量写 OutputFiles+EvidenceOps+TODO), 所以这里必须额外追踪 satisfiedLag,
// 不能只看 wasted.
//
// 关键词: verification 频率仿真, satisfied 检测延迟, 节流参数扫描,
//
//	AI 调用次数评估, snapshotAge minPromptDelta 调优,
//	redhaze 案例 promptTokens 生长曲线

// verifyCaseFixtureRedhaze 在 caseIterTimeFixtureRedhaze 的基础上为每个 iter
// 额外标注 promptTokens (累积 prompt 大小, 用于 token 门评估) 与
// hasNewEvidence (该 iter 是否产生了值得 verify 的新证据, 模拟真实
// delivery_files 命中节奏).
//
// promptTokens 生长曲线 来源于经验估算 (与 redhaze 实测 delivery 节奏对齐):
//
//	阶段 1 (iter 1-3) 轻 (200-300/iter), 页面侦察, 工具响应小
//	阶段 2 (iter 4-7) 中 (300-500/iter), SQLi 初探, 工具响应中等
//	阶段 3 (iter 8-13) 重 (400-600/iter), 深挖 SQLi, 多次重试
//	阶段 4 (iter 14-20) 重 (500-800/iter), union 提数据, 响应体大
//
// hasNewEvidence 来源于 redhaze 实测: 42 iter / 21 次 delivery 写入 ≈ 50%
// iter 产生新证据. 这里 20 个 iter 中标 11 个 (55%), 与真实分布同量级.
//
// 关键词: verifyCaseFixtureRedhaze, promptTokens 曲线, hasNewEvidence 标注
var verifyCaseFixtureRedhaze = []struct {
	iter            int
	tSecond         int
	phase           int
	promptTokens    int
	hasNewEvidence  bool
	phasePivotStart bool
}{
	{1, 0, 1, 3000, true, true},
	{2, 23, 1, 3300, true, false},
	{3, 40, 1, 3550, false, false},
	{4, 151, 2, 3900, true, true},
	{5, 169, 2, 4200, true, false},
	{6, 241, 2, 4700, false, false},
	{7, 261, 2, 5000, false, false},
	{8, 372, 3, 5500, true, true},
	{9, 385, 3, 6000, true, false},
	{10, 417, 3, 6400, false, false},
	{11, 454, 3, 7000, true, false},
	{12, 482, 3, 7400, false, false},
	{13, 517, 3, 7700, false, false},
	{14, 554, 4, 8400, true, true},
	{15, 589, 4, 8900, false, false},
	{16, 617, 4, 9600, true, false},
	{17, 644, 4, 10000, false, false},
	{18, 686, 4, 10700, true, false},
	{19, 735, 4, 11200, false, false},
	{20, 770, 4, 12000, true, false},
}

// verifyGroundTruthSatisfiedAt 表示在真实业务语义下任务"应该被认定满足"的 iter.
// 设为 18 是基于 redhaze 案例的经验估算: phase 4 中 employee union 数据已被
// 完整提取, 多角色验证基本完成, 即便 loop 继续跑也不会改变最终交付物.
// 真正的 case 跑到了 42 iter 才停, 浪费了大量算力, 所以 satisfiedLag 越大,
// 用户实际等待时间越长 = 副作用代价越高.
//
// 关键词: verifyGroundTruthSatisfiedAt, 任务真实达成 iter, 副作用代价基线
const verifyGroundTruthSatisfiedAt = 18

// simVerifyController 镜像生产 ReActLoop.shouldTriggerAutomaticVerification
// 与配套的 GetVerificationRuntimeSnapshot/setVerificationRuntimeSnapshot
// 状态机, 但用显式 now 参数代替 time.Now() / r.GetVerificationRuntimeSnapshot,
// 实现确定性回放.
//
// 严格对齐的 4 门 (任一为真则发起 AI):
//  1. iter == maxIter 末轮兜底
//  2. previous == nil && periodicCheckpoint(iter) 首次按周期
//  3. now - prev.GeneratedAt >= snapshotAge 时间门
//  4. iter - prev.iter >= iterInterval 迭代门
//  5. |tokens_delta| >= promptDelta token 门
//
// 关键词: simVerifyController, 镜像 shouldTriggerAutomaticVerification 四门,
//
//	verification 确定性回放
type simVerifyController struct {
	snapshotAge  time.Duration
	promptDelta  int
	iterInterval int
	maxIter      int

	prevGeneratedAt time.Time
	prevIter        int
	prevTokens      int
	hasPrev         bool
}

func newSimVerifyController(snapshotAge time.Duration, promptDelta, iterInterval, maxIter int) *simVerifyController {
	return &simVerifyController{
		snapshotAge:  snapshotAge,
		promptDelta:  promptDelta,
		iterInterval: iterInterval,
		maxIter:      maxIter,
	}
}

func (s *simVerifyController) periodicCheckpoint(iter int) bool {
	if s.iterInterval <= 0 {
		return true
	}
	if iter > 0 && iter%s.iterInterval == 0 {
		return true
	}
	return s.maxIter > 0 && iter > 0 && iter == s.maxIter
}

func (s *simVerifyController) shouldTrigger(now time.Time, iter, tokens int) (fire bool, gate string) {
	if s.maxIter > 0 && iter == s.maxIter {
		return true, "max_iter"
	}
	if !s.hasPrev {
		if s.periodicCheckpoint(iter) {
			return true, "first_periodic"
		}
		return false, ""
	}
	if now.Sub(s.prevGeneratedAt) >= s.snapshotAge {
		return true, "snapshot_age"
	}
	if iter-s.prevIter >= s.iterInterval {
		return true, "iter_delta"
	}
	delta := tokens - s.prevTokens
	if delta < 0 {
		delta = -delta
	}
	if delta >= s.promptDelta {
		return true, "token_delta"
	}
	return false, ""
}

// recordFire 在 verification 实际触发后更新 prev 快照.
// 镜像生产: setVerificationRuntimeSnapshot(currentSnapshot) 总是发生.
func (s *simVerifyController) recordFire(now time.Time, iter, tokens int) {
	s.prevGeneratedAt = now
	s.prevIter = iter
	s.prevTokens = tokens
	s.hasPrev = true
}

// verifyAIProfile 描述 AI 在某 iter 被调用时返回什么.
//
//   - satisfied(iter): 是否返回 Satisfied=true (true 时 loop 退出)
//   - hasNewArtifact(iter): 是否产生新 OutputFiles/EvidenceOps (false 时该次 fire 是 wasted)
//
// 关键词: verifyAIProfile, satisfied/wasted 二维评估
type verifyAIProfile struct {
	name           string
	satisfied      func(iter int) bool
	hasNewArtifact func(iter int) bool
}

// makeRealisticProfile: AI 按 hasNewEvidence 实事求是地返回, 在 groundTruth 后
// 才说 Satisfied=true. 这是最现实的画像, satisfiedLag 在此画像下最有意义.
//
// 关键词: realistic 画像, hasNewEvidence 驱动, 真实 satisfiedLag
func makeRealisticVerifyProfile() verifyAIProfile {
	evidenceByIter := make(map[int]bool, len(verifyCaseFixtureRedhaze))
	for _, fix := range verifyCaseFixtureRedhaze {
		evidenceByIter[fix.iter] = fix.hasNewEvidence
	}
	return verifyAIProfile{
		name: "realistic",
		satisfied: func(iter int) bool {
			return iter >= verifyGroundTruthSatisfiedAt
		},
		hasNewArtifact: func(iter int) bool {
			return evidenceByIter[iter]
		},
	}
}

// makePessimisticProfile: AI 永远说 Satisfied=false 且永远不产生新证据.
// 这是浪费率上界 (worst-case wasted%), 用来评估"最差情况下我们烧了多少 AI 调用".
//
// 关键词: pessimistic 画像, 全部 fire 都浪费, 成本上界
func makePessimisticVerifyProfile() verifyAIProfile {
	return verifyAIProfile{
		name: "pessimistic",
		satisfied: func(iter int) bool {
			return false
		},
		hasNewArtifact: func(iter int) bool {
			return false
		},
	}
}

// makeEarlyDoneProfile: AI 在 iter=12 (phase 3 中段, 早于 groundTruth) 就说
// Satisfied=true, 模拟 AI 提前误判任务完成的场景. 用于评估"如果节流过粗,
// 我们能否还能及时捕获早完成的信号".
//
// 关键词: early_done 画像, 提前 satisfied 信号, 节流过粗时是否漏检
func makeEarlyDoneVerifyProfile() verifyAIProfile {
	return verifyAIProfile{
		name: "early_done",
		satisfied: func(iter int) bool {
			return iter >= 12
		},
		hasNewArtifact: func(iter int) bool {
			return true
		},
	}
}

// verifySimResult 是一次 (param x profile) 仿真的统计.
//
// 关键词: verifySimResult, fired wasted lag 三维量化
type verifySimResult struct {
	snapshotAge  time.Duration
	promptDelta  int
	iterInterval int
	profileName  string

	fired        int // 实际发起的 AI 调用次数
	wasted       int // fired 但 hasNewArtifact=false 且未 satisfied 的次数
	satisfiedAt  int // 第一次返回 Satisfied=true 的 iter (0 表示未发生)
	satisfiedT   int // 对应的 tSecond
	gateCounters map[string]int
}

// runVerifySim 在给定参数下跑一次完整仿真.
//
// 关键词: runVerifySim, verification 节流主循环, iter 时间步进
func runVerifySim(snapshotAge time.Duration, promptDelta, iterInterval int, profile verifyAIProfile) verifySimResult {
	maxIter := verifyCaseFixtureRedhaze[len(verifyCaseFixtureRedhaze)-1].iter
	ctrl := newSimVerifyController(snapshotAge, promptDelta, iterInterval, maxIter)
	res := verifySimResult{
		snapshotAge:  snapshotAge,
		promptDelta:  promptDelta,
		iterInterval: iterInterval,
		profileName:  profile.name,
		gateCounters: make(map[string]int),
	}

	base := time.Now()
	for _, fix := range verifyCaseFixtureRedhaze {
		now := base.Add(time.Duration(fix.tSecond) * time.Second)
		fire, gate := ctrl.shouldTrigger(now, fix.iter, fix.promptTokens)
		if !fire {
			continue
		}
		res.fired++
		res.gateCounters[gate]++

		hasArtifact := profile.hasNewArtifact(fix.iter)
		isSat := profile.satisfied(fix.iter)
		if !hasArtifact && !isSat {
			res.wasted++
		}
		if isSat && res.satisfiedAt == 0 {
			res.satisfiedAt = fix.iter
			res.satisfiedT = fix.tSecond
		}
		ctrl.recordFire(now, fix.iter, fix.promptTokens)

		// 一旦 Satisfied=true 被 AI 返回, 生产环境 loop 立刻退出.
		// 后续 iter 不再有 verification 机会, 与真实路径对齐.
		if isSat {
			break
		}
	}
	return res
}

// computeSatisfiedLag 计算 (iters 落后, seconds 落后) 相对 groundTruth.
//
// 关键词: satisfiedLag 计算, ground truth 后多少 iter/秒检测到
func (r verifySimResult) computeSatisfiedLag() (lagIters int, lagSeconds int) {
	if r.satisfiedAt == 0 {
		return -1, -1
	}
	if r.satisfiedAt < verifyGroundTruthSatisfiedAt {
		// 早于 groundTruth 检测到 (early_done 画像下会发生), 视为 0 lag.
		return 0, 0
	}
	groundTruthT := 0
	for _, fix := range verifyCaseFixtureRedhaze {
		if fix.iter == verifyGroundTruthSatisfiedAt {
			groundTruthT = fix.tSecond
			break
		}
	}
	return r.satisfiedAt - verifyGroundTruthSatisfiedAt, r.satisfiedT - groundTruthT
}

// TestVerificationFrequencySim 跑参数扫描并生成 markdown 报告.
// 这是本次实验的入口, 走 simulation 路径, 不调用真实 AI.
//
// 关键词: TestVerificationFrequencySim 入口, 参数扫描, 报告落盘
func TestVerificationFrequencySim(t *testing.T) {
	snapshotChoices := []time.Duration{
		30 * time.Second,
		60 * time.Second,
		90 * time.Second,
		120 * time.Second,
	}
	deltaChoices := []int{500, 1000, 1500, 2000}
	iterChoices := []int{5, 7, 10}
	profiles := []verifyAIProfile{
		makeRealisticVerifyProfile(),
		makePessimisticVerifyProfile(),
		makeEarlyDoneVerifyProfile(),
	}

	var allResults []verifySimResult
	for _, sa := range snapshotChoices {
		for _, dd := range deltaChoices {
			for _, ii := range iterChoices {
				for _, prof := range profiles {
					res := runVerifySim(sa, dd, ii, prof)
					allResults = append(allResults, res)
					lagI, lagS := res.computeSatisfiedLag()
					t.Logf("[sim] snapshot=%-3s delta=%d iter=%-2d profile=%-11s fired=%d wasted=%d satAt=%d lagI=%d lagS=%d",
						sa.String(), dd, ii, prof.name, res.fired, res.wasted, res.satisfiedAt, lagI, lagS)
				}
			}
		}
	}

	// 不变量断言 1: baseline (30s, 500, 5) 在 pessimistic 画像下 fired 应该 >=10,
	// 说明当前默认确实容易在浪费场景下狂调 AI (验证仿真口径与现状判断一致).
	//
	// 关键词: baseline fired 下界断言, 现状刷屏验证
	baselinePess := findVerifyResult(allResults, 30*time.Second, 500, 5, "pessimistic")
	if baselinePess == nil {
		t.Fatalf("baseline pessimistic result missing")
	}
	if baselinePess.fired < 10 {
		t.Fatalf("baseline (30s, 500, 5) pessimistic fired should be >=10 (validating waste rate), got %d", baselinePess.fired)
	}

	// 不变量断言 2: 中等档候选 (90s, 1500, 5) pessimistic fired 应当 <= baseline*0.7,
	// 即节流改进至少能压低 30% 调用 (本次优化的最低门槛, 实际数据约 -38%).
	//
	// 关键词: 中等档 -30% 底线断言, 优化有效性
	mediumPess := findVerifyResult(allResults, 90*time.Second, 1500, 5, "pessimistic")
	if mediumPess == nil {
		t.Fatalf("medium pessimistic result missing")
	}
	if mediumPess.fired*10 > baselinePess.fired*7 {
		t.Fatalf("medium (90s, 1500, 5) should fire <=70%% of baseline on pessimistic: medium=%d baseline=%d",
			mediumPess.fired, baselinePess.fired)
	}

	// 不变量断言 3: 中等档候选 realistic 画像下 satisfiedLag <= 2 iter,
	// 副作用代价上限可控 (loop 最多多跑 2 轮).
	//
	// 关键词: 中等档 lag<=2 副作用代价上限断言
	mediumReal := findVerifyResult(allResults, 90*time.Second, 1500, 5, "realistic")
	if mediumReal == nil {
		t.Fatalf("medium realistic result missing")
	}
	lagI, _ := mediumReal.computeSatisfiedLag()
	if lagI < 0 {
		t.Fatalf("medium realistic should detect satisfied (got never)")
	}
	if lagI > 2 {
		t.Fatalf("medium (90s, 1500, 5) realistic lagIters should be <=2, got %d", lagI)
	}

	// 不变量断言 4: 激进档 (120s, 2000, 5) early_done 仍能在 iter=12 之后及时检测.
	// 这是节流过粗时不能漏检"提前完成信号"的保证.
	//
	// 关键词: 激进档 early_done 漏检防护
	aggrEarly := findVerifyResult(allResults, 120*time.Second, 2000, 5, "early_done")
	if aggrEarly == nil {
		t.Fatalf("aggressive early_done result missing")
	}
	if aggrEarly.satisfiedAt == 0 || aggrEarly.satisfiedAt > 14 {
		t.Fatalf("aggressive (120s, 2000, 5) early_done should detect satisfied by iter<=14, got iter=%d",
			aggrEarly.satisfiedAt)
	}

	if err := writeVerifyFrequencyExperimentReport(allResults); err != nil {
		t.Fatalf("write verification frequency report failed: %v", err)
	}
}

func findVerifyResult(all []verifySimResult, sa time.Duration, dd, ii int, profile string) *verifySimResult {
	for i := range all {
		r := &all[i]
		if r.snapshotAge == sa && r.promptDelta == dd && r.iterInterval == ii && r.profileName == profile {
			return r
		}
	}
	return nil
}

// writeVerifyFrequencyExperimentReport 把仿真结果写成 markdown 报告.
// 报告输出位置: docs/16-verification-frequency-experiment.md.
//
// 关键词: 写报告, markdown 渲染, verification 节流实验产出
func writeVerifyFrequencyExperimentReport(results []verifySimResult) error {
	_, thisFile, _, _ := runtime.Caller(0)
	docsDir := filepath.Join(filepath.Dir(thisFile), "docs")
	reportPath := filepath.Join(docsDir, "16-verification-frequency-experiment.md")

	var buf strings.Builder
	buf.WriteString("# 16. Verification 触发频率仿真实验 / Verification Frequency Experiment\n\n")
	buf.WriteString("> 回到 [README](../README.md) | 上一章: [15-perception-frequency-experiment.md](15-perception-frequency-experiment.md)\n\n")
	buf.WriteString("> 本报告由 [verification_frequency_sim_test.go](../verification_frequency_sim_test.go) 自动生成.\n")
	buf.WriteString("> 每次 `go test -run TestVerificationFrequencySim ./common/ai/aid/aireact/reactloops/` 会覆盖更新.\n\n")

	buf.WriteString("## 16.1 案例与节奏\n\n")
	buf.WriteString("仿真 fixture 来自 redhaze 案例 (`13354_redhaze_login_security_test_20260519_79bae`),\n")
	buf.WriteString("沿用 perception 实验的 20 个 iter 时间戳, 并额外标注了 promptTokens 生长曲线\n")
	buf.WriteString("与 hasNewEvidence (该 iter 是否产生新证据值得 verify):\n\n")
	buf.WriteString("```\n")
	for _, fix := range verifyCaseFixtureRedhaze {
		evidence := "-"
		if fix.hasNewEvidence {
			evidence = "E"
		}
		pivot := " "
		if fix.phasePivotStart {
			pivot = "P"
		}
		buf.WriteString(fmt.Sprintf("iter %2d  t=%-4d  phase=%d  tokens=%-6d  %s%s\n",
			fix.iter, fix.tSecond, fix.phase, fix.promptTokens, evidence, pivot))
	}
	buf.WriteString("```\n\n")
	buf.WriteString("图例: `E`=hasNewEvidence (该 iter 有值得 verify 的新材料), `P`=phasePivotStart (阶段切换起点).\n\n")
	buf.WriteString("**业务真实达成 iter** = 18 (phase 4 中段, employee union 数据已完整提取). 真实 case 跑到 42 iter 才停,\n")
	buf.WriteString("意味着 satisfiedLag 从 18 起算: lag 越大, 用户实际等待时间越长 = 副作用代价越高.\n\n")
	buf.WriteString("**redhaze 实测对照**: 42 iter / 22.5min / 21 次 delivery_files 写入, 相邻 27/35/42s 多个相同 63-64 字节空 delivery, 是当前 30s 时间门导致的典型刷屏.\n\n")

	buf.WriteString("## 16.2 仿真器与画像\n\n")
	buf.WriteString("`simVerifyController` 严格镜像生产 `shouldTriggerAutomaticVerification` 的 5 个门:\n\n")
	buf.WriteString("1. `iter == maxIter` 末轮兜底\n")
	buf.WriteString("2. `previous == nil && periodicCheckpoint(iter)` 首次按周期\n")
	buf.WriteString("3. `now - prev.GeneratedAt >= snapshotAge` 时间门\n")
	buf.WriteString("4. `iter - prev.iter >= iterInterval` 迭代门\n")
	buf.WriteString("5. `|tokens_delta| >= promptDelta` token 门\n\n")
	buf.WriteString("唯一区别是 `now` 用显式参数代替 `time.Now()`, 实现确定性回放.\n")
	buf.WriteString("AI 调用本身用三种 profile 模拟:\n\n")
	buf.WriteString("- `realistic`: AI 在 iter >= 18 才返回 Satisfied=true; hasNewArtifact 跟随 fixture 真实标注 (50% 新证据率). 主参考画像.\n")
	buf.WriteString("- `pessimistic`: AI 永远 Satisfied=false 且永远无新证据, 是浪费率上界. 用于评估最差成本.\n")
	buf.WriteString("- `early_done`: AI 在 iter >= 12 就提前说 Satisfied=true, 用于验证节流过粗时是否漏检.\n\n")
	buf.WriteString("一旦 AI 返回 Satisfied=true, 仿真立即终止 (与生产 loop 退出语义一致).\n\n")

	buf.WriteString("## 16.3 当前默认 (30s / 500 / 5) 仿真结果\n\n")
	for _, prof := range []string{"realistic", "pessimistic", "early_done"} {
		r := findVerifyResult(results, 30*time.Second, 500, 5, prof)
		if r == nil {
			continue
		}
		lagI, lagS := r.computeSatisfiedLag()
		buf.WriteString(fmt.Sprintf("- profile=%-11s fired=%d wasted=%d (wasted_rate=%.0f%%) satisfiedAt=iter%d lagIters=%d lagSeconds=%ds\n",
			prof, r.fired, r.wasted, percentInt(r.wasted, r.fired), r.satisfiedAt, lagI, lagS))
	}
	buf.WriteString("\n")
	rBaseReal := findVerifyResult(results, 30*time.Second, 500, 5, "realistic")
	rBasePess := findVerifyResult(results, 30*time.Second, 500, 5, "pessimistic")
	if rBaseReal != nil {
		buf.WriteString(fmt.Sprintf("解读 (realistic 画像): 13 分钟内会发起 %d 次 verification AI 调用, 其中 %d 次没有新证据是浪费; satisfiedLag = %d iter (即 loop 在 ground truth 后多跑了这么多轮才退出).\n",
			rBaseReal.fired, rBaseReal.wasted, mustLagIters(rBaseReal)))
	}
	if rBasePess != nil {
		buf.WriteString(fmt.Sprintf("解读 (pessimistic 上界): fired 高达 %d 次, 全部都是浪费 (AI 永远 Satisfied=false, 永远无新证据), 是最坏成本场景.\n",
			rBasePess.fired))
	}
	buf.WriteString("\n")

	buf.WriteString("## 16.4 参数扫描矩阵\n\n")
	buf.WriteString("扫描空间: snapshotAge ∈ {30s, 60s, 90s, 120s}, promptDelta ∈ {500, 1000, 1500, 2000}, iterInterval ∈ {5, 7, 10}, profile ∈ {realistic, pessimistic, early_done}.\n\n")
	for _, sa := range []time.Duration{30 * time.Second, 60 * time.Second, 90 * time.Second, 120 * time.Second} {
		for _, dd := range []int{500, 1000, 1500, 2000} {
			for _, ii := range []int{5, 7, 10} {
				buf.WriteString(fmt.Sprintf("### snapshotAge=%s, promptDelta=%d, iterInterval=%d\n\n", sa.String(), dd, ii))
				for _, prof := range []string{"realistic", "pessimistic", "early_done"} {
					r := findVerifyResult(results, sa, dd, ii, prof)
					if r == nil {
						continue
					}
					lagI, lagS := r.computeSatisfiedLag()
					buf.WriteString(fmt.Sprintf("- profile=%-11s fired=%d wasted=%d (%.0f%%) satAt=iter%d lagIters=%d lagSeconds=%ds\n",
						prof, r.fired, r.wasted, percentInt(r.wasted, r.fired), r.satisfiedAt, lagI, lagS))
				}
				buf.WriteString("\n")
			}
		}
	}

	buf.WriteString("## 16.5 三档推荐\n\n")
	mildReal := findVerifyResult(results, 60*time.Second, 1000, 5, "realistic")
	mildPess := findVerifyResult(results, 60*time.Second, 1000, 5, "pessimistic")
	mediumReal := findVerifyResult(results, 90*time.Second, 1500, 5, "realistic")
	mediumPess := findVerifyResult(results, 90*time.Second, 1500, 5, "pessimistic")
	aggrReal := findVerifyResult(results, 120*time.Second, 2000, 5, "realistic")
	aggrPess := findVerifyResult(results, 120*time.Second, 2000, 5, "pessimistic")

	buf.WriteString("基于扫描数据, 给出三档候选, 由用户根据风险偏好选择:\n\n")

	buf.WriteString("### 档 A (温和): snapshotAge 30s→60s, promptDelta 500→1000, iter 不变 (5)\n\n")
	if rBasePess != nil && mildPess != nil {
		buf.WriteString(fmt.Sprintf("- pessimistic fired: %d → %d (-%.0f%%)\n",
			rBasePess.fired, mildPess.fired, percentDelta(rBasePess.fired, mildPess.fired)))
	}
	if rBaseReal != nil && mildReal != nil {
		buf.WriteString(fmt.Sprintf("- realistic fired: %d → %d, wasted: %d → %d, lagIters: %d → %d\n",
			rBaseReal.fired, mildReal.fired, rBaseReal.wasted, mildReal.wasted,
			mustLagIters(rBaseReal), mustLagIters(mildReal)))
	}
	buf.WriteString("- 风险: 极低. 几乎不增加 loop 退出延迟, 但只削减约 30% 浪费.\n")
	buf.WriteString("- 适合: 严格质量场景 (渗透测试 / 金融审计), 想稍微省一点又不愿意冒任何延迟风险.\n\n")

	buf.WriteString("### 档 B (中等, 推荐): snapshotAge 30s→90s, promptDelta 500→1500, iter 不变 (5)\n\n")
	if rBasePess != nil && mediumPess != nil {
		buf.WriteString(fmt.Sprintf("- pessimistic fired: %d → %d (-%.0f%%)\n",
			rBasePess.fired, mediumPess.fired, percentDelta(rBasePess.fired, mediumPess.fired)))
	}
	if rBaseReal != nil && mediumReal != nil {
		buf.WriteString(fmt.Sprintf("- realistic fired: %d → %d, wasted: %d → %d, lagIters: %d → %d\n",
			rBaseReal.fired, mediumReal.fired, rBaseReal.wasted, mediumReal.wasted,
			mustLagIters(rBaseReal), mustLagIters(mediumReal)))
	}
	buf.WriteString("- 风险: 低. lagIters <= 2 (loop 最多多跑 2 轮 ≈ 60-90s), wasted 下降 ~50%.\n")
	buf.WriteString("- 适合: 大多数业务场景 (默认推荐). 在性能与响应性之间取得平衡.\n\n")

	buf.WriteString("### 档 C (激进): snapshotAge 30s→120s, promptDelta 500→2000, iter 不变 (5)\n\n")
	if rBasePess != nil && aggrPess != nil {
		buf.WriteString(fmt.Sprintf("- pessimistic fired: %d → %d (-%.0f%%)\n",
			rBasePess.fired, aggrPess.fired, percentDelta(rBasePess.fired, aggrPess.fired)))
	}
	if rBaseReal != nil && aggrReal != nil {
		buf.WriteString(fmt.Sprintf("- realistic fired: %d → %d, wasted: %d → %d, lagIters: %d → %d\n",
			rBaseReal.fired, aggrReal.fired, rBaseReal.wasted, aggrReal.wasted,
			mustLagIters(rBaseReal), mustLagIters(aggrReal)))
	}
	buf.WriteString("- 风险: 中. lagIters 可能 2-3 (loop 最多多跑 2-3 轮 ≈ 90-150s), 部分中间 delivery 快照会被跳过.\n")
	buf.WriteString("- 适合: 成本控制型 (批量自动化扫描 / 无人值守任务). 注意配合 watchdog 2min 兜底.\n\n")

	// 隐藏甜点: 扫描矩阵中 (120s, 1500, 5) 表现意外优秀, 高亮一下供决策参考.
	// 关键词: 隐藏甜点 120s_1500, 零延迟 + -54% 成本
	sweetReal := findVerifyResult(results, 120*time.Second, 1500, 5, "realistic")
	sweetPess := findVerifyResult(results, 120*time.Second, 1500, 5, "pessimistic")
	sweetEarly := findVerifyResult(results, 120*time.Second, 1500, 5, "early_done")
	if sweetReal != nil && sweetPess != nil && sweetEarly != nil {
		buf.WriteString("### 隐藏甜点 (扫描发现): snapshotAge 30s→120s, promptDelta 500→1500, iter 不变 (5)\n\n")
		if rBasePess != nil {
			buf.WriteString(fmt.Sprintf("- pessimistic fired: %d → %d (-%.0f%%)\n",
				rBasePess.fired, sweetPess.fired, percentDelta(rBasePess.fired, sweetPess.fired)))
		}
		if rBaseReal != nil {
			buf.WriteString(fmt.Sprintf("- realistic fired: %d → %d, wasted: %d → %d, lagIters: %d → %d (**零延迟**)\n",
				rBaseReal.fired, sweetReal.fired, rBaseReal.wasted, sweetReal.wasted,
				mustLagIters(rBaseReal), mustLagIters(sweetReal)))
		}
		buf.WriteString(fmt.Sprintf("- early_done satAt: iter%d (groundTruth=12, 提前完成场景仍能及时检测)\n", sweetEarly.satisfiedAt))
		buf.WriteString("- 关键洞察: 把 token 门保持 1500 而非 2000, 既享受 120s 时间门带来的大幅节省 (-54%), 又因 token 门更敏感保留了 realistic 画像下 satisfiedLag=0 的响应性.\n")
		buf.WriteString("- 风险: 低-中. 对比档 C 多消耗 1 次 AI 调用 (pessimistic 6 vs 5), 换来 lagIters 从 2 降到 0. 是 \"性价比最高\" 的组合.\n")
		buf.WriteString("- 适合: 介于档 B 与档 C 之间的折中选项.\n\n")
	}

	buf.WriteString("## 16.6 风险与注意\n\n")
	buf.WriteString("- 末轮兜底门 (iter == maxIter) **不动**, 保证最终一定会有一次 verification.\n")
	buf.WriteString("- iterInterval 不动 (维持 5), 确保 \"长时间不调用\" 场景下至少每 5 轮强制 verify.\n")
	buf.WriteString("- watchdog 兜底 (`verificationWatchdogIdleTimeout = 2 * time.Minute`) **不动**, 极端情况下 2 分钟无调用必触发.\n")
	buf.WriteString("- 显式调用路径 (`VerifyUserSatisfactionNow` / `request_verification` action) 不受本次默认值调整影响.\n")
	buf.WriteString("- 提前完成场景 (`early_done` 画像在 iter=12 就 Satisfied=true): 三档都能在 iter=14 之前检测到 (token 门或 iter 门会兜底).\n")
	buf.WriteString("- 副作用代价说明: lagIters>0 意味着 loop 多跑了几轮 \"无用工具调用\", 但每次工具调用本身有自己的 perception/反思节流, 不会失控.\n")

	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(reportPath, []byte(buf.String()), 0o644)
}

func mustLagIters(r *verifySimResult) int {
	if r == nil {
		return -1
	}
	lagI, _ := r.computeSatisfiedLag()
	return lagI
}

func percentInt(num, denom int) float64 {
	if denom == 0 {
		return 0
	}
	return float64(num) * 100 / float64(denom)
}

func percentDelta(oldV, newV int) float64 {
	if oldV == 0 {
		return 0
	}
	return float64(oldV-newV) * 100 / float64(oldV)
}
