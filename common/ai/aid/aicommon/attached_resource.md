# Attached Resource

Attached Resource 用于把前端或上层任务传入的附加材料转成 ReAct loop 可消费的结构化数据。它分为两层：

- `AttachedResource`：传输壳，只包含 `Type`、`Key`、`Value`。
- `AttachedResourceData`：按 `Type` 反序列化后的结构化资源，负责渲染、可选的 loop 绑定，以及后续被具体 focus loop 消费。

## 文件分工

- `attached_resource.go`：资源 type/key 常量、`AttachedResource` 传输壳。
- `attached_resource_data.go`：`AttachedResourceData` 接口、factory 注册表、统一解析入口。
- `attached_resource_format.go`：跨资源共享的格式化 helper。
- `attached_resource_builtin.go`：轻量内置资源实现，包括 `default`、`file/file_path`、`knowledge_base`。
- `attached_resource_http_flow.go`：`http_flow_id` 资源实现。
- `attached_resource_http_fuzz_request.go`：`http_fuzz_request` 资源实现。
- `attached_resource_selected.go`：`selected` 资源实现。

新增资源实现按 `attached_resource_xxx.go` 命名，和资源 type 保持可读对应。

## 核心接口

```go
type AttachedResourceData interface {
    ToAttachData(loop ReActLoopIF) string
    Type() string
    BindLoopData(loop ReActLoopIF) error
    Unmarshal(raw string) error
}
```

- `Unmarshal(raw)`：把 `AttachedResource.Value` 转成资源结构字段。只做资源本身解析，不写 loop 状态。
- `Type()`：返回规范 type，作为 timeline 聚合 key 的来源。
- `ToAttachData(loop)`：把结构化资源渲染成给模型看的 Markdown 文本。
- `BindLoopData(loop)`：可选的通用绑定逻辑。只有资源本身天然需要写入通用 loop 状态时才使用。

## 执行流程

1. 前端通过 `AttachedResourceInfo` 传入 `Type/Key/Value`。
2. `re-act_free_input.go` 转成 `aicommon.AttachedResource` 并放入 task。
3. loop 初始化时调用 `reactloops.RunAttachedExtraResourcesInit(...)`。
4. `RunAttachedExtraResourcesInit` 对每个资源调用 `ParseAttachedResourceData`。
5. `ParseAttachedResourceData` 根据 `Type` 查 factory，构造结构体并调用 `Unmarshal`。
6. 通用入口依次调用 `BindLoopData`、`ToAttachData`。
7. 渲染结果按 `resource.Type()` 聚合，写入 timeline：`attached_<type>`。
8. `file` 资源在聚合阶段会额外交给 loop 注册的 file handler。图片等非文本文件可以由 handler 解析，例如写入 `attached_image_vision`。
9. `RunAttachedExtraResourcesInit` 返回已解析的 `[]AttachedResourceData`，具体 focus loop 可以按需做二次转移。

注意：资源结构应保持通用。比如 `http_fuzz_request` 是通用的 HTTP 请求包资源，不是 httpfuzz 专用类型；httpfuzz 只是额外把它转移成 `fuzz_request`、`original_request` 等本 loop 需要的字段。

## 内置兜底与特殊内置类型

- `default`：当 `Type` 没有命中任何注册 factory 时，解析为 `DefaultAttachedResourceData`。它保留原始 `Type/Key/Value`，并把 raw value 渲染为普通附加上下文。
- `file`：`Type=file` 解析为 `AttachedFileResourceData`，并识别 `Kind`。`file_path`、`filepath`、`file-path` 只是兼容别名，也会解析为同一结构。
  - `text`：`ToAttachData` 直接读取前 1024 字节作为内容预览。
  - `directory`：`ToAttachData` 调用 `filesys.Glance` 生成目录树，并裁剪到 1024 字节。
  - `image` / `binary`：`ToAttachData` 只输出文件元信息，不 dump 内容；通用入口会把 file resources 交给 loop 注册的 file handler 进一步解析。
- `knowledge_base`：解析为 `AttachedKnowledgeBaseResourceData`，但 `ToAttachData` 返回空，不 dump 内容。default loop 只根据返回的资源列表判断是否需要启动 knowledge enhance loop，具体查询仍由 knowledge enhance 内部处理。

## 新增资源步骤

1. 在 `attached_resource.go` 增加规范 type 常量。

```go
const AttachedResourceTypeExample = "example"
```

2. 新建 `attached_resource_example.go`。

```go
func init() {
    RegisterAttachedResourceDataFactory(
        AttachedResourceTypeExample,
        func() AttachedResourceData { return &AttachedExampleResourceData{} },
        "example_alias",
    )
}

type AttachedExampleResourceData struct {
    Raw string
}

func (d *AttachedExampleResourceData) Type() string {
    return AttachedResourceTypeExample
}

func (d *AttachedExampleResourceData) Unmarshal(raw string) error {
    d.Raw = strings.TrimSpace(raw)
    if d.Raw == "" {
        return utils.Error("example resource is empty")
    }
    return nil
}

func (d *AttachedExampleResourceData) BindLoopData(loop ReActLoopIF) error {
    return nil
}

func (d *AttachedExampleResourceData) ToAttachData(loop ReActLoopIF) string {
    return "## Attached Example\n\n" + d.Raw
}
```

3. 如果内容可能很大，使用 `inlineOrSpillAttachedText` 做 inline preview 和临时文件落盘。
4. 如果某个 focus loop 需要把该资源转移成自己的字段，不要覆盖全局 factory；在该 loop 初始化中消费 `RunAttachedExtraResourcesInit` 返回的 `[]AttachedResourceData` 并做类型断言处理。
5. 补测试：
   - type/alias 能通过 `ParseAttachedResourceData` 解析到正确结构。
   - `Unmarshal` 覆盖合法/非法 payload。
   - `ToAttachData` 输出包含足够的内容指示。
   - focus loop 的字段转移逻辑单独测试。

## 设计约束

- `AttachedResource` 只做传输，不承载业务解析逻辑。
- `AttachedResourceData` 实现放在 `aicommon`，除非只是某个 loop 的字段转移逻辑。
- 不在通用资源 registry 中注册 focus-loop 专用结构，避免同一个资源 type 在不同 loop 中语义漂移。
- `Type` 是稳定协议字段，新增或改名需要考虑前端和历史 payload。
- `BindLoopData` 应保持轻量、可失败可记录；复杂 loop 状态迁移优先放到对应 loop 包。
