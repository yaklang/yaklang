# 15. Perception 触发频率仿真实验 / Perception Frequency Experiment

> 回到 [README](../README.md) | 上一章：[14-streaming-ux.md](14-streaming-ux.md)

> 本报告由 [perception_frequency_sim_test.go](../perception_frequency_sim_test.go) 自动生成。
> 每次 `go test -run TestPerceptionFrequencySim ./common/ai/aid/aireact/reactloops/` 会覆盖更新。

## 15.1 案例与节奏

仿真 fixture 来自 redhaze 案例 (`13354_redhaze_login_security_test_20260519_79bae`),
从 `loop_default_action_calls/` 目录文件 mtime 提取的 20 个 iter 时间戳 (相对 iter1=0, 单位秒):

```
iter  1  t=0     phase=1
iter  2  t=23    phase=1
iter  3  t=40    phase=1
iter  4  t=151   phase=2
iter  5  t=169   phase=2
iter  6  t=241   phase=2
iter  7  t=261   phase=2
iter  8  t=372   phase=3
iter  9  t=385   phase=3
iter 10  t=417   phase=3
iter 11  t=454   phase=3
iter 12  t=482   phase=3
iter 13  t=517   phase=3
iter 14  t=554   phase=4
iter 15  t=589   phase=4
iter 16  t=617   phase=4
iter 17  t=644   phase=4
iter 18  t=686   phase=4
iter 19  t=735   phase=4
iter 20  t=770   phase=4
```

阶段划分 (人工根据 action human_readable_thought 标注):

- phase 1 (iter 1-3): 侦察 - recon login page / fetch_full_login_page / get_login_js
- phase 2 (iter 4-7): 初探 SQLi - sql_inject_guest_uid / retry_sqli_password
- phase 3 (iter 8-13): employee 角色 SQLi 深挖 - time_blind / boolean / error_extract
- phase 4 (iter 14-20): union / enterprise 扩展 - union_column_count / extract_db / enterprise_baseline

真实意图 pivot 只发生 2-3 次 (phase 切换), 其余是同领域 drift, 不需要刷新 capability/knowledge.

## 15.2 仿真器与画像

仿真器 `simController` 严格镜像生产 `perceptionController` 的节流逻辑:
`iterationTriggerInterval` / `shouldSkipDueToInterval` / 退避 `*=2 (>=2 unchanged)` / `hashTopics` 比对.
唯一区别是 `now` 用显式参数代替 `time.Now()`, 实现确定性回放.

三种 AI 行为画像 (注: profile stateful, 基于上次返回的 topics 判定 changed):

- `realistic`: AI 按真实阶段返回 topics, 当本轮 topics 与上次返回不同时报 changed=true. 即"理性 AI": 阶段切换则 pivot, 同阶段 drift 则如实说 changed=false. 用于评估理想节流目标.
- `noisy`: AI 每次都 changed=true 且 topics 微变 (按 iter 编号生成 unique hash), 退避完全失效, 所有 fired 都被 ShouldUpdate 接受, 是 AI 调用次数上界.
- `quiet`: 首次 fire 后 AI 永远 changed=false (节俭 AI), 退避充分起作用, 是 AI 调用次数下界.

## 15.3 当前默认 (iter=2, min=30s) 仿真结果

- profile=realistic candidates=10 fired=10 skipped=0 updated=4 wasted=6 (wasted_rate=60%)
- profile=noisy     candidates=10 fired=10 skipped=0 updated=10 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=10 fired=5 skipped=5 updated=1 wasted=4 (wasted_rate=80%)

解读 (realistic 画像下):
- 13 分钟内会发起 10 次 perception AI 调用 (每次 LiteForge SpeedPriority).
- 其中只有 4 次产生有效 updated (含首次), 浪费 6 次, 浪费率 60%.
- 在 noisy 上界 (AI 永远 changed=true) 下, fired 达到 10 次, 全部刷新下游 (capability search + RAG + midterm recall), 是最坏情况.

## 15.4 参数扫描矩阵

扫描空间: iterationTriggerInterval ∈ {2, 3, 4, 5, 6}, minInterval ∈ {30s, 60s, 90s, 120s, 180s}, profile ∈ {realistic, noisy, quiet}.

### iter=2, min=30s

- profile=realistic candidates=10 fired=10 skipped=0 updated=4 wasted=6 (wasted_rate=60%)
- profile=noisy     candidates=10 fired=10 skipped=0 updated=10 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=10 fired=5 skipped=5 updated=1 wasted=4 (wasted_rate=80%)

### iter=2, min=1m0s

- profile=realistic candidates=10 fired=8 skipped=2 updated=4 wasted=4 (wasted_rate=50%)
- profile=noisy     candidates=10 fired=9 skipped=1 updated=9 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=10 fired=5 skipped=5 updated=1 wasted=4 (wasted_rate=80%)

### iter=2, min=1m30s

- profile=realistic candidates=10 fired=7 skipped=3 updated=4 wasted=3 (wasted_rate=43%)
- profile=noisy     candidates=10 fired=7 skipped=3 updated=7 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=10 fired=4 skipped=6 updated=1 wasted=3 (wasted_rate=75%)

### iter=2, min=2m0s

- profile=realistic candidates=10 fired=5 skipped=5 updated=4 wasted=1 (wasted_rate=20%)
- profile=noisy     candidates=10 fired=5 skipped=5 updated=5 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=10 fired=4 skipped=6 updated=1 wasted=3 (wasted_rate=75%)

### iter=2, min=3m0s

- profile=realistic candidates=10 fired=4 skipped=6 updated=4 wasted=0 (wasted_rate=0%)
- profile=noisy     candidates=10 fired=4 skipped=6 updated=4 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=10 fired=3 skipped=7 updated=1 wasted=2 (wasted_rate=67%)

### iter=3, min=30s

- profile=realistic candidates=6 fired=6 skipped=0 updated=4 wasted=2 (wasted_rate=33%)
- profile=noisy     candidates=6 fired=6 skipped=0 updated=6 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=6 fired=5 skipped=1 updated=1 wasted=4 (wasted_rate=80%)

### iter=3, min=1m0s

- profile=realistic candidates=6 fired=6 skipped=0 updated=4 wasted=2 (wasted_rate=33%)
- profile=noisy     candidates=6 fired=6 skipped=0 updated=6 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=6 fired=4 skipped=2 updated=1 wasted=3 (wasted_rate=75%)

### iter=3, min=1m30s

- profile=realistic candidates=6 fired=6 skipped=0 updated=4 wasted=2 (wasted_rate=33%)
- profile=noisy     candidates=6 fired=6 skipped=0 updated=6 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=6 fired=4 skipped=2 updated=1 wasted=3 (wasted_rate=75%)

### iter=3, min=2m0s

- profile=realistic candidates=6 fired=4 skipped=2 updated=4 wasted=0 (wasted_rate=0%)
- profile=noisy     candidates=6 fired=4 skipped=2 updated=4 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=6 fired=4 skipped=2 updated=1 wasted=3 (wasted_rate=75%)

### iter=3, min=3m0s

- profile=realistic candidates=6 fired=4 skipped=2 updated=4 wasted=0 (wasted_rate=0%)
- profile=noisy     candidates=6 fired=4 skipped=2 updated=4 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=6 fired=3 skipped=3 updated=1 wasted=2 (wasted_rate=67%)

### iter=4, min=30s

- profile=realistic candidates=5 fired=5 skipped=0 updated=3 wasted=2 (wasted_rate=40%)
- profile=noisy     candidates=5 fired=5 skipped=0 updated=5 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=5 fired=5 skipped=0 updated=1 wasted=4 (wasted_rate=80%)

### iter=4, min=1m0s

- profile=realistic candidates=5 fired=5 skipped=0 updated=3 wasted=2 (wasted_rate=40%)
- profile=noisy     candidates=5 fired=5 skipped=0 updated=5 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=5 fired=4 skipped=1 updated=1 wasted=3 (wasted_rate=75%)

### iter=4, min=1m30s

- profile=realistic candidates=5 fired=5 skipped=0 updated=3 wasted=2 (wasted_rate=40%)
- profile=noisy     candidates=5 fired=5 skipped=0 updated=5 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=5 fired=4 skipped=1 updated=1 wasted=3 (wasted_rate=75%)

### iter=4, min=2m0s

- profile=realistic candidates=5 fired=4 skipped=1 updated=3 wasted=1 (wasted_rate=25%)
- profile=noisy     candidates=5 fired=4 skipped=1 updated=4 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=5 fired=3 skipped=2 updated=1 wasted=2 (wasted_rate=67%)

### iter=4, min=3m0s

- profile=realistic candidates=5 fired=3 skipped=2 updated=3 wasted=0 (wasted_rate=0%)
- profile=noisy     candidates=5 fired=3 skipped=2 updated=3 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=5 fired=3 skipped=2 updated=1 wasted=2 (wasted_rate=67%)

### iter=5, min=30s

- profile=realistic candidates=4 fired=4 skipped=0 updated=3 wasted=1 (wasted_rate=25%)
- profile=noisy     candidates=4 fired=4 skipped=0 updated=4 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=4 fired=4 skipped=0 updated=1 wasted=3 (wasted_rate=75%)

### iter=5, min=1m0s

- profile=realistic candidates=4 fired=4 skipped=0 updated=3 wasted=1 (wasted_rate=25%)
- profile=noisy     candidates=4 fired=4 skipped=0 updated=4 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=4 fired=4 skipped=0 updated=1 wasted=3 (wasted_rate=75%)

### iter=5, min=1m30s

- profile=realistic candidates=4 fired=4 skipped=0 updated=3 wasted=1 (wasted_rate=25%)
- profile=noisy     candidates=4 fired=4 skipped=0 updated=4 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=4 fired=4 skipped=0 updated=1 wasted=3 (wasted_rate=75%)

### iter=5, min=2m0s

- profile=realistic candidates=4 fired=4 skipped=0 updated=3 wasted=1 (wasted_rate=25%)
- profile=noisy     candidates=4 fired=4 skipped=0 updated=4 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=4 fired=3 skipped=1 updated=1 wasted=2 (wasted_rate=67%)

### iter=5, min=3m0s

- profile=realistic candidates=4 fired=3 skipped=1 updated=3 wasted=0 (wasted_rate=0%)
- profile=noisy     candidates=4 fired=3 skipped=1 updated=3 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=4 fired=3 skipped=1 updated=1 wasted=2 (wasted_rate=67%)

### iter=6, min=30s

- profile=realistic candidates=3 fired=3 skipped=0 updated=3 wasted=0 (wasted_rate=0%)
- profile=noisy     candidates=3 fired=3 skipped=0 updated=3 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=3 fired=3 skipped=0 updated=1 wasted=2 (wasted_rate=67%)

### iter=6, min=1m0s

- profile=realistic candidates=3 fired=3 skipped=0 updated=3 wasted=0 (wasted_rate=0%)
- profile=noisy     candidates=3 fired=3 skipped=0 updated=3 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=3 fired=3 skipped=0 updated=1 wasted=2 (wasted_rate=67%)

### iter=6, min=1m30s

- profile=realistic candidates=3 fired=3 skipped=0 updated=3 wasted=0 (wasted_rate=0%)
- profile=noisy     candidates=3 fired=3 skipped=0 updated=3 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=3 fired=3 skipped=0 updated=1 wasted=2 (wasted_rate=67%)

### iter=6, min=2m0s

- profile=realistic candidates=3 fired=3 skipped=0 updated=3 wasted=0 (wasted_rate=0%)
- profile=noisy     candidates=3 fired=3 skipped=0 updated=3 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=3 fired=3 skipped=0 updated=1 wasted=2 (wasted_rate=67%)

### iter=6, min=3m0s

- profile=realistic candidates=3 fired=3 skipped=0 updated=3 wasted=0 (wasted_rate=0%)
- profile=noisy     candidates=3 fired=3 skipped=0 updated=3 wasted=0 (wasted_rate=0%)
- profile=quiet     candidates=3 fired=3 skipped=0 updated=1 wasted=2 (wasted_rate=67%)

## 15.5 推荐参数与理由 (v2 更激进)

基于扩展扫描数据, 并结合用户洞察 "意图感知主要为补充 capability/SKILL/knowledge,
大方向不变就行, 细节变动反成累赘", 推荐 v2 默认值:

- `perceptionDefaultIterationInterval`: `2` → `3`
- `perceptionDefaultMinInterval`: `30 * time.Second` → `120 * time.Second`

理由:

- realistic 画像下 fired 从 10 降到 4 (-60%), wasted 从 6 降到 0, updated 保留 4 次覆盖所有 4 个 phase pivot, 浪费率为 0%.
- noisy 上界从 10 次降到 4 次 (-60%), 显著缓解 AI 永远 changed=true 时的下游刷新风暴.
- 关键洞察: min=120s 刚好让紧邻的同阶段 drift 候选 (iter 间隔 <100s) 被时间门跳过,
  而真正的阶段切换 (iter 间隔 >150s) 仍可触发. 这与 perception 的语义匹配 ——
  capability/SKILL 一旦加载就保留, 不需要频繁刷新.
- iter=3 在响应性 (首次感知 iter 3, ~40s) 与节流之间取得最佳平衡:
  iter=4 错过 phase 1 (recon, 首次感知推迟到 ~150s), iter=5/6 首次感知更晚.
- 不动 `perceptionMaxInterval` (5 min) 与 `consecutiveUnchanged >= 2` 退避阈值,
  两者在持续 drift 时仍可继续放大间隔, 改动多了风险大.
- 不引入新的 `WithPerceptionXxx` Option, 用户明确选择 defaults_only 路径.

## 15.6 风险与注意

- iterInterval=3 意味着 perception 最早在 iter 3 才能首次感知; iter 4 (phase 2 起点)
  不是 3 的倍数, 最迟到 iter 6 才能感知 phase 2. 在 13 分钟跨度上落后 1-2 个 iter,
  因 capability 是累积加载, 已加载的 SKILL 仍可使用, 风险可接受.
- minInterval=120s 在快节奏 iter (<60s/iter) 场景下会跳过多个候选,
  但这正是设计目标 —— 同领域内的快速 drift 不需要每次重感知.
- spin / forced / loop_switch 三种 trigger 仍然绕门即时刷新, 不受本次默认值调整影响,
  保证了关键场景 (循环卡死/用户显式请求/子 loop 切换) 的响应性.
- 退避算法 (`*=2 at consecutiveUnchanged>=2`) 不变, 在持续 drift 时仍可继续放大间隔到 max=5min.
