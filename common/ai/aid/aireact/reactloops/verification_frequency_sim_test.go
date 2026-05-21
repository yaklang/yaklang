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
// 的全套节流门 (snapshotAge / promptDelta / iterInterval / cooldown /
// hardPromptDelta / firstFireThreshold). 仿真在多个 fixture (redhaze
// 中等节奏 + 数据爆炸高峰节奏) x 三种 AI 行为画像下评估:
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
//	AI 调用次数评估, cooldown / hardPromptDelta / firstFire 维度,
//	数据爆炸 fixture, redhaze 中等节奏 fixture

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

// verifyCaseFixtureDataExplosion 模拟"数据爆炸场景"的 fixture, 复现用户实测的
// 高峰刷屏问题: 前 3 个 iter 较轻 (页面探测), iter=4 开始进入数据爆炸阶段,
// 每个 iter prompt token 暴涨 1800-2500 tokens (大 HTML 抓取 / union extract /
// 大文件读取). 这是 redhaze 中等节奏 fixture 没有暴露的尖峰场景, 用来验证
// 冷静期 (cooldown) 能否把"每个工具调用都 verify"的高峰节流到合理频率.
//
// 设计参数:
//
//	阶段 1 (iter 1-3) 轻 (200-400/iter), 探测页面结构
//	阶段 2 (iter 4-20) 重 (1800-2500/iter), 数据爆炸高峰, 真实业务在 iter=14 达成
//
// 关键词: verifyCaseFixtureDataExplosion, 数据爆炸高峰, 每个工具都 verify 修复验证
var verifyCaseFixtureDataExplosion = []struct {
	iter           int
	tSecond        int
	phase          int
	promptTokens   int
	hasNewEvidence bool
}{
	{1, 0, 1, 3000, true},
	{2, 15, 1, 3300, false},
	{3, 32, 1, 3600, true},
	{4, 50, 2, 5400, true}, // 数据爆炸开始 +1800
	{5, 75, 2, 7700, false},
	{6, 95, 2, 9900, true}, // +2200
	{7, 122, 2, 12300, false},
	{8, 145, 2, 14500, true}, // +2200
	{9, 178, 2, 16800, false},
	{10, 200, 2, 19000, true},
	{11, 235, 2, 21300, false},
	{12, 261, 2, 23400, true},
	{13, 290, 2, 25900, false},
	{14, 320, 2, 28100, true}, // ground truth 达成
	{15, 348, 2, 30400, false},
	{16, 375, 2, 32500, true},
	{17, 405, 2, 34800, false},
	{18, 438, 2, 37000, true},
	{19, 472, 2, 39100, false},
	{20, 500, 2, 41300, true},
}

const verifyExplosionGroundTruthSatisfiedAt = 14

// simVerifyController 镜像生产 ReActLoop.shouldTriggerAutomaticVerification
// 与配套的 GetVerificationRuntimeSnapshot/setVerificationRuntimeSnapshot
// 状态机, 但用显式 now 参数代替 time.Now() / r.GetVerificationRuntimeSnapshot,
// 实现确定性回放.
//
// 严格对齐的分层门 (按优先级):
//  1. iter == maxIter 末轮兜底
//  2. previous == nil 走首次提前门: iter >= firstFireThreshold 才 fire
//  3. 时间门: now - prev.GeneratedAt >= snapshotAge
//  4. iter 门: iter - prev.iter >= iterInterval (基础节拍)
//  5. 硬 token 门: |tokens_delta| >= hardPromptDelta (豁免冷静期)
//  6. 冷静期: iter - prev.iter < cooldown 时短路, 软 token 门不生效
//  7. 软 token 门: |tokens_delta| >= promptDelta
//
// 关键词: simVerifyController, 镜像 shouldTriggerAutomaticVerification 分层门,
//
//	verification 确定性回放, 首次提前门, 冷静期, 硬 token 门
type simVerifyController struct {
	snapshotAge        time.Duration
	promptDelta        int
	iterInterval       int
	cooldown           int
	hardPromptDelta    int
	firstFireThreshold int
	maxIter            int

	prevGeneratedAt time.Time
	prevIter        int
	prevTokens      int
	hasPrev         bool
}

// simVerifyParams 把仿真参数打包, 简化 controller 创建与扫描矩阵的可读性.
//
// 关键词: simVerifyParams, 仿真参数打包, 扫描矩阵可读性
type simVerifyParams struct {
	snapshotAge        time.Duration
	promptDelta        int
	iterInterval       int
	cooldown           int
	hardPromptDelta    int
	firstFireThreshold int
}

func newSimVerifyController(p simVerifyParams, maxIter int) *simVerifyController {
	return &simVerifyController{
		snapshotAge:        p.snapshotAge,
		promptDelta:        p.promptDelta,
		iterInterval:       p.iterInterval,
		cooldown:           p.cooldown,
		hardPromptDelta:    p.hardPromptDelta,
		firstFireThreshold: p.firstFireThreshold,
		maxIter:            maxIter,
	}
}

func (s *simVerifyController) shouldTrigger(now time.Time, iter, tokens int) (fire bool, gate string) {
	// 末轮兜底
	if s.maxIter > 0 && iter == s.maxIter {
		return true, "max_iter"
	}
	// 首次提前门: baseline 未建立时, iter >= firstFireThreshold 即 fire
	if !s.hasPrev {
		if iter >= s.firstFireThreshold {
			return true, "first_fire"
		}
		return false, ""
	}
	// 时间门
	if now.Sub(s.prevGeneratedAt) >= s.snapshotAge {
		return true, "snapshot_age"
	}
	// iter 门基础节拍
	iterDelta := iter - s.prevIter
	if iterDelta >= s.iterInterval {
		return true, "iter_delta"
	}
	// 硬 token 门: 单次超大爆炸豁免冷静期
	tokenDelta := tokens - s.prevTokens
	if tokenDelta < 0 {
		tokenDelta = -tokenDelta
	}
	if s.hardPromptDelta > 0 && tokenDelta >= s.hardPromptDelta {
		return true, "hard_token"
	}
	// 冷静期: iter delta < cooldown 时抑制软 token 门
	if iterDelta < s.cooldown {
		return false, ""
	}
	// 软 token 门
	if tokenDelta >= s.promptDelta {
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
// 入参 evidenceByIter / groundTruthAt 由调用方按当前 fixture 显式注入,
// 让同一份 profile 工厂能服务于 redhaze 与 dataExplosion 两套 fixture.
//
// 关键词: realistic 画像, hasNewEvidence 驱动, 真实 satisfiedLag, fixture 解耦
func makeRealisticVerifyProfile(evidenceByIter map[int]bool, groundTruthAt int) verifyAIProfile {
	return verifyAIProfile{
		name: "realistic",
		satisfied: func(iter int) bool {
			return iter >= groundTruthAt
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

// makeEarlyDoneProfile: AI 在 earlyDoneAt iter 就说 Satisfied=true, 模拟提前
// 误判任务完成的场景. 用于评估"如果节流过粗, 能否还能及时捕获早完成的信号".
//
// 关键词: early_done 画像, 提前 satisfied 信号, 节流过粗漏检防护
func makeEarlyDoneVerifyProfile(earlyDoneAt int) verifyAIProfile {
	return verifyAIProfile{
		name: "early_done",
		satisfied: func(iter int) bool {
			return iter >= earlyDoneAt
		},
		hasNewArtifact: func(iter int) bool {
			return true
		},
	}
}

// verifyFixtureRow 是 fixture 表的最小接口, 让 redhaze / dataExplosion 两套
// 共享同一份 runSim 主循环.
// 关键词: verifyFixtureRow, fixture 统一接口
type verifyFixtureRow struct {
	iter           int
	tSecond        int
	promptTokens   int
	hasNewEvidence bool
}

func redhazeFixtureAsRows() []verifyFixtureRow {
	rows := make([]verifyFixtureRow, 0, len(verifyCaseFixtureRedhaze))
	for _, f := range verifyCaseFixtureRedhaze {
		rows = append(rows, verifyFixtureRow{
			iter: f.iter, tSecond: f.tSecond,
			promptTokens: f.promptTokens, hasNewEvidence: f.hasNewEvidence,
		})
	}
	return rows
}

func dataExplosionFixtureAsRows() []verifyFixtureRow {
	rows := make([]verifyFixtureRow, 0, len(verifyCaseFixtureDataExplosion))
	for _, f := range verifyCaseFixtureDataExplosion {
		rows = append(rows, verifyFixtureRow{
			iter: f.iter, tSecond: f.tSecond,
			promptTokens: f.promptTokens, hasNewEvidence: f.hasNewEvidence,
		})
	}
	return rows
}

// verifySimResult 是一次 (param x profile x fixture) 仿真的统计.
//
// 关键词: verifySimResult, fired wasted lag 三维量化, fixture 维度
type verifySimResult struct {
	fixtureName string
	params      simVerifyParams
	profileName string

	fired        int // 实际发起的 AI 调用次数
	wasted       int // fired 但 hasNewArtifact=false 且未 satisfied 的次数
	satisfiedAt  int // 第一次返回 Satisfied=true 的 iter (0 表示未发生)
	satisfiedT   int // 对应的 tSecond
	groundTruth  int // 该 fixture 的 ground truth 达成 iter (用于计算 lag)
	gateCounters map[string]int
}

// runVerifySim 在给定参数下跑一次完整仿真.
//
// 关键词: runVerifySim, verification 节流主循环, iter 时间步进, fixture 通用
func runVerifySim(fixtureName string, rows []verifyFixtureRow, groundTruth int, params simVerifyParams, profile verifyAIProfile) verifySimResult {
	maxIter := rows[len(rows)-1].iter
	ctrl := newSimVerifyController(params, maxIter)
	res := verifySimResult{
		fixtureName:  fixtureName,
		params:       params,
		profileName:  profile.name,
		groundTruth:  groundTruth,
		gateCounters: make(map[string]int),
	}

	base := time.Now()
	for _, fix := range rows {
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
	if r.satisfiedAt < r.groundTruth {
		// 早于 groundTruth 检测到 (early_done 画像下会发生), 视为 0 lag.
		return 0, 0
	}
	groundTruthT := 0
	// 找 ground truth iter 对应的 tSecond
	rows := redhazeFixtureAsRows()
	if r.fixtureName == "data_explosion" {
		rows = dataExplosionFixtureAsRows()
	}
	for _, fix := range rows {
		if fix.iter == r.groundTruth {
			groundTruthT = fix.tSecond
			break
		}
	}
	return r.satisfiedAt - r.groundTruth, r.satisfiedT - groundTruthT
}

// verifyFixture 把 (name, rows, groundTruth, earlyDoneAt) 打成一组, 让扫描
// 矩阵能用 for 循环逐 fixture 跑.
//
// 关键词: verifyFixture, 多 fixture 扫描矩阵打包
type verifyFixture struct {
	name        string
	rows        []verifyFixtureRow
	groundTruth int
	earlyDoneAt int
	evidenceMap map[int]bool
}

func buildVerifyFixtures() []verifyFixture {
	redhazeEv := make(map[int]bool, len(verifyCaseFixtureRedhaze))
	for _, f := range verifyCaseFixtureRedhaze {
		redhazeEv[f.iter] = f.hasNewEvidence
	}
	explosionEv := make(map[int]bool, len(verifyCaseFixtureDataExplosion))
	for _, f := range verifyCaseFixtureDataExplosion {
		explosionEv[f.iter] = f.hasNewEvidence
	}
	return []verifyFixture{
		{
			name:        "redhaze",
			rows:        redhazeFixtureAsRows(),
			groundTruth: verifyGroundTruthSatisfiedAt,
			earlyDoneAt: 12,
			evidenceMap: redhazeEv,
		},
		{
			name:        "data_explosion",
			rows:        dataExplosionFixtureAsRows(),
			groundTruth: verifyExplosionGroundTruthSatisfiedAt,
			earlyDoneAt: 8,
			evidenceMap: explosionEv,
		},
	}
}

// TestVerificationFrequencySim 跑参数扫描并生成 markdown 报告.
// 这是本次实验的入口, 走 simulation 路径, 不调用真实 AI.
//
// 扫描矩阵覆盖 6 个维度, 由于全笛卡尔积过大, 这里采用 "固定主参数 + 扫描新维度"
// 策略: snapshotAge/promptDelta/iterInterval 固定为新版生产默认值, 重点扫描
// cooldown / hardDelta / firstFire 这三个新增维度 + 1 个 "baseline 模式"
// (cooldown=0, hardDelta=0, firstFire=interval) 用于对比.
//
// 关键词: TestVerificationFrequencySim 入口, 多 fixture 扫描, cooldown 维度,
//
//	firstFire 维度, baseline 对照
func TestVerificationFrequencySim(t *testing.T) {
	cooldownChoices := []int{0, 2, 3}
	hardDeltaChoices := []int{0, 3000, 5000, 8000}
	firstFireChoices := []int{3, 5, 6}
	fixtures := buildVerifyFixtures()

	// 固定主参数为新版生产默认 (snapshotAge=180s / promptDelta=1500 / iter=6)
	// 关键词: 主参数固定生产默认值, 重点扫描新维度
	baseSnapshotAge := 180 * time.Second
	basePromptDelta := 1500
	baseIterInterval := 6

	var allResults []verifySimResult
	for _, fix := range fixtures {
		profiles := []verifyAIProfile{
			makeRealisticVerifyProfile(fix.evidenceMap, fix.groundTruth),
			makePessimisticVerifyProfile(),
			makeEarlyDoneVerifyProfile(fix.earlyDoneAt),
		}
		for _, cd := range cooldownChoices {
			for _, hd := range hardDeltaChoices {
				for _, ff := range firstFireChoices {
					params := simVerifyParams{
						snapshotAge:        baseSnapshotAge,
						promptDelta:        basePromptDelta,
						iterInterval:       baseIterInterval,
						cooldown:           cd,
						hardPromptDelta:    hd,
						firstFireThreshold: ff,
					}
					for _, prof := range profiles {
						res := runVerifySim(fix.name, fix.rows, fix.groundTruth, params, prof)
						allResults = append(allResults, res)
						lagI, lagS := res.computeSatisfiedLag()
						t.Logf("[sim] fixture=%-14s cd=%d hd=%-5d ff=%d profile=%-11s fired=%d wasted=%d satAt=%d lagI=%d lagS=%d",
							fix.name, cd, hd, ff, prof.name, res.fired, res.wasted, res.satisfiedAt, lagI, lagS)
					}
				}
			}
		}
	}

	// 同时还要跑一个 "完全 baseline" (cooldown=0, hardDelta=0) 对比组, 这就是
	// 在上面 cd=0 hd=0 已经覆盖, 不再单独跑.

	// 不变量断言 1: data_explosion + baseline (cooldown=0, hd=0) pessimistic
	// fired 应当 >= 8, 验证 "数据爆炸阶段无冷静期会狂调 AI" 的现状.
	// 关键词: 数据爆炸 baseline 刷屏断言, 现状验证
	dxBaseline := findVerifyResultBy(allResults, "data_explosion",
		simVerifyParams{baseSnapshotAge, basePromptDelta, baseIterInterval, 0, 0, 6},
		"pessimistic")
	if dxBaseline == nil {
		t.Fatalf("data_explosion baseline pessimistic result missing")
	}
	if dxBaseline.fired < 8 {
		t.Fatalf("data_explosion baseline (cd=0, hd=0) pessimistic fired should be >=8, got %d", dxBaseline.fired)
	}

	// 不变量断言 2: data_explosion + 新默认 (cooldown=3, hd=5000, ff=3) 相对
	// baseline pessimistic fired 至少 -40%, 验证 cooldown 修复的实际效果.
	// 关键词: 数据爆炸 cooldown 修复效果断言, -40% 底线
	dxFix := findVerifyResultBy(allResults, "data_explosion",
		simVerifyParams{baseSnapshotAge, basePromptDelta, baseIterInterval, 3, 5000, 3},
		"pessimistic")
	if dxFix == nil {
		t.Fatalf("data_explosion fixed (cd=3, hd=5000, ff=3) pessimistic result missing")
	}
	if dxFix.fired*10 > dxBaseline.fired*6 {
		t.Fatalf("data_explosion fixed should fire <=60%% of baseline pessimistic: fixed=%d baseline=%d",
			dxFix.fired, dxBaseline.fired)
	}

	// 不变量断言 3: redhaze realistic 在新默认下 satisfiedLag <= 2 iter,
	// 验证 cooldown 修复没有让中等节奏场景退化响应性.
	// 关键词: redhaze 响应性不退化断言, lag<=2
	redhazeReal := findVerifyResultBy(allResults, "redhaze",
		simVerifyParams{baseSnapshotAge, basePromptDelta, baseIterInterval, 3, 5000, 3},
		"realistic")
	if redhazeReal == nil {
		t.Fatalf("redhaze realistic result missing")
	}
	if redhazeReal.satisfiedAt > 0 {
		lagI, _ := redhazeReal.computeSatisfiedLag()
		if lagI > 2 {
			t.Fatalf("redhaze realistic lagIters should be <=2, got %d", lagI)
		}
	}

	// 不变量断言 4: firstFire=3 (新版默认) 在两个 fixture 下首次 fire 都不晚于 iter=3.
	// 验证首次提前门确实让 AI 早期拿到反馈.
	// 关键词: firstFire 早期触发断言, baseline 早期建立
	for _, fixName := range []string{"redhaze", "data_explosion"} {
		r := findVerifyResultBy(allResults, fixName,
			simVerifyParams{baseSnapshotAge, basePromptDelta, baseIterInterval, 3, 5000, 3},
			"pessimistic")
		if r == nil {
			continue
		}
		// pessimistic profile 不会提前 satisfied, fired 数量本身就反映触发节奏
		// 我们需要看 gate 计数中是否有 first_fire
		if r.fired == 0 {
			t.Fatalf("%s pessimistic should fire at least once", fixName)
		}
		if r.gateCounters["first_fire"] == 0 {
			t.Fatalf("%s pessimistic should have first_fire gate triggered with firstFire=3", fixName)
		}
	}

	if err := writeVerifyFrequencyExperimentReport(allResults, baseSnapshotAge, basePromptDelta, baseIterInterval); err != nil {
		t.Fatalf("write verification frequency report failed: %v", err)
	}
}

// findVerifyResultBy 在 allResults 中查找匹配 (fixture, params, profile) 的结果.
//
// 关键词: findVerifyResultBy, 6 维查找
func findVerifyResultBy(all []verifySimResult, fixture string, p simVerifyParams, profile string) *verifySimResult {
	for i := range all {
		r := &all[i]
		if r.fixtureName == fixture && r.profileName == profile && r.params == p {
			return r
		}
	}
	return nil
}

// writeVerifyFrequencyExperimentReport 把仿真结果写成 markdown 报告.
// 报告输出位置: docs/16-verification-frequency-experiment.md.
//
// 关键词: 写报告, markdown 渲染, verification 节流实验产出, 数据爆炸章节
func writeVerifyFrequencyExperimentReport(results []verifySimResult, baseSnapshotAge time.Duration, basePromptDelta, baseIterInterval int) error {
	_, thisFile, _, _ := runtime.Caller(0)
	docsDir := filepath.Join(filepath.Dir(thisFile), "docs")
	reportPath := filepath.Join(docsDir, "16-verification-frequency-experiment.md")

	var buf strings.Builder
	buf.WriteString("# 16. Verification 触发频率仿真实验 / Verification Frequency Experiment\n\n")
	buf.WriteString("> 回到 [README](../README.md) | 上一章: [15-perception-frequency-experiment.md](15-perception-frequency-experiment.md)\n\n")
	buf.WriteString("> 本报告由 [verification_frequency_sim_test.go](../verification_frequency_sim_test.go) 自动生成.\n")
	buf.WriteString("> 每次 `go test -run TestVerificationFrequencySim ./common/ai/aid/aireact/reactloops/` 会覆盖更新.\n\n")

	buf.WriteString("## 16.1 案例与节奏\n\n")
	buf.WriteString("仿真采用两套 fixture, 覆盖不同 token 增长节奏:\n\n")
	buf.WriteString("### 16.1.1 redhaze (中等节奏)\n\n")
	buf.WriteString("来自 redhaze 案例 (`13354_redhaze_login_security_test_20260519_79bae`),\n")
	buf.WriteString("沿用 perception 实验的 20 个 iter 时间戳, 每 iter 增长 200-800 tokens:\n\n")
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
	buf.WriteString(fmt.Sprintf("**业务真实达成 iter** = %d (phase 4 中段, employee union 数据已完整提取).\n\n", verifyGroundTruthSatisfiedAt))

	buf.WriteString("### 16.1.2 data_explosion (数据爆炸高峰)\n\n")
	buf.WriteString("模拟用户实测中 \"前 3 iter 轻量探测, iter=4 起每 iter +1800-2500 tokens\" 的尖峰场景\n")
	buf.WriteString("(union extract / 大 HTML 抓取 / 大文件读取). 这是 redhaze 中等节奏 fixture 没有暴露的尖峰:\n\n")
	buf.WriteString("```\n")
	for _, fix := range verifyCaseFixtureDataExplosion {
		evidence := "-"
		if fix.hasNewEvidence {
			evidence = "E"
		}
		buf.WriteString(fmt.Sprintf("iter %2d  t=%-4d  phase=%d  tokens=%-6d  %s\n",
			fix.iter, fix.tSecond, fix.phase, fix.promptTokens, evidence))
	}
	buf.WriteString("```\n\n")
	buf.WriteString(fmt.Sprintf("**业务真实达成 iter** = %d. 在数据爆炸节奏下, baseline (无 cooldown) 会几乎每 iter 触发 verify, 是本次优化的重点修复对象.\n\n", verifyExplosionGroundTruthSatisfiedAt))

	buf.WriteString("## 16.2 仿真器与画像\n\n")
	buf.WriteString("`simVerifyController` 严格镜像生产 `shouldTriggerAutomaticVerification` 的分层门:\n\n")
	buf.WriteString("1. `iter == maxIter` 末轮兜底\n")
	buf.WriteString("2. `previous == nil && iter >= firstFireThreshold` 首次提前门\n")
	buf.WriteString("3. `now - prev.GeneratedAt >= snapshotAge` 时间门\n")
	buf.WriteString("4. `iter - prev.iter >= iterInterval` iter 门基础节拍\n")
	buf.WriteString("5. `|tokens_delta| >= hardPromptDelta` 硬 token 门 (豁免冷静期)\n")
	buf.WriteString("6. 冷静期: `iter - prev.iter < cooldown` 时短路, 软 token 门不生效\n")
	buf.WriteString("7. `|tokens_delta| >= promptDelta` 软 token 门\n\n")
	buf.WriteString("唯一区别是 `now` 用显式参数代替 `time.Now()`, 实现确定性回放.\n\n")
	buf.WriteString("AI 调用本身用三种 profile 模拟:\n\n")
	buf.WriteString("- `realistic`: AI 在 iter >= groundTruth 才返回 Satisfied=true; hasNewArtifact 跟随 fixture 真实标注. 主参考画像.\n")
	buf.WriteString("- `pessimistic`: AI 永远 Satisfied=false 且永远无新证据, 是浪费率上界. 用于评估最差成本.\n")
	buf.WriteString("- `early_done`: AI 在 iter >= earlyDoneAt 就提前说 Satisfied=true, 用于验证节流过粗时是否漏检.\n\n")
	buf.WriteString("一旦 AI 返回 Satisfied=true, 仿真立即终止 (与生产 loop 退出语义一致).\n\n")

	buf.WriteString(fmt.Sprintf("## 16.3 主参数固定 (snapshotAge=%s / promptDelta=%d / iterInterval=%d)\n\n",
		baseSnapshotAge.String(), basePromptDelta, baseIterInterval))
	buf.WriteString("本轮扫描重点是新增的 cooldown / hardPromptDelta / firstFireThreshold 三个维度,\n")
	buf.WriteString("snapshotAge / promptDelta / iterInterval 固定为新版生产默认值. 完整扫描全笛卡尔积过大,\n")
	buf.WriteString("已在 git 历史版本中保留.\n\n")
	buf.WriteString("扫描空间: cooldown ∈ {0, 2, 3}, hardPromptDelta ∈ {0, 3000, 5000, 8000}, firstFireThreshold ∈ {3, 5, 6}.\n\n")

	buf.WriteString("## 16.4 redhaze fixture 关键结果\n\n")
	redhazeKey := []simVerifyParams{
		{baseSnapshotAge, basePromptDelta, baseIterInterval, 0, 0, 6},     // baseline (无 cooldown)
		{baseSnapshotAge, basePromptDelta, baseIterInterval, 3, 5000, 3},  // 新版默认
		{baseSnapshotAge, basePromptDelta, baseIterInterval, 2, 5000, 3},  // cooldown=2 对比
		{baseSnapshotAge, basePromptDelta, baseIterInterval, 3, 8000, 3},  // 硬门更高
		{baseSnapshotAge, basePromptDelta, baseIterInterval, 3, 3000, 3},  // 硬门更低
		{baseSnapshotAge, basePromptDelta, baseIterInterval, 3, 5000, 5},  // firstFire 推后
	}
	writeFixtureSection(&buf, results, "redhaze", redhazeKey)

	buf.WriteString("## 16.5 data_explosion fixture 关键结果\n\n")
	writeFixtureSection(&buf, results, "data_explosion", redhazeKey)

	buf.WriteString("## 16.6 推荐与默认值\n\n")
	dxBaseline := findVerifyResultBy(results, "data_explosion",
		simVerifyParams{baseSnapshotAge, basePromptDelta, baseIterInterval, 0, 0, 6},
		"pessimistic")
	dxFix := findVerifyResultBy(results, "data_explosion",
		simVerifyParams{baseSnapshotAge, basePromptDelta, baseIterInterval, 3, 5000, 3},
		"pessimistic")
	redhazeFix := findVerifyResultBy(results, "redhaze",
		simVerifyParams{baseSnapshotAge, basePromptDelta, baseIterInterval, 3, 5000, 3},
		"realistic")
	if dxBaseline != nil && dxFix != nil {
		buf.WriteString(fmt.Sprintf("- data_explosion pessimistic fired: %d (baseline cd=0) → %d (cd=3, hd=5000, ff=3), 削减 **%.0f%%** 的 verification 调用\n",
			dxBaseline.fired, dxFix.fired, percentDelta(dxBaseline.fired, dxFix.fired)))
	}
	if redhazeFix != nil {
		lagI, _ := redhazeFix.computeSatisfiedLag()
		buf.WriteString(fmt.Sprintf("- redhaze realistic 在新默认下 fired=%d, satisfiedLag=%d iter (响应性不退化)\n",
			redhazeFix.fired, lagI))
	}
	buf.WriteString("\n生产默认值已锁定为:\n")
	buf.WriteString(fmt.Sprintf("- `verificationAutoTriggerMaxSnapshotAge = %s`\n", baseSnapshotAge.String()))
	buf.WriteString(fmt.Sprintf("- `verificationAutoTriggerMinPromptDelta = %d`\n", basePromptDelta))
	buf.WriteString(fmt.Sprintf("- `verificationIterationTriggerInterval = %d` (== aicommon.DefaultPeriodicVerificationInterval)\n", baseIterInterval))
	buf.WriteString("- `verificationTokenGateMinIterCooldown = 3`\n")
	buf.WriteString("- `verificationAutoTriggerHardPromptDelta = 5000`\n")
	buf.WriteString("- `verificationFirstFireIterationThreshold = 3`\n\n")

	buf.WriteString("## 16.7 数据爆炸与冷静期 + 首次提前触发\n\n")
	buf.WriteString("本轮优化围绕用户实测反馈展开: 在数据爆炸阶段 (单 iter prompt token 涨 1800-2500),\n")
	buf.WriteString("此前 (snapshotAge=120s / promptDelta=1500 / iterInterval=5) 的 5 个门是 OR 关系,\n")
	buf.WriteString("token 门以 1500 阈值在数据爆炸节奏下几乎每 iter 都过门, 把 iter 门 \"每 5 轮一次\"\n")
	buf.WriteString("的基础节拍打成 \"每 1-2 轮就 verify\". 用户原话:\n\n")
	buf.WriteString("> \"前 5 个工具不触发, 之后每 1-2 个工具就 verify, 高峰时每次工具都 verify.\n")
	buf.WriteString("> 按理说, 时间到了触发的一次, 应该让每 5 个这个重新开始基数才对, 不然多次触发累积一起不好用了.\"\n\n")
	buf.WriteString("修复策略 (本次落地):\n\n")
	buf.WriteString("- **基础节拍门**: iter 门 (5→6) + 时间门 (120s→180s) + 末轮兜底, 必须遵守, 加速器不能越过\n")
	buf.WriteString("- **首次提前门**: baseline 未建立时 iter>=3 即 fire, 让 AI 早期校准方向 (相比旧版需要等 iter=5 提前 2 轮)\n")
	buf.WriteString("- **加速器门 + 冷静期**: 软 token 门 1500 不变, 但只在 iter 差 >= 3 之后才允许触发, 数据爆炸阶段不能反复打断 iter 节拍\n")
	buf.WriteString("- **硬兜底门**: 硬 token 门 5000, 单次超大爆炸豁免冷静期, 不丢响应性\n\n")
	buf.WriteString("**触发时序示例** (数据爆炸每 iter +1800 tokens):\n\n")
	buf.WriteString("```\n")
	buf.WriteString("iter=1: baseline=nil, 1 < 3 firstFire → 不触发\n")
	buf.WriteString("iter=2: baseline=nil, 2 < 3 firstFire → 不触发\n")
	buf.WriteString("iter=3: baseline=nil, 3 >= 3 firstFire → fire (首次提前), baseline 建立\n")
	buf.WriteString("iter=4: iterDelta=1, tokenDelta=1800<5000 hard, 1<3 冷静期 → 不触发\n")
	buf.WriteString("iter=5: iterDelta=2, tokenDelta=3600<5000 hard, 2<3 冷静期 → 不触发\n")
	buf.WriteString("iter=6: iterDelta=3, tokenDelta=5400>=5000 hard → fire (硬门)\n")
	buf.WriteString("iter=7-8: 冷静期内 → 不触发\n")
	buf.WriteString("iter=9: iterDelta=3, 软门 >= 1500 → fire (软门解禁)\n")
	buf.WriteString("iter=12: iter 门 (差=6) → 兜底 fire\n")
	buf.WriteString("```\n\n")
	buf.WriteString("相比修复前的 \"几乎每 iter 都 fire\", 修复后在数据爆炸节奏下 fired 数显著下降,\n")
	buf.WriteString("同时保留了 satisfied 检测的及时性 (硬门 + iter 门兜底).\n\n")

	buf.WriteString("## 16.8 风险与注意\n\n")
	buf.WriteString("- 末轮兜底门 (iter == maxIter) **不动**, 保证最终一定会有一次 verification.\n")
	buf.WriteString("- watchdog 兜底 (`verificationWatchdogIdleTimeout = 2 * time.Minute`) **不动**, 极端情况下 2 分钟无调用必触发.\n")
	buf.WriteString("- 显式调用路径 (`VerifyUserSatisfactionNow` / `request_verification` action) 不受本次默认值调整影响.\n")
	buf.WriteString("- 提前完成场景 (`early_done` 画像): firstFire=3 让首次反馈最早在 iter=3 拿到, 后续靠 iter 门/硬门 兜底.\n")
	buf.WriteString("- 副作用代价说明: lagIters>0 意味着 loop 多跑了几轮 \"无用工具调用\", 但每次工具调用本身有自己的 perception/反思节流, 不会失控.\n")

	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(reportPath, []byte(buf.String()), 0o644)
}

// writeFixtureSection 把一个 fixture 的多个关键参数组合结果写入 buf.
//
// 关键词: writeFixtureSection, fixture 子节渲染
func writeFixtureSection(buf *strings.Builder, results []verifySimResult, fixtureName string, paramsList []simVerifyParams) {
	for _, p := range paramsList {
		label := "cooldown=0, hardDelta=0, firstFire=6 (旧版 baseline)"
		switch {
		case p.cooldown == 3 && p.hardPromptDelta == 5000 && p.firstFireThreshold == 3:
			label = "**cooldown=3, hardDelta=5000, firstFire=3 (新版默认)**"
		case p.cooldown == 2:
			label = fmt.Sprintf("cooldown=2, hardDelta=%d, firstFire=%d (cooldown 更短对比)", p.hardPromptDelta, p.firstFireThreshold)
		case p.hardPromptDelta == 8000:
			label = fmt.Sprintf("cooldown=%d, hardDelta=8000, firstFire=%d (硬门更高)", p.cooldown, p.firstFireThreshold)
		case p.hardPromptDelta == 3000:
			label = fmt.Sprintf("cooldown=%d, hardDelta=3000, firstFire=%d (硬门更低)", p.cooldown, p.firstFireThreshold)
		case p.firstFireThreshold == 5:
			label = fmt.Sprintf("cooldown=%d, hardDelta=%d, firstFire=5 (firstFire 推后)", p.cooldown, p.hardPromptDelta)
		}
		buf.WriteString(fmt.Sprintf("### %s\n\n", label))
		for _, prof := range []string{"realistic", "pessimistic", "early_done"} {
			r := findVerifyResultBy(results, fixtureName, p, prof)
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
