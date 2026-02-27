# 全局 HotPatch（Global HotPatch）设计与落地计划

状态：Draft  
范围：后端（yaklang core / yakgrpc），前端仅作为背景说明  

## 1. 背景

当前 HotPatchTemplate（表：`hot_patch_template`）支持对不同模块（`Type`: `fuzzer`/`mitm`/`httpflow-analyze`）存储/管理热加载代码模板；并且在运行时：

- **MITM**：通过 `MixPluginCaller.LoadHotPatch(...)` 将 HotPatch 代码加载为一个可执行 hook 插件（脚本名固定为 `@HotPatchCode`），在请求/响应生命周期中执行（例如 `beforeRequest`、`afterRequest` 等）。
- **WebFuzzer**：通过 `MutateHookCaller(...)` 将 HotPatch 代码解析为一组 hook（`beforeRequest/afterRequest/mirrorHTTPFlow/retryHandler/...`），并通过 `Fuzz_WithAllHotPatch(...)` 将 `{{yak(...)}}` / `{{yak:dyn(...)}}` 等 tag 注入到 fuzztag 渲染链路中。

目前 HotPatch 主要是“**模块级**”能力：每个模块（MITM/WebFuzzer）各自持有一份 `HotPatchCode` 或一份被加载的 HotPatch 插件。

## 2. 需求（Feature 定义）

新增“**全局 HotPatch**”能力（Global Layer）：

1. 前端已有“全局热加载插件管理”界面（基于 `HotPatchTemplate` 的增删改查，后端可能需要扩展），可以 **启动/停止** 某个模板作为全局生效的 HotPatch。
2. **全局只允许启用一个**（v1 仅支持单个全局插件）。
3. 生效范围：**MITM + WebFuzzer** 两条链路均会执行该全局 HotPatch。
4. 当模块自身也启用了 HotPatch 时，执行顺序必须为：

   **全局 HotPatch → 模块 HotPatch**

5. 生效时机：只要求对“之后的新 HTTP 请求/新 Fuzzer 任务”生效；历史请求不回放。

## 3. 非目标（明确不做）

- 不做历史流量重放、历史任务重算。
- v1 不做多全局插件并发启用、不做全局插件的排序管理（只允许一个）。
- 不引入新的“静默降级/兜底”行为：全局 HotPatch 加载失败应显式失败并保留原有状态（Debug-First）。

## 4. 术语与约定

- **Global HotPatch**：全局层 HotPatch（最多一个）。
- **Module HotPatch**：模块层 HotPatch（MITM / WebFuzzer 各自已有逻辑）。
- **Hook 顺序**：对同一 hook 点（如 `beforeRequest`），先执行 Global，再执行 Module。
- **串联语义（pipeline）**：对于会“修改请求/响应”的 hook（如 `beforeRequest/afterRequest`），前一个 hook 的输出应作为后一个 hook 的输入，以保证“全局→模块”的数据可见性。

## 5. 现状梳理（关键实现点）

### 5.1 MITM：HotPatch 加载与执行

- 加载入口：`common/yakgrpc/grpc_mitm.go`（收到 `YakScriptContent` 时调用 `LoadHotPatch`）。
- 加载实现：`common/yak/hook_mixed_plugin_caller.go` 内部固定脚本名 `@HotPatchCode`，先 Remove 再 Add。
- 执行点：MITM 请求/响应处理链路中调用 `mitmPluginCaller.CallBeforeRequestWithCtx(...)` / `CallAfterRequestWithCtx(...)` 等。

### 5.2 WebFuzzer：HotPatch 的两条通路

1) **Hook code**：`yak.MutateHookCaller(ctx, req.HotPatchCode, ...)` 解析出 `beforeRequest/afterRequest/mirrorHTTPFlow/...` 等函数并注入执行。  
2) **FuzzTag**：`yak.Fuzz_WithAllHotPatch(ctx, req.HotPatchCode)` 注入 `yak` / `yak:dyn` 两个 tag（注意：同名 tag 在 tagMethodMap 中是覆盖语义，无法简单叠加两份实现）。

## 6. 总体方案（v1）

### 6.1 设计原则（优雅/可维护/不破坏旧功能）

1. **分层而不是拼接代码**：不通过拼接字符串的方式把 Global/Module 代码合成一份脚本，避免同名函数覆盖导致顺序失真、调试困难。
2. **尽量不改变既有模块行为**：当 Global 未启用时，MITM/WebFuzzer 的行为应与现在完全一致。
3. **最小侵入**：全局能力以“可选的一层 wrapper/manager”形式接入，避免修改底层通用引擎语义（例如不要为了全局而改变所有插件的调用语义）。
4. **显式失败**：加载/编译失败直接返回错误（或产生明确日志/通知），不做 silent fallback。

### 6.2 数据与状态（不改表结构，使用 Profile KV 记录“当前启用配置”）

HotPatchTemplate 表结构保持不变（`common/schema/hotpatch_template.go`）。

新增一份全局启用配置存储在 `GeneralStorage`（Profile DB KV）中：

- Key（建议）：`GLOBAL_HOTPATCH_CONFIG`（常量建议加在 `common/consts/global.go`，与 `GLOBAL_NETWORK_CONFIG` 同类）
- Value（建议 JSON；配置化接口，天然可扩展为多个全局插件并支持顺序）：

```json
{
  "enabled": true,
  "version": 1,
  "items": [
    {
      "name": "加解密模板",
      "type": "global",
      "enabled": true
    }
  ]
}
```

说明：
- `type` 建议引入新的逻辑类型值 `global`（字符串，不涉及 DB 迁移），用于区分“全局模板”与 `mitm/fuzzer/httpflow-analyze` 模板。
- v1 仍然可以在服务端校验 `items` 最多 1 个；未来如果需要支持多个全局插件并支持前端拖拽排序，只需放开校验即可，KV 与接口无需变化。

### 6.3 接口（yakgrpc）建议（可扩展）

保留现有 HotPatchTemplate CRUD 接口不动（兼容前端既有页面）。

新增 Global HotPatch 管理接口（建议采用“配置资源”模型，以便未来扩展为多个全局插件 + 顺序编排）：

- `GetGlobalHotPatchConfig()`：获取全局 HotPatch 配置（ordered list + version）
- `SetGlobalHotPatchConfig(config, expected_version)`：整体替换配置（用于启用/禁用/排序/批量更新）
- `ResetGlobalHotPatchConfig()`：恢复默认（等价于禁用或清空）

接口语义建议：
- `items` 的 **列表顺序** 即执行顺序（从前到后执行），天然支持未来的“拖拽排序”。
- v1 可以在服务端限制 `items` 最大长度为 1，但接口层面保持 `repeated`，后续放开即可。
- `expected_version` 可选，用于乐观锁（避免多端同时编辑互相覆盖）；不传则 last-write-wins。

可选（v1 可以不做，v1.1 再加）：
- `GetGlobalHotPatchResolvedCode()`：调试用途，返回当前生效的代码（注意权限/敏感信息问题）

### 6.4 执行链路（核心：Global → Module）

#### 6.4.1 MITM：通过“两个 MixPluginCaller 实例”实现分层（低侵入）

在 MITM server 生命周期内维护两套 caller：

- `globalCaller`：仅加载全局 HotPatchTemplate 的代码（如果启用）
- `moduleCaller`：现有 `mitmPluginCaller`（含模块级 HotPatch 与其它 MITM 插件）

在每个 hook 点按顺序调用：

**beforeRequest**（修改 request 的 hook）：

1) `req1 = globalCaller.beforeRequest(req0)`（若返回空，视为不修改）  
2) `req2 = moduleCaller.beforeRequest(req1)`  
3) 最终使用 `req2` 继续流程

**afterRequest**（修改 response 的 hook）同理：

1) `rsp1 = globalCaller.afterRequest(rsp0)`  
2) `rsp2 = moduleCaller.afterRequest(rsp1)`  
3) 最终使用 `rsp2`

其它 hook（如 `mirrorHTTPFlow`、`hijack*`、`mockHTTPRequest`）同样采用“先 global 后 module”的顺序；对于返回值型 hook，采用“串联语义（pipeline）”或“明确合并规则”（根据 hook 类型定义）。

为什么这样做：
- 不修改 `YakToCallerManager` 的通用调用策略，避免改变既有插件系统的行为。
- 全局只一个，独立实例最清晰，也便于 future 扩展（例如未来全局插件支持多个时，再引入排序）。

#### 6.4.2 WebFuzzer：组合两套 HotPatch（Hook + Tag）

**Hook code（MutateHookCaller）**：
- 分别对 `globalCode` 与 `moduleCode` 调用 `MutateHookCaller` 获取两套 hooks。
- 返回一套“合成 hooks”，内部按顺序执行 global→module，并将输出串联传递。

**FuzzTag（yak / yak:dyn）**：
- 由于 tagMethodMap 按 tag 名覆盖写入，无法注册两份同名 tag。
- v1 采用“复合 tag handler”策略：注入一个 `yak` tag 与一个 `yak:dyn` tag，但其 handler 内部按规则调用：
  - 默认：`module` 优先（如果 module code 内存在对应 handle，则使用 module；否则 fallback 到 global）
  - 或者：明确支持 `global.handleX` / `module.handleX` 这种命名约定（可选，v1.1）

这里必须给出清晰规则，避免用户在同名 handle 时产生“到底跑了哪个”的困惑。

### 6.5 变更检测与生效时机

由于只要求对“新请求/新任务”生效，推荐采用“内存缓存 + 版本号”：

- Global HotPatch 启用/停用时：
  - 写 KV
  - 刷新内存缓存（global code / global enabled / version++）
- MITM/WebFuzzer 在处理新请求/新任务时：
  - 读取内存缓存（不访问 DB）
  - 如果发现 version 变化，可重建/重载 global caller（MITM）或重新组合 hooks（WebFuzzer）

模板内容更新的策略（建议两阶段）：
- v1：更新模板后需重新点击“启用”使其刷新（最简单，低风险）
- v1.1：在 `UpdateHotPatchTemplate` 里检测“是否为当前启用项”，如果是则自动 bump version 并刷新缓存（用户体验更好）

## 7. 兼容性与回归风险控制

必须保证以下行为不变：

1. **Global 未启用时**：MITM/WebFuzzer 的 HotPatch 行为与当前版本一致。
2. **模块 HotPatch 独立使用**：不受全局功能引入影响（接口字段、执行时机、报错行为一致）。
3. **失败语义**：
   - 启用全局模板时如果编译/加载失败：应返回错误，并保持“未启用/旧启用状态”不变。
   - 运行时 hook 执行出现 panic/error：沿用现有模块的错误可见性策略（日志/通知），不吞掉错误。

## 8. 落地步骤（建议拆分）

### Phase 0：文档与接口冻结
- 明确全局模板的 `Type` 约定（建议 `global`）。
- 冻结接口：Global HotPatch 的 enable/disable/get（以及返回字段）。

### Phase 1：后端最小可用
- 新增 KV key + yakgrpc 接口（Enable/Disable/Get）。
- 实现 Global HotPatch Manager（内存缓存 + version）。
- MITM：引入 `globalCaller` 并在关键 hook 点按 global→module 顺序执行。
- WebFuzzer：实现 hook 组合（global→module）+ fuzz tag 复合 handler。

### Phase 2：完善与可观测性
- 为全局层补充日志/通知（启用成功/失败、当前启用模板名）。
- 补充单测：顺序、串联语义、global off 兼容性。

### Phase 3：体验优化（可选）
- 模板更新自动生效（Update 时刷新 active global）。
- 更强的 tag handle 冲突解决策略（例如显式命名空间）。

## 9. 测试计划（最低要求）

- 单元测试：
  - Global off：WebFuzzer/MITM 结果与原逻辑一致（关键路径回归）。
  - Global on + Module on：验证执行顺序与串联语义（global 输出被 module 输入消费）。
  - Global on + Module off：只跑 global。
  - Global enable 失败：状态不改变，错误可见。
- 关键集成测试（可选）：基于现有 `grpc_*_hotpatch_test.go` 增补用例。

## 10. 风险与对策

- **风险：同名 handle/tag 的覆盖与可解释性差**  
  对策：对 fuzztag 定义明确优先级（module 优先），并在文档/前端提示。

- **风险：改动底层插件系统会引入不可预期回归**  
  对策：MITM 分层用“双 caller 实例”方式，避免修改 `YakToCallerManager` 的通用语义。

- **风险：每请求访问 DB 性能差**  
  对策：全局配置使用内存缓存 + version，避免 per-request DB I/O。
