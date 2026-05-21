# 16. Verification 触发频率仿真实验 / Verification Frequency Experiment

> 回到 [README](../README.md) | 上一章: [15-perception-frequency-experiment.md](15-perception-frequency-experiment.md)

> 本报告由 [verification_frequency_sim_test.go](../verification_frequency_sim_test.go) 自动生成.
> 每次 `go test -run TestVerificationFrequencySim ./common/ai/aid/aireact/reactloops/` 会覆盖更新.

## 16.1 案例与节奏

仿真采用两套 fixture, 覆盖不同 token 增长节奏:

### 16.1.1 redhaze (中等节奏)

来自 redhaze 案例 (`13354_redhaze_login_security_test_20260519_79bae`),
沿用 perception 实验的 20 个 iter 时间戳, 每 iter 增长 200-800 tokens:

```
iter  1  t=0     phase=1  tokens=3000    EP
iter  2  t=23    phase=1  tokens=3300    E 
iter  3  t=40    phase=1  tokens=3550    - 
iter  4  t=151   phase=2  tokens=3900    EP
iter  5  t=169   phase=2  tokens=4200    E 
iter  6  t=241   phase=2  tokens=4700    - 
iter  7  t=261   phase=2  tokens=5000    - 
iter  8  t=372   phase=3  tokens=5500    EP
iter  9  t=385   phase=3  tokens=6000    E 
iter 10  t=417   phase=3  tokens=6400    - 
iter 11  t=454   phase=3  tokens=7000    E 
iter 12  t=482   phase=3  tokens=7400    - 
iter 13  t=517   phase=3  tokens=7700    - 
iter 14  t=554   phase=4  tokens=8400    EP
iter 15  t=589   phase=4  tokens=8900    - 
iter 16  t=617   phase=4  tokens=9600    E 
iter 17  t=644   phase=4  tokens=10000   - 
iter 18  t=686   phase=4  tokens=10700   E 
iter 19  t=735   phase=4  tokens=11200   - 
iter 20  t=770   phase=4  tokens=12000   E 
```

**业务真实达成 iter** = 18 (phase 4 中段, employee union 数据已完整提取).

### 16.1.2 data_explosion (数据爆炸高峰)

模拟用户实测中 "前 3 iter 轻量探测, iter=4 起每 iter +1800-2500 tokens" 的尖峰场景
(union extract / 大 HTML 抓取 / 大文件读取). 这是 redhaze 中等节奏 fixture 没有暴露的尖峰:

```
iter  1  t=0     phase=1  tokens=3000    E
iter  2  t=15    phase=1  tokens=3300    -
iter  3  t=32    phase=1  tokens=3600    E
iter  4  t=50    phase=2  tokens=5400    E
iter  5  t=75    phase=2  tokens=7700    -
iter  6  t=95    phase=2  tokens=9900    E
iter  7  t=122   phase=2  tokens=12300   -
iter  8  t=145   phase=2  tokens=14500   E
iter  9  t=178   phase=2  tokens=16800   -
iter 10  t=200   phase=2  tokens=19000   E
iter 11  t=235   phase=2  tokens=21300   -
iter 12  t=261   phase=2  tokens=23400   E
iter 13  t=290   phase=2  tokens=25900   -
iter 14  t=320   phase=2  tokens=28100   E
iter 15  t=348   phase=2  tokens=30400   -
iter 16  t=375   phase=2  tokens=32500   E
iter 17  t=405   phase=2  tokens=34800   -
iter 18  t=438   phase=2  tokens=37000   E
iter 19  t=472   phase=2  tokens=39100   -
iter 20  t=500   phase=2  tokens=41300   E
```

**业务真实达成 iter** = 14. 在数据爆炸节奏下, baseline (无 cooldown) 会几乎每 iter 触发 verify, 是本次优化的重点修复对象.

## 16.2 仿真器与画像

`simVerifyController` 严格镜像生产 `shouldTriggerAutomaticVerification` 的分层门:

1. `iter == maxIter` 末轮兜底
2. `previous == nil && iter >= firstFireThreshold` 首次提前门
3. `now - prev.GeneratedAt >= snapshotAge` 时间门
4. `iter - prev.iter >= iterInterval` iter 门基础节拍
5. `|tokens_delta| >= hardPromptDelta` 硬 token 门 (豁免冷静期)
6. 冷静期: `iter - prev.iter < cooldown` 时短路, 软 token 门不生效
7. `|tokens_delta| >= promptDelta` 软 token 门

唯一区别是 `now` 用显式参数代替 `time.Now()`, 实现确定性回放.

AI 调用本身用三种 profile 模拟:

- `realistic`: AI 在 iter >= groundTruth 才返回 Satisfied=true; hasNewArtifact 跟随 fixture 真实标注. 主参考画像.
- `pessimistic`: AI 永远 Satisfied=false 且永远无新证据, 是浪费率上界. 用于评估最差成本.
- `early_done`: AI 在 iter >= earlyDoneAt 就提前说 Satisfied=true, 用于验证节流过粗时是否漏检.

一旦 AI 返回 Satisfied=true, 仿真立即终止 (与生产 loop 退出语义一致).

## 16.3 主参数固定 (snapshotAge=3m0s / promptDelta=1500 / iterInterval=6)

本轮扫描重点是新增的 cooldown / hardPromptDelta / firstFireThreshold 三个维度,
snapshotAge / promptDelta / iterInterval 固定为新版生产默认值. 完整扫描全笛卡尔积过大,
已在 git 历史版本中保留.

扫描空间: cooldown ∈ {0, 2, 3}, hardPromptDelta ∈ {0, 3000, 5000, 8000}, firstFireThreshold ∈ {3, 5, 6}.

## 16.4 redhaze fixture 关键结果

### cooldown=0, hardDelta=0, firstFire=6 (旧版 baseline)

- profile=realistic   fired=5 wasted=3 (60%) satAt=iter20 lagIters=2 lagSeconds=84s
- profile=pessimistic fired=5 wasted=5 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=3 wasted=0 (0%) satAt=iter14 lagIters=0 lagSeconds=0s

### **cooldown=3, hardDelta=5000, firstFire=3 (新版默认)**

- profile=realistic   fired=6 wasted=4 (67%) satAt=iter20 lagIters=2 lagSeconds=84s
- profile=pessimistic fired=6 wasted=6 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter14 lagIters=0 lagSeconds=0s

### cooldown=2, hardDelta=5000, firstFire=3 (cooldown 更短对比)

- profile=realistic   fired=6 wasted=4 (67%) satAt=iter20 lagIters=2 lagSeconds=84s
- profile=pessimistic fired=6 wasted=6 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter14 lagIters=0 lagSeconds=0s

### cooldown=3, hardDelta=8000, firstFire=3 (硬门更高)

- profile=realistic   fired=6 wasted=4 (67%) satAt=iter20 lagIters=2 lagSeconds=84s
- profile=pessimistic fired=6 wasted=6 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter14 lagIters=0 lagSeconds=0s

### cooldown=3, hardDelta=3000, firstFire=3 (硬门更低)

- profile=realistic   fired=6 wasted=4 (67%) satAt=iter20 lagIters=2 lagSeconds=84s
- profile=pessimistic fired=6 wasted=6 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter14 lagIters=0 lagSeconds=0s

### cooldown=3, hardDelta=5000, firstFire=5 (firstFire 推后)

- profile=realistic   fired=5 wasted=1 (20%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=6 wasted=6 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter15 lagIters=0 lagSeconds=0s

## 16.5 data_explosion fixture 关键结果

### cooldown=0, hardDelta=0, firstFire=6 (旧版 baseline)

- profile=realistic   fired=9 wasted=4 (44%) satAt=iter14 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=15 wasted=15 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=3 wasted=0 (0%) satAt=iter8 lagIters=0 lagSeconds=0s

### **cooldown=3, hardDelta=5000, firstFire=3 (新版默认)**

- profile=realistic   fired=5 wasted=1 (20%) satAt=iter15 lagIters=1 lagSeconds=28s
- profile=pessimistic fired=7 wasted=7 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=3 wasted=0 (0%) satAt=iter9 lagIters=0 lagSeconds=0s

### cooldown=2, hardDelta=5000, firstFire=3 (cooldown 更短对比)

- profile=realistic   fired=7 wasted=5 (71%) satAt=iter15 lagIters=1 lagSeconds=28s
- profile=pessimistic fired=10 wasted=10 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter9 lagIters=0 lagSeconds=0s

### cooldown=3, hardDelta=8000, firstFire=3 (硬门更高)

- profile=realistic   fired=5 wasted=1 (20%) satAt=iter15 lagIters=1 lagSeconds=28s
- profile=pessimistic fired=7 wasted=7 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=3 wasted=0 (0%) satAt=iter9 lagIters=0 lagSeconds=0s

### cooldown=3, hardDelta=3000, firstFire=3 (硬门更低)

- profile=realistic   fired=7 wasted=5 (71%) satAt=iter15 lagIters=1 lagSeconds=28s
- profile=pessimistic fired=10 wasted=10 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter9 lagIters=0 lagSeconds=0s

### cooldown=3, hardDelta=5000, firstFire=5 (firstFire 推后)

- profile=realistic   fired=4 wasted=2 (50%) satAt=iter14 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=6 wasted=6 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=2 wasted=0 (0%) satAt=iter8 lagIters=0 lagSeconds=0s

## 16.6 推荐与默认值

- data_explosion pessimistic fired: 15 (baseline cd=0) → 7 (cd=3, hd=5000, ff=3), 削减 **53%** 的 verification 调用
- redhaze realistic 在新默认下 fired=6, satisfiedLag=2 iter (响应性不退化)

生产默认值已锁定为:
- `verificationAutoTriggerMaxSnapshotAge = 3m0s`
- `verificationAutoTriggerMinPromptDelta = 1500`
- `verificationIterationTriggerInterval = 6` (== aicommon.DefaultPeriodicVerificationInterval)
- `verificationTokenGateMinIterCooldown = 3`
- `verificationAutoTriggerHardPromptDelta = 5000`
- `verificationFirstFireIterationThreshold = 3`

## 16.7 数据爆炸与冷静期 + 首次提前触发

本轮优化围绕用户实测反馈展开: 在数据爆炸阶段 (单 iter prompt token 涨 1800-2500),
此前 (snapshotAge=120s / promptDelta=1500 / iterInterval=5) 的 5 个门是 OR 关系,
token 门以 1500 阈值在数据爆炸节奏下几乎每 iter 都过门, 把 iter 门 "每 5 轮一次"
的基础节拍打成 "每 1-2 轮就 verify". 用户原话:

> "前 5 个工具不触发, 之后每 1-2 个工具就 verify, 高峰时每次工具都 verify.
> 按理说, 时间到了触发的一次, 应该让每 5 个这个重新开始基数才对, 不然多次触发累积一起不好用了."

修复策略 (本次落地):

- **基础节拍门**: iter 门 (5→6) + 时间门 (120s→180s) + 末轮兜底, 必须遵守, 加速器不能越过
- **首次提前门**: baseline 未建立时 iter>=3 即 fire, 让 AI 早期校准方向 (相比旧版需要等 iter=5 提前 2 轮)
- **加速器门 + 冷静期**: 软 token 门 1500 不变, 但只在 iter 差 >= 3 之后才允许触发, 数据爆炸阶段不能反复打断 iter 节拍
- **硬兜底门**: 硬 token 门 5000, 单次超大爆炸豁免冷静期, 不丢响应性

**触发时序示例** (数据爆炸每 iter +1800 tokens):

```
iter=1: baseline=nil, 1 < 3 firstFire → 不触发
iter=2: baseline=nil, 2 < 3 firstFire → 不触发
iter=3: baseline=nil, 3 >= 3 firstFire → fire (首次提前), baseline 建立
iter=4: iterDelta=1, tokenDelta=1800<5000 hard, 1<3 冷静期 → 不触发
iter=5: iterDelta=2, tokenDelta=3600<5000 hard, 2<3 冷静期 → 不触发
iter=6: iterDelta=3, tokenDelta=5400>=5000 hard → fire (硬门)
iter=7-8: 冷静期内 → 不触发
iter=9: iterDelta=3, 软门 >= 1500 → fire (软门解禁)
iter=12: iter 门 (差=6) → 兜底 fire
```

相比修复前的 "几乎每 iter 都 fire", 修复后在数据爆炸节奏下 fired 数显著下降,
同时保留了 satisfied 检测的及时性 (硬门 + iter 门兜底).

## 16.8 Fire 完成后基线对齐 (清零公平)

用户在多门交叉触发场景下提出补充诉求:

> "在工具调用结束的 verification 中, 这个 gate 有很多层面, 但是很多层面都是交叉的.
> 有一个改动的需求是说, 当交叉出现的时候, 下一轮触发不应该太频繁, 比如说时间兜底
> 触发的时候, 迭代次数应该清零等操作. 并且迭代次数要及时清零, 这样整体才能公平公正."

背景与现状分析 (修复前):

- 自动路径 `MaybeVerifyUserSatisfaction` 触发 fire 后, 使用 fire **开始前** 计算的
  `currentSnapshot` 作为新基线落盘 (`setVerificationRuntimeSnapshot(currentSnapshot)`).
- 显式路径 `VerifyUserSatisfactionNow` 触发 fire 后, 使用 fire **结束后** 重新构造的
  实时 snapshot 作为新基线 (`setVerificationRuntimeSnapshot(r.buildVerificationRuntimeSnapshot(time.Now()))`).
- 两条路径的清零语义不一致. 在自动路径下, `prev.GeneratedAt` 比 fire 实际完成时间早
  AI 调用耗时 (常 5-30s), 等于把这段时间 "白送" 给时间门, 下一轮时间门 (180s) 会被
  提前到位. 多个门交叉触发后, 整体 verification 频率被不公平地推高.

修复策略:

- 自动路径在 fire 完成后, 改为用 `r.buildVerificationRuntimeSnapshot(time.Now())`
  重新构造实时 snapshot 作为新基线, 与显式路径行为完全对齐.
- 时间门 / iter 门 / 软 token 门冷静期下次判断都从 fire 真正完成那一刻起算,
  AI 调用耗时不再被任意一个门白送.
- 主循环 fire 期间是同步阻塞的, `currentIterationIndex` 与 `LoopPromptTokens` 在 fire
  期间不会变化, 所以这两个维度的值与 currentSnapshot 一致; 唯一发生变化的是
  `GeneratedAt` (从 fire 开始时间 → fire 完成时间).

仿真模型说明:

- `simVerifyController` 一直采用 "fire 瞬时" 假设 (`recordFire(now)` 与 fire 判断的
  `now` 是同一个时刻), 隐含的就是 "fire 完成时刻 == 基线时刻" 这一理想模型.
- 修复前生产代码偏离这一理想模型 (基线时刻是 fire 开始时刻), sim 报告里的 fired/lag
  数据其实低估了真实生产的频率.
- 修复后生产代码与 sim 模型对齐, 现有 sim 报告的 fired/lag 数据可以直接代表真实
  生产频率, 无需再叠加 AI 调用耗时的折算.

可见的端到端测试断言:

- `TestMaybeVerifyUserSatisfaction_BaselineRebuildAfterFire`: 模拟 60ms AI 延迟,
  断言 fire 完成后 `prev.GeneratedAt` 比 fire 开始时间晚至少 50ms, 反映真实结束时刻.
- `TestMaybeVerifyUserSatisfaction_TimeGateRefreshAfterFire`: 用时间门触发 fire,
  断言 fire 完成后 `prev.GeneratedAt` 远离旧 baseline 且晚于 fire 开始时间 50ms+,
  说明时间门基线已被同步推进到 fire 完成时刻.

## 16.9 风险与注意

- 末轮兜底门 (iter == maxIter) **不动**, 保证最终一定会有一次 verification.
- watchdog 兜底 (`verificationWatchdogIdleTimeout = 2 * time.Minute`) **不动**, 极端情况下 2 分钟无调用必触发.
- 显式调用路径 (`VerifyUserSatisfactionNow` / `request_verification` action) 与自动路径
  在清零基线时刻上已对齐, 都使用 fire 结束时刻作为新基线 (见 16.8).
- 提前完成场景 (`early_done` 画像): firstFire=3 让首次反馈最早在 iter=3 拿到, 后续靠 iter 门/硬门 兜底.
- 副作用代价说明: lagIters>0 意味着 loop 多跑了几轮 "无用工具调用", 但每次工具调用本身有自己的 perception/反思节流, 不会失控.
