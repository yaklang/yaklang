# airaghttp 前端 / AI 对接指南

`airaghttp` 是一个**独立、纯 HTTP** 的 RAG 知识库服务，可脱离 yakit / aihttp 单独部署在任意机器上。它只提供检索能力（流式 AI 问答 + 同步向量搜索），没有管理员后台，所有配置只通过 `rag-server.yaml` 完成。

- 路由统一前缀：`/api/rag-server`
- 跨域：**完全放开**（`Access-Control-Allow-Origin: *`），前端可随意跨域调用
- 认证：可选 Bearer。`auth_token` 配置非空时，所有请求需带 `Authorization: Bearer <token>`；为空则不认证

## 1. 部署工作流

服务启动**必须**先有可用知识库，否则会报错拒绝启动。标准流程：

```bash
# 1) 查看在线可下载的知识库
yak rag-list

# 2) 下载并导入到本地 profile 数据库
yak rag-download --name "CWE 知识库"      # 单个（中/英文名均可）
yak rag-download --all                      # 全部
yak rag-download --name xxx --force         # 强制覆盖重新下载

# 3) 启动服务（不带 --config 用默认配置；本地无知识库则报错退出）
yak rag-server
yak rag-server --config rag-server.yaml
```

也可以不下载在线库，直接用本地 `.rag` 文件启动：

```bash
yak rag-server --rag-files /data/kb1.rag,/data/kb2.rag
```

## 2. rag-server.yaml 配置

```yaml
host: "0.0.0.0"
port: 9093
route_prefix: "/api/rag-server"
auth_token: ""          # 空=不认证；非空=要求 Authorization: Bearer <token>
concurrent: 3           # 最大同时进行的 chat 请求数，超过返回 429
timeout: 180            # 单次请求总超时（秒）
max_iteration: 1        # Agentic RAG 最大检索轮数
language: "zh"          # 回答语言偏好
collections: []         # 空=使用 profile DB 内全部已存在集合；非空=只挂载指定集合
rag_files: []           # 启动时导入的本地 .rag 文件
ai:
  type: "aibalance"     # AI 服务类型
  model: ""             # 空=走全局分级 aiconfig；非空=切换为单 callback 覆盖模式
  api_key: ""
  domain: ""
```

命令行参数可覆盖 yaml：`--host --port --prefix --auth-token --rag-files --concurrent --ai-type --ai-model --ai-apikey --ai-domain --debug`。

> 快速生成配置模板：`yak rag-server --gen-config rag-server.yaml`（已存在则需加 `--gen-config-force` 覆盖）。生成的模板带注释，修改其中 `ai.api_key` 等字段后用 `yak rag-server --config rag-server.yaml` 启动。

## 3. HTTP 接口

所有接口均在前缀 `/api/rag-server` 下。下文示例假设服务监听 `http://127.0.0.1:9093`。

### 3.1 GET /health 健康检查

```bash
curl http://127.0.0.1:9093/api/rag-server/health
```

返回：

```json
{
  "ok": true,
  "title": "RAG 知识库",
  "collectionCount": 2,
  "collections": ["rag_ab12cd34_cwe", "syntaxflow-aikb-rag"],
  "concurrent": 3,
  "inflight": 0,
  "language": "zh",
  "maxIteration": 1,
  "memoryDisabled": true,
  "timeout": 180,
  "authRequired": false,
  "ai": {
    "quality": { "mode": "lightweight", "type": "aibalance", "model": "memfit-light-free" },
    "speed":   { "mode": "lightweight", "type": "aibalance", "model": "memfit-light-free" }
  }
}
```

> `title` 为服务端配置的页面/品牌标题，前端可直接用于自有页面的标题展示；`memoryDisabled` 表示记忆系统是否被禁用。

### 3.2 GET /collections 列出可用知识库

```bash
curl http://127.0.0.1:9093/api/rag-server/collections
```

```json
{
  "ok": true,
  "total": 2,
  "collections": [
    { "name": "rag_ab12cd34_cwe", "description": "...", "modelName": "...", "dimension": 1024 }
  ]
}
```

### 3.3 POST /search 同步向量搜索（快速，无 AI 问答）

请求体：

```json
{
  "query": "什么是 XSS 漏洞",
  "collections": [],   // 可选，空=使用全部可用集合
  "limit": 10          // 可选，默认 10
}
```

```bash
curl -X POST http://127.0.0.1:9093/api/rag-server/search \
  -H "Content-Type: application/json" \
  -d '{"query":"什么是 XSS 漏洞","limit":5}'
```

返回：

```json
{
  "ok": true,
  "query": "什么是 XSS 漏洞",
  "total": 5,
  "results": [
    { "content": "...", "score": 0.83, "source": "rag_ab12cd34_cwe", "type": "result", "data": {} }
  ]
}
```

### 3.4 GET|POST /chat 流式 AI 问答（SSE，Agentic RAG）

问题来源：query 参数 `q`，或 POST body `{"question": "..."}` / `{"q": "..."}`。

```bash
# GET + SSE
curl -N "http://127.0.0.1:9093/api/rag-server/chat?q=如何修复SQL注入"

# POST + SSE
curl -N -X POST http://127.0.0.1:9093/api/rag-server/chat \
  -H "Content-Type: application/json" \
  -d '{"question":"如何修复SQL注入"}'
```

响应为 `text/event-stream`，事件协议如下（每个事件的 `data` 都是单行 JSON）：

| event     | data 字段                                   | 说明                         |
| --------- | ------------------------------------------- | ---------------------------- |
| `start`   | `question, collectionCount, collections, ai`| 开始处理                     |
| `log`     | `kind, label, message, type, nodeId`        | 检索/思考/任务等过程日志     |
| `thought` | `chunk`                                     | 思考过程流式片段（增量拼接） |
| `answer`  | `chunk`                                     | 最终答案流式片段（增量拼接） |
| `error`   | `code, message`                             | 错误                         |
| `end`     | `durationMs, ok`                            | 结束                         |

并发超限时直接返回 HTTP `429`，并发送一条 `error`（code=429）+ `end`（reason=server_busy）。

`log.kind` 可能的取值：`progress`（实时进度，如"执行搜索中""压缩搜索结果中""评估"）、`search`（检索）、`thought`（思考）、`task`（任务）、`plan`（计划）、`timeline`（时间线，含搜索条件/结果摘要）、`title`（会话标题）、`event`（其它）。

> 内置只读前端会把这些过程统一归并为"思考过程"，并映射为中文步骤标签（思考 / 获取资料 / 正在压缩 / 评估 …）。后端已对所有 `message` 做本地路径脱敏，并过滤 `notify`、`filesystem_pin_*`、`session_title` 等噪声事件。

### SSE 解析示例（浏览器）

```js
const es = new EventSource(
  "http://127.0.0.1:9093/api/rag-server/chat?q=" + encodeURIComponent("如何修复SQL注入")
)

let answer = ""
es.addEventListener("start", (e) => console.log("start", JSON.parse(e.data)))
es.addEventListener("log", (e) => console.log("log", JSON.parse(e.data)))
es.addEventListener("thought", (e) => console.log("thinking:", JSON.parse(e.data).chunk))
es.addEventListener("answer", (e) => { answer += JSON.parse(e.data).chunk })
es.addEventListener("error", (e) => console.error("error", e.data))
es.addEventListener("end", (e) => { console.log("done", JSON.parse(e.data), answer); es.close() })
```

> 注意：`EventSource` 不支持自定义请求头，无法携带 `Authorization`。若开启了 `auth_token`，请改用 `fetch` 读取流式响应（见下），或在反向代理层注入鉴权。

### 带 Bearer 鉴权的 fetch 流式读取

```js
const resp = await fetch("http://127.0.0.1:9093/api/rag-server/chat", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "Authorization": "Bearer YOUR_TOKEN",
  },
  body: JSON.stringify({ question: "如何修复SQL注入" }),
})

const reader = resp.body.getReader()
const decoder = new TextDecoder()
let buffer = ""
while (true) {
  const { value, done } = await reader.read()
  if (done) break
  buffer += decoder.decode(value, { stream: true })
  // 以空行分隔 SSE 事件帧，按 "event:" / "data:" 自行解析
  const frames = buffer.split("\n\n")
  buffer = frames.pop()
  for (const frame of frames) {
    const lines = frame.split("\n")
    const event = lines.find((l) => l.startsWith("event:"))?.slice(6).trim()
    const data = lines.find((l) => l.startsWith("data:"))?.slice(5).trim()
    console.log(event, data && JSON.parse(data))
  }
}
```

## 4. AI 模式说明

模型分为两个相互独立的通道（已移除 `ai_tier`），各自由对应配置块的 `api_key` 是否填写决定：

- **`ai`（质量优先 / 高质模型）**：关键推理与最终回答走此通道。填了 `ai.api_key` → 使用你的 `ai.type` + `ai.model`（+ 可选 `domain`）；未填 → 回退内置轻量模型 `memfit-light-free`。`/health` 中 `ai.quality.mode = custom | lightweight`。
- **`ai_lightweight`（速度优先 / 小尺寸模型）**：ReAct 循环里的搜索 / 记忆 / 压缩等高频调用走此通道，用小模型省成本。填了 `ai_lightweight.api_key` → 使用你配置的小模型；未填 → 回退内置 `memfit-light-free`。`/health` 中 `ai.speed.mode = custom | lightweight`。

> 设计意图：高频的速度调用别烧高质模型的 token，只有真正需要质量的关键推理才用高质模型。
> 启动时后端会检测并打印两个通道当前所用模型（`AI Model:` 行 / 日志）。任一块只填 `model` / `domain` 但缺 `api_key` 都视为未配置，回退到轻量模型。

## 4.1 其它定制项 (rag-server.yaml)

| 配置项 | 默认 | 说明 |
| --- | --- | --- |
| `title` | `"RAG 知识库"` | 页面 `<title>` 与左上角品牌名；同时通过 `/health` 的 `title` 返回，供自有前端复用。CLI 可用 `--title` 覆盖。 |
| `max_iteration` | `1` | 知识增强检索的迭代轮数，有效范围 1-10（1 最快）。CLI 可用 `--max-iteration` 覆盖。 |
| `disable_memory` | `true` | 默认禁用记忆系统（不构建/不入库/不检索），更快更省。设为 `false` 可让引擎累积并复用记忆；CLI 用 `--enable-memory` 显式开启。 |
| `system_prompt` | `""` | 自定义预设提示词，作为 `USER_PRESET` 注入每次请求（约 4000 token 上限，超长自动截断），用于设定角色/回答规则。CLI 可用 `--system-prompt` 覆盖。 |

> 用 `yak rag-server --gen-config <file>` 生成的模板已包含上述带注释字段，改完用 `--config <file>` 启动即可。

## 5. 安全提示

- 本服务**有意不做任何跨域限制**，请部署在可信网络或前置反向代理。
- 需要鉴权时务必设置 `auth_token`，并通过 HTTPS / 反向代理保护传输。

## 6. 前端 / UI 设计指导

本节为后续编写前端提供落地建议。后端只提供 4 个只读接口（`/health`、`/collections`、`/search`、`/chat`），不含任何管理端，前端可做成一个纯静态单页应用（SPA），直接跨域访问任意一台 `rag-server`。

> 内置只读页面：`rag-server` 默认在根路径 `/` 内置了一个 Codex 风格的只读搜索页（go:embed 进二进制，纯白主题，SSE 流式 + "Thinking..." 状态）。启动后浏览器直接打开 `http://<host>:<port>/` 即可使用；用 `--fe=false` 可关闭。下文是希望自研更完整前端时的设计参考。

### 6.1 推荐信息架构（布局）

建议三段式布局，桌面优先、移动端可折叠：

```
+----------------------------------------------------------+
| TopBar:  [服务地址] [连接状态点] [Token 输入] [模式切换]    |
+------------------+---------------------------------------+
| Sidebar          | Main                                  |
|  知识库列表       |  ┌─ 对话/检索结果区 (可滚动) ─────────┐ |
|  (/collections)  |  │ 流式 answer / thought / 检索卡片    │ |
|  - kb_a  [选中]   |  │                                     │ |
|  - kb_b          |  └─────────────────────────────────────┘ |
|  - kb_c          |  ┌─ 输入区 [多行输入] [发送] [停止] ───┐ |
|                  |  └─────────────────────────────────────┘ |
+------------------+---------------------------------------+
```

- TopBar：展示当前 `rag-server` 地址、健康状态（轮询 `/health`）、可选 Token 输入框、模式切换（问答 chat / 检索 search）。
- Sidebar：来自 `/collections` 的知识库列表，支持多选；选中项作为 `collections` 字段传给 `/search` 或 `/chat`（不选 = 全部）。
- Main：上方为消息/结果流，下方为输入区。两种模式复用同一输入区，仅渲染逻辑不同。

### 6.2 两种交互模式的 UI 差异

| 维度 | 问答模式 `/chat` (SSE) | 检索模式 `/search` (JSON) |
|------|----------------------|--------------------------|
| 触发 | 流式，长连接 | 一次性请求 |
| 渲染 | 思考过程折叠 + 答案逐字流式 | 结果卡片列表（带分数/来源） |
| 加载态 | "思考中…" 骨架 + 打字光标 | 列表 skeleton |
| 适用 | 自然语言问答、需要 AI 归纳 | 快速找原文片段、可离线、低延迟 |

建议默认进入问答模式，并在结果区提供"查看命中的原始片段"的入口（切到检索模式或并排展示）。

### 6.3 SSE 状态机（问答模式核心）

`/chat` 的事件顺序固定为：`start` → (`thought` / `answer` / `log` / `search` / `plan` / `timeline` …)* → `end`，异常时穿插 `error`。前端按下面的状态机渲染：

```
idle ──发送──> connecting ──收到 start──> thinking
thinking ──收到 thought chunk──> thinking(累积思考)
thinking ──收到首个 answer chunk──> answering(逐字追加)
answering ──收到 end──> done
任意状态 ──收到 error──> error(展示 message, 仍等待 end)
任意状态 ──收到 end──> done
连接断开/超时 ──> error(可重试)
```

事件渲染建议：

- `start`：清空上一轮、显示打字光标。`start.data` 含 `question` / `collectionCount` / `collections` / `ai`，可用于在 UI 回显本轮命中的知识库范围。注意当前每次 `/chat` 都是**独立单轮**请求（服务端不保存会话上下文），多轮对话需前端自行把历史拼进 `question`。
- `thought`：累加 `chunk` 到"思考过程"可折叠区域（默认折叠，调试可展开）。
- `answer`：累加 `chunk` 到答案气泡，按 Markdown **流式渲染**（增量 append，不要每次整体重渲）。
- `log` / `timeline` / `task` / `search` / `plan`：归入"执行轨迹"时间线，弱化展示（次要色、可整体折叠）。
- `error`：以警示样式插入一条记录，`code=429` 时提示"服务繁忙，请稍后重试"。
- `end`：移除打字光标、停用"停止"按钮；`end.data.ok=false` 时整体标记为失败。

### 6.4 关键交互细节（容易踩坑）

- **EventSource 局限**：浏览器原生 `EventSource` 只支持 GET 且**无法自定义 Header**。需要 Bearer 鉴权时，用 `fetch` + `ReadableStream` 手动解析 SSE（见第 3 节示例），不要用 `EventSource`。
- **停止/取消**：用 `AbortController` 取消 `fetch`，从而中断流式输出；UI 的"停止"按钮绑定到它。
- **429 退避**：收到 `code:429` 时不要立刻重试，采用指数退避（如 1s/2s/4s，最多 3 次），并提示用户。
- **多行输入**：`Enter` 发送、`Shift+Enter` 换行；发送中禁用输入框并显示"停止"。
- **健康轮询**：每 10~15s 轮询 `/health`，断线时 TopBar 状态点转红并禁用发送。

### 6.5 必备的页面状态（务必都设计）

- 加载中（loading）：knowledge base 列表 / 检索结果 skeleton。
- 空知识库（empty）：`/collections` 为空时，引导用户用 CLI `yak rag-list` / `yak rag-download` 下载（服务端无下载接口）。
- 空结果（no result）：检索/问答无命中时的友好文案。
- 错误（error）：网络错误、401（Token 错误/缺失）、429（繁忙）分别给出不同文案。
- 未鉴权（unauthorized）：401 时高亮 Token 输入框。

### 6.6 鉴权 UI

- 提供 Token 输入框，存入 `localStorage`；所有 `fetch` 自动带 `Authorization: Bearer <token>`。
- 未设置 `auth_token` 的服务端不会校验，前端可在 `/health` 探测到（无需 401 即代表免鉴权）。

### 6.7 最小 React 参考骨架（伪代码）

```jsx
const API = "http://your-rag-server:8765/api/rag-server"

async function streamChat({ question, collections, token, onEvent, signal }) {
  const res = await fetch(`${API}/chat`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify({ question, collections }),
    signal,
  })
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buf = ""
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buf += decoder.decode(value, { stream: true })
    const blocks = buf.split("\n\n")
    buf = blocks.pop() // 末尾可能是半个事件
    for (const block of blocks) {
      const event = block.match(/^event:\s*(.*)$/m)?.[1]?.trim()
      const data = block.match(/^data:\s*(.*)$/m)?.[1]?.trim()
      if (event) onEvent(event, data ? JSON.parse(data) : null)
    }
  }
}
```

```jsx
function ChatPanel() {
  const [phase, setPhase] = useState("idle")     // idle|thinking|answering|done|error
  const [thought, setThought] = useState("")
  const [answer, setAnswer] = useState("")
  const ctrl = useRef(null)

  const send = (q, collections, token) => {
    setPhase("connecting"); setThought(""); setAnswer("")
    ctrl.current = new AbortController()
    streamChat({ question: q, collections, token, signal: ctrl.current.signal,
      onEvent: (ev, d) => {
        if (ev === "start")   setPhase("thinking")
        if (ev === "thought") setThought(t => t + (d?.chunk || ""))
        if (ev === "answer") { setPhase("answering"); setAnswer(a => a + (d?.chunk || "")) }
        if (ev === "error")   setPhase("error")
        if (ev === "end")     setPhase("done")
      },
    }).catch(() => setPhase("error"))
  }
  const stop = () => ctrl.current?.abort()
  // 渲染: thought 折叠区 + answer(Markdown) + 输入/停止按钮 ...
}
```

### 6.8 视觉与可访问性建议

- 风格保持严肃专业（安全/工程场景），建议中性深灰底 + 单一强调色；提供深浅色两套主题。
- 思考过程与执行轨迹用次要色弱化，答案用主文本色突出；分数/来源用等宽字体。
- 流式输出区域设置 `aria-live="polite"`，便于读屏；所有按钮可键盘操作。
- 检索结果卡片建议展示：来源知识库（`source`）、相似度分数（`score`，保留 3 位）、内容摘要、展开全文。
