# 16. Verification 触发频率仿真实验 / Verification Frequency Experiment

> 回到 [README](../README.md) | 上一章: [15-perception-frequency-experiment.md](15-perception-frequency-experiment.md)

> 本报告由 [verification_frequency_sim_test.go](../verification_frequency_sim_test.go) 自动生成.
> 每次 `go test -run TestVerificationFrequencySim ./common/ai/aid/aireact/reactloops/` 会覆盖更新.

## 16.1 案例与节奏

仿真 fixture 来自 redhaze 案例 (`13354_redhaze_login_security_test_20260519_79bae`),
沿用 perception 实验的 20 个 iter 时间戳, 并额外标注了 promptTokens 生长曲线
与 hasNewEvidence (该 iter 是否产生新证据值得 verify):

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

图例: `E`=hasNewEvidence (该 iter 有值得 verify 的新材料), `P`=phasePivotStart (阶段切换起点).

**业务真实达成 iter** = 18 (phase 4 中段, employee union 数据已完整提取). 真实 case 跑到 42 iter 才停,
意味着 satisfiedLag 从 18 起算: lag 越大, 用户实际等待时间越长 = 副作用代价越高.

**redhaze 实测对照**: 42 iter / 22.5min / 21 次 delivery_files 写入, 相邻 27/35/42s 多个相同 63-64 字节空 delivery, 是当前 30s 时间门导致的典型刷屏.

## 16.2 仿真器与画像

`simVerifyController` 严格镜像生产 `shouldTriggerAutomaticVerification` 的 5 个门:

1. `iter == maxIter` 末轮兜底
2. `previous == nil && periodicCheckpoint(iter)` 首次按周期
3. `now - prev.GeneratedAt >= snapshotAge` 时间门
4. `iter - prev.iter >= iterInterval` 迭代门
5. `|tokens_delta| >= promptDelta` token 门

唯一区别是 `now` 用显式参数代替 `time.Now()`, 实现确定性回放.
AI 调用本身用三种 profile 模拟:

- `realistic`: AI 在 iter >= 18 才返回 Satisfied=true; hasNewArtifact 跟随 fixture 真实标注 (50% 新证据率). 主参考画像.
- `pessimistic`: AI 永远 Satisfied=false 且永远无新证据, 是浪费率上界. 用于评估最差成本.
- `early_done`: AI 在 iter >= 12 就提前说 Satisfied=true, 用于验证节流过粗时是否漏检.

一旦 AI 返回 Satisfied=true, 仿真立即终止 (与生产 loop 退出语义一致).

## 16.3 当前默认 (30s / 500 / 5) 仿真结果

- profile=realistic   fired=11 wasted=4 (wasted_rate=36%) satisfiedAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=13 wasted=13 (wasted_rate=100%) satisfiedAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=7 wasted=0 (wasted_rate=0%) satisfiedAt=iter13 lagIters=0 lagSeconds=0s

解读 (realistic 画像): 13 分钟内会发起 11 次 verification AI 调用, 其中 4 次没有新证据是浪费; satisfiedLag = 0 iter (即 loop 在 ground truth 后多跑了这么多轮才退出).
解读 (pessimistic 上界): fired 高达 13 次, 全部都是浪费 (AI 永远 Satisfied=false, 永远无新证据), 是最坏成本场景.

## 16.4 参数扫描矩阵

扫描空间: snapshotAge ∈ {30s, 60s, 90s, 120s}, promptDelta ∈ {500, 1000, 1500, 2000}, iterInterval ∈ {5, 7, 10}, profile ∈ {realistic, pessimistic, early_done}.

### snapshotAge=30s, promptDelta=500, iterInterval=5

- profile=realistic   fired=11 wasted=4 (36%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=13 wasted=13 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=7 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=30s, promptDelta=500, iterInterval=7

- profile=realistic   fired=10 wasted=4 (40%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=12 wasted=12 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=6 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=30s, promptDelta=500, iterInterval=10

- profile=realistic   fired=7 wasted=3 (43%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=9 wasted=9 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=3 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=30s, promptDelta=1000, iterInterval=5

- profile=realistic   fired=10 wasted=5 (50%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=12 wasted=12 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=6 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=30s, promptDelta=1000, iterInterval=7

- profile=realistic   fired=9 wasted=5 (56%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=11 wasted=11 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=5 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=30s, promptDelta=1000, iterInterval=10

- profile=realistic   fired=7 wasted=4 (57%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=9 wasted=9 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=3 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=30s, promptDelta=1500, iterInterval=5

- profile=realistic   fired=10 wasted=5 (50%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=12 wasted=12 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=6 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=30s, promptDelta=1500, iterInterval=7

- profile=realistic   fired=9 wasted=5 (56%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=11 wasted=11 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=5 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=30s, promptDelta=1500, iterInterval=10

- profile=realistic   fired=7 wasted=4 (57%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=9 wasted=9 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=3 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=30s, promptDelta=2000, iterInterval=5

- profile=realistic   fired=10 wasted=5 (50%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=12 wasted=12 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=6 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=30s, promptDelta=2000, iterInterval=7

- profile=realistic   fired=9 wasted=5 (56%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=11 wasted=11 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=5 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=30s, promptDelta=2000, iterInterval=10

- profile=realistic   fired=7 wasted=4 (57%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=9 wasted=9 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=3 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=1m0s, promptDelta=500, iterInterval=5

- profile=realistic   fired=10 wasted=3 (30%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=12 wasted=12 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=6 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=1m0s, promptDelta=500, iterInterval=7

- profile=realistic   fired=9 wasted=3 (33%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=11 wasted=11 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=5 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=1m0s, promptDelta=500, iterInterval=10

- profile=realistic   fired=7 wasted=3 (43%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=9 wasted=9 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=3 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=1m0s, promptDelta=1000, iterInterval=5

- profile=realistic   fired=8 wasted=4 (50%) satAt=iter19 lagIters=1 lagSeconds=49s
- profile=pessimistic fired=9 wasted=9 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=5 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=1m0s, promptDelta=1000, iterInterval=7

- profile=realistic   fired=7 wasted=4 (57%) satAt=iter19 lagIters=1 lagSeconds=49s
- profile=pessimistic fired=8 wasted=8 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=1m0s, promptDelta=1000, iterInterval=10

- profile=realistic   fired=5 wasted=2 (40%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=6 wasted=6 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=2 wasted=0 (0%) satAt=iter12 lagIters=0 lagSeconds=0s

### snapshotAge=1m0s, promptDelta=1500, iterInterval=5

- profile=realistic   fired=7 wasted=3 (43%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=8 wasted=8 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=5 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=1m0s, promptDelta=1500, iterInterval=7

- profile=realistic   fired=6 wasted=3 (50%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=7 wasted=7 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=1m0s, promptDelta=1500, iterInterval=10

- profile=realistic   fired=5 wasted=2 (40%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=6 wasted=6 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=2 wasted=0 (0%) satAt=iter12 lagIters=0 lagSeconds=0s

### snapshotAge=1m0s, promptDelta=2000, iterInterval=5

- profile=realistic   fired=7 wasted=3 (43%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=8 wasted=8 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=5 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=1m0s, promptDelta=2000, iterInterval=7

- profile=realistic   fired=6 wasted=3 (50%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=7 wasted=7 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=1m0s, promptDelta=2000, iterInterval=10

- profile=realistic   fired=5 wasted=2 (40%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=6 wasted=6 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=2 wasted=0 (0%) satAt=iter12 lagIters=0 lagSeconds=0s

### snapshotAge=1m30s, promptDelta=500, iterInterval=5

- profile=realistic   fired=10 wasted=3 (30%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=12 wasted=12 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=6 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=1m30s, promptDelta=500, iterInterval=7

- profile=realistic   fired=9 wasted=3 (33%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=11 wasted=11 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=5 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=1m30s, promptDelta=500, iterInterval=10

- profile=realistic   fired=7 wasted=3 (43%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=9 wasted=9 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=3 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=1m30s, promptDelta=1000, iterInterval=5

- profile=realistic   fired=7 wasted=1 (14%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=8 wasted=8 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=5 wasted=0 (0%) satAt=iter14 lagIters=0 lagSeconds=0s

### snapshotAge=1m30s, promptDelta=1000, iterInterval=7

- profile=realistic   fired=6 wasted=1 (17%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=7 wasted=7 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter14 lagIters=0 lagSeconds=0s

### snapshotAge=1m30s, promptDelta=1000, iterInterval=10

- profile=realistic   fired=5 wasted=2 (40%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=6 wasted=6 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=2 wasted=0 (0%) satAt=iter12 lagIters=0 lagSeconds=0s

### snapshotAge=1m30s, promptDelta=1500, iterInterval=5

- profile=realistic   fired=7 wasted=2 (29%) satAt=iter19 lagIters=1 lagSeconds=49s
- profile=pessimistic fired=8 wasted=8 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=5 wasted=0 (0%) satAt=iter14 lagIters=0 lagSeconds=0s

### snapshotAge=1m30s, promptDelta=1500, iterInterval=7

- profile=realistic   fired=6 wasted=2 (33%) satAt=iter19 lagIters=1 lagSeconds=49s
- profile=pessimistic fired=7 wasted=7 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter14 lagIters=0 lagSeconds=0s

### snapshotAge=1m30s, promptDelta=1500, iterInterval=10

- profile=realistic   fired=4 wasted=2 (50%) satAt=iter19 lagIters=1 lagSeconds=49s
- profile=pessimistic fired=5 wasted=5 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=2 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=1m30s, promptDelta=2000, iterInterval=5

- profile=realistic   fired=6 wasted=3 (50%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=7 wasted=7 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter12 lagIters=0 lagSeconds=0s

### snapshotAge=1m30s, promptDelta=2000, iterInterval=7

- profile=realistic   fired=5 wasted=3 (60%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=6 wasted=6 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=3 wasted=0 (0%) satAt=iter12 lagIters=0 lagSeconds=0s

### snapshotAge=1m30s, promptDelta=2000, iterInterval=10

- profile=realistic   fired=4 wasted=2 (50%) satAt=iter19 lagIters=1 lagSeconds=49s
- profile=pessimistic fired=5 wasted=5 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=2 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=2m0s, promptDelta=500, iterInterval=5

- profile=realistic   fired=10 wasted=3 (30%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=12 wasted=12 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=6 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=2m0s, promptDelta=500, iterInterval=7

- profile=realistic   fired=9 wasted=3 (33%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=11 wasted=11 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=5 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=2m0s, promptDelta=500, iterInterval=10

- profile=realistic   fired=7 wasted=3 (43%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=9 wasted=9 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=3 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=2m0s, promptDelta=1000, iterInterval=5

- profile=realistic   fired=6 wasted=0 (0%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=7 wasted=7 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter14 lagIters=0 lagSeconds=0s

### snapshotAge=2m0s, promptDelta=1000, iterInterval=7

- profile=realistic   fired=6 wasted=1 (17%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=7 wasted=7 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter14 lagIters=0 lagSeconds=0s

### snapshotAge=2m0s, promptDelta=1000, iterInterval=10

- profile=realistic   fired=5 wasted=2 (40%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=6 wasted=6 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=2 wasted=0 (0%) satAt=iter12 lagIters=0 lagSeconds=0s

### snapshotAge=2m0s, promptDelta=1500, iterInterval=5

- profile=realistic   fired=5 wasted=1 (20%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=6 wasted=6 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=4 wasted=0 (0%) satAt=iter15 lagIters=0 lagSeconds=0s

### snapshotAge=2m0s, promptDelta=1500, iterInterval=7

- profile=realistic   fired=5 wasted=2 (40%) satAt=iter19 lagIters=1 lagSeconds=49s
- profile=pessimistic fired=6 wasted=6 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=3 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=2m0s, promptDelta=1500, iterInterval=10

- profile=realistic   fired=4 wasted=2 (50%) satAt=iter20 lagIters=2 lagSeconds=84s
- profile=pessimistic fired=4 wasted=4 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=2 wasted=0 (0%) satAt=iter14 lagIters=0 lagSeconds=0s

### snapshotAge=2m0s, promptDelta=2000, iterInterval=5

- profile=realistic   fired=5 wasted=2 (40%) satAt=iter20 lagIters=2 lagSeconds=84s
- profile=pessimistic fired=5 wasted=5 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=3 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=2m0s, promptDelta=2000, iterInterval=7

- profile=realistic   fired=5 wasted=3 (60%) satAt=iter20 lagIters=2 lagSeconds=84s
- profile=pessimistic fired=5 wasted=5 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=3 wasted=0 (0%) satAt=iter13 lagIters=0 lagSeconds=0s

### snapshotAge=2m0s, promptDelta=2000, iterInterval=10

- profile=realistic   fired=3 wasted=1 (33%) satAt=iter18 lagIters=0 lagSeconds=0s
- profile=pessimistic fired=4 wasted=4 (100%) satAt=iter0 lagIters=-1 lagSeconds=-1s
- profile=early_done  fired=2 wasted=0 (0%) satAt=iter14 lagIters=0 lagSeconds=0s

## 16.5 三档推荐

基于扫描数据, 给出三档候选, 由用户根据风险偏好选择:

### 档 A (温和): snapshotAge 30s→60s, promptDelta 500→1000, iter 不变 (5)

- pessimistic fired: 13 → 9 (-31%)
- realistic fired: 11 → 8, wasted: 4 → 4, lagIters: 0 → 1
- 风险: 极低. 几乎不增加 loop 退出延迟, 但只削减约 30% 浪费.
- 适合: 严格质量场景 (渗透测试 / 金融审计), 想稍微省一点又不愿意冒任何延迟风险.

### 档 B (中等, 推荐): snapshotAge 30s→90s, promptDelta 500→1500, iter 不变 (5)

- pessimistic fired: 13 → 8 (-38%)
- realistic fired: 11 → 7, wasted: 4 → 2, lagIters: 0 → 1
- 风险: 低. lagIters <= 2 (loop 最多多跑 2 轮 ≈ 60-90s), wasted 下降 ~50%.
- 适合: 大多数业务场景 (默认推荐). 在性能与响应性之间取得平衡.

### 档 C (激进): snapshotAge 30s→120s, promptDelta 500→2000, iter 不变 (5)

- pessimistic fired: 13 → 5 (-62%)
- realistic fired: 11 → 5, wasted: 4 → 2, lagIters: 0 → 2
- 风险: 中. lagIters 可能 2-3 (loop 最多多跑 2-3 轮 ≈ 90-150s), 部分中间 delivery 快照会被跳过.
- 适合: 成本控制型 (批量自动化扫描 / 无人值守任务). 注意配合 watchdog 2min 兜底.

### 隐藏甜点 (扫描发现): snapshotAge 30s→120s, promptDelta 500→1500, iter 不变 (5)

- pessimistic fired: 13 → 6 (-54%)
- realistic fired: 11 → 5, wasted: 4 → 1, lagIters: 0 → 0 (**零延迟**)
- early_done satAt: iter15 (groundTruth=12, 提前完成场景仍能及时检测)
- 关键洞察: 把 token 门保持 1500 而非 2000, 既享受 120s 时间门带来的大幅节省 (-54%), 又因 token 门更敏感保留了 realistic 画像下 satisfiedLag=0 的响应性.
- 风险: 低-中. 对比档 C 多消耗 1 次 AI 调用 (pessimistic 6 vs 5), 换来 lagIters 从 2 降到 0. 是 "性价比最高" 的组合.
- 适合: 介于档 B 与档 C 之间的折中选项.

## 16.6 风险与注意

- 末轮兜底门 (iter == maxIter) **不动**, 保证最终一定会有一次 verification.
- iterInterval 不动 (维持 5), 确保 "长时间不调用" 场景下至少每 5 轮强制 verify.
- watchdog 兜底 (`verificationWatchdogIdleTimeout = 2 * time.Minute`) **不动**, 极端情况下 2 分钟无调用必触发.
- 显式调用路径 (`VerifyUserSatisfactionNow` / `request_verification` action) 不受本次默认值调整影响.
- 提前完成场景 (`early_done` 画像在 iter=12 就 Satisfied=true): 三档都能在 iter=14 之前检测到 (token 门或 iter 门会兜底).
- 副作用代价说明: lagIters>0 意味着 loop 多跑了几轮 "无用工具调用", 但每次工具调用本身有自己的 perception/反思节流, 不会失控.
