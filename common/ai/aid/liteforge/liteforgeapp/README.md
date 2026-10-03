# LiteForge 应用

本包集中维护使用 LiteForge 的应用与 Yak 导出。`liteforge` 核心负责协议选择、提示词投影、流式 arguments、字段回调、schema 校验和重试；应用在这些能力上完成分析、知识提炼和索引。本包不导入 `aiforge` 或任一 coordinator。

| 类别 | 源码 | 用途 |
| --- | --- | --- |
| 调用与导出 | `liteforge.go`、`exports.go` | Go 构造选项和 Yak `liteforge.*`；最终调用核心 `Request → Execute` |
| 文件与分块 | `analyze.go`、`split.go`、`utils.go` | 分派输入类型、分块、拆分静态指令与当次材料 |
| 多媒体 | `analyze_image.go`、`analyze_audio.go`、`analyze_video.go`、`analyze_video_omni.go`、`video_archiver.go` | 图片、音频、视频分析及视频归档 |
| 知识提炼 | `refine.go`、`refine_config.go` | 构建知识库及控制提炼参数 |
| 索引 | `index_knowledge.go`、`search_index.go` | 知识分片、问题索引和检索索引 |
| ERM | `erm.go` | 实体和关系分析 |
| 共享配置与结果 | `analyze_config.go`、`analyze_result.go` | 分析上下文、回调与结果接口 |

提示词与 schema 由 `promptloader` 加载，资源归入 `prompts/ai/aid/liteforge/liteforgeapp`；核心的两套协议模板保存在 `prompts/ai/aid/liteforge`。迁移资源时保持内容一致，目录位置不影响提示词字节和缓存前缀。

Go 应用导入本包；返回值使用 `aicommon.ForgeResult`，不携带多步 Forge 的 `ForgeBlueprint`。`aiforge.RegisterLiteForge` 只在多步 Forge 注册表边界转换返回值，执行仍由本包和核心完成。

Yak 继续使用 `liteforge.Execute`、多媒体及知识构建选项，`aiagent.CreateLiteForge` 和 `rag.*` 导出也指向本包。LLVM 模块登记的导入路径同步更新。函数调用与文本流的选择仍由 `aicommon.Config.EnableFunctionCallMode` 控制。

## 验证

已有 LiteForge 应用测试随实现迁入本包，覆盖辅助请求生命周期、流式字段、协议透传、内存校验、缓存分段和速度优先循环。

```powershell
go test ./common/ai/aid/liteforge/... ./common/aiforge
```

核心目录保留独立 Yak 冒烟脚本；直接执行，不需要 Go 注入业务脚本变量：

```powershell
yak common/ai/aid/liteforge/smoke_default_task.yak
```

本包的 [smoke.yak](smoke.yak) 使用 `ai.MockAIService` 验证文本流和 function call 两套真实处理链路，各请求一次，保留未知键和任意 JSON 值。脚本只 mock 模型响应，直接用 Yak CLI 执行：

```powershell
yak common/ai/aid/liteforge/liteforgeapp/smoke.yak
```
