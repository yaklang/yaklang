# LSP 缓存与增量编译优化

## 概述

本次优化实现了智能的 LSP 缓存策略，大幅提升了代码编辑时的响应速度和用户体验。

## 核心改进

### 1. 文档状态管理 (DocumentManager)

**文件**: `document_manager.go`

**功能**:
- 统一管理所有打开的文档状态
- 追踪文档版本和编辑历史
- 维护多层缓存（Syntax Cache、SSA Cache）
- 自动驱逐最老的文档以控制内存

**关键特性**:
- 并发安全的文档访问
- 编辑爆发检测（EditCount 和 IsTyping）
- 缓存过期标记（Stale flag）

### 2. 层级化哈希策略 (Hash Utils)

**文件**: `hash_utils.go`

**三级哈希体系**:

1. **Full Hash**: 全文 SHA256，最快判断代码是否完全相同
2. **Structure Hash**: 去除注释和多余空白后的哈希，判断结构是否变化
3. **Semantic Hash**: 提取语义 token（关键字、标识符类型、运算符）后的哈希，判断语义是否变化

**智能判断**:
- 仅注释/格式变化 → 语义哈希不变 → 复用 SSA
- 局部代码修改 → 结构哈希变化 → 重新解析但可能复用部分 AST
- 新增/修改 import → 强制重编译

**优势**:
```
原方案：改一个空格 → 全文 hash 变化 → 完全重编译
新方案：改一个空格 → 语义 hash 不变 → 复用 SSA ✓
```

### 3. 编辑调度器 (EditScheduler)

**文件**: `edit_scheduler.go`

**Debounce 策略**:
- **输入中**（连续编辑）：不触发 SSA 编译，避免卡顿
- **短暂停** (400ms)：触发语法分析（快速）
- **长暂停** (1.5s) 或保存：触发 SSA 编译（完整）

**优先级队列**:
- P0 (高频快速): Completion, Hover, SignatureHelp
- P1 (精确语义): Definition, References
- P2 (跨文件): 依赖分析
- P3 (优化): GVN, LICM 等

**工作线程池**:
- 4 个并发 worker
- 避免阻塞 LSP 主线程
- 支持请求驱动的同步编译（超时保护）

### 4. 文档同步事件处理

**修改文件**: `server.go`, `http_server.go`

**事件响应**:
- `didOpen`: 初始化 DocumentState，触发背景分析
- `didChange`: 更新内容和版本，debounce 延迟分析
- `didSave`: 立即触发高优先级分析
- `didClose`: 清理缓存（延迟一段时间）

**原方案 vs 新方案**:
```
原方案：所有同步事件被忽略 → 每次请求都从文件系统读取
新方案：维护内存中的文档状态 → 直接从缓存获取 ✓
```

### 5. 请求优先级分级

**P0 快速响应** (Completion, Hover, SignatureHelp):
- 允许使用 5 秒内的旧缓存
- 后台异步更新
- 超时 3 秒后降级使用语法分析

**P1 精确语义** (Definition, References):
- 若缓存过期 > 5s，阻塞等待新 SSA
- 否则使用旧 SSA + 后台更新

## 性能指标

### 预期提升

| 场景 | 原方案 | 新方案 | 提升 |
|------|--------|--------|------|
| 纯格式变更 | 完整编译 | 复用缓存 | >95% 缓存命中 |
| 局部编辑 | 完整编译 | 增量/缓存 | >60% 缓存命中 |
| 连续输入期 | 每次编译 | 延迟编译 | 0 次编译 |
| Completion 响应 | 200-500ms | <100ms | 2-5x 提速 |

### 内存占用

- 每个文档: ~10MB (SSA + AST + 元数据)
- 最大文档数: 50 (可配置)
- 自动 LRU 驱逐

## 测试覆盖

### 单元测试

**document_manager_test.go**:
- ✓ 基本文档操作（打开、更新、关闭）
- ✓ 编辑计数和输入爆发检测
- ✓ 缓存设置和过期
- ✓ 最大文档数限制
- ✓ 并发访问安全性

**hash_utils_test.go**:
- ✓ 三级哈希计算
- ✓ 注释变化不影响语义哈希
- ✓ 空白变化不影响结构哈希
- ✓ 语义变化触发重编译
- ✓ Token 提取和哈希一致性

## 使用方式

### 启动 LSP 服务器

**Stdio 模式** (VS Code 等):
```bash
yak lsp
```

**HTTP 模式** (Web IDE):
```bash
yak lsp --http --host 127.0.0.1 --port 9633
```

### 调试模式

```bash
yak lsp --debug --log-file /tmp/yaklang-lsp.log
```

查看日志以了解缓存命中情况：
```
[LSP Cache] SSA program cache hit for yak (hash: abc12345)
[LSP DocMgr] updated document: file:///test.yak (version: 5, editCount: 3)
[LSP Scheduler] cache hit for request-driven analysis: file:///test.yak
```

## 架构设计

```
┌─────────────────────────────────────────────────────────┐
│                    LSP Request                          │
│              (Completion/Hover/etc.)                    │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│              DocumentManager                            │
│  ┌─────────────────────────────────────────┐            │
│  │  DocumentState (per file)               │            │
│  │  - Content & Version                    │            │
│  │  - SyntaxCache (AST + AntlrCache)      │            │
│  │  - SSACache (Program + Hash)           │            │
│  └─────────────────────────────────────────┘            │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│              EditScheduler                              │
│  - Debounce Timer (per document)                       │
│  - Priority Queue (P0/P1/P2/P3)                        │
│  - Worker Pool (4 threads)                             │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│              Hash Utils                                 │
│  - ComputeCodeHash (3 levels)                          │
│  - ShouldRecompileSSA / ShouldReparseAST              │
└─────────────────────────────────────────────────────────┘
```

## 配置参数

可在代码中调整以下参数：

**DocumentManager**:
```go
maxCacheAge  = 5 * time.Minute  // SSA 缓存最大年龄
maxDocuments = 50               // 最大并发文档数
```

**EditScheduler**:
```go
shortDebounce = 400 * time.Millisecond   // 短暂停
longDebounce  = 1500 * time.Millisecond  // 长暂停
workers       = 4                         // 工作线程数
```

**Request Priority**:
```go
maxStaleAge = 5 * time.Second   // P0 请求允许的最大缓存年龄
timeout     = 3 * time.Second   // 请求驱动编译的超时
```

## 未来优化方向

### 短期 (可选)
1. **增量 AST 解析**: 仅重新解析变化的函数
2. **函数级 SSA**: 仅重建受影响的函数
3. **更精细的语义 token**: 区分变量定义和引用

### 长期 (研究方向)
1. **基本块级增量 SSA**: CFG 片段重建
2. **数据流缓存**: 缓存 dominator 树和 phi 节点
3. **调用图增量更新**: 优化跨文件依赖分析

## 兼容性说明

- ✓ 完全向后兼容现有 LSP 客户端
- ✓ 不影响非 LSP 场景的编译流程
- ✓ 可通过环境变量禁用缓存（如需要）

## 相关文件

### 核心实现
- `document_manager.go` - 文档状态管理
- `hash_utils.go` - 层级化哈希
- `edit_scheduler.go` - 编辑调度和 debounce
- `server.go` - Stdio LSP 服务器集成
- `http_server.go` - HTTP LSP 服务器集成

### 测试
- `document_manager_test.go` - 文档管理测试
- `hash_utils_test.go` - 哈希工具测试

### 已有集成
- `../../../yakgrpc/language_server.go` - 现有的 SSA 分析逻辑
- `../../../static_analyzer/static_analyzer.go` - 静态分析入口

## 贡献

如果发现缓存策略的问题或有改进建议，欢迎提交 Issue 或 PR。

## License

与 yaklang 项目保持一致。

