# AIHTTP 前端 API 指南

> 适用对象：接入 `common/yakgrpc/aihttp` 网关的前端开发者（Web / CLI / TUI）。

## 1. 概览

`aihttp` 是构建在 yakgrpc 流式 API 之上的 HTTP / SSE 网关。

- 基础路由前缀（默认）：`/agent`
- 主要分组：
  - `/run/*`：运行时执行与事件流
  - `/ypb.Yak/*`：兼容 gRPC 的标准路由
  - `/session/*`：会话元数据与管理
  - `/setting/*`：聊天设置、模型 / Provider / Focus 选项

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
4. 调用 `POST /ypb.Yak/StartAIReAct/{run_id}`，并发送首个输入事件
5. 输出过程中，用户仍可通过 `POST /ypb.Yak/StartAIReAct/{run_id}` 或兼容旧版的 `POST /run/{run_id}/events/push` 继续发送输入

对于**已有会话**：

1. 确保该会话已存在于后端运行时映射中（或使用已知 `run_id` 调用 `POST /session` 重新恢复）
2. 连接 SSE
3. 调用 `/ypb.Yak/StartAIReAct/{run_id}` 或兼容旧版的 `/run/{run_id}/events/push` 继续对话

取消当前运行：

- `POST /ypb.Yak/StartAIReAct/{run_id}/cancel`

---

## 4. 数据模型（面向前端）

## 4.1 `AIParams`

用于创建会话以及运行输入参数。

```json
{
  "forge_name": "optional",
  "review_policy": "manual|auto|ai|ai-auto",
  "ai_service": "openai|aibalance|...",
  "ai_model_name": "model-name",
  "max_iteration": 100,
  "react_max_iteration": 100,
  "disable_tool_use": false,
  "use_default_ai": true,
  "attached_files": ["/path/a", "/path/b"],
  "enable_system_file_system_operator": true,
  "disallow_require_for_user_prompt": true,
  "ai_review_risk_control_score": 0.5,
  "ai_call_auto_retry": 3,
  "ai_transaction_retry": 5,
  "enable_ai_search_tool": true,
  "enable_ai_search_internet": false,
  "enable_qwen_no_think_mode": false,
  "allow_plan_user_interact": true,
  "plan_user_interact_max_count": 3,
  "timeline_item_limit": 100,
  "timeline_content_size_limit": 20,
  "user_interact_limit": 0,
  "timeline_session_id": "optional"
}
```

## 4.2 `ypb.AIInputEvent`

以下接口的标准请求体：

- `POST /ypb.Yak/StartAIReAct/{run_id}`
- `POST /ypb.Yak/StartAIReAct/{run_id}/events/push`
- 旧版 `POST /run/{run_id}`
- 旧版 `POST /run/{run_id}/events/push`

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
  "FocusModeLoop": "optional"
}
```

请求中至少需要包含以下任意一种负载：

- `IsStart=true`，用于仅启动的一帧
- `IsConfigHotpatch`
- 交互输入
- 同步输入
- 自由输入
- `FocusModeLoop`
- `AttachedFilePath`

为了兼容历史调用方式，仍然接受旧版 `PushEventRequest` 的 snake_case 请求体；但新的调用方应直接发送 `ypb.AIInputEvent`。

## 4.3 `ypb.AIEventQueryRequest`

用于 `POST /ypb.Yak/QueryAIEvent` 的标准请求体。

```json
{
  "Filter": {
    "SessionID": "run-id"
  },
  "Pagination": {
    "Page": 1,
    "Limit": 20,
    "OrderBy": "id",
    "Order": "desc"
  }
}
```

## 4.4 `RunEvent`（SSE 核心负载）

```json
{
  "id": "uuid",
  "type": "stream|structured|thought|done|error|...",
  "coordinator_id": "optional",
  "ai_model_name": "optional",
  "node_id": "optional",
  "is_system": false,
  "is_stream": true,
  "is_reason": false,
  "stream_delta": "optional",
  "content": "optional",
  "timestamp": 1730000000,
  "task_index": "optional",
  "event_uuid": "optional",
  "task_uuid": "optional"
}
```

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
  - 对缺失的核心字段应用默认值

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

### `POST /setting/appconfigs/template/get`

- 用途：获取第三方应用配置表单模板（透传 gRPC `GetThirdPartyAppConfigTemplate`）
- 请求体：`{}`（或空 JSON 对象）
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
- 请求体：`{}`（或空 JSON 对象）
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
- 请求体：`{}`（可选）
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

## 5.2 会话类 API

### `POST /session`

- 用途：创建一个运行会话（或恢复内存中已有的 `run_id`）
- 请求：

```json
{
  "run_id": "optional-custom-id",
  "params": { "...AIParams" }
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

---

## 5.3 运行类 API

### `POST /ypb.Yak/StartAIReAct/{run_id}`

- 用途：为某次运行提交首个 / 普通输入事件
- 说明：
  - 需要该会话已存在于 run manager 中
  - 如果这是第一个有效输入，后端会自动启动 gRPC 流
- 请求：`ypb.AIInputEvent`
- 响应：

```json
{
  "run_id": "uuid",
  "status": "accepted"
}
```

### `POST /ypb.Yak/StartAIReAct/{run_id}/events/push`

- 用途：在运行处于 pending / running 状态时继续推送事件
- 典型使用场景：
  - 用户在流式输出过程中输入新消息
  - 交互式审核响应
  - 运行时热补丁
- 请求：`ypb.AIInputEvent`
- 响应：

```json
{ "status": "accepted" }
```

以下旧版别名仍然可用：

- `POST /run/{run_id}`
- `POST /run/{run_id}/events/push`
- `GET /run/{run_id}/events`
- `POST /run/{run_id}/cancel`

### `POST /ypb.Yak/QueryAIEvent`

- 用途：以 gRPC 原生请求 / 响应结构查询已持久化事件
- 请求：`ypb.AIEventQueryRequest`
- 响应：`ypb.AIEventQueryResponse`

### `GET /ypb.Yak/StartAIReAct/{run_id}/events`（SSE）

- 用途：订阅流式输出事件
- 响应 content-type：`text/event-stream`
- 服务端会发送：
  - 立即返回就绪事件：

```json
{ "type": "listener_ready", "status": "ok", "run_id": "..." }
```

  - 大约每 15 秒一次心跳：

```json
{ "type": "heartbeat", "timestamp": 1730000000 }
```

  - AI 输出，序列化后的 `RunEvent`
  - 终态状态：

```json
{ "type": "done", "status": "completed|cancelled|failed" }
```

### `POST /ypb.Yak/StartAIReAct/{run_id}/cancel`

- 用途：取消一个正在运行 / 等待中的任务
- 请求体：无
- 响应：

```json
{
  "run_id": "uuid",
  "status": "cancelled"
}
```

---

## 6. 前端接入注意事项

1. **新运行一定要先连接 SSE**
   - 等待 `listener_ready` 后，再调用 `/run/{run_id}`。

2. **流式输出期间的用户输入**
   - 使用 `type=free_input` 调用 `/run/{run_id}/events/push`。

3. **Provider / Model 选择流程**
   - 先通过 `GET /setting` 获取当前设置
   - 再通过 `/setting/providers/get` 获取 providers
   - 确定 provider 后调用 `/setting/aimodels/get`
   - 最后通过 `POST /setting` 更新选中的 provider / model

4. **设置项命名规则**
   - 前端发 patch 请求时，推荐使用 snake_case
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
  if (data.type === "listener_ready") {
    // 3) 首次运行输入
    await fetch(`/agent/run/${runID}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ type: "free_input", free_input: "你好" })
    });
  }
};

// 4) 在流式输出过程中继续发送输入
await fetch(`/agent/run/${runID}/events/push`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ type: "free_input", free_input: "继续" })
});
```
