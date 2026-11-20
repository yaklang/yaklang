# Agent 图片输入、视觉工作流与上下文管理任务清单

## 2026-11-05：按两个消费入口收敛（当前工作）

此前“完整完成”的记录与勾选是旧方案的实现记录，不代表下述重构已通过验收；组件测试和模型直调也不代表自主 agent 闭环验收。

- [x] R1：资源层仅保留原图、稳定引用、来源和读取；移除独立 Active/Pinned、摘要及 token 校准状态。
- [x] R2：主模型图片来自本次实际渲染的 Timeline/工具结果，与提示词同一快照发送；不按最近 N 张另行选图。
- [x] R3：被动识图在附件/工具结果摄入时执行，结果和失败正式写入 Timeline；提示词组装不调用模型。
- [x] R4：主动识图支持指定图片和问题，复用被动识图能力；重新查看直接产生新的普通上下文记录。
- [x] R5：请求预算仅统计实际发送的图片；上下文压缩使用原有 Timeline，不能自动移除图片却保留原样来源记录。
- [x] R6：补齐图文关联、两种识图触发、压缩/删除不复活旧图、恢复、重试和两种动作协议的回归测试。
- [x] R7：同步文档及受影响的展示协议，报告实际测试范围，不扩大验收结论。

### 本轮实现与验证证据

- `aimedia.Catalog` 仅管理资源；`TextTimelineItem.Images`／`ToolResult.Images` 保留来源记录，提示词渲染同时产生 `PromptImage` 快照。压缩头中提到图片 ID 不会重读图片；轻量窗口外的图片也不会发送。
- 附件及工具摄入负责被动识图；`analyze_image` 主动识图复用同一个 `Config.DescribeImage`，结果通过 `RecordImageObservation` 正式写入 Timeline。新问题不会命中旧摘要缓存。
- 删除旧图片状态机、按最近 N 张选图、资源内摘要／拒图／usage 校准，以及不再使用的 LiteForge 隐藏缩放选项。`MaxActiveImages` 仅接受旧配置，不生效，前端移除对应设置。
- 主模型压缩请求可以看到其来源图片；提交普通 Timeline 压缩后，来源图片退出后续请求。媒体无法装入请求时明确失败，不把图片逐步缩到 64 像素或静默丢弃。
- 新增真实 ReAct 主循环＋工具调用／产物摄入的确定性联测，覆盖 native/describe × 文本动作/原生工具调用四种组合。模型是测试 callback，截图是合成 PNG；不是外部模型自主浏览器验收。
- 全量通过：`aimedia`、`aicommon`、`reactloops`、`aireact`、`sessionruntime`、`aitool`、`aispec`、`aiengine`。扩大回归暴露的零值 Config 克隆兼容问题已修复。
- 图片相关 `-race` 测试通过：`aimedia`、`aicommon`、`reactloops`、`aireact`；编译器有既有 macOS 链接警告，无竞态报告。
- 本地 Chat Completions/Responses HTTP 请求确认：经过提示词分段投影后，图片和对应的 Timeline 说明仍在 user 内容中。浏览器截图／MCP／Yak 图片适配和 `ReadAIImage` 定向测试通过。
- 前端通过图片组件／事件测试、主进程图片协议测试（3 文件、5 测试），及实际 renderer `tsconfig.app.json` 类型检查。资源预览明确不代表本轮模型输入，移除“供模型使用的图片”查看选项。
- 新增 protobuf 消息移到文件末尾，保留旧类型顺序并减少生成代码噪声；不是手改生成的描述符。
- 未执行：本轮外部模型 smoke、真实模型自主浏览器长任务、完整 Electron 启动／打包；不把旧方案 M6 勾选当成本轮这些项目的证据。代码仍未提交、推送或发布。

## 目标与当前状态

最终目标：让图片可靠地参与当前 agent 的完整工作过程，包括用户上传图片、执行工具后观察截图、对比操作效果、追问局部细节，并具备完善的模型路由、上下文预算、压缩、存储与恢复能力。

- 工作分支：`feat/agent-native-multimodal-input`。
- 初始源码基线：`b65387139`。
- 建立日期：2026-11-04。
- 当前阶段：顶部 R1–R7 的两条路线重构与本地验证完成；下方 M0–M6 是待重新核验的旧方案记录。
- 当前交付：本地未提交的实现及前端工作区；不宣称已完成外部模型的自主长任务验收。
- 完成记录日期：2026-11-05；[用户与开发接入说明](../agent-image-context.md)。
- 当前范围不包含提交、推送或创建 PR；实施和发布状态分别记录。

任务使用稳定编号；`[x]` 只表示该项工作有对应完成证据。源码阅读、确定性测试、真实模型验证和前端验收不能互相替代。后续每次推进应更新本文件的任务状态、证据、待决事项和下一步。

## 已确认的范围与设计原则

1. 用户附件与工具产生的图片采用统一的资源与输入机制，覆盖浏览器截图、Yakit 截图，以及后续桌面截图工具接入。
2. 支持主模型原生看图和独立视觉模型转述两条路线；主模型支持多模态时能够直接结合图片、文字和工具结果工作。
3. 图片是模型上下文的一部分。按本次实际请求包含的图片估算视觉 token；文件字节、base64 长度、会话累计费用与上下文占用分别统计。
4. 上下文压缩必须同时处理文本与图片。历史图片可以退出当前模型上下文，但其资源引用与必要观察记录应保留，以支持重新查看。
5. 原图、供模型使用的派生图、界面缩略图明确区分。缩放和裁剪不得破坏原图，坐标变换必须可追溯。
6. 图片必须关联来源、轮次、任务和工具调用；截图应关联捕获时间与页面/窗口，避免把旧画面当作当前状态。
7. 图片内容属于用户或工具提供的数据，不得因转换为 OCR、摘要或观察记录而提升为系统指令。
8. 完成标准包含连续观察与操作、压缩后继续工作、会话恢复；单次图片问答成功不足以证明功能完成。

已实现契约：`AIStartParams.ImageInput` 使用 `auto / native / describe`；`AIModelConfig.SupportsImageInput` 是 optional bool，区分支持、不支持和未知。默认 auto 对未知能力保持视觉转述兼容性。图片保存于会话工作目录的独立 scope，并通过会话归属校验读取。

## 当前源码证据

以下是基线的静态检查结果，实施时需重新核对被修改的调用链。

| 位置 | 已确认的行为 | 需要补齐的部分 |
| --- | --- | --- |
| [附件图片 handler](../../common/ai/aid/aireact/reactloops/attached_resource_handlers/image_vision.go) | 调用 AnalyzeImageFile，将摘要、Dump 和 OCR 写入 Timeline | 能力分流、图片引用、原图进入主循环 |
| [图片分析](../../common/ai/aid/liteforge/liteforgeapp/analyze_image.go) | 临时 LiteForge 接收图片并返回结构化分析 | 面向截图验证目标的观察与重复查看 |
| [AI callback](../../common/ai/aid/aicommon/aicaller.go) | GetImageList 转换为 WithImageBase64 / WithImageRaw | 主循环与工具循环接入，验证两种动作协议 |
| [Gateway 路由](../../common/ai/gateway.go) | 通用调用可根据 Images 选择 TierVision | 直传图片时保持本次选定的主模型 |
| [消息构造](../../common/ai/aispec/base.go) | 支持图文内容，额外媒体可合并进最后一条 user 消息 | 历史轮次归属、工具图片和供应商适配 |
| [浏览器工具](../../common/ai/aid/aitool/buildinaitools/yakscripttools/yakscriptforai/browser/use_browser.yak) | screenshot 保存临时 PNG、输出路径，另取截图输出 base64 前缀日志 | 同一截图作为正式媒体结果返回，避免重复捕获形成不同画面 |
| [MCP 截图服务](../../common/mcp/screenshot.go) | 小图返回 ImageContent，大图返回引擎文件路径与元数据 | 在消费端解析图片与资源位置，保持现有服务协议兼容 |
| [MCP 工具适配](../../common/ai/aid/aitool/mcp_server_loader.go) | ImageContent 结构体转为文字占位；map 可能走通用字符串输出 | 同时处理结构体与反序列化 map，保留媒体内容 |
| [工具结果](../../common/ai/aid/aitool/tool_invoke_result.go) | 结果进入文本 Timeline 渲染 | 明确的媒体字段与独立资源生命周期 |
| [主循环预算](../../common/ai/aid/aireact/reactloops/prompt.go) | MeasureTokens(result.Prompt) 参与压缩触发 | 覆盖实际请求中的图片与其他非文本输入 |
| [主循环执行](../../common/ai/aid/aireact/reactloops/exec.go) | 文本 token 参与压缩进度与溢出重试判断 | 统一图文预算后的进度、重试与观测 |
| [现有图片压缩](../../common/ai/aid/liteforge/liteforgeapp/exports.go) | 超过 300 KiB 尝试 ffmpeg 压缩，失败保留原数据 | 通用派生图策略、可读性、预算和错误处理 |

公开实现参考：[Codex 上下文统计](https://github.com/openai/codex/blob/main/codex-rs/core/src/context_manager/history.rs)、[Gemini CLI token 估算](https://github.com/google-gemini/gemini-cli/blob/main/packages/core/src/utils/tokenCalculation.ts)、[Gemini CLI 压缩](https://github.com/google-gemini/gemini-cli/blob/main/packages/core/src/context/chatCompressionService.ts)。2026-11-04 阅读的版本会计入图片估算或服务端 usage；具体常量属于各实现，不作为本项目统一成本公式。正式采用某个算法时记录版本与适用模型。

## 阶段总览

| 阶段 | 交付物 | 依赖 | 状态 |
| --- | --- | --- | --- |
| M0 | 调用链审计、契约与兼容决策 | 无 | 已完成 |
| M1 | 统一图片资源与工具结果 | M0 | 已完成 |
| M2 | 主模型直传与独立视觉两条路线 | M1、M0 路由决策 | 已完成 |
| M3 | 完整请求预算与图片派生处理 | M1、M0 模型能力契约 | 已完成 |
| M4 | 图文共同参与上下文压缩 | M2、M3 | 已完成 |
| M5 | 持久化恢复、并发边界与展示 | M1，联调依赖 M2–M4 | 已完成 |
| M6 | 连续视觉任务验收与交付 | M2–M5 | 本地及外部合成验收通过；未发布，未运行完整打包 Electron 应用 |

M2 与 M3 可在契约确定后并行。图片直传在完整预算与压缩通过验证前保持受控启用，不能只接通传输就宣布交付。

## M0：调用链审计与契约

- [x] IMG-001：确认当前附件图片经过视觉模型转成文字进入 Timeline。
- [x] IMG-002：确认浏览器截图、MCP 图片消费与主循环文本预算存在衔接缺口。
- [x] IMG-003：确认底层已具备图片请求字段与图文编码能力，可复用。
- [x] IMG-004：追踪文本动作协议、原生工具调用协议的实际请求构造、工具结果摄入与重试，记录准确改动位置。
- [x] IMG-005：核对会话/任务/子 agent 的上下文继承、存储、清理机制，优先复用已有资源管理设施。
- [x] IMG-006：确定模型图片能力来源、三态语义、模式默认值、会话继承、热更新及单模型模式行为。
- [x] IMG-007：确定统一媒体结果、资源引用、来源关联与模型消息映射契约；明确远程 MCP 文件读取方式。
- [x] IMG-008：核对前端仓库、现有附件展示协议和桌面截图工具；记录跨仓库依赖与首期支持边界。

验收：形成有源码入口的实施映射，能解释一次用户上传、一次工具截图如何抵达下一轮请求，以及该请求如何计量和压缩。

## M1：统一图片资源与工具结果

- [x] IMG-101：建立图片资源记录，包含稳定 ID、MIME、原始尺寸、哈希、存储位置、来源、任务/轮次/工具调用关联与捕获时间。
- [x] IMG-102：支持原图与派生图引用，明确读取、保存、保留期限、会话恢复和清理归属；复用已有存储接口。
- [x] IMG-103：扩展工具结果媒体表达，文本与图片同时保留；base64 不进入普通日志、文本 Timeline 或文本 tokenizer。证据见下方首批实施记录。
- [x] IMG-104：接入用户附件；稳定多图顺序，重复资源可复用存储，但不同观察事件不能因同图哈希被合并丢失。
- [x] IMG-105：改造浏览器 screenshot，一次捕获得到的图片同时用于资源登记、工具返回和展示。真实 Chrome 捕获、视觉识别及前端图片组件验证已通过，范围见最终记录。
- [x] IMG-106：接入 MCP ImageContent 的结构体和 map 形式；支持已有截图服务的小图内联与大图文件结果。
- [x] IMG-107：区分本地路径、远程服务资源与 URL；仅按明确媒体协议解析，不能从任意工具文字猜路径并读取。
- [x] IMG-108：设置解码尺寸/像素/字节限制，验证 MIME 与实际格式，处理缺失、损坏、超限和取消。首批限额是防止解码/传输失控的保护值，不代表模型视觉 token 预算。

验收：附件与工具截图均产生可读取、可追溯的图片资源；媒体不会在工具适配或文本序列化时丢失。

## M2：两条视觉路线与连续工具循环

- [x] IMG-201：按实际选定模型的能力和会话模式分流；未知能力保留兼容行为，明确强制直传的失败语义。
- [x] IMG-202：将选中的图片注入主模型请求，保留与文字、轮次、工具结果的关联；不因带图被误路由到 TierVision。
- [x] IMG-203：覆盖文本动作与原生工具调用协议；对不支持 tool 图片内容的接口使用可验证的关联消息转换。
- [x] IMG-204：工具截图完成后自动进入后续模型请求；多轮调用、并行工具结果与多图顺序保持正确。
- [x] IMG-205：独立视觉调用携带当前目标、相关用户要求和最近操作；返回事实、可见文字、判断依据、不确定性及必要坐标。
- [x] IMG-206：提供按 image_id 重新查看、指定问题分析和区域裁剪的能力；两条路线共用资源与派生处理。
- [x] IMG-207：定义不支持图片的回退及可观测事件；认证、网络、限流错误沿用对应错误处理，不冒充能力不支持。
- [x] IMG-208：视觉结果标明来源与失败状态，图片处理失败不能被包装成“已经理解图片”；主模型不据截图调用成功推断任务成功。

验收：两条路线都跑通“操作 → 截图 → 观察 → 再操作 → 验证”，主模型直传时不额外强制执行一次视觉转述。

## M3：完整请求预算与图片派生处理

- [x] IMG-301：建立完整请求计量入口，分别记录文本/工具定义、图片估算、其他协议内容、输出预留与安全余量。
- [x] IMG-302：图片估算按模型、尺寸、detail/切片规则实现；未知模型使用明确的保守估算并记录估算来源。
- [x] IMG-303：只统计本次实际模型输入；区分活跃上下文、资源存储量、累计费用；缓存输入仍计入对应上下文占用。
- [x] IMG-304：使用服务端输入 usage 校准估算，记录请求覆盖范围；避免把已含图片的 usage 与图片估算重复相加。重写历史或切换模型后重算基线。
- [x] IMG-305：独立设置请求字节、单图像素、图片数量和总视觉预算限制；base64 传输开销不当作视觉 token。
- [x] IMG-306：生成供模型使用的派生图；支持视口概览、保持小字清晰、按需原图局部裁剪，阈值通过样本验证后确定。
- [x] IMG-307：记录原图尺寸、派生尺寸、裁剪区域、设备像素比和坐标空间；验证模型坐标到浏览器/桌面输入坐标的转换。
- [x] IMG-308：缓存派生图，避免重复解码和编码；观察摘要的缓存键还需包含问题、模型与分析参数，不能仅依据图片哈希。
- [x] IMG-309：预算判断与最终请求使用同一份图片选择结果；供应商不支持的格式、detail 或工具图片转换不得绕开计量。

验收：相同尺寸、不同编码体积的图片不会按文件字节误算上下文；只有图片导致超限的请求也能在发送前识别。

## M4：图文共同参与上下文压缩

- [x] IMG-401：将完整请求占用接入主循环压缩触发、上下文观测与溢出重试，替换仅依赖文本 prompt 的判断。
- [x] IMG-402：定义活跃图片选择策略：优先当前输入、最新相关画面和明确需要比较的历史图；保留理由可观测。
- [x] IMG-403：压缩选择支持保留原图输入、使用派生图、以观察摘要替代、移出活跃上下文并保留引用；不静默丢失图片信息。
- [x] IMG-404：摘要保留可见事实、操作结果、时间、图片 ID 与不确定性；明确区分“先前观察”与“当前仍能直接看图”。
- [x] IMG-405：压缩后支持重新查看旧图和局部细节；缺失或过期资源明确报告，不能仅凭路径声称已看到图片。
- [x] IMG-406：压缩操作完成后按最终图文请求重新计量；压缩无进展、单张图超过预算和供应商溢出有有限重试或明确错误。
- [x] IMG-407：压缩请求本身也受预算限制；压缩模型不支持图片时使用已有视觉观察或受控视觉调用，不能发送无效媒体请求。
- [x] IMG-408：图片移出上下文后，实际发送的历史消息中同步移除对应媒体，避免只修改显示或统计而仍然发送原图。
- [x] IMG-409：完整图文选择作为一致快照处理，验证并发工具返回、取消和重试不会漏图、重复加图或错配压缩边界。

验收：长时间连续截图时请求占用保持在预算内，压缩能实际减少视觉输入，并且仍能完成后续操作及追问历史画面。

## M5：恢复、边界与展示

- [x] IMG-501：会话重启恢复图片引用、观察摘要、来源与必要派生信息；明确资源过期后的行为。
- [x] IMG-502：验证任务、会话和子 agent 的图片可见范围；显式传递资源，避免串图和跨会话共享错误。
- [x] IMG-503：完成会话清理、取消、失败、重复导入与并发访问的资源生命周期处理。
- [x] IMG-504：输出图片捕获、准备、直传/转述、失败、压缩和重新加载事件；记录实际模型与图片版本。
- [x] IMG-505：接入前端缩略图、原图查看、工具截图关联与恢复后的展示；前端未验证时单独标记为未完成。
- [x] IMG-506：观测完整请求预算：文本 token、图片估算、实际 usage、活跃图片数、请求字节及压缩前后变化；日志不包含完整图片编码。
- [x] IMG-507：验证纯文本会话、现有视觉转述、现有 MCP 截图客户端及旧会话数据兼容性。

验收：图片可追溯、可恢复、可清理，模型实际看到的版本能够从事件与资源记录中确认。

## M6：验收场景与交付

| 编号 | 场景 | 关键判定 | 状态 |
| --- | --- | --- | --- |
| V01 | 用户上传一张/多张图并追问 | 图文同请求、顺序与轮次正确，两路线均可工作 | 通过（见最终证据） |
| V02 | 浏览器操作后截图检查效果 | 模型确实接收画面并根据可见结果继续操作 | 通过（见最终证据） |
| V03 | MCP 内联图片、大图文件与远程资源 | 媒体不丢失，路径归属正确，无 base64 文本膨胀 | 通过（内联与原服务资源读取） |
| V04 | 最新画面与操作前画面对比 | 时间和来源正确，不把旧图当现状 | 通过（见最终证据） |
| V05 | 小字报错、终端、长网页与高 DPI | 派生图可读，支持原图裁剪与坐标转换 | 通过（见最终证据） |
| V06 | 文本很少、图片占用导致超限 | 发送前触发预算处理，压缩后实际输入减少 | 通过（见最终证据） |
| V07 | 多轮连续截图与多次上下文压缩 | 活跃输入有界，关键证据保留，旧图可重读 | 通过（见最终证据） |
| V08 | 服务端 usage、缓存、历史重写与模型切换 | 不漏算或重复计入图片，估算基线更新正确 | 通过（见最终证据） |
| V09 | 两种动作协议及单模型模式 | 路由正确，工具循环中图片仍然可用 | 通过（见最终证据） |
| V10 | 重启恢复、过期文件、取消、并发与子 agent | 状态一致，资源隔离，错误明确 | 通过（见最终证据） |
| V11 | 纯文本和旧视觉链路回归 | 不改变既有可用行为 | 通过（见最终证据） |
| V12 | 前端展示与实际模型输入核对 | 缩略图/原图/派生图关联一致，状态可信 | 通过（见最终证据） |

- [x] IMG-601：建立可重复的本地截图样本和 mock provider 请求捕获测试，覆盖实际发送的内容、路由与预算，而非仅测试辅助函数。
- [x] IMG-602：在支持图片的主模型与文本主模型＋视觉模型组合上执行真实任务验收，记录具体模型、配置和源码版本。
- [x] IMG-603：记录视觉调用次数、首轮准备延迟、请求占用、usage 与压缩结果；比较同任务两条路线的行为和成本。
- [x] IMG-604：完成适用范围内的针对性测试、资源并发检查和跨端联调；失败、未覆盖协议及外部依赖分别记录。
- [x] IMG-605：补齐用户配置和开发接入说明，说明能力未知、资源过期、压缩与回退行为。

最终完成条件：M0–M5 所需任务及 V01–V12 的适用验收均有证据；不适用项明确说明原因。新增完整桌面控制系统不作为默认范围，但统一图片通路应允许其截图工具接入。真实模型或前端未验证时不能宣称整体交付完成。

## 待决事项

| 事项 | 当前建议 | 决定时机 |
| --- | --- | --- |
| 图片能力来源与模式配置位置 | 优先明确的模型能力配置；未知时兼容现有转述路线 | M0 |
| 存储归属与保留期限 | 复用现有会话资源存储，先确认恢复和清理契约 | M0–M1 |
| 供应商协议覆盖范围 | 按当前项目实际使用的适配器确定，逐项验收 | M0–M2 |
| 图片估算与安全余量 | 模型相关估算＋usage 校准，不照搬其他 agent 的固定值 | M3 |
| 图像质量和活跃图片预算 | 基于 UI、终端、长网页样本确定，支持配置 | M3–M4 |
| 前端与桌面截图依赖 | 核对现有入口，记录所需仓库和跨端任务 | M0、M5 |

## 推进记录与下一步

### 2026-11-04：建立任务清单

- 已完成：独立分支、附件视觉链路调查、工具截图与 MCP 消费缺口调查、当前文本预算入口调查，以及公开 agent 图片计量方式参考。
- 验证方式：源码阅读；尚未进行实现测试或真实模型任务验收。
- 决策：用户附件与工具截图统一规划；两条视觉路线均纳入目标；图片上下文预算和压缩属于核心交付。
- 下一步：IMG-004–IMG-008，先完成实际请求、资源生命周期与能力配置契约，再实施 M1。

后续记录格式：日期、完成的任务 ID、改动位置、验证命令/证据、未解决事项、下一步。任务实现完成后附对应测试或提交引用，保留先前失败与范围变化记录。

### 2026-11-04：首批实现——图片资源与工具媒体通道

已实现：

- 新增 `common/ai/aid/aimedia`。图片包含内容哈希 ID、实际 MIME、尺寸、字节数、原图路径，以及来源/工具调用/任务/观察时间。捕获时间与接收时间分开，未知捕获时间不伪造为已知。
- 原始 bytes 不参加 JSON 或普通格式化输出。资源通过显式读取取得；持久化引用在读取时核对字节数和 SHA-256，缺失或内容变化明确失败。
- 原图存入现有 tool-call artifact 目录的 `images/`，沿用产物生命周期。相同目录内内容相同的图片共用文件，不合并观察事件；并发安装不覆盖已有资源。
- 工具 callback 可返回 `MediaResult`，`ToolExecutionResult.Images` 与 `ToolResult.Images` 保持图片独立通道。文本缩略后仍保留图片引用，manifest 与 checkpoint JSON 保存元数据和路径。
- MCP 适配识别 SDK 结构体和真实 JSON-RPC map，不再将已识别的图片转成文字占位或整段 base64 日志。文本工具返回保持原有类型。远程文件路径只保留为元数据，不擅自读取本地同名文件。
- `aiagent.ImageResult(raw, mimeType, result)` 提供 Yak 工具的显式图片结果；运行时绑定调用上下文，保留其他导出。浏览器截图使用同一次捕获的数据，不再额外截图打印 base64 前缀。
- 单图最多 20 MiB / 4000 万像素；单个工具结果最多 16 张、总原始字节不超过 20 MiB。数值属于当前解码保护策略，待 M3 引入供应商预算与可配置策略。
- 图片持久化或 artifact 完整提交失败时返回明确错误，清除将失效的引用；纯文本产物保存失败维持既有语义。

验证与证据（当前分支工作区，尚无提交）：

- `aimedia` 测试覆盖完整解码、伪造大尺寸头、MIME 不一致、坏 base64、缓冲区所有权、JSON 恢复、损坏/缺失文件、并发保存与取消。
- `aitool` 测试覆盖 SDK/map 两种 MCP 图片、实际 HTTP MCP 客户端传输、原有文本结果、媒体数量限制和日志无图片编码；针对性用例通过 race detector。
- `aicommon/toolcall_images_test.go` 覆盖大文本缩略后的图片保留、checkpoint JSON 往返、重放、图片保存失败与 manifest/report 提交失败。
- `yak/ai_image_result_test.go` 覆盖真实 Yak 工具 RESULT 类型传递、上下文绑定、其他导出保留和浏览器脚本静态分析。初次测试发现脚本 `CTX` 变量不可依赖，已改为运行时绑定并通过复测。
- 验证命令：

  ```sh
  go test -race ./common/ai/aid/aimedia ./common/ai/aid/aitool -run 'TestImage|TestStore|TestMCPImage|TestToolExecutionResult|TestExecuteToolWithCapture|TestInvokeWithParams_Cancelled|TestToolResultIntegration|TestToolResultDumpAndTimelineUseNeutralProtocolSemantics' -count=1
  go test -race ./common/ai/aid/aicommon -run 'TestToolImages|TestToolImage|TestToolArtifact|TestNormalizeToolResultData|TestSmallToolResult' -count=1
  SKIP_SYNC_BUILD_IN_AI_TOOL=1 go test ./common/yak -run '^(TestYakToolImageResultChannel|TestYakImageResultBindingPreservesExportsAndCancellation|TestBrowserScreenshotScriptStaticAnalysis|TestYakAIDetachInvocationBinding|TestAIToolRuntimeBindingsPreserveExports)$' -count=1
  ```

边界：当前仅完成工具图片的保留与资源通道，尚未把这些图片自动发送给主模型或独立视觉模型；未改变既有附件视觉解析。主循环预算仍为原有文本逻辑，因此原生图片输入尚未开放。用户附件统一导入、远程文件获取、活跃图片选择、完整会话恢复、UI 和真实 Chrome/模型任务验收均未完成。

下一步：继续完成 IMG-004–IMG-008 中的能力/消息/会话契约，接入附件资源与主循环图片选择；M2 与 M3 联动推进，使用同一份输入快照完成路由与图文计量，之后进入 M4 压缩策略。

### 2026-11-05：完整实现与验收

交付状态：以上 M0–M5 及 IMG-601–IMG-605 的适用实现与验收已完成。本文较早记录中的“未接入”等边界属于当时状态，由本条记录替代。发布、PR 和安装包运行未执行，也不冒充已完成。

源码与接口决策：

- `aicommon/image_config.go` 承载会话策略、模型能力来源、scope 与资源登记；配置可经 gRPC、aiengine/Yak、模型 YAML/JSON 进入。模型、会话配置和缓存的拷贝保留 optional bool 的未知/false/true 语义。
- `reactloops/image_context.go` 为每轮生成一致图片快照，按实际模型配置选择 native/describe，保持用户附件顺序与工具观察时间顺序，使用已校验的派生图片构造请求。
- 普通文本动作和原生 function-call 都在 `exec_call_ai_loop_transaction.go` 接收同一快照。模型配置在本次请求固定；`PreserveImageModel` 防止带图导致通用 Gateway 自动切到 TierVision。
- `prompt.go`、`exec.go`、`prompt_overflow_retry.go` 统一计入图片与工具定义和输出预留。图片本身造成的压力能触发处理；压缩轮不携带原图，不把归档图片标成“当前可见”。
- `aimedia.Context` 保存观察目录、摘要、活跃/归档状态、变换、模型估算校准和能力拒绝记录；`inspect_image`/`list_images` 是核心动作，分页读取有文本预算。
- 派生图和原图分开；原图像素映射、裁剪、重复图片传输去重、文件字节/像素/保留量约束均已接入。没有视觉摘要的旧原生图片保留引用，不伪造摘要；已有摘要和普通证据可继续参与文本压缩。
- 实际 usage 包含图片与缓存输入，只用于校准对应模型的非文本估算，且支持 action 提交后迟到的 usage。没有覆盖已有用户 usage callback。
- 不可修复的输入过大与明确图片拒绝从全局无限网络重试返回到压缩/路由层。请求序列化完成后再次检查真实字节，不把 base64 当文本 token。
- MCP 直接工具 loader 和通用 `call_mcp_tool` 两条入口都保留图片；`resourceUri`/图片资源链接通过原 MCP 服务读取，Yakit 截图文件结果带限时句柄，绝不猜测远端文件的本机路径。
- 会话恢复读取原工作目录；子 agent 的索引隔离，完整继承与 task-only 区别明确。不可变文件可硬链接复用，原图、scope 与会话归属均校验。
- `ReadAIImage` 按会话/图片/scope 提供缩略图、原图、模型派生图。前端位于独立工作区 `/Users/rookie/Code/work4-yakit-images`，同名分支 `feat/agent-native-multimodal-input`，基线 `66299e458d`；实现模型能力表单、处理模式/预算设置、图片卡片、类型与 protobuf/预加载 IPC 协议。

关键验收对应：

| 验收 | 证据 |
| --- | --- |
| V01、V04、V06、V07 | `image_context_test.go`：多图上传顺序、连续八次截图、归档重载/裁剪、实际 payload 变小、图片触发压缩、压缩轮无图片 |
| V02、V05 | `TestLiveBrowserScreenshotImageResult`：实际 Chrome 打开合成网页、点击、前后截图哈希不同；启用外部验收后真实视觉模型读出点击后的 Saved 标题；裁剪/坐标通过独立几何测试 |
| V03 | `mcp_media_test.go` 的真实 HTTP 内联与资源读取，`screenshot_resource_test.go` 的注册/过期/内容变化校验，以及真实 Yak `TestMCPYakTools` 的通用入口图片与错误语义 |
| V08、V09 | usage 校准/迟到报告、模型 pin 跨 tier 热更新、两种动作协议，以及 Chat/Responses 实际 HTTP 请求图文内容测试 |
| V10、V11 | scope/会话隔离、JSON/磁盘恢复、失效图片、并发保存/快照、取消、纯文本与既有主循环/会话回归 |
| V12 | `ReadAIImage` 变体/归属测试、前端协议 round-trip、图片卡片加载/切换/过期显示、流事件持久化与设置/历史会话回归 |

真实模型记录（公共免费 `memfit-vision-free`，仅生成色块与合成网页，不使用用户截图或私有模型配置）：

- native：一轮调用，约 1.970 秒，本地视觉估算 292 token，服务端总输入 740 / 输出 2 token，回答 red。
- describe：视觉分析加文本主调用两轮，约 6.339 秒；文本主调用图片输入数为 0，回答 red。
- 实际 Chrome + 公共视觉模型：点击后读取 Saved 标题，验收测试总耗时约 6.18 秒。
- 这些是本次合成验收的测量值，不是产品性能或全部供应商的成本承诺。

验证命令与结果：

```sh
go test ./common/ai/aispec ./common/ai/aid/aimedia ./common/ai/aid/aitool ./common/ai/aid/aireact/reactloops ./common/ai/aid/aireact/reactloops/loopinfra ./common/ai/aid/aireact/sessionruntime ./common/aiengine -count=1 -timeout 120s
go test -race ./common/ai/aid/aicommon ./common/ai/aid/aireact/reactloops ./common/ai/aid/aimedia ./common/ai/aid/aitool ./common/yakgrpc ./common/mcp -run 'TestImage|TestNativeImage|TestDescribeImage|TestBothActionProtocols|TestStore|TestMCPImage|TestMainModelImageWire|TestToolImages|TestToolImage|TestReadAIImage|TestMultipleImage|TestPinnedImage|TestScreenshot' -count=1 -timeout 120s
SKIP_SYNC_BUILD_IN_AI_TOOL=1 go test ./common/ai/aid/aitool/buildinaitools/yakscripttools/test -run '^TestMCPYakTools$' -count=1
YAK_IMAGE_REAL_SMOKE=1 go test ./common/ai/aid/aireact/reactloops -run '^TestRealImageProviderSmoke$' -count=1 -timeout 75s -v
SKIP_SYNC_BUILD_IN_AI_TOOL=1 YAK_IMAGE_REAL_SMOKE=1 go test ./common/yak -run '^TestLiveBrowserScreenshotImageResult$' -count=1 -timeout 50s -v
```

以上对应执行通过。前端相关八个文件共 50 个 Vitest 通过，完整 renderer `tsconfig.app.json` 类型检查、renderer boundary 检查与两仓 diff whitespace 检查通过。复用现有依赖，测试使用可用的 Node 24；未安装或改变全局 Node。

保留失败与修复记录：

- 最初 Yak 图片入口依赖 CTX 变量，集成测试发现变量不可依赖，改为调用级上下文绑定。
- HTTP Responses 的异步流消费者触发既有 capture buffer 读写竞争，改为互斥快照捕获后针对性 race 测试通过。
- 通用 MCP 脚本最初在块内首次创建 RESULT，未进入外层结果通道；显式创建外层变量后修复。错误打印也改为实际支持的 builtin 输出，保留远端错误文本而不输出图片编码。
- 首次真实模型测试误用 nil emitter 导致测试消费者提前结束，改用实际 emitter 后真实两路线及浏览器验收通过。
- 前端有一处 NodeIdVerbose 使用了字符串，完整类型检查发现后改为中英文字段结构；复测通过。

验证边界：已覆盖本地、真实 Chrome、真实 MCP HTTP 与公共视觉服务；没有声称对每个第三方模型/协议扩展或每种显示器 DPI 做穷举验收，也未启动/打包/发布完整 Electron 应用。新增独立桌面控制系统仍属于最初明确的范围外；其截图可通过已实现的媒体通道接入。
