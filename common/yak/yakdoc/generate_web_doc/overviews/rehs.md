`rehs` 库是零外部依赖的多正则批量匹配引擎，借鉴 Hyperscan 的「统一编译、一次扫描」(compile then scan) 模型：把成百上千条正则编译为一个不可变的 `Group`，对输入只做一次预过滤再验证候选，避免「几百条正则逐条全量匹配」的 `O(N_patterns × N_bytes)` 开销。

典型使用场景：

- **规则集复用扫描**：`rehs.BuildGroup(patterns)` 编译一次，对大量流量反复 `group.Match` / `group.Find` / `group.Scan`。
- **存在性打标**：MITM 染色、IOC 分流等只需「哪些规则命中」时，配合 `rehs.existenceOnly()` 走纯存在性快路径（偏移上报 -1，吞吐更高）。
- **一次性判定**：偶发场景用 `rehs.MatchAny(patterns, data)`，无需维护 Group 生命周期。

与相邻库的关系：`re` / `re2` 面向单条正则的匹配与提取；`rehs` 面向**多条正则的批量存在性/定位扫描**。规则数 N 较大、字面量丰富且命中稀疏时优势最明显。默认后端为自托管 mvscan（`CGO_ENABLED=1` 时编入纯 C99 内核，无 CGO 时退化为纯 Go，结果逐字节一致）；可用 `group.Info()` 观测实际生效后端。
