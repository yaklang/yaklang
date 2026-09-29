# Evidence 生命周期与测试入口

Evidence 的事实源是 Timeline journal。`evidenceJSON` / 数据库 `quoted_evidence` 仅为业务镜像和旧数据导入来源；semi 是已冻结证据的提示词视图，不是另一套存储。

## 写入、冻结、展示

1. `save_evidence` 校验并规范化内容；缺少语义 ID 时按内容生成稳定 ID。plan 的 `output_evidence` 兼容入口复用相同写入函数。
2. Config 在会话锁内调用 Timeline；Timeline 锁内读取当前状态、应用操作、执行容量裁剪、比较差异，并一次追加全部 UPSERT / DELETE。内容不变不新增 delta；预算淘汰同样生成 DELETE。
3. 未冻结 delta 依照原 item 顺序进入普通 Open Timeline。后续修改只追加；不会折叠或重写已有 delta。主循环和参数生成共享该记录源。
4. 追加时检查公共时间/字节桶；delta 大小参与预算。freeze 在同一把锁内提交边界并聚合证据：同 ID 最后一次 UPSERT 生效，DELETE 移除。显式 FreezeAll 可封尾；压缩前只封选中的历史范围。
5. 构建提示词只读：Open 渲染未冻结 delta；semi 按 ID 排序展示有效快照；普通 frozen 历史不再重复展示证据控制项。
6. 保存后回执只保留 ID / 状态。旧记录中曾重复写入的全文在提示词投影时剔除，原始用户审计记录不变。
7. 保存与恢复后的 flush 使用同一个持久化函数，同次更新 `quoted_timeline` 和 `quoted_evidence`；fork 分支不覆盖父会话数据库。恢复后即使没有新写入，再次恢复也不能丢失 journal / 冻结状态。
8. 压缩不把证据控制项送入模型，也不删除这些记录。fork / merge / rollback / dump / restore 通过 journal 和冻结元数据重建状态。

冻结后的新更新先留在 Open，因此 semi 的旧值可能暂时仍在；后面的 UPSERT / TOMBSTONE 明确覆盖它。下一次 freeze 才修改 semi。这是保持稳定前缀的约定。

重启会重新分配 Timeline ID；本进程未冻结追加的前缀稳定性与跨重启的状态一致性分开验证，不承诺跨重启的 Open 标签逐字节相同。

## 测试布局

所有 Evidence 专属测试均命名为 `evidence_*_test.go`，留在所属 Go 包，便于测试内部实现并避免循环导入。跨功能测试中仅作为普通输入出现的 “evidence” 单词不表示独立 Evidence 测试。

| 层次 | 文件（相对于本目录） | 主要覆盖 |
| --- | --- | --- |
| 内容与操作 | `aicommon/evidence_markdown_test.go`、`evidence_store_test.go` | 规范化、稳定 ID、非法输入、操作元数据、旧 JSON |
| Timeline 生命周期 | `aicommon/evidence_timeline_test.go` | Open 原位及字节前缀、读操作无副作用、更新/删除、预算、联合冻结、两轮聚合、UI 事件、并发、fork/merge/rollback、恢复 |
| 历史压缩 | `aicommon/evidence_compression_test.go` | mock reducer 和 emergency 路径、排除精确证据、压缩后持久化恢复 |
| 历史投影 | `aicommon/evidence_projection_test.go` | 旧回执正文与 shrink 缓存不会重复注入，原始审计保留 |
| 投影隔离 | `aiprojection/evidence_nonce_test.go` | Evidence 中伪造 AITAG 不干扰真实 replay |
| Action 局部 | `aireact/reactloops/loopinfra/evidence_action_test.go` | 注册、写入、幂等、更新、缺省 ID、非法输入 |
| 完整小链路 | `aireact/evidence_lifecycle_test.go` | 真实 Action→Config→Timeline→完整 loop prompt；回执不重复；freeze；删除 |
| Prompt 位置 | `aireact/evidence_prompt_test.go`、`evidence_params_prompt_test.go` | 主循环、TODO 相对位置和参数生成 Open/semi 路由 |
| 数据库恢复 | `aireact/evidence_persistence_test.go` | 跨运行持久化、连续空闲重启、冻结范围保留、删除不复活 |
| Plan | `evidence_plan_action_test.go`、`evidence_plan_context_test.go` | 兼容动作共享 journal，旧 plan_evidence 不再进入动态提示词 |
| 验证/指导提示词 | `aireact/evidence_verification*_test.go`、`evidence_policy_test.go`、`evidence_tool_history_test.go`、`aireact/reactloops/loop_plan/evidence_guidance_test.go` | mock 验证输出解析、证据纪律、历史工具信息和指导文档 |
| 子任务 | `aireact/reactloops/evidence_subagent_context_test.go` | 排队快照、继承/隔离模式、清洁时间线的上下文继承 |
| 专用分析循环 | `aireact/reactloops/loop_http_flow_analyze/evidence_findings_prompt_test.go` | 独立 evidence 字段及答复职责 |

查找测试：

```powershell
rg --files common/ai/aid -g 'evidence_*_test.go'
```

运行全部 Evidence 测试（无需远端模型；持久化测试使用测试运行时的本地数据库）：

```powershell
go test -p 1 ./common/ai/aid/aicommon ./common/ai/aid/aiprojection ./common/ai/aid/aireact ./common/ai/aid ./common/ai/aid/aireact/reactloops ./common/ai/aid/aireact/reactloops/loopinfra ./common/ai/aid/aireact/reactloops/loop_plan ./common/ai/aid/aireact/reactloops/loop_http_flow_analyze -run Evidence -count=1 -timeout 120s
```

## 清理边界

删除旧 SessionPromptState 独立写入、镜像单写数据库 helper、plan_evidence 私有 Markdown 存储/合并器及动态 prompt 的共享证据全文入口。mock 同样走 Config/Timeline。还删除了无人调用的旧 Evidence AITAG 解析链、独立展示模板和旧 prompt observation builder；perception 不再读取无人维护的 plan_evidence 键。

保留有实际用途的读取兼容：旧 evidence JSON 在初始化时一次性导入、旧控制条目的 prompt 过滤、plan 输入历史标签的清理、仍在使用的 output_evidence Action。它们不构成第二套可写存储或独立 Open 展示入口。

## 本次验证（2026-09-27）

共 20 个 Evidence 测试文件、42 个顶层测试，全部执行通过。加上 Timeline freeze / promotion、后台子任务、FACTS 解析的相邻回归，本次在 8 个包执行 76 个顶层测试，全部通过。模型相关场景使用 mock，不访问远端模型。本次遇到 Windows 链接器报错后，以 `-p 1` 串行构建重跑通过。
