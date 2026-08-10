# SyntaxFlow 规则分组与扫描策略设计

状态：Accepted（grilling 2026-07-24）→ **去 Package 枢轴 2026-08-05**  
范围：后端 schema / sfdb / sync / policy / grpc；前端另开 PR。

验证时间：2026-08-05T16:44:57+08:00

---

## 0. 结论（当前实现）

**没有 Package 抽象。** 分发桶与规则归属统一用 **Group**：

| 能力 | 实现 |
|------|------|
| **互斥归属** | `SyntaxFlowRule.RuleGroup`（列 `rule_group`）：`builtin` / `agent` / `custom` / … |
| **组目录** | `SyntaxFlowGroup`（`GroupName` + `IsBuildIn`）；**无** many2many |
| **旧中间表** | `syntax_flow_rule_and_group` 闲置不删（软兼容） |
| **分类** | 原子 `Tags` + 列字段 Language/Severity/Purpose |
| **扫描选规则** | Policy = 预设 Filter（Tag/列/GroupNames） |
| **内置 sync** | embed 目录 `buildin/`→`builtin`，`agent/`→`agent`（无 package.yaml） |

Wire：`SyntaxFlowRule.GroupName` 为 `repeated`，实际长度 ≤ 1。

已删除：`SyntaxFlowPackage` 表、PackageYAML、Package gRPC/CLI、`PackageName` / `PackageNames` 字段。

---

## 1. 模型

```text
Group catalog (SyntaxFlowGroup)
        ▲
        │ RuleGroup (scalar)
        │
      Rule ──N:M──► Tag（原子短标）
        │
        ▼
     Filter（列字段 + Tag + GroupNames + 关键词）
        │
        ▼
     Policy（预设 Filter；可跨 Group）
```

保留组名：

- `builtin`：默认 embed 扫描规则
- `agent`：第二套内置规则
- `custom`：用户新建默认组
- `imported-*`：导入时可选用

---

## 2. 前端要点

- 规则管理：用现有 Group API（Query/Create/Update/Delete）+ Filter.GroupNames / Tags
- 不要接 Package RPC（已移除）
- 扫描：选 Policy（`QuerySyntaxFlowScanPolicies` / `SyntaxFlowScanPolicyToRuleFilter`），或直接 Filter；Group 仅作归属筛选，不是 Policy 本体

### Policy gRPC

| RPC | 用途 |
|-----|------|
| `QuerySyntaxFlowScanPolicies` | 列出策略、分类、自定义可选 Tag；每条带默认 `Filter` |
| `SyntaxFlowScanPolicyToRuleFilter` | 将 `PolicyId`（+ custom Tags/Severity/Purpose）展开为 `SyntaxFlowRuleFilter`，可叠加 Language/GroupNames |
---

## 3. 历史备注

早期设计曾引入独立 Package 表与 `package.yaml`；2026-08-05 起全部回退为 Group-only。
