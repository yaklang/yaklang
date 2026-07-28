# SyntaxFlow 规则包（Rule Package）设计

状态：Accepted（grilling 2026-07-24）  
范围：后端一个大 PR（yaklang schema / sfdb / sync / policy / grpc / cli）；前端另开 PR，本文给出详细前端计划，**不写前端代码**。

验证时间：2026-07-24T22:57:31+08:00

---

## 1. 背景与问题

现状把三件不同的事揉在一起：

| 能力 | 现状 | 问题 |
|------|------|------|
| **分发** | 仅一份 embed `buildin/` + `IsBuildInRule` bool；ZIP 整库导入导出；在线按 filter 逐条下载 | 无法表达第二套内置包（如 agent）；没有包级 semver / 整包 sync |
| **分类** | `SyntaxFlowGroup` 自动挂语言/等级/用途/OWASP/框架；`Tag` 是路径 `|` 串 | 与列字段（Language/Severity/Purpose）三重重复；Group 过载 |
| **扫描选规则** | `scan_policies.yaml` → `rule_groups` → `RuleFilter.GroupNames` | Policy 绑在复杂组名上；前端选择成本高 |

目标操作面拆清：

1. **导入 / 导出 / 下载 / sync** → **Package**
2. **规则管理页查看 / 搜索** → **Filter**（列字段 + Tag + 关键词）
3. **扫描选规则** → **Policy**（预设 Filter）

---

## 2. 目标模型（五层）

```text
Package ──1:N──► Rule ──N:M──► Tag（原子短标）
                      │
                      ▼
                   Filter（查询：列字段 + Tag + 搜索）
                      │
                      ▼
                   Policy（预设 Filter；可跨 Package）
```

### 2.1 Package（分发单元）

- **互斥归属**：一条规则只属于一个 Package；跨包 `RuleId` 与 `RuleName` 均全局唯一。
- **职责**：导入、导出、online 下载/上传、内置 sync 的单位。
- **不负责**：扫描隔离。多包可同时被同一 Policy 命中（互补规则一起工作）。
- **版本**：包级 **semver** 作为「是否进入更新」的闸门；细则用规则自身 `Version`。
- **升级语义**：
  - 包 semver 更新 → 进入同步；
  - 同 `RuleId` + 同 `RuleName` 且规则 Version 更高 → 更新；
  - 双键不完全对齐（同 id 不同名 / 同名不同 id）→ **单条冲突**，回传前端/CLI，用户选覆盖或丢弃；
  - 新包中已删除的规则 → **本地同步删除**。
- **来源**：
  - 内置：`builtin`、`agent`（embed 同级目录）；
  - 外部：online / 本地 zip（同格式）；
  - 用户：默认 `custom`（可改名；可再建用户包）；
  - 无 `package.yaml` 的旧 ZIP：每次导入一个独立包（用户命名或 `imported-<timestamp>`）。

### 2.2 Rule

保持现有核心字段，新增：

| 字段 | 说明 |
|------|------|
| `PackageName` | 所属包名（索引） |
| `Tags` | 原子 Tag 列表（结构化；替代路径 `|` 串的主用途） |
| `Language` / `Severity` / `Purpose` / `CWE` | **继续用列字段**；不再镜像进 Tag |

兼容：短期内可保留旧列 `Tag string`（路径遗留），读写以 `Tags` 为准；迁移完成后可弃用。

### 2.3 Tag（原子短标）

**标准 Tag 策略：**

| 类 | 格式示例 | 规则 |
|----|----------|------|
| CWE | `cwe:89` | 漏洞维优先 |
| 技术点 | `sqli` / `xss` / `ssrf` … | **无 CWE 时**再打 |
| 框架/组件 | `spring` / `shiro` / `fastjson` … | 独立打，与漏洞维无关 |

**禁止进 Tag：** `language` / `severity` / `purpose` / 长 OWASP 组名。

- 允许用户自定义 Tag（非标准）。
- 默认 Policy **只引用标准 Tag**（+ 列字段条件）。

### 2.4 Filter

管理页 / 查询 / 在线筛选的统一条件，对应扩展后的 `SyntaxFlowRuleFilter`：

- 列字段：`Language` / `Severity` / `Purpose` / `RuleIds` / …
- `Tags`（标准 + 自定义）
- `PackageNames`（可选，管理用；扫描主路径通常不靠它隔离）
- `Keyword` / `RuleNames` 搜索
- **废弃作为主路径**：`GroupNames`（硬切后不再由 sync 写入；RPC 可短 stub）

### 2.5 Policy（预设 Filter）

- Policy = 命名的 Filter 模板（主要：标准 Tags + Severity/Purpose 等字段条件）。
- 例：`owasp-web` = 一组 CWE Tag（及必要字段）；`critical-high` = `severity ∈ {critical,high}`；框架类 Policy = 聚合 `spring` 等 Tag。
- 扫描时：**Policy → Filter**，再加 **program 语言** 圈定规则。
- 存储：演进现有 `scan_policies.yaml`（`rule_groups` → `tags` + 可选 `severity`/`purpose`）。

### 2.6 Group

**废弃。** 硬切：更新/sync 时重建 Tag，不保留旧 Group 关系（内置更新本就会重建）。用户自建组不迁移。

---

## 3. 内置布局与 `package.yaml`

```text
common/syntaxflow/sfbuildin/
  builtin/                 # 包名 builtin（由现 buildin/ 迁入或兼容别名）
    package.yaml
    **/*.sf
  agent/                   # 包名 agent（AI Agent 扫描规则）
    package.yaml
    **/*.sf
  standards/               # enricher 映射等（非规则包）
```

过渡期允许目录仍叫 `buildin/`，但 `package.yaml` 的 `name` 必须为 `builtin`；目标结构为同级 `builtin/` + `agent/`。

### 3.1 `package.yaml`（单文件，CI 维护）

```yaml
name: builtin                 # 包名，全局唯一
version: "1.2.0"              # semver，更新闸门
description: "默认代码扫描规则"
source: embed                 # embed | online | local | user
rules:
  - rule_id: "uuid..."
    rule_name: "检测Java SQL字符串拼接查询"   # 或文件侧稳定名，以实现为准
    version: "20260724.0001"
    hash: "sha256..."         # 可选，CI 生成
```

- **人改 `.sf`；CI 扫描生成/更新 `rules` 清单与版本**（替代旁路全局 `rule_versions.json` 的长期方案；迁移期可双写）。
- CI 强制：`builtin` 与 `agent`（及任意内置包）之间 **RuleId、RuleName 零交集**。

### 3.2 外部包格式

- **新包（强制）**：zip 内含 `package.yaml` + 规则文件；与内置同逻辑；online 上传同样校验全局唯一。
- **旧 ZIP（兼容）**：无 yaml → 导入为独立包（命名或 `imported-<ts>`）；规则写入该包。

---

## 4. 更新、冲突、删除

```text
比较 package.semver
  ├─ 本地 ≥ 远端 → 跳过整包（或仅展示）
  └─ 本地 < 远端 → 进入逐条对齐
        ├─ 远端有、本地无 → 新增
        ├─ 同 RuleId + 同 RuleName → 比规则 Version → 更新或跳过
        ├─ 双键不对齐 → Conflict（单条）；UI/CLI：overwrite | discard
        └─ 本地有、远端无（同包） → 删除本地
```

- ZIP 导入：改为版本感知 + 冲突回调（取代当前 `AllowOverwrite=true` 无脑覆盖）。
- 在线下载：在现有 RuleId/Version/`NeedUpdate` 逻辑上抬升到 **包级闸门**。
- CLI 非交互默认：**冲突条拒绝（discard）**，退出码非 0 并打印清单；`--force-overwrite-conflicts` 可选。

---

## 5. 后端切片与模块

### 5.1 Schema

- 新增 `SyntaxFlowPackage`（Profile DB）。
- `SyntaxFlowRule` 增加 `PackageName`、`Tags`（`StringArray`）。
- `SyntaxFlowGroup`：停止业务写入；表可暂留，RPC 标记废弃。

### 5.2 sfdb / sync

- `SyncEmbedPackages`：遍历内置包目录，读 `package.yaml`，按包 sync。
- Tag enricher：产出 `cwe:N` / 框架短名 / 无 CWE 时技术点；**不再** `addGroupsForRule(language/severity/…)`。
- Import/Export：以 Package 为单位；metadata 含 package + rules；恢复时写 `PackageName`/`Tags`。

### 5.3 Policy / Tag

- `scan_policies.yaml`：`rule_groups` → `tags` + 可选 `severity` / `purpose`。
- `ScanPolicyConfig.MapToFilter()` → 填充 `RuleFilter.Tags` / Severity 等，**不再**写 `GroupNames`。
- gRPC：`GetSyntaxFlowScanPolicies`（若尚无）返回策略列表供前端。

### 5.4 gRPC（草案）

| RPC | 说明 |
|-----|------|
| `QuerySyntaxFlowPackages` | 列包（含 version、source、rule count） |
| `CreateSyntaxFlowPackage` / `UpdateSyntaxFlowPackage` / `DeleteSyntaxFlowPackage` | 用户包 CRUD（内置不可删） |
| `ExportSyntaxFlowPackage` | 导出单包 zip（含 package.yaml） |
| `ImportSyntaxFlowPackage` | 导入 zip；流式进度 + **Conflict** 条目 |
| `SyncSyntaxFlowPackage` / `ApplySyntaxFlowPackageUpdate` | 内置/online 更新；冲突交互 |
| `DownloadSyntaxFlowPackage` / `UploadSyntaxFlowPackage` | online 整包 |
| 既有 `QuerySyntaxFlowRule` | Filter 增加 `PackageNames`；`Tag` 按原子 Tag 匹配 |
| 既有 Group RPC | Deprecated：返回明确错误或空，文档标明用 Tag/Package |

冲突进度消息建议字段：`rule_id` / `rule_name` / `local_version` / `remote_version` / `reason` / `resolution`。

### 5.5 CLI

```text
yak syntaxflow-package list
yak syntaxflow-package sync [--package builtin|agent|...]
yak syntaxflow-package export --name <pkg> -o out.zip
yak syntaxflow-package import -i in.zip [--name ...] [--force-overwrite-conflicts]
yak syntaxflow-package update --name <pkg>   # online / embed 检查闸门
```

扫描侧：`code-scan` / 现有配置继续走 Policy；Policy 内部改 Tag Filter。

### 5.6 迁移（硬切）

1. 部署新版本 → sync 内置包 → 丢弃/忽略旧 Group 关系，按 enricher 重写 `Tags` + `PackageName`。
2. 用户规则无包 → 归入 `custom`。
3. 旧 Policy 配置若仍带 GroupNames：后端兼容一期可读并 **警告日志**，映射失败则要求前端改用新 Policy API（前端同发 PR）。

---

## 6. 非目标（本 PR 不做）

- 前端页面实现（另 PR）。
- 扫描时按 Package 强制隔离（本设计明确多包可一起命中）。
- 保留 Group 作为长期收藏夹（若需要，后续用自定义 Tag 或新「收藏」模型）。
- 修改 online 服务端仓库（本仓库只定客户端契约与校验；服务端对齐另跟）。

---

## 7. 测试计划（后端）

- Package CRUD + 互斥（同 RuleId/Name 跨包拒绝）。
- Embed sync：`builtin` / `agent` 独立 hash/semver；删除远端规则则本地删。
- 导入：新格式 / 旧 ZIP → `imported-*`；冲突交互与 CLI discard 默认。
- Policy：`owasp-web` / `critical-high` 只靠 Tag + 字段，不再依赖 Group。
- Filter：按 Tag / PackageName / Language 查询。
- 回归：既有 rule 导入、扫描 smoke（`scripts/ssa-test.sh` 相关包）。

---

## 8. 前端实现计划（详细，不写代码）

前端另开一个 PR，依赖后端新契约稳定。下面按页面与组件拆分。

### 8.1 受影响页面（现有能力映射）

| 页面 / 场景 | 现状 | 目标 |
|-------------|------|------|
| **代码规则管理** | 按 Group 树/列表筛选；Tag 展示弱；导入导出整库 ZIP | 按 **Package** 管理分发；列表用 **Filter**（语言/等级/用途/Tag/搜索）；导入导出 **按包** |
| **代码扫描** | 选 Policy（背后 GroupNames）或自定义勾选组 | 选 **Policy**（展示描述）；高级模式用 Filter（Tag + 字段）；**不**要求用户先懂 Group |
| **规则详情 / 编辑** | 显示 Groups；编辑 Group 关联 | 显示/编辑 **Tags**；显示所属 **Package**（内置只读） |
| **在线规则市场**（若有） | 按规则 filter 下载 | 增加 **按包浏览/下载/更新**；保留按规则高级下载（可选） |

### 8.2 新契约 → UI 变化

1. **Package 侧栏或顶栏 Tabs**  
   - 数据：`QuerySyntaxFlowPackages`  
   - 展示：name、version、source（内置/用户/导入/online）、规则数、更新状态  
   - 操作：同步内置、从文件导入、导出、删除（非内置）、重命名用户包、新建空包  

2. **规则列表 Filter 条**  
   - 控件：Language、Severity、Purpose、Tags（多选，标准 Tag 分组 + 自定义）、Keyword  
   - `PackageNames`：侧栏选中包时自动带入；支持「全部已安装包」  
   - 去掉 Group 树作为主导航（硬切）

3. **扫描页 Policy 选择器**  
   - 数据：策略列表 API（含 name/description/icon/categories）  
   - 选中后预览「将命中的规则数」（可选：调用 Query 用展开后的 Filter）  
   - 自定义策略：Tag 多选 + Severity/Purpose，**不是**勾选 Group  
   - 文案说明：规则来自所有已安装包中匹配 Tag/字段的规则  

4. **导入 / 更新冲突 Modal**  
   - 流式进度中收集 `Conflict` 条目  
   - 表格：规则名、本地/远端版本、原因  
   - 批量：全部覆盖 / 全部丢弃 / 逐条选择  
   - 确认后带 `resolutions` 调用 Apply/Continue（以后端最终 RPC 为准）

5. **规则编辑**  
   - Tags：芯片输入；标准 Tag 可搜索提示（CWE / 框架 / 技术点）  
   - 禁止把语言/等级写成 Tag（UI 校验 + 后端校验）  
   - Package：下拉（默认 `custom`）；内置规则禁用更换包或整卡只读  

### 8.3 建议组件拆分

```text
SyntaxFlowRuleManagePage
  ├── PackageSidebar | PackageTabs
  │     ├── PackageListItem
  │     ├── PackageImportDialog
  │     ├── PackageExportDialog
  │     └── PackageSyncButton
  ├── RuleFilterBar
  │     ├── FieldSelect (language/severity/purpose)
  │     ├── TagMultiSelect (standard + custom)
  │     └── KeywordSearch
  ├── RuleTable | RuleCardList
  └── RuleEditorDrawer
        ├── TagEditor
        └── PackageSelect

SyntaxFlowScanPage
  ├── PolicyPicker (categories + cards)
  ├── PolicyCustomBuilder (Tag + fields → Filter)
  └── RulePreviewCount (optional)

shared/
  ├── ConflictResolutionModal
  └── PackageBadge / SourceBadge
```

### 8.4 状态与兼容

- 删除所有「规则组管理」主入口；若旧路由保留，重定向到 Tag Filter 或显示「已废弃」。
- 本地缓存的 GroupNames 扫描配置：升级时清空或迁移向导（映射表仅覆盖已知标准组→Tag；失败则让用户重选 Policy）。
- i18n：Package / Tag / Policy 术语统一（中文：规则包 / 标签 / 扫描策略）。

### 8.5 前端验收清单

- [ ] 仅安装 `builtin` 时可按 Policy 扫描；再导入/同步 `agent` 后，同 Policy 命中规则数增加（共享标准 Tag 时）。
- [ ] 导出 `custom` 再导入为新包名，列表出现独立包且无 RuleId 冲突。
- [ ] 制造同名不同 id 冲突包，Modal 可覆盖/丢弃且结果符合预期。
- [ ] 扫描页不再出现 Group 勾选作为主路径。
- [ ] 规则编辑无法把 severity 存成 Tag。

### 8.6 前后端协作顺序

1. 后端 PR 合入（或至少 proto + 行为说明冻结）。  
2. 前端 PR：先 Package 管理 + Filter 列表，再扫页 Policy，最后冲突 Modal 与 online。  
3. 联调：用固定 fixture zip（含/不含 package.yaml）与内置 sync。

---

## 9. 实现顺序（后端本 PR）

可编译垂直切片，同一 PR 内顺序提交：

1. **Schema**：`SyntaxFlowPackage` + Rule.`PackageName`/`Tags`  
2. **sfdb/sync**：`package.yaml` 模型、按包 sync、导入导出、冲突结构  
3. **Policy/Tag**：enricher 改 Tag；`scan_policies.yaml` 改绑；MapToFilter  
4. **gRPC/CLI**：包 RPC + Filter 字段 + Group Deprecated  
5. **测试与 CI**：包零交集校验、清单生成脚本  

---

## 10. 决策记录（grilling）

| # | 决策 |
|---|------|
| 规则包本质 | 分发单元（非扫描隔离） |
| 唯一键 | RuleId + RuleName 全局唯一 |
| 冲突 | 单条拒绝 + UI/CLI 选覆盖/丢弃 |
| 包版本 | semver 作更新闸门 |
| 包内删规则 | 同步删除本地 |
| Tag | 原子短标；优先 CWE，否则技术点；框架独立；等级不进 Tag |
| Policy | 预设 Filter；可跨包；可用字段条件 |
| Group | 废弃，硬切 |
| 内置布局 | `builtin/` 与 `agent/` 同级；单 `package.yaml` 含 rules 清单，CI 维护 |
| 外部格式 | 与内置同；旧 ZIP → 每次独立导入包 |
| 新建规则 | 默认 `custom`，可改名/建包 |
| 一期范围 | 一次做完主模型（后端一大 PR） |
