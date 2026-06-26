`rehs` 库是多正则批量匹配引擎，借鉴 Intel Hyperscan 的"统一编译，一次扫描"模型：把成百上千条正则统一编译为一个不可变的 Group，对输入数据只扫描一次即可返回所有命中，从而避免"几百条正则逐条匹配"造成的 O(正则数 × 数据量) 性能问题。全程零外部依赖，默认 CGO 构建启用自带位并行内核，无 CGO 时自动退化为纯 Go 参考实现，全平台可移植。

典型使用场景：

- 批量编译与匹配：`rehs.BuildGroup` 把一组正则编译为可复用、并发安全的 Group，`group.Match` 判定是否命中，`group.Find` 返回全部命中（含偏移与内容）。
- 存在性快路径：`rehs.existenceOnly` 选项只判"哪些规则命中"而不取精确偏移，走纯位运算快路径换取更高吞吐（适合打标/分流等只需存在性的场景）。
- 命中枚举：`group.MatchedPatterns` / `group.MatchedIndexes` 返回命中的正则集合，`group.Count` 返回命中总次数，`group.Scan` 流式回调。
- 一次性判定：`rehs.MatchAny` 编译后判定数据是否命中任意一条（不复用 Group，适合临时判定）。

与相邻库的关系：`re` / `re2` 是单正则工具（逐条编译、逐条匹配），`rehs` 面向多正则批量场景（一次编译、一次扫描）。`re` 基于标准 RE2（不支持反向引用），`rehs` 同样基于 RE2 自动机方法。当规则量较大时优先用 `rehs`，单条正则匹配仍用 `re`。

快速上手（本地编译与匹配，不出网即可验证）：

```yak
// 把多条正则统一编译为一个 Group
group = rehs.BuildGroup(["admin", "(?i)password", "token=\\w+"])~

// 存在性判定：命中即停，最快
assert group.Match("see admin here"), "should match admin"

// 取全部命中：含正则、偏移与命中文本
for m in group.Find("admin token=abc123") {
    println(m.Pattern, m.From, m.To, m.Value)
}

// 枚举命中的正则集合
pats = group.MatchedPatterns("admin password token=zzz")
assert len(pats) == 3, "should match all three"

// 查看后端信息
info = group.Info()
log.info("backend=%v tier=%v simd=%v patterns=%v", info.Backend, info.Tier, info.SIMD, info.NumPatterns)
group.Close()
```
