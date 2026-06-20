`suricata` 库提供 Suricata 规则的解析、匹配与流量生成能力，可加载 IDS 规则对数据包做匹配，或据规则生成"混沌流量"用于检测设备验证，常用于流量检测与 IDS/IPS 规则研究。

典型使用场景：

- 规则解析与匹配：`suricata.ParseSuricata(raw)` 解析规则，`suricata.NewSuricataMatcher` / `suricata.NewSuricataMatcherGroup` 创建匹配器对数据包匹配。
- 规则库：`suricata.LoadSuricataToDatabase` 入库，`suricata.YieldRules` / `suricata.YieldRulesByKeyword` 检索规则。
- 流量生成：`suricata.TrafficGenerator()` 创建 ChaosMaker，据规则生成符合特征的流量（用于验证检测能力）。

与相邻库的关系：`suricata` 与 `pcapx`（抓包/造包）配合做流量层匹配与生成，是流量侧检测与红蓝对抗的工具。
