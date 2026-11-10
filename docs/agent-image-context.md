# Agent 图片资源与两条消费路线

图片是资源，不是另一套推理历史。资源层负责保留原图、稳定引用、来源和读取；Timeline 负责当前上下文、历史观察、压缩和恢复。

## 主模型直接处理

用户附件以带图片引用的普通 Timeline 记录摄入；工具截图保留在 `ToolResult.Images`。二进制和 base64 不进入普通文本日志。

提示词渲染在同一个 Timeline 快照内返回文本和图片引用。运行时只读取这个快照中的图片，按对应输入／工具结果的顺序附到 `AIRequest.ImageData`，并提供“附件序号 → Timeline item → 图片 ID／来源／尺寸”的请求内说明。添加图片不会把主模型切换为 TierVision。

资源目录不会参与选图：其中存在一张图片不意味着本轮模型能看见它。某个来源记录被压缩、删除或移出轻量提示词窗口后，其图片不会从资源索引中重新附加。摘要里的图片 ID 只是引用，不会自动加载原图。

## 视觉模型处理

两种触发复用 `Config.DescribeImage` 与 `RecordImageObservation`：

- 被动：文本主模型接收附件或工具图片后，在摄入流程中调用 vision；工具结果先进入 Timeline，随后写入文字观察。
- 主动：agent 调用 `analyze_image(image_id, question)`，把指定问题的视觉观察写入 Timeline。多模态主模型也可主动使用这项能力。

观察记录包含图片 ID、来源、问题、可见事实及不确定性。失败明确写成“尚未理解图片”，不把截图调用成功视为任务成功。同图新问题会重新分析；自动摄入重放按来源事件去重，不缓存一个永远复用的图片摘要。

提示词生成不会调用视觉模型，也不会把另一个索引中的摘要临时拼进上下文。视觉结果是普通 Timeline 文字，使用现有持久化与压缩机制。

## 配置

`AIStartParams.ImageInput.Mode`：

| 模式 | 行为 |
| --- | --- |
| `auto`（默认） | 实际主模型明确支持图片时直传；不支持或未知时转述 |
| `native` | 强制主模型接收图文；明确声明不支持则报错 |
| `describe` | 使用配置的视觉 callback，主模型只接收文字观察 |

图片能力来自 `AIModelConfig.SupportsImageInput`，未知能力不通过模型名称猜测。自定义 callback 使用 `WithImageInputMode("auto", true)` 声明能力。视觉 callback 可通过 `WithVisionPriorityAICallback` 单独配置；它与普通主模型 callback 分开，已有全局单模型／分档策略仍适用于未显式覆盖的 callback。

```go
aicommon.WithImageInput(&ypb.AIImageInputConfig{
    Mode: "auto",
    VisualTokenLimit: 24000,
    MaxLongEdge: 1536,
    OutputReserveTokens: 4096,
    MaxRequestBytes: 24 * 1024 * 1024,
})
```

旧字段 `MaxActiveImages` 只为配置兼容保留，不再影响图片选择。Yakit 不再显示独立“活跃图片上限”。模型 YAML/JSON 的 `supports_image_input` 与 `image_token_estimator` 仍可使用；后者为估算策略，不是通用计费公式。

`auto` 遇到明确的 HTTP 400/422 图片能力拒绝时，在请求失败边界调用 vision 并把结果写入 Timeline，再以文字上下文重试。认证、限流、网络和普通参数错误不触发转述。拒绝仅按本运行中的模型配置记录，不持久化到资源目录。

## 截图、重新查看与展示

浏览器 `screenshot` 一次捕获，正式返回图片与语义结果。Go 工具使用 `aitool.ImageResult`／`MediaResult`；Yak 工具使用：

```yak
RESULT = aiagent.ImageResult(pngBytes, "image/png", {"operation": "screenshot"})~
```

MCP 的内联图片和明确资源引用走相同通道，包括通用 `call_mcp_tool`。远端资源由原服务执行 `resources/read`，不会从文字路径猜测并读取本机文件。

- `list_images`：分页列出可读取的资源；列表不表示图片已附到当前请求。
- `inspect_image`：产生新的普通上下文记录；多模态主模型直看，文本主模型接收 vision 文字。
- `analyze_image`：明确调用独立视觉模型并把回答写入 Timeline。

后两项支持 `x/y/width/height`，坐标相对于指定图片。裁剪生成新资源，保留原图引用、尺寸和区域；原图不会被修改。普通请求可生成 `MaxLongEdge` 限制内的视图，不因预算不足把图无限缩成不可读小图。

Yakit 卡片只提供缩略图／原图资源预览，不宣称是本轮模型的实际输入。`ReadAIImage` 继续验证会话和子 scope 归属；旧 `model` 变体只是标准概览兼容入口，不是一次请求的截图快照。

## 预算、压缩与恢复

请求预算是本次文本、工具定义、实际发送图片的视觉 token 估算和输出预留之和；请求字节另设上限。不会把所有历史图片的文件大小累计为上下文占用，也不把 base64 当成文字 token。

原生路线超出整体／视觉预算时触发现有 Timeline 压缩，不单独淘汰“旧活跃图片”。多模态压缩请求携带其实际渲染来源中的图片，使主模型能够保留视觉事实；摘要替换来源后，图片自然退出后续请求。压缩请求本身也检查预算；不能装入的媒体请求明确失败，建议裁剪或调整概览尺寸，而非静默丢图。

服务端 usage 是实际总输入观测，不把“总输入减本地文字估算”的差值当成真实图片 token，也不据此修改资源索引。

原图存于会话工作目录的 `.ai_images/<scope>/`，引用随 Timeline／工具结果持久化。资源索引只保存图片元数据和会话归属，不保存 Active/Pinned、视觉摘要、模型拒图或 token 校准。读取时校验大小与 SHA-256；缺失或损坏会明确报错。

完整会话子 agent 可继承独立资源目录和相应 Timeline；task-only 不继承历史资源。资源保存不等于模型可见性。删除／压缩上下文不删除原图，清理由现有会话工作目录生命周期负责。

## 验证范围

[任务清单](exec-plans/agent-image-context.md) 顶部 R1–R7 记录本次重构。确定性测试覆盖实际主循环和工具摄入的两条路线、两种动作协议、图文快照、压缩后不复活图片、持久化、裁剪、失败与重试。

Chat Completions／Responses 的本地 HTTP 测试验证真实编码。真实浏览器截图测试使用本机 Chrome；公共视觉模型 smoke 测试必须显式设置 `YAK_IMAGE_REAL_SMOKE=1`。确定性回调与单独模型直调不能替代真实模型自主浏览器任务验收。
