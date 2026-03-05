# AIHTTP Frontend API Guide

> Audience: frontend developers (Web/CLI/TUI) integrating `common/yakgrpc/aihttp` gateway.

## 1. Overview

`aihttp` is an HTTP/SSE gateway built on top of yakgrpc streaming APIs.

- Base route prefix (default): `/agent`
- Main groups:
  - `/run/*`: runtime execution and event streaming
  - `/session/*`: session metadata and management
  - `/setting/*`: chat setting, model/provider/focus options

The gateway supports:

- Create/reuse a run session
- Stream AI output via SSE
- Push runtime input/hotpatch/interactive events while running
- Update settings and fetch provider/model/focus metadata

---

## 2. Authentication & Common Behavior

## 2.1 Optional auth headers

If gateway enables auth, frontend should provide:

- `Authorization: Bearer <jwt>`
- `X-TOTP-Code: <totp_code>`

If missing/invalid, API returns `401`.

## 2.2 CORS

Gateway always returns:

- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS`
- `Access-Control-Allow-Headers: Content-Type, Authorization, X-TOTP-Code`

## 2.3 Error response format

All HTTP errors are JSON:

```json
{
  "error": "Bad Request",
  "code": 400,
  "message": "invalid request body: ..."
}
```

---

## 3. Recommended Frontend Flow

For a **new conversation**:

1. `POST /session` to get `run_id`
2. Connect `GET /run/{run_id}/events` SSE first
3. Wait for SSE event: `listener_ready`
4. Call `POST /run/{run_id}` with first input event
5. During output, user can still send input via `POST /run/{run_id}/events/push`

For an **existing session**:

1. Ensure session exists in backend runtime map (or recreate by `POST /session` with known `run_id`)
2. Connect SSE
3. Call `/run/{run_id}` or `/run/{run_id}/events/push` to continue

Cancel current run:

- `POST /run/{run_id}/cancel`

---

## 4. Data Models (Frontend-facing)

## 4.1 `AIParams`

Used in session creation and run input params.

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

## 4.2 `PushEventRequest`

Used by `/run/{run_id}` and `/run/{run_id}/events/push`.

```json
{
  "type": "free_input|interactive|sync",
  "content": "optional fallback text",
  "params": { "...AIParams" },

  "is_config_hotpatch": false,
  "hotpatch_type": "AIService|ModelName|...",
  "is_start": false,

  "is_interactive_message": false,
  "interactive_id": "event_uuid",
  "interactive_json_input": "{\"approved\":true}",

  "is_sync_message": false,
  "sync_type": "optional",
  "sync_json_input": "{}",
  "sync_id": "optional",

  "is_free_input": true,
  "free_input": "user text",
  "attached_files": [],
  "focus_mode_loop": "optional"
}
```

At least one payload is required:

- `is_config_hotpatch`
- interactive input
- sync input
- free input
- `focus_mode_loop`
- `attached_files`

## 4.3 `RunEvent` (SSE payload core)

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

## 5. API Details

All paths below are relative to `<base>/<prefix>`, default prefix is `/agent`.

## 5.1 Setting APIs

### `GET /setting`

- Purpose: get current AI agent chat setting
- Request body: none
- Response: current setting JSON (keys are **PascalCase** in current backend response)

Example response:

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

- Purpose: patch and save setting
- Behavior:
  - merges request patch into existing setting
  - accepts both snake_case and PascalCase keys
  - applies defaults for missing core fields

Example patch:

```json
{
  "ai_service": "openai",
  "selected_provider_id": 3,
  "selected_model_name": "gpt-4o-mini",
  "review_policy": "manual"
}
```

Response: full saved setting (PascalCase keys).

---

### `GET /setting/global`

- Purpose: get global network config (passthrough gRPC `GetGlobalNetworkConfig`)
- Request body: none
- Response: `GetGlobalNetworkConfigResponse`

### `POST /setting/global`

- Purpose: update global network config (passthrough gRPC `SetGlobalNetworkConfig`)
- Request body: `GlobalNetworkConfig`
- Response: same config object saved

`GlobalNetworkConfig` fields include (not exhaustive):

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

### `POST /setting/providers/get`

- Purpose: fetch AI providers list
- Request body: `{}` (or empty JSON object)
- Response: gRPC passthrough `ListAIProvidersResponse`

Example response:

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

- Purpose: fetch models by provider config
- Request body supports multiple forms:

1) Legacy string:

```json
{ "Config": "openai" }
```

2) Legacy JSON string:

```json
{ "Config": "{\"Type\":\"openai\",\"APIKey\":\"***\"}" }
```

3) Object payload:

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

4) Flat object (same fields at top level) is also accepted.

Response:

```json
{
  "ModelName": ["gpt-4o", "gpt-4o-mini"]
}
```

### `POST /setting/aifocus/get`

- Purpose: get available AI focus modes
- Request body: `{}` (optional)
- Response: gRPC passthrough `QueryAIFocusResponse`

Example:

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

## 5.2 Session APIs

### `POST /session`

- Purpose: create a run session (or resume existing run_id in memory)
- Request:

```json
{
  "run_id": "optional-custom-id",
  "params": { "...AIParams" }
}
```

- Response (`201` for new, `200` for existing):

```json
{
  "run_id": "uuid",
  "status": "pending"
}
```

### `GET /session/all`

- Purpose: list all sessions (active in-memory + persisted metadata)
- Response:

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

- Purpose: update session title metadata
- Request:

```json
{ "title": "new title" }
```

- Response:

```json
{
  "run_id": "uuid",
  "title": "new title",
  "status": "updated"
}
```

---

## 5.3 Run APIs

### `POST /run/{run_id}`

- Purpose: submit first/normal input event for a run
- Notes:
  - requires existing session in run manager
  - if first valid input, backend starts grpc stream automatically
- Request: `PushEventRequest`
- Response:

```json
{
  "run_id": "uuid",
  "status": "accepted"
}
```

### `POST /run/{run_id}/events/push`

- Purpose: push additional runtime event while run is pending/running
- Typical use:
  - user enters new message during streaming
  - interactive review response
  - runtime hotpatch
- Request: `PushEventRequest`
- Response:

```json
{ "status": "accepted" }
```

### `GET /run/{run_id}/events` (SSE)

- Purpose: subscribe to streaming output events
- Response content-type: `text/event-stream`
- Server emits:
  - immediate ready:

```json
{ "type": "listener_ready", "status": "ok", "run_id": "..." }
```

  - heartbeat every ~15s:

```json
{ "type": "heartbeat", "timestamp": 1730000000 }
```

  - AI output as serialized `RunEvent`
  - terminal status:

```json
{ "type": "done", "status": "completed|cancelled|failed" }
```

### `POST /run/{run_id}/cancel`

- Purpose: cancel a running/pending run
- Request body: none
- Response:

```json
{
  "run_id": "uuid",
  "status": "cancelled"
}
```

---

## 6. Frontend Integration Notes

1. **Always connect SSE first for new run**
   - wait for `listener_ready`, then call `/run/{run_id}`.

2. **User input during streaming**
   - call `/run/{run_id}/events/push` with `type=free_input`.

3. **Provider/model selection**
   - get current setting via `GET /setting`
   - get providers via `/setting/providers/get`
   - decide provider, then call `/setting/aimodels/get`
   - update selected provider/model via `POST /setting`.

4. **Setting key naming**
   - write with snake_case is recommended for frontend patch requests
   - backend response currently uses PascalCase key names.

5. **Deprecated old APIs**
   - old `/session/{run_id}/send` and `/session/{run_id}/close` flow is replaced by `/run/*` flow.

---

## 7. Minimal Frontend Sequence Example

```ts
// 1) create session
const createResp = await fetch("/agent/session", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({})
}).then(r => r.json());
const runID = createResp.run_id;

// 2) connect SSE
const es = new EventSource(`/agent/run/${runID}/events`);
es.onmessage = async (ev) => {
  const data = JSON.parse(ev.data);
  if (data.type === "listener_ready") {
    // 3) first run input
    await fetch(`/agent/run/${runID}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ type: "free_input", free_input: "你好" })
    });
  }
};

// 4) send input during streaming
await fetch(`/agent/run/${runID}/events/push`, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ type: "free_input", free_input: "继续" })
});
```

