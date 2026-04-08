# AIHTTP 前端 API 指南

> 适用对象：接入 `common/yakgrpc/aihttp` 网关的前端开发者（Web / CLI / TUI）。

## 1. 概览

`aihttp` 是构建在 yakgrpc 流式 API 之上的 HTTP / SSE 网关。

- 基础路由前缀（默认）：`/agent`
- 主要分组：
  - `/run/*`：运行时执行与事件流
  - `/session/*`：会话元数据与管理
  - `/setting/*`：聊天设置、模型 / Provider / Focus 选项
  - `/forge/*`：AI Forge 管理与导入导出

说明：本文仅覆盖当前对外提供的 `/run/*`、`/session/*`、`/setting/*`、`/forge/*` 四组 HTTP 接口；底层运行逻辑会桥接到 gRPC `StartAIReAct` 等能力。

该网关支持：

- 创建 / 复用运行会话
- 通过 SSE 流式接收 AI 输出
- 在运行过程中推送运行时输入 / 热补丁 / 交互事件
- 更新设置并拉取 provider / model / focus 元数据

---

## 2. 认证与通用行为

## 2.1 可选认证头

如果网关开启了认证，前端应携带：

- `Authorization: Bearer <jwt>`
- `X-TOTP-Code: <totp_code>`

如果缺失或无效，API 会返回 `401`。

## 2.2 CORS

网关始终会返回：

- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS`
- `Access-Control-Allow-Headers: Content-Type, Authorization, X-TOTP-Code`

## 2.3 错误响应格式

所有 HTTP 错误都使用 JSON：

```json
{
  "error": "Bad Request",
  "code": 400,
  "message": "invalid request body: ..."
}
```

---

## 3. 推荐的前端接入流程

对于**新会话**：

1. 调用 `POST /session` 获取 `run_id`
2. 先连接 `GET /run/{run_id}/events` SSE
3. 等待 SSE 事件：`listener_ready`
4. 调用 `POST /run/{run_id}` 提交首条输入（网关会在内部启动并桥接到底层 gRPC 流）
5. 输出过程中，如需继续输入，统一使用 `POST /run/{run_id}/events/push`

对于**已有会话**：

1. 确保后端已存在该会话；如果不确定，可使用已知 `run_id` 再次调用 `POST /session` 进行恢复
2. 连接 SSE
3. 继续对话时，按场景调用 `/run/{run_id}` 或 `/run/{run_id}/events/push`

取消当前运行：

- `POST /run/{run_id}/cancel`

---

## 4. 数据模型（面向前端）

## 4.1 `CreateSessionRequest`

用于 `POST /session`。

说明：创建会话阶段只需要 `run_id`。运行时配置请通过 `POST /run/{run_id}` 的 `ypb.AIInputEvent.Params` 传入。

```json
{
  "run_id": "optional-custom-id"
}
```

## 4.2 `ypb.AIInputEvent`

以下接口的标准请求体：

- `POST /run/{run_id}`
- `POST /run/{run_id}/events/push`

```json
{
  "IsStart": false,
  "Params": {
    "AIService": "openai",
    "AIModelName": "gpt-4.1",
    "UseDefaultAIConfig": true
  },
  "IsConfigHotpatch": false,
  "HotpatchType": "AIService",
  "IsInteractiveMessage": false,
  "InteractiveId": "event_uuid",
  "InteractiveJSONInput": "{\"approved\":true}",
  "IsSyncMessage": false,
  "SyncType": "optional",
  "SyncJsonInput": "{}",
  "SyncID": "optional",
  "IsFreeInput": true,
  "FreeInput": "user text",
  "AttachedFilePath": [],
  "AttachedResourceInfo": [
    {
      "Key": "optional",
      "Type": "optional",
      "Value": "optional"
    }
  ],
  "FocusModeLoop": "optional"
}
```

以下任一项命中，即视为请求中包含“有效输入负载”：

- `IsConfigHotpatch`
- 交互输入
- 同步输入
- 自由输入
- `FocusModeLoop`
- `AttachedFilePath`

- `POST /run/{run_id}`：既可以发送仅启动的一帧（`IsStart=true` 且无其他有效输入负载），也可以直接发送普通输入事件。
- `POST /run/{run_id}/events/push`：必须携带上面的任一有效输入负载；仅传 `IsStart=true` 会被判定为无效请求。
- `AttachedResourceInfo` 字段当前可随请求一并传入，但**单独传它本身不会被网关视为有效输入负载**。

`/run/{run_id}` 与 `/run/{run_id}/events/push` 现仅接受 `ypb.AIInputEvent` 请求体，不再兼容旧版 `PushEventRequest`。

## 4.3 `ypb.AIOutputEvent`（运行接口与 SSE 统一使用的响应负载）

```json
{
	"CoordinatorId": "optional",
	"Type": "accepted|listener_ready|heartbeat|completed|cancelled|failed|stream|structured|thought|...",
	"NodeId": "optional",
	"IsSystem": false,
	"IsStream": true,
	"IsReason": false,
	"StreamDelta": "base64-encoded bytes",
	"IsJson": false,
	"IsResult": false,
	"Content": "base64-encoded bytes",
	"Timestamp": 1730000000,
	"TaskIndex": "optional",
	"EventUUID": "optional",
	"TaskUUID": "optional"
}
```

说明：`Content` / `StreamDelta` 是 `bytes` 字段，通过 HTTP JSON 返回时会按 protobuf JSON 规则编码为 base64 字符串。

这里只展示常用字段；完整字段定义以 proto 为准。

---

## 5. API 详情

下文所有路径都相对于 `<base>/<prefix>`，默认前缀为 `/agent`。

## 5.1 设置类 API

### `GET /setting`

- 用途：获取当前 AI Agent 聊天设置
- 请求体：无
- 响应：当前设置 JSON（当前后端响应中的键名为 **PascalCase**）

响应示例：

```json
{
  "UseDefaultAIConfig": true,
  "AIService": "aibalance",
  "AIModelName": "glm-4-flash-free",
  "ReviewPolicy": "manual",
  "SelectedProviderID": 1,
  "SelectedModelName": "glm-4-flash-free",
  "SelectedModelTier": "intelligent"
}
```

### `POST /setting`

- 用途：增量更新并保存设置
- 行为：
  - 将请求补丁合并到现有设置中
  - 同时接受 snake_case 和 PascalCase 键名
  - 未提供的字段保留当前值

补丁示例：

```json
{
  "ai_service": "openai",
  "selected_provider_id": 3,
  "selected_model_name": "gpt-4o-mini",
  "review_policy": "manual"
}
```

响应：完整保存后的设置（PascalCase 键名）。

---

### `GET /setting/global`

- 用途：获取全局网络配置（透传 gRPC `GetGlobalNetworkConfig`）
- 请求体：无
- 响应：`GetGlobalNetworkConfigResponse`

### `POST /setting/global`

- 用途：更新全局网络配置（透传 gRPC `SetGlobalNetworkConfig`）
- 请求体：`GlobalNetworkConfig`
- 响应：保存后的同一份配置对象

`GlobalNetworkConfig` 包含的字段包括（不限于）：

- `DisableSystemDNS`
- `CustomDNSServers`
- `GlobalProxy`
- `EnableSystemProxyFromEnv`
- `AppConfigs`
- `PrimaryAIType`
- `AiApiPriority`
- `EnableTieredAIModelConfig`
- `TieredAIModelConfig`

---

### `GET /setting/aiconfig`

- 用途：获取 AI 全局配置（透传 gRPC `GetAIGlobalConfig`）
- 请求体：无
- 响应：`AIGlobalConfig`

### `POST /setting/aiconfig`

- 用途：更新 AI 全局配置（透传 gRPC `SetAIGlobalConfig`）
- 请求体：`AIGlobalConfig`
- 响应：保存后的同一份配置对象

---

### `POST /setting/appconfigs/template/get`

- 用途：获取第三方应用配置表单模板（透传 gRPC `GetThirdPartyAppConfigTemplate`）
- 请求体：可为空；也可以传 `{}`
- 响应：`GetThirdPartyAppConfigTemplateResponse`

模板项字段：

- `Name`：配置键名
- `Verbose`：展示标签
- `Type`：输入类型（`string` / `number` / `bool` / `list`）
- `Required`：该键是否必填
- `DefaultValue`：字段默认值
- `Desc`：描述文本
- `Extra`：附加元数据

---

### `POST /setting/providers/get`

- 用途：获取 AI Provider 列表
- 请求体：可为空；也可以传 `{}`
- 响应：透传 gRPC `ListAIProvidersResponse`

响应示例：

```json
{
  "Providers": [
    {
      "Id": 1,
      "Config": {
        "Type": "aibalance",
        "Domain": "aibalance.yaklang.com",
        "Disabled": false
      }
    }
  ]
}
```

### `POST /setting/providers/query`

- 用途：分页查询 AI Provider
- 请求体：透传 gRPC `QueryAIProvidersRequest`
- 响应：透传 gRPC `QueryAIProvidersResponse`

### `POST /setting/aimodels/get`

- 用途：根据 provider 配置获取模型列表
- 请求体支持多种格式：

1) 旧版字符串：

```json
{ "Config": "openai" }
```

2) 旧版 JSON 字符串：

```json
{ "Config": "{\"Type\":\"openai\",\"APIKey\":\"***\"}" }
```

3) 对象负载：

```json
{
  "config": {
    "Type": "openai",
    "APIKey": "***",
    "Domain": "api.openai.com",
    "Proxy": "",
    "NoHttps": false
  }
}
```

4) 也接受扁平对象（同样字段直接放在顶层）。

响应：

```json
{
  "ModelName": ["gpt-4o", "gpt-4o-mini"]
}
```

### `POST /setting/aifocus/get`

- 用途：获取可用的 AI Focus 模式
- 请求体：可为空
- 响应：透传 gRPC `QueryAIFocusResponse`

示例：

```json
{
  "Data": [
    {
      "Name": "default",
      "VerboseName": "Default",
      "VerboseNameZh": "默认模式",
      "Description": "..."
    }
  ]
}
```

---

## 5.2 Forge 类 API

### `POST /forge/create`

- 用途：创建 AI Forge
- 请求体：透传 gRPC `AIForge`
- 响应：透传 gRPC `DbOperateMessage`

### `POST /forge/update`

- 用途：更新 AI Forge
- 请求体：透传 gRPC `AIForge`
- 响应：透传 gRPC `DbOperateMessage`

### `POST /forge/delete`

- 用途：删除 AI Forge
- 请求体：透传 gRPC `AIForgeFilter`
- 响应：透传 gRPC `DbOperateMessage`

### `POST /forge/query`

- 用途：分页查询 AI Forge
- 请求体：透传 gRPC `QueryAIForgeRequest`
- 响应：透传 gRPC `QueryAIForgeResponse`

### `POST /forge/get`

- 用途：按名称或 ID 获取单个 AI Forge
- 请求体：透传 gRPC `GetAIForgeRequest`
- 响应：透传 gRPC `AIForge`

### `POST /forge/export`

- 用途：导出一个或多个 AI Forge
- 请求体：透传 gRPC `ExportAIForgeRequest`
- 响应：SSE 流，逐条透传 gRPC `GeneralProgress`

SSE 示例：

```text
data: {"Percent":0,"Message":"start export","MessageType":"info"}

data: {"Percent":100,"Message":"export completed","MessageType":"success"}
```

### `POST /forge/import`

- 用途：导入 AI Forge 压缩包
- 请求体：透传 gRPC `ImportAIForgeRequest`
- 响应：SSE 流，逐条透传 gRPC `GeneralProgress`

---

## 5.3 会话类 API

### `POST /session`

- 用途：创建一个运行会话（或恢复内存中已有的 `run_id`）
- 请求：

```json
{
  "run_id": "optional-custom-id"
}
```

- 响应（新建返回 `201`，已存在返回 `200`）：

```json
{
  "run_id": "uuid",
  "status": "pending"
}
```

### `GET /session/all`

- 用途：列出全部会话（内存中活跃会话 + 已持久化元数据）
- 响应：

```json
{
  "sessions": [
    {
      "run_id": "uuid",
      "title": "optional",
      "status": "pending|running|completed|cancelled|failed",
      "created_at": "2026-03-05T12:00:00+08:00",
      "is_alive": true
    }
  ]
}
```

### `POST /session/{run_id}/title`

- 用途：更新会话标题元数据
- 请求：

```json
{ "title": "new title" }
```

- 响应：

```json
{
  "run_id": "uuid",
  "title": "new title",
  "status": "updated"
}
```

### `POST /session/del`

- 用途：删除会话
- 行为：会先取消内存中的运行，再透传调用 gRPC `DeleteAISession`
- 请求：
  - 必须从 body 传删除参数
  - 支持直接透传 `DeleteAISessionRequest`
  - 也兼容直接传 `DeleteAISessionFilter`
- 示例：

```json
{
  "Filter": {
    "SessionID": ["uuid"],
    "AfterTimestamp": 1700000000,
    "BeforeTimestamp": 1800000000
  },
  "DeleteAll": false
}
```

- 响应：

```json
{
  "TableName": "ai_sessions_v1",
  "Operation": "delete",
  "EffectRows": 0,
  "ExtraMessage": "deleted_sessions=1 deleted_runtimes=0 deleted_events=0"
}
```

---

## 5.4 运行类 API

### `POST /run/{run_id}`

- 用途：为某次运行提交首个 / 普通输入事件
- 说明：
  - 需要该会话已存在于 run manager 中
  - 如果这是第一个有效输入，后端会自动启动底层 gRPC 流
- 请求：`ypb.AIInputEvent`
- 响应：`ypb.AIOutputEvent`（仅表示网关已接收该输入；实际输出仍通过 SSE 推送）

```json
{
	"Type": "accepted",
	"IsSystem": true,
	"IsResult": true,
	"Timestamp": 1730000000,
	"EventUUID": "uuid"
}
```

### `POST /run/{run_id}/events/push`

- 用途：在运行处于 pending / running 状态时继续推送事件
- 典型使用场景：
  - 用户在流式输出过程中输入新消息
  - 交互式审核响应
  - 运行时热补丁
- 请求：`ypb.AIInputEvent`
- 响应：`ypb.AIOutputEvent`（仅表示网关已接收该输入；实际输出仍通过 SSE 推送）

```json
{
	"Type": "accepted",
	"IsSystem": true,
	"IsResult": true,
	"Timestamp": 1730000000,
	"EventUUID": "uuid"
}
```

### `GET /run/{run_id}/events`（SSE）

- 用途：订阅流式输出事件
- 响应 content-type：`text/event-stream`
- 每条消息都以 `data: <AIOutputEvent JSON>` 的形式下发
- 服务端会发送：
  - 立即返回就绪事件：

```json
{
	"Type": "listener_ready",
	"IsSystem": true,
	"Timestamp": 1730000000,
	"EventUUID": "uuid"
}
```

  - 大约每 15 秒一次心跳：

```json
{
	"Type": "heartbeat",
	"IsSystem": true,
	"Timestamp": 1730000000,
	"EventUUID": "uuid"
}
```

  - AI 输出，序列化后的 `ypb.AIOutputEvent`
  - 终态状态：

```json
{
	"Type": "completed|cancelled|failed",
	"IsSystem": true,
	"IsResult": true,
	"Timestamp": 1730000000,
	"EventUUID": "uuid"
}
```

### `POST /run/{run_id}/cancel`

- 用途：取消一个正在运行 / 等待中的任务
- 请求体：可为空；若传入则使用 `ypb.AIInputEvent` 结构（当前仅做结构校验，不参与取消逻辑）
- 响应：`ypb.AIOutputEvent`

```json
{
	"Type": "cancelled",
	"IsSystem": true,
	"IsResult": true,
	"Timestamp": 1730000000,
	"EventUUID": "uuid"
}
```

---

## 6. 前端接入注意事项

1. **新运行一定要先连接 SSE**
   - 等待 `listener_ready` 后，再调用 `/run/{run_id}`。

2. **流式输出期间的用户输入**
   - 使用 `ypb.AIInputEvent{IsFreeInput: true, FreeInput: ...}` 调用 `/run/{run_id}/events/push`。

3. **Provider / Model 选择流程**
   - 先通过 `GET /setting` 获取当前设置
   - 再通过 `/setting/providers/get` 获取 providers
   - 确定 provider 后调用 `/setting/aimodels/get`
   - 最后通过 `POST /setting` 更新选中的 provider / model

4. **设置项命名规则**
   - 前端发送设置更新请求时，推荐使用 snake_case
   - 后端当前响应使用 PascalCase 键名

5. **废弃的旧 API**
   - 旧的 `/session/{run_id}/send` 与 `/session/{run_id}/close` 流程已被 `/run/*` 流程替代。

---

## 7. 最小前端时序示例

```ts
// 1) 创建会话
const createResp = await fetch("/agent/session", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({})
}).then(r => r.json());
const runID = createResp.run_id;

// 2) 连接 SSE
const es = new EventSource(`/agent/run/${runID}/events`);
es.onmessage = async (ev) => {
  const data = JSON.parse(ev.data);
  if (data.Type === "listener_ready") {
    // 3) 首次运行输入
    await fetch(`/agent/run/${runID}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ IsStart: true, IsFreeInput: true, FreeInput: "你好" })
    });
    // 这里拿到的是 accepted 确认，实际输出继续从 SSE 接收
  }
};

// 4) 在流式输出过程中继续发送输入
await fetch(`/agent/run/${runID}/events/push`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ IsFreeInput: true, FreeInput: "继续" })
});

// 5) 监听终态后按需关闭连接
// if (["completed", "cancelled", "failed"].includes(data.Type)) {
//   es.close();
// }
```
