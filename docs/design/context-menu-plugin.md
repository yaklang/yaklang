# 右键插件（Context Menu Plugin）设计

状态：Backend v1 与 Frontend v1 已在本地功能分支实现，待联调验收
更新时间：2026-08-20
范围：Yak 插件模型、Yak Engine、yakgrpc、Profile DB、本机右键管理与前端结果承载

## 1. 背景

当前右键扩展能力主要挂靠在 CODEC 插件及若干历史执行入口上。CODEC 插件本身的稳定契约是：

```yak
handle = func(input) {
    return output
}
```

它适合编码、解码、文本转换和 FuzzTag 等“输入值到输出值”的纯变换场景；而 History 单选/多选、HTTP 数据包处理、快捷键触发、参数弹窗和结构化结果展示，属于“在特定工作上下文中执行一个动作”。两者在以下方面存在本质差异：

- 输入模型不同：CODEC 输入通常是一个字符串；右键动作输入可能是 HTTPFlow、多个 HTTPFlow、请求/响应数据包以及页面上下文。
- 生命周期不同：CODEC 可能被 FuzzTag 或转换链频繁调用；右键动作由用户主动触发，应按需执行并支持取消。
- 输出模型不同：CODEC 主要返回一个转换结果；右键动作可能输出文本、日志、表格、图、风险或显式的数据包替换结果。
- 管理方式不同：CODEC 不需要占用右键菜单；右键插件需要启用、隐藏、排序、数量限制、核心插件保护和快捷键绑定。

仓库中也已经存在与该问题相关、但目前彼此分散的实现：

- `common/yak/yak_to_caller_manager.go` 中 CODEC 调用明确校验插件类型，并固定调用 `handle(input)`。
- `common/yak/yakscript/exec_param.go` 中 CODEC 独立执行同样固定调用 `handle(input)`，并生成文本结果。
- `common/yakgrpc/grpc_plugin_packet_hack.go` 中存在旧的 `packet-hack` 数据包执行路径。
- `common/schema/menu_item.go` 与 `common/yakgrpc/grpc_menu.go` 提供通用菜单的分组、模式和排序能力，但没有右键插件所需的核心锁定、启用上限、快捷键、结果容器和稳定动作标识。

因此，本设计新增与现有主要插件类型同级的“右键插件”，并保持 CODEC 现有行为不变。

## 2. 目标

### 2.1 功能目标

1. 新增独立插件类型 `context-menu`，产品界面名称为“右键插件”。
2. v1 至少支持以下场景：
   - History 单选。
   - History 多选。
   - HTTP 请求/响应数据包。
3. Hook 采用类似 MITM 插件的可选函数定义方式。
4. 每个 Hook 的第一个参数固定为 `ctx`，其类型为右键动作专用的 `ActionContext`。
5. 支持现有 YakScript 参数定义和参数输入交互。
6. 支持弹框、抽屉、新 TAB 三种结果容器，并复用现有 `ExecResult`、`yakit.Output`、Table、Text、Graph 等输出协议。
7. 右键插件默认按需执行，不因安装数量增加而常驻加载大量 Yak Engine。
8. 提供独立的右键管理：
   - 核心/引擎内置右键插件不可停用或移除。
   - 用户可安装任意数量的右键插件；History 单选、History 多选、HTTP 数据包每个场景分别最多启用 15 个自定义插件。
   - 支持排序、快捷键绑定和结果展示方式覆盖。
9. 右键点击和快捷键触发复用同一执行链。
10. 为旧 CODEC 右键配置和 `packet-hack` 提供兼容迁移路径。

### 2.2 设计目标

- CODEC 的 `handle(input)`、FuzzTag 和现有执行方式保持不变。
- Hook 签名在 v1 冻结后可以长期兼容；新增上下文能力优先通过扩展 `ActionContext` 方法完成。
- 前端不执行插件代码来判断菜单是否可见。
- 服务端是核心插件保护、启用上限、场景分发和调用函数白名单的最终校验方。
- 插件输出协议和结果容器解耦，同一份插件代码无需感知当前使用弹框、抽屉还是 TAB。
- 遵循第一性原理，优先解决已经明确存在的用户问题，避免为了假设中的未来能力引入过度抽象、过度设计和范围发散。

### 2.3 第一性原理与最小设计原则

本需求的基本事实只有三个：

1. 用户在一个明确的工作上下文中选择一个插件动作。
2. 引擎把该上下文和业务数据传给一个确定的 Hook，并管理这次调用的生命周期。
3. 插件产生展示结果，或返回一个明确、可校验的业务动作结果。

v1 的设计应直接围绕以上事实展开。最小必要组成只有：

```text
插件类型 + 固定 Hook + ActionContext + 按需执行 + 本机 Binding + 现有输出流
```

除非当前需求无法通过上述组成完成，否则不增加新的框架层。具体约束如下：

- 不为尚未出现的场景提前设计通用事件总线、任意 Hook 注册中心或完整工作流编排引擎。
- 不因为未来“可能有多个动作”就在 v1 引入动态 Action DSL；v1 一个标准 Hook 对应一个固定 ActionID。
- 不因为未来“可能支持更多页面”就把所有业务输入塞进无类型 map；v1 保留三个明确 Hook 和强类型业务参数。
- 不重新设计 Yak 插件参数系统、输出协议、表格协议或快捷键系统；能够复用现有能力时只增加必要的适配边界。
- 不把 `ActionContext` 扩展成同时负责 UI 控制、数据库写入、菜单管理和结果渲染的万能对象；它只描述本次调用并承载生命周期。
- 不为减少少量重复代码而合并 CODEC、MITM 和右键插件的运行语义；清晰边界优先于表面的框架统一。
- 不以“以后可能需要”为单独立项理由。新增抽象必须指出当前具体问题、最小替代方案为何不足，以及可以验证的收益。
- 实现优先选择能够独立测试、容易删除和容易回退的局部改动，避免一次性重构整个插件体系。

评审任何新增设计时，依次回答：

1. 它解决的是当前已经确认的问题，还是假设中的未来问题？
2. 删除这一层后，v1 的哪一项验收标准无法完成？
3. 是否可以复用现有数据结构、执行器或输出协议，以更小改动完成？
4. 它是否增加新的状态来源、生命周期或兼容分支？如果增加，收益是否足以覆盖复杂度？
5. 能否先用固定枚举、固定 Hook 或显式适配器落地，等第二个真实用例出现后再抽象？

如果第 2 个问题没有明确答案，该设计默认不进入 v1。

## 3. 非目标

v1 明确不包含以下内容：

- 不把 MITM 插件自动转换为右键插件。
- 不允许右键插件注册任意前端页面或任意 Hook 名称。
- 不在菜单展开时加载或执行插件。
- 不把所有安装的右键插件自动加入菜单。
- 不把右键插件作为 MITM 一样的常驻 Hook 加载。
- 不允许插件通过普通 `[]byte` 返回值隐式覆盖当前数据包。
- 不在 v1 一次性支持“选中全部查询结果”的无界批量 HTTPFlow 加载。
- 不改变现有 CODEC 插件类型或旧 CODEC 数据记录。
- 不把本次需求扩展成通用插件动作平台、跨页面自动化系统或可视化工作流系统。
- 不为没有第二个真实用例的扩展点提前建立可配置 DSL、注册协议或多层抽象。

## 4. 术语

- **右键插件 / Context Menu Plugin**：`YakScript.Type == "context-menu"` 的一等插件类型；管理页还会把具有旧右键标签的 CODEC 作为兼容动作统一管理，但不会改写其类型或执行方式。
- **Context Action**：一个可由右键、快捷键或受控 API 触发的动作。v1 中一个 Hook 对应一个 Action。
- **ActionContext**：一次 Context Action 调用的只读执行上下文，Yak Hook 中命名为 `ctx`。
- **Scene**：决定调用哪个 Hook 的标准场景，例如 `history-single`。
- **Source**：产生本次上下文的具体功能页面，例如 `history`、`mitm` 或 `web-fuzzer`。
- **Trigger**：本次动作的触发入口，例如右键或快捷键。
- **Result Mode**：前端承载插件结果的容器模式。
- **Binding**：用户本机对右键插件的启用、排序、快捷键和展示覆盖配置。
- **Core Action**：来自核心右键插件，或来自具有旧右键能力且 `IsCorePlugin == true` 的兼容 CODEC；必须启用且不可移除。

## 5. 插件类型边界

新增类型常量：

```go
const SCRIPT_TYPE_CONTEXT_MENU = "context-menu"
```

插件类型关系为：

```text
yak
mitm
port-scan
codec
context-menu
nuclei
syntaxflow
...
```

职责约定：

| 类型 | 核心用途 | 入口契约 | 生命周期 |
|---|---|---|---|
| `codec` | 编码、解码、文本变换 | `handle(input)` | 由 CODEC/FuzzTag 等调用方管理 |
| `context-menu` | 工作上下文动作 | 本文定义的可选 Hook | 用户触发时按需执行 |
| `mitm` | 在线流量监听、镜像、劫持 | MITM Hooks | MITM 会话期间常驻 |

旧 CODEC 插件不改变类型。新建或编辑 CODEC 插件时，不再为新插件开放“用于数据包右键”“用于 History 单选/多选”等右键专用配置；旧数据由兼容层继续识别。

## 6. v1 Hook 契约

### 6.1 最终函数签名

v1 固定以下三个可选 Hook：

```yak
handleOneHTTPFlow = func(ctx, flow) {
    // ctx:  *ActionContext
    // flow: *schema.HTTPFlow
}

handleMultiHTTPFlows = func(ctx, flows) {
    // ctx:   *ActionContext
    // flows: []*schema.HTTPFlow
}

handleHTTPPacket = func(ctx, request, response) {
    // ctx:      *ActionContext
    // request:  []byte，可能为空
    // response: []byte，可能为空
}
```

插件至少实现其中一个 Hook。三个 Hook 均为可选；静态分析在一个 Hook 都没有实现时报告错误。

### 6.2 Hook 与场景映射

| Scene | Hook | 展示条件 |
|---|---|---|
| `history-single` | `handleOneHTTPFlow` | 当前明确选中一个 HTTPFlow，且插件实现该 Hook |
| `history-multi` | `handleMultiHTTPFlows` | 当前明确选中多个 HTTPFlow，且插件实现该 Hook |
| `http-packet` | `handleHTTPPacket` | 当前页面可提供请求或响应数据包，且插件实现该 Hook |

服务端必须维护 Scene 到 Hook 的固定映射。客户端只能提交标准 Scene/ActionID，不能直接提交任意函数名供服务端调用。

### 6.3 不做隐式 Hook 降级

- 只实现 `handleMultiHTTPFlows` 的插件不会自动出现在 History 单选中。
- 只实现 `handleOneHTTPFlow` 的插件不会对多选逐条隐式执行。
- 插件作者需要同时支持单选和多选时，应显式实现两个 Hook，并在插件内部复用辅助函数。

示例：

```yak
handleFlow = func(ctx, flow) {
    // 复用的业务逻辑
}

handleOneHTTPFlow = func(ctx, flow) {
    handleFlow(ctx, flow)
}

handleMultiHTTPFlows = func(ctx, flows) {
    for flow in flows {
        if ctx.Err() != nil {
            return
        }
        handleFlow(ctx, flow)
    }
}
```

### 6.4 Hook 参数选择原则

`ctx` 承担所有场景共享的动态上下文；其余参数承担当前 Hook 的强类型业务数据。

不采用单一的：

```yak
handle = func(ctx)
```

原因是单一入口不能通过函数定义直观看出插件支持的场景，静态分析和 IDE 补全也难以提供可靠能力判断。

不采用：

```yak
handleHTTPPacket = func(ctx, config, request, response)
```

原因是 `ctx` 与 `config` 都会承载调用级信息，职责重叠。`ActionContext` 统一承担原计划中的 config 能力。

## 7. ActionContext 设计

### 7.1 定位

`ActionContext` 不是只包含 KV 的静态配置，也不是只暴露取消信号的裸 `context.Context`。它是一次右键动作的完整只读执行上下文：

- 包装真实的 Go `context.Context`。
- 提供取消、Deadline 和错误状态。
- 提供 Scene、Source、Trigger、HTTPS 状态和数据存在性。
- 提供本次已经校验并完成默认值填充的参数快照。
- 提供 Runtime ID、Plugin UUID 和 Action ID 等可观测信息。

Yak 插件只接收一个 `ctx`，不额外接收 `config`。

### 7.2 生命周期 API

建议暴露：

```yak
ctx.Done()
ctx.Err()
ctx.Deadline()
```

其中：

- 前端点击停止、关闭绑定生命周期的结果容器或 gRPC stream 取消时，底层 Go context 必须被取消。
- Yak Engine、插件执行器以及支持 context 的网络请求 API 应使用同一底层 context。
- 普通短任务不要求插件作者主动轮询；长循环、批处理或插件自行创建的并发任务应检查 `ctx.Err()` 或 `ctx.Done()`。
- Hook 返回后，插件不得继续持有或异步使用本次 `ctx`、Flow 或 Packet 引用。

仅仅把 `ctx` 作为参数传入、但不向引擎和网络调用传播取消，不视为完成该能力。

### 7.3 场景 API

建议暴露：

```yak
ctx.Scene()
ctx.Source()
ctx.Trigger()
ctx.SelectionCount()
```

标准值：

```text
Scene:
- history-single
- history-multi
- http-packet

Source:
- history
- mitm
- web-fuzzer
- packet-editor
- repeater
- unknown

Trigger:
- context-menu
- shortcut
- api
```

`Scene` 决定调用哪个 Hook；`Source` 表示业务数据来自哪个页面。一个 `http-packet` Scene 可以来自 MITM、Web Fuzzer 或独立数据包编辑器，因此二者不能合并。

### 7.4 HTTPS 四态

单个 bool 无法区分“明确为 HTTP”和“调用方没有提供 HTTPS 信息”，History 多选还可能同时包含 HTTP 与 HTTPS。v1 使用以下四态：

```go
type HttpsState string

const (
    HttpsStateUnknown HttpsState = "unknown"
    HttpsStateHTTP    HttpsState = "http"
    HttpsStateHTTPS   HttpsState = "https"
    HttpsStateMixed   HttpsState = "mixed"
)
```

建议暴露：

```yak
ctx.HttpsState()
ctx.HasHttpsInfo()
ctx.IsHttps()
ctx.ContainsHttps()
```

语义固定为：

| 状态 | `HasHttpsInfo()` | `IsHttps()` | `ContainsHttps()` |
|---|---:|---:|---:|
| `unknown` | false | false | false |
| `http` | true | false | false |
| `https` | true | true | true |
| `mixed` | true | false | true |

其中，`IsHttps() == false` 不等于“明确为 HTTP”；插件需要区分时必须先检查 `HasHttpsInfo()` 或直接读取 `HttpsState()`。

状态生成规则：

- History 单选：由对应 `HTTPFlow.IsHTTPS` 和记录来源生成明确状态。
- History 多选：全 HTTP 为 `http`，全 HTTPS 为 `https`，同时存在两类为 `mixed`；来源无法确认时按保守规则生成 `unknown` 或由具体 Flow 状态聚合。
- HTTP Packet：优先采用调用页面明确提供的协议状态；如果调用页面不具备该信息则为 `unknown`。
- 不仅根据端口 443 猜测 HTTPS。
- 新 RPC 使用四态字符串字段，不复用无法表达 unknown/mixed 的旧 `bool IsHttps` 作为唯一协议字段。

### 7.5 数据存在性 API

建议暴露：

```yak
ctx.HasRequest()
ctx.HasResponse()
ctx.RequestSize()
ctx.ResponseSize()
```

`handleHTTPPacket` 中的 request 和 response 都允许为空。插件不能只通过 `len(response) == 0` 判断响应是否存在，因为“没有响应”和“存在一个空响应”语义不同。

示例：

```yak
handleHTTPPacket = func(ctx, request, response) {
    if !ctx.HasRequest() {
        yakit.Error("当前上下文没有请求数据")
        return
    }

    if !ctx.HasResponse() {
        yakit.Warn("当前只有请求，没有响应")
    }

    if !ctx.HasHttpsInfo() {
        yakit.Warn("当前页面没有提供 HTTP/HTTPS 信息")
    }
}
```

### 7.6 参数 API

右键插件继续使用现有 `YakScript.Params`、`ExecParamItem` 和参数表单能力。`ActionContext` 提供本次参数的只读访问：

```yak
ctx.HasParam("keyword")
ctx.Param("keyword")
ctx.ParamString("keyword")
ctx.ParamBool("verbose")
ctx.ParamInt("limit")
ctx.Params()
```

约束：

- `ctx.Params()` 返回副本或只读视图，插件不能修改用户的参数预设。
- `ctx.Param*` 与现有 `cli.*` 读取同一份参数快照，不维护两套默认值和类型转换规则。
- 服务端执行 Hook 前完成必填校验、默认值填充和类型校验。
- 敏感参数默认不进入“记住上次参数”持久化数据。

为兼容现有插件开发习惯，v1 可以同时允许：

```yak
keyword = cli.String("keyword")
```

和：

```yak
keyword = ctx.ParamString("keyword")
```

但两者必须得到相同值。

### 7.7 运行时与标识 API

建议暴露：

```yak
ctx.RuntimeID()
ctx.PluginUUID()
ctx.PluginName()
ctx.ActionID()
```

以下管理信息不进入 `ctx`：

- 用户绑定的具体快捷键。
- 右键排序。
- 是否占用自定义额度。
- 是否允许用户停用。
- 本机管理页面的筛选状态。

这些信息属于 Binding/管理域，不属于插件业务运行时。

### 7.8 Go 侧概念结构

以下代码用于说明职责，不作为最终字段布局约束：

```go
type ActionContext struct {
    context.Context

    scene   ActionScene
    source  ActionSource
    trigger ActionTrigger

    httpsState HttpsState

    hasRequest  bool
    hasResponse bool
    requestSize int64
    responseSize int64

    selectionCount int
    params         map[string]any

    runtimeID  string
    pluginUUID string
    pluginName string
    actionID   string
}
```

实现要求：

- 构造后对插件只读。
- `Context` 不允许为 nil；无外部 context 时使用 `context.Background()`。
- `Params()` 不返回内部可变 map。
- RuntimeID 在一次执行及全部流式输出中保持一致。
- ActionID 是稳定标识，不使用本地化菜单标题。

## 8. 插件能力发现与静态分析

### 8.1 能力来源

右键插件支持哪些 Scene，由其实现的标准 Hook 决定：

```text
handleOneHTTPFlow    -> history-single
handleMultiHTTPFlows -> history-multi
handleHTTPPacket     -> http-packet
```

保存插件时执行静态分析。v1 不增加 capabilities 持久化字段：单个场景的菜单查询只对核心插件和该场景最多 15 个已启用插件检查 Hook 定义；管理页显式请求全部插件时再检查全部已安装右键插件。SSA 可以使用现有缓存，但不会执行插件代码。

这个选择少维护一个元数据状态源，避免插件内容与 capabilities 字段不一致。只有性能数据证明现有 SSA 缓存仍不足时，才考虑在保存时持久化能力索引。

### 8.2 静态检查

至少检查：

- 至少实现一个标准 Hook。
- 同一标准 Hook 不可重复定义。
- Hook 参数数量符合 v1 契约。
- 不支持的 Hook 名给出提示，但普通辅助函数不受限制。
- 插件类型为 `context-menu` 时提供 `ActionContext`、`HTTPFlow` 等对应的 SSA 外部类型和补全信息。

### 8.3 版本策略

v1 不增加 `api_version` 字段。三个固定 Hook 的函数名和参数数量就是当前契约；出现第二套真实且不兼容的契约前，不提前建立版本协商层。

## 9. 参数交互

参数定义继续复用现有 YakScript Params，不创建第二套参数 DSL。

执行前交互规则：

1. 插件没有参数：直接执行。
2. 参数均有合法默认值：可以直接执行；用户可在 Binding 中配置“执行前总是询问”。
3. 存在未填写的必填参数：展示参数窗口，确认后执行。
4. 快捷键触发与右键触发执行相同的参数检查。
5. 用户可以保存本机参数预设；敏感字段默认不持久化。
6. 参数窗口取消时，不创建插件执行任务。

前端表单提交后，服务端再次执行参数校验；不能只依赖前端。

## 10. 输出与结果容器

### 10.1 Result Mode

支持：

```text
auto
dialog
drawer
tab
```

- `dialog`：当前页面弹框，适合一次性短文本、Markdown 或小表格。
- `drawer`：适合流式日志、中等表格及需要保留当前工作上下文的结果。
- `tab`：适合大型表格、图、长时间任务或多区域结果。
- `auto`：由前端根据输出类型和统一产品策略选择。

展示方式优先级：

```text
用户 Binding 值 > auto > 前端统一系统默认
```

v1 不增加插件作者默认 Result Mode 字段。用户在本机 Binding 中选择具体容器；没有选择时后端返回 `auto`，由前端统一解析。

### 10.2 输出协议复用

结果容器只决定结果出现的位置，不改变插件输出协议。右键插件复用现有能力：

- `yakit.Info`、`yakit.Warn`、`yakit.Error`。
- `yakit.Output`。
- `yakit.NewTable`。
- `yakit.EnableTable` / `yakit.TableData`。
- `yakit.EnableText` / `yakit.TextTabData`。
- Graph、Risk、HTTPFlow 和其他已有 `ExecResult`/Yakit Feature。

示例：

```yak
handleOneHTTPFlow = func(ctx, flow) {
    table = yakit.NewTable("字段", "值")
    table.Append("URL", flow.Url)
    table.Append("HTTPS", ctx.HttpsState())
    table.Append("状态码", flow.StatusCode)
    yakit.Output(table)
}
```

同一插件无论最终在 dialog、drawer 还是 tab 中展示，代码保持一致。

### 10.3 流式执行

执行接口返回流式结果。首次元事件包含：

```text
RuntimeID
Status=started
PluginName
ResultMode
```

后续事件沿用现有 `ExecResult`。完成事件应明确区分：

- completed
- failed
- cancelled
- timeout

关闭结果容器是否取消任务由容器策略决定，但用户显式点击停止必须取消底层 context。

## 11. 显式动作结果

### 11.1 展示输出与业务动作分离

`yakit.Output` 负责展示数据；Hook 的结构化返回值负责请求前端执行受控业务动作。

普通 `string`、`[]byte`、Table 或日志输出不能被解释为“替换当前数据包”。否则插件原本只想展示 bytes 时可能意外覆盖用户编辑内容。

### 11.2 PacketActionResult

HTTP 数据包变形使用明确的结构化结果，例如：

```yak
handleHTTPPacket = func(ctx, request, response) {
    newRequest = normalizeRequest(request)

    return context.NewPacketResult(
        context.ReplaceRequest(newRequest),
    )
}
```

概念结构：

```text
PacketActionResult
- Request: bytes
- Response: bytes
- ReplaceRequest: bool
- ReplaceResponse: bool
- RequireConfirmation: bool
```

约束：

- 只有 `http-packet` Scene 接受 PacketActionResult。
- 前端应用修改前验证当前编辑器版本/数据摘要，避免长任务结束后覆盖用户的新编辑内容。
- 默认提供确认或可撤销机制；核心受信动作可根据产品策略减少确认。
- 返回空结果等价于没有数据包变更。
- 展示输出和 PacketActionResult 可以在一次执行中同时存在。

History Flow 的数据库修改不通过“修改传入 flow 指针后自动保存”实现。需要持久化修改时必须使用明确的 Yak API，避免隐式副作用。

## 12. 执行模型

### 12.1 按需执行

右键插件数量可能很大，因此不使用“安装即自动加载”的模型：

```text
打开菜单
  -> 读取本机已启用 Binding
  -> 按当前 Scene 和 SSA Hook 定义过滤
  -> 渲染菜单，不执行插件
  -> 用户点击或按快捷键
  -> 校验 Binding、Scene、参数及输入限制
  -> 创建 ActionContext
  -> 按需创建执行实例或使用安全的编译缓存
  -> 调用固定 Hook
  -> 流式输出
  -> Hook 返回/取消/超时后回收运行实例
```

允许缓存解析或编译产物，但默认不复用带可变全局状态的运行实例。除非后续能够证明隔离、并发和清理语义可靠，否则每次动作执行使用独立运行环境。

### 12.2 并发与取消

- 每次执行有独立 RuntimeID 和 context。
- 同一插件可否并发执行由执行器的通用限制控制，v1 不允许共享可变 Engine 状态。
- 服务端设置默认 Deadline，并允许核心动作按受控配置调整。
- 前端断流、用户停止和服务端超时都必须进入同一取消链。
- 插件自行启动的 goroutine/异步任务必须绑定本次 ctx；Hook 返回后遗留任务不受支持。

### 12.3 输入不可隐式持久化

- HTTPFlow 参数在语义上是本次调用的输入快照。
- 修改 `flow` 字段不自动保存数据库。
- Packet bytes 在语义上是输入快照。
- 修改本地 byte slice 不自动写回编辑器。
- 所有外部变更通过显式 API 或 ActionResult 完成。

## 13. History 单选与多选

### 13.1 数据传递

前端传递选中的 Flow ID 和项目标识，后端从当前项目数据库加载完整 HTTPFlow。前端不重复上传完整 request/response 原文。

服务端必须：

- 校验 Flow 属于当前可访问项目。
- 保持前端选中顺序，或在协议中明确按 ID 排序；v1 建议保持选中顺序。
- 对已删除或不存在的 Flow 返回明确错误，不静默跳过。
- 为多选设置数量和总数据量上限。

### 13.2 批量上限

v1 仅支持明确选中的有限 Flow ID。对“选择当前查询全部结果”不一次性展开无界 `[]*HTTPFlow`。

当前后端上限为一次 200 条 HTTPFlow，且选中 Flow 的 request/response 数据合计不超过 32 MiB。超限或某条 Flow 不存在时直接返回明确错误，不静默截断。

后续如需支持大规模处理，增加批量选择描述和服务端迭代器，而不是无限提高 v1 数组上限。

## 14. HTTP Packet Scene

HTTP Packet 输入允许以下组合：

| Request | Response | 是否允许 |
|---|---|---|
| 有 | 有 | 是 |
| 有 | 无 | 是 |
| 无 | 有 | 是 |
| 无 | 无 | 否 |

单个 request 或 response 的首版上限为 16 MiB。

请求需携带：

- request bytes 与存在性。
- response bytes 与存在性。
- HttpsState（`unknown/http/https/mixed` 字符串）。
- Source。
- 当前编辑内容的版本号或摘要，用于安全应用 PacketActionResult。

旧 `ExecutePacketYakScriptParams.IsHttps bool` 无法表达 unknown，新的 Context Action RPC 不以它作为唯一协议状态。

## 15. 右键插件元数据

插件定义和用户本机管理配置必须分开。

### 15.1 插件定义

v1 直接复用 `YakScript`：

- `Uuid` 是 Binding 使用的稳定插件身份；保存新的右键插件时若为空，由后端生成。
- `Content` 中实际定义的标准 Hook 决定 capabilities。
- `Params` 继续描述参数表单。
- `IsCorePlugin` 与可管理的右键能力共同标识核心动作：一等 `context-menu` 插件，或带旧右键标签的兼容 CODEC。

不新增 APIVersion、Icon、Group、默认结果容器或独立 capabilities 表；它们都不是首版三类动作执行的必要条件。

### 15.2 用户 Binding

概念字段：

```text
ContextMenuBinding
- PluginUUID
- ActionID
- Enabled
- Locked
- Sort
- Shortcut
- ResultModeOverride
- AskBeforeRun
```

它存储在 Profile DB，描述这台电脑上的用户偏好。

Binding 使用稳定 Plugin UUID 和 ActionID，不使用可变的 ScriptName 或本地化标题作为主关联键。插件改名后，绑定、排序和快捷键保持有效。

`MenuItem` 目前服务于通用菜单/导航语义，且缺少右键插件的约束字段和稳定动作身份。v1 建议新增专用 Binding 模型，而不是继续向 `MenuItem` 叠加核心锁定、数量限制和快捷键等职责。

## 16. ActionID

v1 一个标准 Hook 对应一个稳定 ActionID：

```text
history-single
history-multi
http-packet
```

完整动作身份为：

```text
PluginUUID + ActionID
```

不以菜单标题作为动作身份。一个插件实现多个 Hook 时，每个场景可以独立绑定快捷键和结果容器覆盖；但自定义启用额度按 Plugin UUID 计数，而不是按 ActionID 计数。

如果未来允许同一插件在同一 Scene 贡献多个菜单动作，需要在新 API 版本中引入插件声明的稳定 ActionID；v1 不提前开放这一复杂度。

## 17. 右键管理

### 17.1 管理能力

管理界面交互参考“快捷键管理”，至少提供：

- 搜索已安装的右键插件。
- 从已配置和待添加列表按 Plugin UUID 直接打开对应插件编辑器；保存后刷新管理列表和右键菜单。
- 查看支持的 Scene。
- 启用/停用自定义插件。
- 查看核心锁定状态。
- 拖动排序。
- 为具体 Action 绑定快捷键。
- 覆盖 Result Mode。
- 配置是否执行前总是询问参数。
- 清除本机参数预设。

### 17.2 核心插件

引擎内置约 5 个核心右键插件。v1 对核心身份采用现有字段和可用右键动作的最小组合：

```text
(YakScript.Type == "context-menu" || 具有旧右键能力的 YakScript.Type == "codec")
&& YakScript.IsCorePlugin == true
```

其行为是：

- 不可停用。
- 不可从右键 Binding 中移除。
- 不要求必须存在 Binding；查询和执行时后端直接视为已启用。
- 可以按产品策略允许用户调整排序、快捷键和 Result Mode。
- 核心身份只从服务端数据库中的插件类型与核心字段共同判断，不接受客户端在 Binding 中自行声明核心身份。

v1 不增加第二套 Builtin Registry。`IsCorePlugin` 单独看确实还包含其他插件类型，但与“新右键插件类型或旧 CODEC 右键能力”组合后已经能准确表达当前约束；额外注册中心只会增加同步和恢复分支。

所有删除、批量替换、导入和重置接口都必须在后端执行核心保护。前端隐藏删除按钮不足以保证约束。

### 17.3 15 个自定义插件上限

v1 规则：

```text
内置核心右键插件：不占自定义额度，始终展示
History 单选：最多启用 15 个自定义右键插件或兼容 CODEC
History 多选：最多启用 15 个自定义右键插件或兼容 CODEC
HTTP 数据包：最多启用 15 个自定义右键插件或兼容 CODEC
已安装但未启用插件：数量不限
```

计数规则：

- 在每个 Scene 内按不同 Plugin UUID 去重计数。
- 一个插件实现多个 Scene 时，在每个已启用的 Scene 中分别占一个名额；不同 Scene 的额度互不影响。
- 同一插件在同一 Scene 提供多个兼容 Action 时只占一个名额。
- 只有 `Enabled == true` 且非 core 的插件占额度。
- 新安装的普通右键插件默认不自动启用。
- 当前 Scene 达到 15 个后，在该 Scene 启用第 16 个时返回明确错误，并提示先停用该场景的一个插件；其他 Scene 仍可继续配置。

上限必须在 Profile DB 事务中由后端原子校验。不能只由前端控制开关，否则多个窗口、旧客户端或直接 RPC 可能绕过限制。

### 17.4 菜单展示

菜单仅展示同时满足以下条件的动作：

1. 插件是 core，或对应 Binding 已启用。
2. 插件 capabilities 包含当前 Scene。
3. 当前上下文满足 Hook 的基本输入条件。
4. 插件本体存在且未损坏。

建议默认按 Group 和 Sort 排序。插件数量上限不代表每个菜单都展示全部插件；Scene 过滤会进一步减少实际菜单项。

## 18. 快捷键

快捷键绑定对象为：

```text
PluginUUID + ActionID
```

而不是仅绑定 Plugin UUID。原因是同一个插件可能同时支持 History 单选、多选和 HTTP Packet，快捷键必须知道要触发哪个动作。

快捷键触发流程：

1. 从当前焦点页面构造标准 Scene 和输入描述。
2. 查询匹配 `PluginUUID + ActionID` 的启用 Binding。
3. 验证 ActionID 是否适用于当前 Scene。
4. 执行与右键点击相同的参数检查。
5. 调用同一个 `ExecuteContextMenuAction` 接口，仅 Trigger 不同。

当前页面不支持该动作时给出轻量提示，不自动改调插件的另一个 Hook。

快捷键冲突检测可以复用现有快捷键管理组件，但持久化对象仍是 Context Action Binding。

## 19. RPC 边界建议

### 19.1 查询可用动作

```text
QueryContextMenuActions(ContextMenuQuery) -> ContextMenuActionList
```

查询条件包含 Scene 和是否返回未启用项；返回已经结合 Hook 定义、Binding、核心标记和排序的菜单元数据，不执行插件。Source 属于实际调用上下文，不影响同一 Scene 的菜单发现，因此不进入查询 RPC。

### 19.2 执行动作

```text
ExecuteContextMenuAction(ExecuteContextMenuActionRequest)
    -> stream ContextMenuActionEvent
```

概念请求：

```text
ExecuteContextMenuActionRequest
- PluginUUID
- ActionID
- Source
- Trigger
- HttpsState
- FlowIDs
- Request
- HasRequest
- Response
- HasResponse
- PacketRevision
- Params
```

服务端执行以下校验：

- PluginUUID 存在且类型为 `context-menu`。
- Binding 已启用，或动作是 core。
- ActionID 是三个固定值之一。
- 插件 capabilities 包含对应 Hook。
- 输入数据存在且大小在限制内。
- HttpsState 不认识时按 `unknown` 处理。
- 参数合法。
- Result Mode 在允许枚举内。

### 19.3 管理 Binding

v1 使用单条幂等写入：

```text
QueryContextMenuActions(Scene, IncludeDisabled)
SetContextMenuActionBinding(PluginUUID, ActionID, Enabled, Sort,
                            Shortcut, ResultMode, AskBeforeRun)
```

Profile DB 事务和进程内写锁负责首版并发保护。服务端在写入时统一执行：

- 核心动作不可停用。
- 当前 Scene 的 15 个自定义启用上限。
- Result Mode 校验。
- 插件存在性检查。

v1 不增加整体配置版本、Reset RPC 或新的快捷键语法系统。快捷键字符串沿用前端现有管理组件；出现真实的跨窗口覆盖问题后，再基于证据增加版本控制。

## 20. 安全与资源限制

- 菜单查询不执行插件代码。
- 执行 RPC 不接受任意函数名。
- 服务端根据 PluginUUID 查找插件，不信任客户端上传脚本类型。
- Flow ID 必须属于当前授权项目。
- PacketActionResult 应基于 PacketRevision/摘要防止覆盖新编辑内容。
- 每次执行设置 Deadline、最大输出量和输入大小限制。
- 多选设置 Flow 数量与累计数据量限制。
- 前端断流后取消执行，避免后台遗留任务。
- 参数预设中的敏感字段默认不落盘。
- 插件输出进行现有 UI 渲染层的安全处理，不允许借 Result Mode 绕过输出协议。

## 21. 兼容与迁移

### 21.1 CODEC 保持不变

以下行为必须保持：

- `YakScript.Type == "codec"`。
- `handle(input)` 函数签名。
- CODEC 页面执行。
- FuzzTag/Codec caller 调用。
- 现有输入输出语义。

不能通过把旧插件记录的 Type 从 `codec` 改为 `context-menu` 完成迁移，因为现有 CODEC 调用方会校验类型，直接修改会破坏原能力。

### 21.2 旧右键配置

迁移分阶段进行：

#### 阶段 1：兼容

- 旧 CODEC 的四个能力标签继续保留，插件类型和 `handle(input)` 执行语义不变。
- Legacy Adapter 将能力标签映射为可管理的 ActionID 和 Scene，但执行时仍调用原 CODEC 链路。
- 旧插件不再绕过管理配置自动全部加载；未配置的旧插件出现在“可添加插件”列表中。
- 已写入的本机 Binding 继续按 Plugin UUID 与 ActionID 生效。
- 管理界面标记“CODEC 兼容”，且不开放新 Result Mode 覆盖。
- 新创建的 CODEC 不再开放旧右键专用配置。

v1 使用固定映射，不引入动态兼容 DSL：

| 旧 CODEC 标签 | 兼容 ActionID | Scene | 执行方式 |
|---|---|---|---|
| `allow-custom-single-history-mutate` | `legacy-codec-history-single` | `history-single` | 原 CODEC History 执行入口 |
| `allow-custom-multiple-history-mutate` | `legacy-codec-history-multi` | `history-multi` | 原 CODEC History 执行入口 |
| `allow-custom-context-menu-execute` | `legacy-codec-http-packet-context` | `http-packet` | 原 CODEC 数据包右键入口 |
| `allow-custom-http-packet-mutate` | `legacy-codec-http-packet-mutate` | `http-packet` | 原 CODEC 数据包变形入口 |

#### 可选后续：复制迁移（不属于 v1）

- 只有在用户确实需要修改旧实现时，再评估提供“复制为右键插件”。
- 原插件同时承担 CODEC 与右键功能时，保留原 CODEC，创建新的 `context-menu` 插件。
- 原插件只承担右键功能时，也由用户明确确认后迁移，不静默改 Type。

#### 长期收敛

- 插件商店的新版本使用 `context-menu` 类型发布。
- 旧执行入口保留兼容周期后再评估废弃。

### 21.3 packet-hack

旧 `packet-hack` 路径可以作为内部兼容入口继续存在，但新的右键插件不通过该模板执行。新执行链统一使用：

```yak
handleHTTPPacket(ctx, request, response)
```

从而明确：

- 参数顺序。
- request/response 的存在性。
- HTTPS unknown/http/https/mixed 状态。
- context 取消与 Deadline。
- 结构化输出和 PacketActionResult。

## 22. 插件示例

### 22.1 History 单选

```yak
handleOneHTTPFlow = func(ctx, flow) {
    table = yakit.NewTable("字段", "值")
    table.Append("URL", flow.Url)
    table.Append("Method", flow.Method)
    table.Append("Status", flow.StatusCode)
    table.Append("HTTPS", ctx.HttpsState())
    table.Append("RuntimeID", ctx.RuntimeID())
    yakit.Output(table)
}
```

### 22.2 History 单选和多选复用逻辑

```yak
outputFlow = func(ctx, flow, table) {
    if ctx.Err() != nil {
        return
    }
    table.Append(flow.Method, flow.Url, flow.StatusCode)
}

handleOneHTTPFlow = func(ctx, flow) {
    table = yakit.NewTable("Method", "URL", "Status")
    outputFlow(ctx, flow, table)
    yakit.Output(table)
}

handleMultiHTTPFlows = func(ctx, flows) {
    table = yakit.NewTable("Method", "URL", "Status")
    for flow in flows {
        if ctx.Err() != nil {
            yakit.Warn("任务已取消")
            return
        }
        outputFlow(ctx, flow, table)
    }
    yakit.Output(table)
}
```

### 22.3 带参数的数据包分析

```yak
handleHTTPPacket = func(ctx, request, response) {
    keyword = ctx.ParamString("keyword")

    if !ctx.HasRequest() {
        yakit.Error("当前没有请求数据")
        return
    }

    table = yakit.NewTable("项目", "结果")
    table.Append("来源", ctx.Source())
    table.Append("HTTPS", ctx.HttpsState())
    table.Append("请求包含关键字", str.Contains(string(request), keyword))

    if ctx.HasResponse() {
        table.Append("响应包含关键字", str.Contains(string(response), keyword))
    }

    yakit.Output(table)
}
```

### 22.4 显式修改请求包

```yak
handleHTTPPacket = func(ctx, request, response) {
    if !ctx.HasRequest() {
        return
    }

    newRequest = poc.ReplaceHTTPPacketHeader(request, "X-From-Plugin", "true")
    return context.NewPacketResult(
        context.ReplaceRequest(newRequest),
        context.RequireConfirmation(true),
    )
}
```

`context.NewPacketResult`、`context.ReplaceRequest`、`context.ReplaceResponse` 和 `context.RequireConfirmation` 已作为 v1 API 落地。

## 23. 可观测性

每次执行至少记录：

- RuntimeID。
- PluginUUID、PluginName、ActionID。
- Scene、Source、Trigger。
- 开始时间、结束时间、耗时。
- success/failed/cancelled/timeout。
- 输入数量与大小摘要，不记录敏感原始报文。
- 最终 Result Mode。
- 是否产生 PacketActionResult，以及前端是否成功应用。

错误消息应区分：

- 插件不存在或类型错误。
- Binding 未启用。
- Scene/Action 不匹配。
- Hook 未实现。
- 参数校验失败。
- 输入过大。
- 插件编译失败。
- Hook 执行失败。
- 用户取消。
- Deadline exceeded。
- 数据包版本冲突。

## 24. 落地阶段与当前状态

当前状态：后端 v1 最小闭环、前端插件编辑入口、独立右键管理 TAB、History/HTTP Packet 统一入口以及三类结果容器均已在本地功能分支落地，当前进入联调与回归阶段。

### Phase 0：契约冻结（已完成）

- 确认类型名 `context-menu`。
- 冻结三个 v1 Hook 签名。
- 冻结 ActionContext v1 方法及 HTTPS 四态语义。
- 冻结 ActionID 与 Scene 映射。
- 确认每个 Scene 的 15 个上限不包含 core，且不同 Scene 额度互不影响。
- 确认 Result Mode 枚举。

### Phase 1：插件类型和静态分析（已完成）

- 增加 schema/plugin type 常量。
- 增加静态分析规则和 SSA 外部类型。
- 增加编辑器补全、模板和 smoking evaluate 支持。
- 使用 SSA 定义结果发现 capabilities，不新增持久化元数据。

### Phase 2：后端执行链（已完成最小闭环）

- 实现 ActionContext。
- 实现 History 单选、多选和 HTTP Packet 输入构造。
- 实现场景到 Hook 的白名单分发。
- 复用现有参数和流式 ExecResult。
- 接入取消、Deadline、输入/输出限制。
- 实现 PacketActionResult。

### Phase 3：管理与前端容器（已完成）

- 新增 ContextMenuBinding Profile 表及 RPC。
- 使用 `Type + IsCorePlugin` 实现 core 不可停用、不可删除校验。
- 实现按 Scene 分别计算的自定义启用上限 15 的事务校验。
- 实现按 Scene 管理、搜索、添加/移除、排序、快捷键和 Result Mode 覆盖。
- 从管理列表一键进入插件编辑器，并在切换编辑目标前保护未保存内容。
- 使用同一结果渲染器承载 dialog/drawer/tab。
- “管理右键插件”打开或聚焦独立单例 TAB，并定位到当前 Scene。

### Phase 4：兼容收口（v1 已完成）

- 接入旧 CODEC 右键配置 Legacy Adapter。
- 新 CODEC 编辑器屏蔽旧右键开关。
- 保留并标记旧 `packet-hack` 执行路径。
- “复制为右键插件”不是 v1 必需能力；出现明确迁移需求后再单独设计，避免在首版引入转换分支。

## 25. 测试计划

### 25.1 CODEC 回归

- `handle(input)` 行为不变。
- CODEC 页面结果不变。
- FuzzTag/Codec caller 行为不变。
- 旧 CODEC 数据不被自动改 Type。

### 25.2 Hook 与静态分析

- 三个 Hook 分别能够被发现。
- 一个 Hook 都未实现时报错。
- 重复 Hook、参数数量错误时报错。
- 单选不会隐式调用多选，反之亦然。
- 服务端拒绝任意函数名调用。

### 25.3 ActionContext

- Scene、Source、Trigger 正确。
- RuntimeID 在完整输出流中一致。
- 参数默认值与 `cli.*` 一致。
- Params 不可修改内部状态。
- 取消、Deadline 能够终止执行及绑定的网络调用。
- Hook 返回后没有遗留执行实例。

### 25.4 HTTPS 状态

- unknown/http/https/mixed 四态正确。
- `HasHttpsInfo`、`IsHttps`、`ContainsHttps` 真值表正确。
- 无 HTTPS 信息时不会被误判为 HTTP。
- History 多选 HTTP+HTTPS 聚合为 mixed。

### 25.5 输入场景

- History 单选正确加载 Flow。
- History 多选保持约定顺序。
- Flow 不存在时错误明确。
- 多选数量或总数据量超过限制时拒绝执行。
- request-only、response-only 和 request+response 均正确。
- request/response 都不存在时拒绝执行。

### 25.6 输出和数据包修改

- Text、Table、Fixed Table、Graph、Risk 等现有输出可在三类容器渲染。
- 用户 Result Mode 覆盖优先于插件默认。
- 普通 `[]byte` 返回不会修改数据包。
- PacketActionResult 可以替换 request、response 或二者。
- PacketRevision 不匹配时拒绝覆盖新编辑内容。
- 用户取消应用结果时不修改编辑器。

### 25.7 管理约束

- core 无法通过 Binding 写入停用，也无法通过现有插件删除接口移除。
- 每个 Scene 分别可以启用 15 个自定义插件。
- 当前 Scene 的第 16 个启用请求被原子拒绝，不影响其他 Scene。
- 同一插件在同一 Scene 的多个 Action 只占一个额度。
- 插件改名后 Binding 仍通过 UUID 生效。
- 新安装自定义插件默认不启用。

### 25.8 快捷键

- 快捷键和右键进入同一 Execute RPC。
- Trigger 值不同，其余 ActionContext 语义一致。
- 当前 Scene 不匹配时不调用其他 Hook。
- 快捷键冲突被管理层发现。
- 快捷键触发仍执行必填参数交互。

## 26. 风险与对策

### 风险 1：把 `ctx` 变成无边界的大对象

对策：`ctx` 主要提供调用环境的只读查询和生命周期能力；UI 控制、Binding 管理和数据包修改使用独立协议。

### 风险 2：只提供 `ctx` 参数但取消没有贯穿执行链

对策：将 gRPC stream、Yak Engine、网络 API 和插件子任务绑定到同一个底层 Go context，并以集成测试验证取消。

### 风险 3：右键插件过多导致启动或菜单卡顿

对策：菜单阶段只对核心和已启用插件做缓存 SSA 定义检查；每个 Scene 最多启用 15 个自定义插件；点击后按需执行。

### 风险 4：结果展示逻辑分裂成三套

对策：dialog/drawer/tab 仅作为相同结果渲染器的容器，全部消费同一 ExecResult/Yakit Feature 流。

### 风险 5：插件修改数据包造成意外覆盖

对策：普通返回值不产生修改；只接受 PacketActionResult；应用前校验 PacketRevision，并支持确认或撤销。

### 风险 6：核心插件保护只在前端实现

对策：后端只在新右键插件或具有旧右键能力的 CODEC 上接受 Binding，并结合 `IsCorePlugin == true` 判断核心身份，在 Binding 与插件删除入口校验。

### 风险 7：兼容迁移破坏 CODEC

对策：不修改旧 CODEC Type；使用 Legacy Adapter；迁移采用复制而不是原地转换作为默认行为。

## 27. 已冻结决策与前端容器策略

后端 v1 已冻结：

1. Yak 方法使用现有项目命名习惯：`HttpsState()`、`HasHttpsInfo()`、`IsHttps()`、`ContainsHttps()`。
2. 数据包结果 API 为 `context.NewPacketResult`，选项为 `ReplaceRequest`、`ReplaceResponse`、`RequireConfirmation`。
3. 未设置 Result Mode 时返回 `auto`，具体系统默认容器由前端统一决定。
4. History 多选最多 200 条、Flow 数据合计 32 MiB；单个 Packet request/response 各 16 MiB。
5. core 可以保存排序、快捷键、Result Mode 和 AskBeforeRun，但不能停用。
6. v1 不新增插件默认元数据或 capabilities 表。

前端 v1 将 `auto` 解析为 dialog；用户仍可在管理页显式选择 dialog、drawer 或 tab。运行任务继续由统一 execution registry 和 gRPC 取消链管理，三个容器不复制执行状态。

无论上述细节如何选择，以下契约不再变化：

- CODEC 行为保持不变。
- 新类型为独立右键插件。
- 三个 v1 Hook 均以 `ctx` 为第一个参数。
- `ctx` 是包装真实 Go context 的 ActionContext，不再额外传 config。
- HTTPS 使用可表达 unknown 和 mixed 的状态模型。
- 右键和快捷键复用同一执行链。
- 核心插件保护和每个 Scene 分别 15 个的自定义启用上限由服务端保证。
