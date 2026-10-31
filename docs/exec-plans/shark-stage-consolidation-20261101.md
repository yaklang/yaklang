# Shark 两阶段分支整理：2026-11-01

按用户指令只保留 M1、M2 两个活动阶段；不把协议阶段整理当成所有核心协议验收完成。

| 阶段 | 活动分支 / PR | 基线与边界 |
|---|---|---|
| M1 | `codex/shark-protocol-infrastructure` / [#5353](https://github.com/yaklang/yaklang/pull/5353) | 实际 rebase 到 main `b22442c81d64f7d825f133931d571f7d0f4d6586`；最终 `a9b8df4f64364065765fc0f7f1df4ed9d68cf7dc`。不含 M2 功能 |
| M2 | `codex/shark-m2-protocol-correctness` / [#5371](https://github.com/yaklang/yaklang/pull/5371) | 以新 M1 为直接阶段基线，保持 draft；代码归并/移基后的检查点 `8b7f4f2d6e96040521b36da4e6c41c188cb731d0`，最终说明提交及其 CI 另在 PR 记录 |

M1 的293个提交重放成功。普通rebase省略了旧merge提交附带的15行历史说明，已原样恢复，
并单列本次记录；与原761bd254相比只有一份里程碑说明变化，生产代码、测试、捕获ZIP与配置相同。
用精确旧远端SHA的force-with-lease发布，不覆盖其他人的并发提交。M1没有合入main，仍走原PR审查。

## 已归并的批次

以下9个PR均已实际合并到M2阶段分支，并保留GitHub合并记录，而不是仅关闭或删除：

| PR | 原批次提交 |
|---|---|
| #5376 | `a4077dd1162bac1fb672a0cd91dfb487117cdaa3` |
| #5377 | `d47f13b6b8a7ac8fdcdf6288b7be0061a88dae51` |
| #5378 | `174e141a46d59ef065dc23356704c06666ea8e77` |
| #5379 | `8ed202b62d56ceaedf8a00cea0806090daeed193` |
| #5380 | `f743b953bf6cfd5097a5d6db597064f2525ed2ca` |
| #5381 | `39ca64b4468c81a43c2104ee909b0d21063ab5a9` |
| #5382 | `13ae993d4f81a363d1b78d20439374184f62550c` |
| #5383 | `26bc40ff20b9c4e88313c578895be0dd1f3400f6` |
| #5384 | `456dec17ebdab2e9df9d32adbb7d97e17b0ece9d` |

汇总后的原M2提交为`7cd239bda531c1ba52cdf5cfb15559c1a9dcd963`，原M2阶段
`f9faae9f82c0dbe6a657bfaa94d7d6acdc845380`及以上各批次提交均为其祖先。
之后用rebase-merges把M2移到新的M1，保留批次合并结构，避免阶段PR再次列出旧M1提交。
发布M2也使用精确旧汇总SHA的lease。

仅两份状态文档发生集成冲突：主验收清单及SMB搁置记录。按原#5381合并的完整文档解决，
保留更新的CI证据和所有未关闭缺口；没有为消除冲突改写协议算法或测试预期。

移基前先从原M1基线39729081、原M2汇总树及新M1树独立计算三方合并结果。
当前Git的merge-tree不支持指定merge-base参数，首轮工具失败保留；第二轮通过仅有相同基线父节点的
临时commit对象固定三方输入，未创建额外活动分支。预期树在候选重放前冻结为
`68cefd7391f6e6bd6e567d32bc79c0bf59a5be6b`，M2移基后完整文件树精确相同。
这证明归并内容与冻结的集成结果一致，不是Wireshark语义正确性的证明。

## SMB和其他开放项

SMB按用户要求继续跳过。[搁置清单](shark-m2-smb-deferred-20261031.md)的M2-04/05/36及子项不关闭。
#5380已有代码只随M2历史保存，不继续分析/补齐SMB；原最终fuzz取消、SDK/大包/Windows重建缺口仍保留。
完整回归中既有SMB测试正常执行，也不替代那些profile的运行验收。

主表仍为43项：34有限完成、6进行中、3SMB搁置。非SMB N01–N13仍7有限完成、6开放；
HTTP永久缺方法、TLS1.2历史恢复、QUIC缺握手、TCP/UDP分区及CertificateRequest正文/身份资格门不关闭。
详见[主清单](shark-m2-correctness-checklist.md)、[非SMB逐项清单](shark-m2-non-smb-differences-20261031.md)。
本文及旧报告中的分批SHA、分支和PR状态属于各次执行时的历史事实，当前活动分支以本页为准。

## 历史保留与验证

删除批次分支前保存以下归档标签；它们是恢复点，不是第三个活动阶段：

- `archive/shark-m1-before-rebase-20261031` → 原M1 `761bd25449d1a27bd024785be7b760208a1b58cb`。
- `archive/shark-m1-pre-25cd-20261031` → 更早M1备份 `25cd26324924dad8d1babd6f05101fab1238109b`。
- `archive/shark-m2-merged-before-rebase-20261101` → 原M2完整汇总 `7cd239bda531c1ba52cdf5cfb15559c1a9dcd963`，包含全部原批次历史。

这次未新增协议功能或捕获样本，不修改中央ZIP/manifest/index，也不把外部PCAP、真实抓包、
keylog/密钥、完整导出或fuzz corpus入库。完整状态/合并回执/树比较/最终CI证据留在仓库外
`C:/tmp/yak-shark-ESP-SSH-UI-20261028/stage-consolidation-20261031-01/`。
两个阶段均按各自精确最终SHA验证四组CI、实际checkout、完整产物及必需测试清单；
历史成功、skipped或仅本地通过均不替代本次必需CI。最终结果以两个阶段PR的交接说明为准。
