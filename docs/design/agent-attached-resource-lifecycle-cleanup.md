# Agent 附件统一处理设计

## 1. 目标

把目前散落在输入处理、Task 创建、会话持久化和各个 focus loop 中的附件逻辑，收敛为一条固定管线：

```text
AIInputEvent
  -> NormalizeAttachments
  -> Task / Session 状态
  -> PrepareAttachments
  -> Prompt / ToolCaller / focus loop
```

目标是：

- 附件只归一化一次。
- 附件只解析一次。
- 通用 Prompt 只渲染一次。
- 会话持久化使用归一化后的数据。
- focus loop 消费已解析资源，不重复遍历原始附件。
- `AttachedFilePath` 等旧协议只做兼容适配，不保留独立处理分支。

## 2. 当前散点

当前附件逻辑主要分散在以下位置：

| 位置 | 当前职责 |
|---|---|
| `handleFreeValue` | 统一 `AttachedFilePath` 和 `AttachedResourceInfo` 的输入入口 |
| `buildReTaskFromEvent` | 筛选控制资源、写 Task、单独缓存和继承 browser |
| `buildSessionTaskInput` | 再次转换原始事件并写会话输入表 |
| `RunAttachedExtraResourcesInit` | 解析、BindLoopData、渲染并写 Timeline |
| 各 focus loop `buildInitTask` | 重复解析、筛选和转移附件 |
| ToolCaller | 不同工具再次从 Task 或原始事件读取附件 |
| 资源查询接口 | 从输入历史重新计算资源并集 |

由此产生的问题：

- `AttachedFilePath` 和其他文件附件的生命周期不同。
- 同一个文件可能同时进入 ContextProvider 和 Timeline。
- 每个 focus loop 自行决定是否解析附件。
- 浏览器作为 session 状态被重新塞回每轮 Task。
- 持久化数据和运行时实际使用的数据不是同一份。

## 3. 统一数据结构

在 input ingress 直接把附件和控制信息拆成两个返回值：

```go
type scheduledTaskInputControl struct {
    uuid        string
    name        string
    scheduledAt string
    trigger     string
}

type taskInputControl struct {
    userInputUUID string
    inputSource   string
    schedule      *scheduledTaskInputControl
}
```

要求：

- resources 中只允许 canonical type/key/value。
- 控制字段独立返回，不再伪装成普通附件。
- browser 在 ingress 解析后直接更新 session 状态，不进入 resources。
- 归一化失败时不修改已有 session 状态。

## 4. 统一入口

入口函数返回资源和控制两个值：

```go
func normalizeInputAttachments(
    event *ypb.AIInputEvent,
) ([]*aicommon.AttachedResource, *taskInputControl)
```

调用点应位于 `enqueueReTaskWithID` 之前或内部，保证后续代码不再直接分析原始 `AIInputEvent.AttachedResourceInfo`。

处理顺序：

1. 解析 `user_free_input_uuid`、输入来源、调度信息。
2. 将 `AttachedFilePath` 转换为 canonical file resource。
3. 解析和更新 browser selection mutation。
4. 校验并规范化 `AttachedResourceInfo`。
5. 返回 resources 和 control。

之后：

- `buildReTaskFromEvent` 只接收 resources 和 control。
- `task.SetAttachedDatas()` 使用 resources。
- `buildSessionTaskInput` 只接收 resources；控制字段从已写入的 Task 读取。
- 诊断和日志使用同一份归一化结果。

## 5. Legacy 适配

### 5.1 AttachedFilePath

`AttachedFilePath` 只保留 wire compatibility：

```text
AttachedFilePath[]
  -> AttachedResource{
       Type: "file",
       Key: "file_path",
       Value: path,
     }
```

要求：

- 转换逻辑只在 ingress 中实现一次。
- 删除 `handleFreeValue` 中为 `AttachedFilePath` 单独注册 `FileContextProvider` 的分支。
- 删除 `buildSessionTaskInput` 中独立的 `AttachedFilePath` 转换。
- `AttachedFilePath` 归一化后必须与 `AttachedResourceInfo` 的 `file/file_path` 完全同权。
- protobuf 字段暂时保留，直到客户端完成迁移。

### 5.2 其他旧格式

同类兼容统一放在 ingress：

- `http_flow` -> canonical `http_flow_id`。
- 旧 type alias -> canonical type。
- `file/file_content` -> 明确的 internal task resource。

业务代码不再感知旧格式。

## 6. Task 附件准备

公共 Loop 基础设施在 Task 执行开始时统一调用一次：

```go
prepared := PrepareAttachments(r, loop, task)
```

职责：

- `ParseAttachedResourceData` 每个资源一次。
- 按 canonical type 分组。
- 调用一次通用 Prompt renderer。
- 保存解析结果供 focus loop 使用。

结果建议放在 Loop 的 typed state 中：

```go
type PreparedAttachments struct {
    Resources []*AttachedResourceData
    ByType    map[string][]*AttachedResourceData
}
```

约束：

- focus loop 不再直接调用 `RunAttachedExtraResourcesInit`。
- focus loop 不再直接遍历 `task.GetAttachedDatas()` 做解析。
- focus loop 按 type 从 `PreparedAttachments` 获取资源。
- 某个资源由 focus loop 专用消费时，通用 renderer 必须显式跳过，避免重复注入。

## 7. ContextProvider

附件进入模型上下文只能有一条主路径。

当前 `file/file_path` 和 `file/file_content` 已不再通过 ContextProvider
注入，而是与其他结构化附件一样进入 Task，并由 `attached_file` Timeline
交给模型。后续不应在输入入口重新引入 file ContextProvider 旁路。

如确实需要 ContextProvider 的 diff/auto-context 能力，应由统一附件处理层
创建 Task-scoped provider，并保证同一附件不会被 Timeline 重复注入。

## 8. 浏览器

浏览器不是 Task attachment，而是 session mutation。

```text
@A
  -> ingress 解析 BrowserBinding
  -> 更新 ReAct session BrowserBindingState
  -> 不进入 Task.AttachedDatas
```

每次 Loop 启动读取当前 BrowserBinding snapshot：

- Prompt 生成当前浏览器绑定说明。
- ToolCaller 使用 snapshot 做浏览器 allowlist 和 Device ID 映射。
- 不重新反序列化 browser resource。
- 不每轮追加 `attached_browser` Timeline。

显式 clear 需要新增控制资源或独立 input 字段。不能用空 `browser/device_id` 表示清除。

## 9. 资源消费归属

| 资源 | 统一准备后由谁消费 |
|---|---|
| `file/file_path`、`file/directory_path` | 通用 renderer；图片额外走 vision handler |
| `code/file_path` | 通用层不渲染；Yaklang loop 消费 |
| `selected/content` | 通用 renderer；Yaklang/SyntaxFlow 可消费 typed data |
| `selected/syntaxflow_rule` | 通用层跳过；SyntaxFlow loop 独占 |
| `knowledge_base/*` | 通用偏好渲染；默认 loop 触发 knowledge enhance |
| `http_flow_id` | 通用层不渲染；HTTP flow analyze loop 消费 |
| `http_fuzz_request` | 通用层可渲染；HTTP fuzz loop 消费 |
| `risk/ids` | 通用摘要渲染；Risk Tool runtime 消费 |
| `aitool/name`、`aiforge/name` | 暂保持 default 渲染，后续确认是否保留 |
| `browser/device_id` | ingress 转 session state，不进入 TaskResources |

## 10. 实施顺序

### 第一步：统一 ingress

已完成：

- `AttachedFilePath` 在 ingress 转换为 `file/file_path`。
- `buildReTaskFromEvent` 和 `buildSessionTaskInput` 共用归一化结果。
- `handleFreeValue` 的 file ContextProvider 副作用已删除。

### 第二步：统一 Task 准备

- 新增 `PrepareAttachments`。
- 公共 Loop 执行入口调用一次。
- 将解析结果放入 Loop typed state。
- 各 focus loop 改为读取 prepared resources。

### 第三步：删除旁路

- 删除 `handleFreeValue` 中普通文件 ContextProvider 注册。
- 删除 focus loop 对 `RunAttachedExtraResourcesInit` 的重复调用。
- 删除 `buildSessionTaskInput` 的独立附件转换。
- 删除 Browser 从 Task attachment 的继承逻辑。

## 11. 验收测试

1. `AttachedFilePath=/a.txt` 与 `Type=file,Key=file_path,Value=/a.txt` 产生完全相同的 TaskResources。
2. `AttachedFilePath` 和 `file/file_path` 不创建独立 ContextProvider。
3. Task、会话持久化和 Prompt 使用同一份归一化附件。
4. 同一 Task 中每个附件只解析一次。
5. 同一文件不会同时被 ContextProvider 和 Timeline 完整注入。
6. focus loop 不再直接解析 `task.GetAttachedDatas()`。
7. `@A` 只更新 session browser state，不进入 TaskResources。
8. 第二轮没有 `@` 时不重新解析 browser，也不重复写 Timeline。
9. 显式切换和清除浏览器状态行为确定。
10. `http_flow` 正确归一化为 `http_flow_id`，空列表不发送资源。
