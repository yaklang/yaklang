# 当前解析范围（2026-09-07）

PR #5013 交付样本范围内的协议字段解析、完整解析路径优化和明确消息范围的 P0 验收。Cassandra / Memcached 的样本入口及性能口径见 [README](README.md)；具体协议和字段限制以 [协议目录](protocol_catalog.go) 及各回归测试为准。

LDAP、MySQL、PostgreSQL、SMB3 按 [P0 消息合同](P0_SCOPE.md) 执行独立正负例、字段、边界与原始样本检查，能力目录继续为 `partial`。新增入口不代表完整协议、完整会话或穷尽分支支持。

以下 14 项不计为本次完成：

- 外层范围：AnyDesk、DingTalk、DoH、DoQ、DoT、HTTP/3、IMAPS、SMTPS、T.38、WeChat/MicroMsg。
- 识别范围：爱奇艺 P2P、腾讯游戏、网易游戏、米哈游/HoYoverse。

已有封装、识别和负例仍用于回归，不将它们升级为完整应用字段能力。流亲和、TCP 重组、实时捕获容量、客户端和 aid 集成属于后续工作。
