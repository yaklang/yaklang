# Shark 两阶段分支整理：2026-11-01

按用户指令只保留 M1、M2 两个活动阶段；不把协议阶段整理当成所有核心协议验收完成。

最新main又前进到 `238faf87f36e9eff5ec1ea0ff06e8c06d473214c`，已通过普通merge
`49bb60b02655bb73c75be39ffae9c26a8836c582` 纳入M2；保留两边既有修改并确定性重建压缩规则。
当时修正合并测试/干净构建为 `d1a6e62123ea8c6b6d3b43838c12beb86943ddaa`；随后8a检查点已47checks/2statuses及313158精确traffic ID全绿，实际checkout和完整产物已核验。
[本轮真实Finished、Windows、完整来源及最终CI门](shark-m2-tls-finished-20261101.md)。
当前非SMB修复/干净构建为 `488147e9cd490f863edbca5720ff39a85d8cfcf1`，
[CertificateVerify、实际Windows可见方案及原始来源](shark-m2-tls-certificate-verify-20261101.md)。
继续同一M2阶段PR #5371；后续文档head的CI单独验收，不能沿用8a成功。
后续631已47checks/2statuses及313256精确traffic ID全绿。最新非SMB代码/干净构建为
`0fc42e285292b38090c24e77d114bdb7da009d61`，
[QUIC资格、真实流量、83个Windows状态及大捕获逐对象核对](shark-m2-quic-crypto-qualification-20261101.md)。
本轮四个代码提交继续同一M2，无新阶段分支/强推；SMB仍跳过。最终说明head的CI独立验收，不能沿用631成功。
随后同一M2的代码/干净构建为 `788c8e27ba38d9037a4a75284bbcada74f843d7e`，
[捕获TCP误QUIC、未知流观察及IPv4原始长度修复](shark-m2-tcp-carrier-and-length-20261101.md)。
54项中44个有限完成、10未关闭（含3个SMB搁置）；实际Windows、完整大捕获和127新终态已核，
最终说明head的完整CI仍待独立核验。维持M1已合并/M2唯一活动阶段，没有新的SMB语义分析或强推。
后续同树/相同patch哈希只描述f99当时连接M1 squash的历史动作，不适用于新增main改动后的整棵树。

后续状态更新：M1 #5353 已 squash 合并到 main `dba37562abddd57fc6386818dbd7953eb52d314c`，
其整个树 `9744c0cb14e9cc80fe3e7c0d82479438c5a0eb89` 与已验收 a9 完全相同。
M2 #5371 已自动改以 main 为基线，现以普通 merge
`f99ef2b7ad97c135d9f2e49add582cf8b8e46e2d` 连接该基线。合并前后整个 M2 树相同，
相对 M1/main 的 patch SHA256 均为
`f0bc0aa074b1c0d56f65180ab1abaf2fec7ebd38bae4cbcb090c224e9fe6d804`。
未改写提交或强推；阶段文件差异不重复引入 M1。M1为已合并阶段，后续仅维护 M2 活动分支。
下文未合并/待review等状态是整理时的历史快照；新正文/Windows/回放见
[CertificateRequest记录](shark-m2-tls-certificate-request-20261101.md)，最终SHA的CI由阶段PR另验。

| 阶段 | 活动分支 / PR | 基线与边界 |
|---|---|---|
| M1 | `codex/shark-protocol-infrastructure` / [#5353](https://github.com/yaklang/yaklang/pull/5353) | 实际 rebase 到 main `b22442c81d64f7d825f133931d571f7d0f4d6586`；最终 `a9b8df4f64364065765fc0f7f1df4ed9d68cf7dc`。不含 M2 功能 |
| M2 | `codex/shark-m2-protocol-correctness` / [#5371](https://github.com/yaklang/yaklang/pull/5371) | 以新 M1 为直接阶段基线，保持 draft；代码归并/移基后的检查点 `8b7f4f2d6e96040521b36da4e6c41c188cb731d0`，最终说明提交及其 CI 另在 PR 记录 |

M1 的293个提交重放成功。普通rebase省略了旧merge提交附带的15行历史说明，已原样恢复，
并单列本次记录；与原761bd254相比只有一份里程碑说明变化，生产代码、测试、捕获ZIP与配置相同。
整理时曾用精确旧远端SHA的force-with-lease发布，不覆盖他人并发提交；“M1没有合入main、仍走原PR审查”为当时快照。现M1已合并，后续main同步均为普通merge，没有新增强推。

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
