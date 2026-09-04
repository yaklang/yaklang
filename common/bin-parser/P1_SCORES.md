# P1 协议交付打分

对照 [PROTOCOL_DELIVERY.md](PROTOCOL_DELIVERY.md)。机器可读记录在 `p1_scores.go`，由 `TestP1ScorecardsCovered` 校验：每个 P1 名称都有计分卡；`Status: done` 必须是 **A**（总分 ≥ 90）。

维度（满分 100）：Schema 25 / 真实流量 25 / 测试 20 / 分支覆盖 20 / 栈集成 10。硬门槛 G1–G8 全过才计分。

别名与主规则共用同一张卡（见 `AliasOf`）。样本来源包括 gopacket 测试帧、RFC 完整 PDU，以及 Ethernet+IP+L4 整帧断言。
