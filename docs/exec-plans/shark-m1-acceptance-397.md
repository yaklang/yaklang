# M1 验收交接：39729081a3

M1 已通过本阶段约定范围。最终提交为 `39729081a3300381154d96cf1b4866f23976260b`，
已 rebase 到 main `0614803cb02aafded4bccaf6a562e28db664904f`；
[PR5353](https://github.com/yaklang/yaklang/pull/5353) 已 ready、无合并冲突。
本记录在后续 M2 分支保存，M1 PR 的代码边界保持不动。

验收范围见 [M1 说明](https://github.com/yaklang/yaklang/blob/39729081a3300381154d96cf1b4866f23976260b/docs/exec-plans/shark-m1-milestone-20261030.md)
及其逐 profile 证据。这里不声明所有核心协议完备，也不把回放连续性等同于全部
Wireshark 字段、重组、会话和 fuzz 正确性。

## 最终提交的检查

| 检查 | 实际结果 |
|---|---|
| [Essential](https://github.com/yaklang/yaklang/actions/runs/40988285886) | 41 个 job 成功 |
| [Diff](https://github.com/yaklang/yaklang/actions/runs/40988285899) | 成功 |
| [路径大小写](https://github.com/yaklang/yaklang/actions/runs/40988285940) | 成功 |
| [完整流量专项](https://github.com/yaklang/yaklang/actions/runs/40988285890) | 成功；实际 checkout、Python 控制和 v191 full inventory 均已核对 |
| GitHub checks / statuses | 47 个 check 全部 success；`essential-tests`、`diff-code-check` 均 success；四项 workflow 首轮通过 |
| 本地七包全量 | 285,985 pass /24 个原有 skip /0 fail；286,009 个 run 各有唯一 terminal，原五包测试 ID 和 skip 集合不变 |
| 最终存储 / Windows ACL / AI 消费者 | 70 pass /0 skip /0 fail，8 个必需父测试均实际执行 |
| Python / 可重复生成 | 17 项控制通过；ZIP 和 manifest 完整字节与版本库相同 |
| 新构建公共输入 | 37 项实际执行；307,819 个 native 事件、384,518 条 CLI 行和 2 份完整 flow 文件逐字节一致；无 ID 重映射或 metadata stripping |
| 独立 SSH API / CLI / descriptor | 两组均零差异；38 项原负控仍全部拒绝 |
| Windows 实际页面 | 3 项，物理帧/PDU 全字节、同值不同位置高亮、无范围上下文、来源切换、tabs/mouse/resize、CP936 恢复通过 |
| live Yak fixture loader | 实际读取中央 ZIP 并验证 709-byte 输入；未声称模型/API E2E |

Windows 图片来自真实控制台 cells/attributes 的还原，不称为 Windows Terminal 像素
截图。主/签名格式的 11-byte 字段分别核对 `[14,25)` 与 `[105,116)`；连接上下文不制造
当前位置范围。执行使用 Go 1.22.12、TShark 4.6.9 的既有隔离参考配置；有测试 keylog
和无 keylog 的输入分开，原始数据、导出、日志与图片继续留在仓库外。

## 捕获归档

- 受版本控制的物理存储文件 **38 → 1**：36 个原 ZIP 与两个原散装输入。
- 新文件：`internal/trafficfixture/testdata/captures.zip`，**18,916,097 bytes**，
  SHA256 `0371e1304d6711497f697ea8a7622c707140e7e5378215d67bde7f0985bd8213`。
- ZIP 有 **38 个成员**，原相对路径、长度、SHA256、CRC 与原 Git blob 均一致，
  无遗漏/重复；排序、时间、权限、方法和额外元数据固定。
- 原索引不变：288 个逻辑批次、24,826 个全部成员、25,174 个路径别名。其中历史
  捕获成员 11,990 个、唯一内容 hash 11,980 个、捕获路径别名 12,147 个，另有两个
  原散装输入；这些不是“38 个唯一协议样本”的同一计数。
- 来源、许可、说明、期望保持；实际路径消费者使用私有临时目录，源码检出目录没有
  重新生成散装捕获。外部大包、真实本机抓包、敏感密钥/keylog和完整分析导出未入库。

## 保留的失败与证据口径

Windows JSON-text 路径断言的失败保留，改为解码数组、核对原主路径和精确别名集合，
整包复验通过。中断运行与磁盘保护暂停只续用已有完整 producer/hash 的输出。

rebase 后两个注释合同 Go 文件仅发生 LF→CRLF 的检出变化。旧验收器的原始字节 hash
拒绝也保留；通过前后 Git blob 全等、精确换行变换和反例拒绝建立两文件桥接，再运行
原有全部协议断言。没有修改候选输出、源报文字节、字段预期或原验收器。

最终仓库外汇总 `M1-accepted-final57-01/summary.json` SHA256 为
`c4bee2bd79464c7eebef2c341f037f18b6c8d497d770e15e6ef271c4a9e62998`；
本地全量日志 SHA256 为
`b483f1d47c5cf4de8c5aa3734b09a858aecb0b227aa92aac9602035b7b2d14df`。
这里提交摘要和证据指纹，完整运行产物留在仓库外。

## 继续到 M2

仍未关闭：SMB1→SMB2 的 107 个完整记录/121 个命令头、SMB body/compound/会话与
状态错误、TLS/QUIC 高级恢复和复用组合、其余协议/字段/会话/fuzz 差异。见
[M2 验收清单](shark-m2-correctness-checklist.md)。原 94/39 的 133 项已有历史分类，
不能统称 133 个未修 parser bug；当前阶段复验证据与新发现分开记录。

M2 从上述全绿提交创建 `codex/shark-m2-protocol-correctness`。M1 未合并时，后续 PR
以 M1 分支为基线；合并后再调整，避免重复展示 M1。GitHub main 当前另有两个 review
的审核规则，`REVIEW_REQUIRED` 与本次全部 CI 成功分开，未更改审核或保护规则。
