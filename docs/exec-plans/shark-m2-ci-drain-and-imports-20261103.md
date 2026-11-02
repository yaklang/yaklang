# M2-80：修复最终 CI 暴露的结果丢失和错误类型身份

本项是 M2-11 最终提交验收的前置修复，不是新的协议覆盖声明。父提交为
`c292544105682dac07d90f2d08399c2e4f0aecd1`。该提交的流量专项已经通过，但整体 CI 未通过：

- [手动 Essential](https://github.com/yaklang/yaklang/actions/runs/41524204300)
  的 AI/RAG job `125550092758` 中，`TestYieldKHop_ConcurrentAccess` 期望 3 个结果，只返回 2 个。
- [自动 Essential](https://github.com/yaklang/yaklang/actions/runs/41524118855)
  的 SSA frontend job `125549283784` 中，`TestTypeNamePriority` 期望 2 个类型名，得到 3 个。
- [流量专项](https://github.com/yaklang/yaklang/actions/runs/41524197531)
  的实际 checkout 为 c292。官方 artifact `13180167186` 为 17051141 字节，SHA256
  `7665f5be7ae37488edc61052041ba0dda04a2e72b2a183f7367387046558c0e4`。
  下载前核对官方摘要/大小，下载后核对 ZIP CRC、三个成员、实际 checkout 与上传日志；
  再逐 ID 对冻结清单：316025 原始记录，316000 pass、25 历史 optional skip；
  85 新增和 131084 required 全 pass/0 skip。此成功不能替代失败的 Essential。

## 原因和修复

`YieldKHopEx` 在结果生产者返回时取消共享 context。`UnlimitedChan.Close()` 只关闭输入，
其缓冲仍需异步排空；紧接着取消会丢弃已经排队的尾部。现在由输出转发者在全部结果交付后
取消内部管线；调用者取消仍能中止阻塞的交付。保留去重、limit 以及全部旧断言。

Java 显式导入包尚未绑定时，并发编译会创建临时 blueprint，先附加导入者包的猜测全名，
再附加实际显式导入名。例如 `org.target.Owned` 错误地同时获得 `org.importer.Owned`。
仅在这个 fallback 路径存在显式导入且两个全名不同的情况下移除该错误猜测，保留其他名称、
显式导入/SCA 处理以及未导入未知类型的本包 fallback。没有改全局 typeName 去重或弱化数量断言。
本项不宣称 Java 全部并发身份绑定已完备。

## 本地验收

环境：Windows、Go 1.22.12、`GOMAXPROCS=3`、`GOMEMLIMIT=2GiB`、`GOFLAGS=-p=1`。
证据均在仓库外 `C:/tmp/yak-shark-ESP-SSH-UI-20261028/`，不提交完整日志、外部捕获或密钥。

| Profile | 实际结果 |
|---|---|
| `CI-core-repro1530-01` | 旧代码两个原失败项单独重复 100 轮全通过，说明单项重跑不足以关闭问题 |
| `CI-Java-full-repro1531-01` | 旧代码完整 Java 包重复 3 轮，3 次失败动作涉及原 `TestTypeNamePriority` / `TestParamTypeName` |
| `CI-Java-import-red1533-01` | 新回归覆盖 1/2/4 workers 和两种文件顺序；旧代码 70 次运行有 43 次失败动作，含父子项，不能算 43 个独立 bug |
| `CI-RAG-drain-red1534-01` | 新回归延迟读取、验证具体三条路径；旧代码 20 轮中 18 轮失败 |
| `CI-core-green1537-01` | 修复后新回归及三个原失败项重复 20 轮，220 次测试动作全 pass |
| `CI-core-full1538-01` | 两个受影响包完整重复 5 轮：447 个唯一 ID、442 pass/5 历史 skip；2235 次动作中 2210 pass/25 skip，0 fail |

完整复跑的 stdout SHA256 为
`ac2ad2c17103e75dabee27f688d1605b4cf89e3ffe68530cca3442d82c188722`。
执行前后固定 HEAD、index、状态及受影响 AI/Java/SSA/chanx/测试输入摘要，未在读取期间修改源码。
新增 8 个测试 ID；全部继承断言保持。资格摘要 `CI-core-qualification1539-01/summary1539.json`
SHA256 为 `c9cd322449ff0b8fc57875729503a67166dd7485fdcddd62ffcb72f27184f3ec`。

没有修改 Shark 生产解析、fixture ZIP 或 CI 工作流。最近一次实际 Windows 和完整大捕获证据
仍是 [e714 的有限 QUIC 合同](shark-m2-quic-early-order-20261103.md)，不冒充本次新构建。
新的最终 SHA 必须重新执行全部约定 CI 和流量专项；c292 的失败记录保留，不能靠重跑覆盖。

## 未关闭事项

本地已复现先到 0-RTT 无已选套件/握手 context 的恢复缺口，15 个新回归在旧代码全部失败。
该未实现原型暂存仓库外 `M2-QUIC-early-recovery1538-01/`，SHA256
`fc79e282fc5672bc83418dee0d3e2fa898ac45134701970dc28371c8a1c47f6c`；
没有把失败原型混入本次 CI 修复。N09.R03/R04、N10、N13、N14 继续开放，SMB S01–S06 继续跳过。
当前 80 个父项中 70 个在各自有限合同完成，7 个进行中、3 个 SMB 搁置，共 10 个未关闭。
非 SMB 四个开放工作包的既有实施粗估仍为 17–34 AI 小时，执行等待和新增问题另计。
