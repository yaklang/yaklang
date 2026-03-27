# HotPatch Pipeline Phase 模型设计草案
状态：Draft  
范围：`common/yak` / `common/yakgrpc` / WebFuzzer / MITM  
目标：为 Global HotPatch 与 Module HotPatch 提供一套优雅、简单、可维护的生命周期模型，避免后续持续追加 `beforeXxx/afterXxx/finalizeXxx`

## 1. 问题定义
当前全局热加载已经支持基础 pipeline：
```text
global.beforeRequest -> module.beforeRequest -> send -> global.afterRequest -> module.afterRequest
```
这套模型足够支撑“Global 先做一层通用处理，Module 再做任务级处理”的第一阶段需求，但已经暴露出结构性问题：
- 有些需求发生在“对象进入链路时”
- 有些需求发生在“对象即将离开链路时”
- 有些需求发生在“在线链路结束、准备落库时”
- 当前 `beforeRequest` / `afterRequest` 把这些位置全部压扁成了两个名字

这意味着：  
现在缺的不是再补一个 `finalizeRequest` 之类的新名字，而是缺一套真正稳定的生命周期边界模型。

这份文档要回答：
1. HotPatch 生命周期应该如何设计，才能支撑未来更多协议与业务场景？
2. 从当前后端代码出发，哪些能力已经具备，哪些地方还明显不支持？

## 1.1 当前分支实现快照
以下内容描述的是 `feature/go0p/hotpatch-phase-model` 这条分支上已经落地的部分，而不是未来设想：

已支持：
- HotPatch 脚本可以显式使用 phase hook：
  - `requestIngress`
  - `requestProcess`
  - `requestEgress`
  - `responseIngress`
  - `responseProcess`
  - `responseEgress`
  - `flowArchive`
- 同一份脚本中，如果同时定义 legacy hook 和 phase hook，加载时会显式报错，不允许静默混用
- MITM 全局热加载链路已经支持 phase runtime：
  - request phase 的 Global / Module 调度
  - response phase 的 Global / Module 调度
  - `flowArchive` 阶段接入
  - `SetClientResponse()` 在请求侧会被翻译成 MITM mock response
- WebFuzzer HTTP hook 链路已经支持 phase runtime：
  - request / response phase 顺序调度
  - `SetClientResponse()` 会桥接到 `mockHTTPRequest`

当前限制：
- WebFuzzer 发包路径已经使用 request-local phase ctx，`ctx.State` / `ctx.Meta` / `ctx.Tags` 会在同一条请求生命周期内贯穿 request / response phase
- WebFuzzer 侧的 `Drop()` 还没有等价控制通道，当前会显式打日志而不是静默模拟
- Global=phase / Module=legacy 这类跨脚本 mixed mode，在 WebFuzzer 发包路径已经改成显式报错；`MutateHookCallerChained` 兼容链路里仍保留 legacy fallback
- `flowArchive` 当前更适合通过 `ctx.Flow` 直接改 `HTTPFlow`；`Meta` 还没有统一持久化映射

## 2. 当前后端模型
### 2.1 已有能力
- MITM 侧已经有显式的 Global -> Module pipeline，见 `common/yakgrpc/mitm_global_hotpatch_pipeline.go`
- WebFuzzer 侧已经通过 `HotPatchChain` 串联 Global / Module，见 `common/yak/hotpatch_chain.go`
- 当前核心 request/response 修改点只有两个：`beforeRequest`、`afterRequest`

### 2.2 当前实现中写死的地方
以下事实说明，当前模型本质上仍然是“固定 hook 名字 + 固定调用点”：
- hook 名字在 `common/yak/hook_mixed_plugin_caller.go` 中是显式常量，`HOOK_BeforeRequest` / `HOOK_AfterRequest` 被写死进 `MITMAndPortScanHooks`
- WebFuzzer 解析脚本时，只显式提取了 `beforeRequest` 与 `afterRequest`，见 `common/yak/script_engine_for_fuzz.go`
- Global / Module 的链路拼装是按固定函数做的，见 `common/yak/hotpatch_chain.go`
- 参数适配目前也只给 `beforeRequest` / `afterRequest` 配了默认规则，见 `common/yak/yak_to_caller_manager.go`

这说明：当前已经有“串起来执行”的基础，但还没有“按边界调度”的 runtime。

## 3. 为什么不建议继续追加 `beforeXxx/afterXxx/finalizeXxx`
如果继续沿用“每出现一个新需求就补一个 hook 名字”的方式，短期看似快，长期一定膨胀。原因很直接：
- 每新增一个 hook 名字，都要改 hook 常量注册
- 每新增一个 hook 名字，都要改脚本解析
- 每新增一个 hook 名字，都要改 Global / Module 的链路拼装
- 每新增一个 hook 名字，都要考虑 MITM / WebFuzzer 两条链路都支持

更关键的是，很多未来需求并不是“新功能”，而只是“发生在另一个边界位置上的处理”。例如：
- 请求刚进入链路时，要读取共享状态或补运行时上下文
- 请求即将发出时，要做最后一层归一化、重编码、收口处理
- 响应刚回来时，要先把包装层拆掉再交给后续逻辑
- 响应即将回给客户端时，要决定外部看到什么
- 在线处理结束后，保存到 flow 的内容未必和在线返回完全一样

如果把这些都长成 `beforeXxx/afterXxx/finalizeXxx`，模型最终会出现两个问题：
- 名字越来越多，但彼此边界不清晰
- 用户必须先记“有哪些具体功能”，而不是先记“对象现在走到哪里了”

## 4. 设计原则
### 4.1 Phase 只描述边界，不描述功能
一个合格的 phase 名称，应该在不提任何具体协议能力的前提下仍然成立。

也就是说：
- phase 应该描述“对象正处在哪个生命周期边界”
- phase 不应该描述“这一阶段最常见会做什么功能”

反过来说，如果一个 phase 名称必须靠“签名 / 加解密 / challenge / csrf / 重放”才能解释清楚，那这个名字就是坏名字。

### 4.2 Global / Module 描述作用域，不描述功能类别
这也是前一版设计里最容易被误读的地方：
- `Global` 不是“协议层”
- `Module` 也不是“业务层”

更准确的理解是：
- `Global` 是复用范围更大的外层脚本
- `Module` 是和当前任务、当前插件、当前靶场更绑定的内层脚本

phase 是生命周期位置；Global / Module 是执行作用域。  
这两个维度应该正交，不能互相绑死。

### 4.3 行为用 Action 表达，不用 Phase 膨胀
很多看起来像“需要一个新 hook”的诉求，本质上不是新阶段，而是一个控制动作。例如：
- 重试
- 短路返回
- 丢弃请求
- 替换在线回包
- 替换落库存档

这些更适合做成 action，而不是继续增加 phase 数量。

### 4.4 `ctx` 是 phase 之间的共享总线
如果 phase 只是时间点，而没有统一 `ctx`，那模型还是会退化回“几个函数名 + 一堆零散参数”。

所以新模型里必须有统一 `ctx`，用来承载：
- 当前工作副本
- 原始报文
- 共享状态
- 元数据
- 标签
- 控制动作

## 5. 推荐的 Phase 模型
### 5.1 固定为 7 个边界 phase
```text
1. requestIngress
2. requestProcess
3. requestEgress
4. responseIngress
5. responseProcess
6. responseEgress
7. flowArchive
```

这 7 个名字只描述边界位置：
- `Ingress`：进入当前边界
- `Process`：在当前边界内进行主要处理
- `Egress`：离开当前边界
- `Archive`：离开在线链路，进入保存/沉淀边界

### 5.2 每个 phase 的语义
- `requestIngress`
  请求刚进入 HotPatch runtime，但还没有进入主要处理阶段。这里适合做读状态、补上下文、建立共享变量、做统一视图归一化。
- `requestProcess`
  请求处于主要处理阶段。这里允许做参数修改、路径改写、header/body 调整、策略判断、任务级变异。
- `requestEgress`
  请求即将离开 HotPatch runtime 并发往网络。这里适合做最后一层收口、规范化、重编码、外发前整理。
- `responseIngress`
  响应刚从网络返回并进入 HotPatch runtime。这里适合把外部返回转换成内部更易处理的视图。
- `responseProcess`
  响应处于主要处理阶段。这里适合做匹配、提取、判断、归因、标记、策略决策。
- `responseEgress`
  响应即将离开 HotPatch runtime 并流向客户端或下一个消费者。这里适合决定“外部最终看到什么”。
- `flowArchive`
  在线链路已经结束，数据进入保存、审计、结果沉淀边界。这里适合决定“记录什么”和“带着什么元信息记录”。

需要强调的是：
- 上面这些是边界定义，不是功能定义
- 签名、加密、解密、解压、拆 envelope、重封包，都只是这些边界上的常见用法示例
- 这些能力不应该反过来成为 phase 的名字

### 5.3 推荐执行顺序
如果引入 phase runtime，推荐顺序如下：
```text
requestIngress:  global -> module
requestProcess:  global -> module
requestEgress:   module -> global
send
responseIngress: global -> module
responseProcess: global -> module
responseEgress:  module -> global
flowArchive:     module -> global
```

这个顺序不是按“谁做协议、谁做业务”划分，而是按边界流向划分：
- `Ingress` 代表对象刚进入外层 runtime，先让外层看到，再让内层看到
- `Egress` 代表对象即将离开外层 runtime，先让内层收尾，再让外层收口
- `flowArchive` 放在最后，并采用 `module -> global`，这样内层先补分析结果，外层再决定最终存档策略

### 5.4 这套模型怎样覆盖“全局 -> 模块 -> 全局”的夹心结构
如果拿“Global 先做准备、Module 再改业务、Global 最后收口”这个需求来看，新模型里的表达应该是：
```text
global.requestIngress
module.requestProcess
global.requestEgress
```

如果拿“响应先做一层通用展开、Module 再分析、Global 最后决定对外呈现”这个需求来看，新模型里的表达应该是：
```text
global.responseIngress
module.responseProcess
global.responseEgress
```

这样就能保留 pipeline 的“夹心结构”，但 phase 名字本身并不再绑定某一种具体协议功能。

## 6. 为什么不建议脚本 API 直接暴露 `handle(ctx, next)` 作为主入口
`handle(ctx, next)` 方向本身没有错，但不适合直接作为用户主入口。主要问题有三个：
- 它会把已经明确的生命周期边界重新压扁成一个 middleware
- 用户最终还是要自己在一个函数里手工区分“这是进入时逻辑，还是离开时逻辑”
- Global / Module 的夹心顺序会重新变得隐式，不利于理解、调试和兼容

更合理的做法是：
- 内核 runtime 可以用 middleware 风格实现
- 用户级 API 仍然优先暴露“固定 phase + ctx + action”

如果未来真的要提供 `handle(ctx, next)`：
- 它更适合作为高级接口
- 前提是 phase runtime 已经稳定
- 它不应该取代 phase API 成为 v1 的主模型

## 7. 推荐的脚本 API 形态
建议把新脚本 API 显式声明为 phase runtime：
```yak
hotpatch = {
  apiVersion: 2,
  runtime: "phase",
}

requestIngress = func(ctx) {}
requestProcess = func(ctx) {}
requestEgress = func(ctx) {}
responseIngress = func(ctx) {}
responseProcess = func(ctx) {}
responseEgress = func(ctx) {}
flowArchive = func(ctx) {}
```

### 7.1 `ctx` 的最小建议字段
`ctx` 至少建议包含：
- `ctx.Request`
- `ctx.Response`
- `ctx.OriginRequest`
- `ctx.OriginResponse`
- `ctx.IsHttps`
- `ctx.URL`
- `ctx.Method`
- `ctx.Path`
- `ctx.Source`
- `ctx.State`
- `ctx.Meta`
- `ctx.Tags`

这里建议把 `ctx.State` 作为 phase 间共享状态总线，把 `ctx.Meta` / `ctx.Tags` 作为结果补充层，而不是继续让脚本靠零散全局变量隐式传值。

### 7.2 `ctx` 的设计目标
`ctx` 不是单纯为了“把参数包起来”，而是为了提供统一语义：
- 当前阶段正在处理的工作副本放在 `ctx.Request` / `ctx.Response`
- 原始进入边界时的报文放在 `ctx.OriginRequest` / `ctx.OriginResponse`
- phase 间共享信息放在 `ctx.State`
- 分析结论、标注、解释信息放在 `ctx.Meta` / `ctx.Tags`

换句话说：
- phase 定义“何时执行”
- `ctx` 定义“操作什么”
- action 定义“控制什么”

## 8. Phase 之外，还应该有 Action
不是所有未来需求都应该长成一个新 phase。很多需求本质上只是控制动作。

建议统一抽象成 action，例如：
- `ctx.Retry()`
- `ctx.Drop()`
- `ctx.Stop()`
- `ctx.SetState(key, value)`
- `ctx.SetMeta(key, value)`
- `ctx.SetTag(key, value)`
- `ctx.SetClientResponse(raw)`
- `ctx.SetArchiveResponse(raw)`
- `ctx.SkipArchive()`

这类 action 解决的是“链路怎么走”的问题，而不是“当前对象处在哪个边界”的问题。

例如：
- 某个请求命中条件后直接返回本地结果，这不是新 phase，而是 `SetClientResponse`
- 某个响应命中失效态后要重试，这不是新 phase，而是 `Retry`
- 某些流量不应进入数据库，这不是新 phase，而是 `SkipArchive`

### 8.1 从使用者视角看，如何替代旧 `hijackXXXX`
对使用者来说，新模型最重要的变化不是“少了几个 hook 名字”，而是：
- 以前是“实现一个特定 hook，再调用它暴露的 `forward/drop/mock`”
- 现在是“在某个 phase 里操作 `ctx`，再调用统一 action”

如果只看用户体感，可以把下面这条映射先记住：
- 旧 `hijackHTTPRequest`
  常见写法约等于：在 `requestEgress(ctx)` 里改 `ctx.Request`，或调用 `ctx.Drop()` / `ctx.SetClientResponse()`
- 旧 `mockHTTPRequest`
  常见写法约等于：在请求侧 phase 中直接 `ctx.SetClientResponse()`，然后 `ctx.Stop()`
- 旧 `hijackHTTPResponse`
  常见写法约等于：在 `responseEgress(ctx)` 里改 `ctx.Response`，或直接覆盖客户端看到的内容
- 旧 `hijackSaveHTTPFlow`
  常见写法约等于：在 `flowArchive(ctx)` 里修改归档视图、补标签，或 `ctx.SkipArchive()`

示例 1：最接近旧 `hijackHTTPRequest` 的写法
```yak
requestEgress = func(ctx) {
  if !str.Contains(ctx.URL, "/api/login") {
    return
  }

  ctx.Request = poc.ReplaceHTTPHeader(ctx.Request, "X-Test", "1")
  ctx.Request = poc.ReplaceBody(ctx.Request, `{"username":"admin","password":"123456"}`)

  // 直接 return，表示继续发送
  return
}
```

示例 2：命中条件后直接丢弃请求
```yak
requestEgress = func(ctx) {
  if str.Contains(ctx.URL, "/api/admin/delete") {
    ctx.Drop("blocked by hotpatch")
    return
  }
}
```

示例 3：不发到服务器，直接返回本地响应
```yak
requestEgress = func(ctx) {
  if !str.Contains(ctx.URL, "/api/user/profile") {
    return
  }

  ctx.SetClientResponse(`HTTP/1.1 200 OK
Content-Type: application/json

{"code":0,"data":{"name":"mock-user","role":"admin"}}`)
  ctx.Stop()
}
```

示例 4：最接近旧 `hijackHTTPResponse` 的写法
```yak
responseEgress = func(ctx) {
  if !str.Contains(ctx.URL, "/api/order/search") {
    return
  }

  ctx.Response = poc.ReplaceBody(ctx.Response, `{"code":0,"items":[]}`)
  return
}
```

示例 5：最接近旧 `hijackSaveHTTPFlow` 的写法
```yak
flowArchive = func(ctx) {
  if !str.Contains(ctx.URL, "/api/order/search") {
    return
  }

  ctx.SetTag("teaching-demo", "pipeline")
  ctx.SetMeta("archive-note", "rewritten by phase runtime")

  if str.Contains(string(ctx.Response), "heartbeat") {
    ctx.SkipArchive()
    return
  }
}
```

所以从用户体感上，可以把旧模型和新模型近似理解成：
```text
hijackHTTPRequest(req, forward, drop)
==
requestEgress(ctx.Request) + ctx.Drop() + ctx.SetClientResponse()
```

更准确地说：
- phase 负责决定“用户应该把逻辑写在什么时候”
- action 负责决定“用户在这个时点可以让 runtime 做什么”
- 旧 `hijackXXXX` 则是把这两层揉在一起暴露出来

## 9. 未来最可能出现的需求评估
### 9.1 更适合建模为 Phase 的需求
以下需求更适合建模为 phase，因为它们对应的是稳定边界，而不是某个具体动作：
- 请求刚进入运行时，需要先建立内部工作视图
- 请求即将离开运行时，需要做最后一层外发整理
- 响应刚回到运行时，需要先把外部表示转换成内部表示
- 响应即将离开运行时，需要决定对客户端暴露的最终表示
- 在线处理结束后，需要决定存档视图和附加元信息

这些边界是长期稳定的，不会因为具体协议从“签名”换成“压缩”“分片”“掩码”“封装”就改变。

### 9.2 更适合建模为 Action 的需求
以下需求更适合作为 action，而不是 phase：
- 自动重试
- 丢弃请求
- 本地短路返回
- 切换客户端视图
- 切换归档视图
- 打标签、补注释、补状态
- 跳过存档

### 9.3 不应再出现的坏味道
如果后续需求再次长成下面这种命名趋势，基本可以视为模型正在退化：
- `finalizeRequest`
- `recoverResponse`
- `decryptResponse`
- `signRequest`
- `packForClient`
- `saveFlowEx`

因为这些名字描述的都是功能，而不是边界。  
一旦继续往这个方向走，HotPatch 很快又会回到“靠堆 hook 名字解决问题”的状态。

## 10. 当前后端代码还不支持的点
### 10.1 还没有 phase-aware runtime
当前正式支持的核心阶段仍然只有 `beforeRequest` / `afterRequest`：
- hook 常量固定写在 `common/yak/hook_mixed_plugin_caller.go`
- Fuzzer 解析只提取 `beforeRequest` 与 `afterRequest`，见 `common/yak/script_engine_for_fuzz.go`

这意味着当前 runtime 还无法显式区分：
- `requestIngress`
- `requestProcess`
- `requestEgress`
- `responseIngress`
- `responseProcess`
- `responseEgress`
- `flowArchive`

### 10.2 还没有“按 phase 决定调用方向”的调度器
当前 `HotPatchChain` 的 request/response 串联本质上都是固定顺序：
```text
global.beforeRequest -> module.beforeRequest
global.afterRequest -> module.afterRequest
```

但新的 phase 模型要求支持：
- `Ingress` 多数时候 `global -> module`
- `Egress` 多数时候 `module -> global`
- `flowArchive` 需要有独立于在线 response 的末端边界

这说明当前缺的是 phase 调度器，而不是简单的“再链一个函数”。

### 10.3 还没有统一 `ctx`
当前 hook 输入输出仍然是“原始字节 + 少量位置参数”：
- `beforeRequest(isHttps, originReq, req)`
- `afterRequest(isHttps, originReq, req, originRsp, rsp)`

这对简单改包够用，但不利于承载：
- phase 间共享状态
- 元数据传递
- 统一 action
- 在线视图与归档视图分离

### 10.4 还没有统一 Action 总线
当前虽然已经有一些离散能力，例如：
- `retryHandler`
- `mockHTTPRequest`
- `hijackSaveHTTPFlow`

但它们还不是统一的 pipeline action，也没有绑定到统一 `ctx`。

从 phase runtime 的视角看，未来更需要的是：
- 统一的控制接口
- 清晰的生效边界
- 可以在不同 phase 中复用的动作模型

### 10.5 现有 Global / Module 关系仍然偏“固定 hook 串联器”
当前 `HotPatchChain` 做到的是：
- 能把 Global / Module 串起来
- 能保持基本执行顺序

但它还没有做到：
- 按 phase 组织执行
- 按边界方向决定顺序
- 在 phase 间共享统一 `ctx`
- 在 phase 上挂统一 action

所以它更像“固定 hook 串联器”，而不是“稳定 phase runtime”。

## 11. 兼容策略与落地顺序
### 11.1 兼容目标
现有大量用户脚本仍然使用旧 API，例如：
- `beforeRequest`
- `afterRequest`
- `retryHandler`
- `mockHTTPRequest`
- `hijackSaveHTTPFlow`

因此新模型不能做破坏性替换。兼容目标应该是：
- 已有脚本在不修改代码的情况下，继续按旧语义运行
- 新 phase API 作为显式 opt-in 能力引入
- 新旧脚本混用时，不允许做静默重解释

### 11.2 脚本方言
建议把脚本分成两种方言：

`Legacy API`
- 使用 `beforeRequest` / `afterRequest` 等旧 hook 名字
- 语义保持今天的行为不变

`Phase API`
- 使用 `requestIngress` / `requestProcess` / `requestEgress` / `responseIngress` / `responseProcess` / `responseEgress` / `flowArchive`
- 明确进入新的 phase runtime

建议识别规则：
- 如果脚本定义了任意新 phase 名字，则视为 `Phase API`
- 如果脚本只定义旧 hook，则视为 `Legacy API`
- 如果脚本同时定义新旧两套名字，且没有显式声明兼容模式，则加载时报错

### 11.3 调度规则
推荐调度矩阵如下：

`global=legacy, module=legacy`
- 继续走旧 runtime
- 顺序保持不变：
```text
global.beforeRequest -> module.beforeRequest -> send -> global.afterRequest -> module.afterRequest
```

`global=phase, module=phase`
- 走新 phase runtime

`global=legacy, module=phase`
- 默认不做静默映射
- 加载时报错，要求显式声明适配策略

`global=phase, module=legacy`
- 默认不做静默映射
- 加载时报错，要求显式声明适配策略

这样设计的原因是：
- 旧 `beforeRequest` / `afterRequest` 往往混合了承上启下的多种职责
- 运行时无法安全判断它到底属于哪个边界
- 如果做 silent remap，很容易把现有脚本悄悄搞坏

### 11.4 适配器策略
如果确实要让旧脚本参与 phase runtime，应该采用“显式适配器”，而不是隐式重解释。

建议支持两种方式：

`compatProfile`
- 使用官方预设

`compatMap`
- 用户显式声明旧 hook 应该落到哪个 phase

示例：
```yak
hotpatch = {
  apiVersion: 2,
  runtime: "phase-adapter",
  beforeRequestPhase: "requestProcess",
  afterRequestPhase: "responseProcess",
}
```

推荐只提供少量官方 profile：
- `module-process`
  - `beforeRequest -> requestProcess`
  - `afterRequest -> responseProcess`
- `global-request-egress`
  - `beforeRequest -> requestEgress`
- `global-response-ingress`
  - `afterRequest -> responseIngress`

注意：
- 不提供“自动把一个旧 `beforeRequest` 同时拆成 `requestIngress + requestEgress`”这种默认行为
- 不提供“自动把一个旧 `afterRequest` 同时拆成 `responseIngress + responseEgress`”这种默认行为
- 因为运行时无法判断旧脚本内部逻辑究竟落在哪个边界

### 11.5 常见迁移落点
下面这些只能作为迁移建议，不能作为运行时默认兼容规则：
- 对多数旧模块脚本而言：
  - `beforeRequest` 常见迁移到 `requestProcess`
  - `afterRequest` 常见迁移到 `responseProcess`
- 对多数旧全局脚本而言：
  - 作者通常需要显式决定它究竟属于 `requestIngress`、`requestEgress`、`responseIngress`、`responseEgress`，还是需要拆成多个 phase

这点非常关键，因为旧全局脚本经常同时承担多段职责，不能被运行时静默自动拆分。

### 11.6 建议的落地顺序
`Phase A`
- 冻结固定 phase 集合
- 冻结 `ctx` 最小字段集合
- 冻结 action 集合
- 冻结 phase 调度方向规则

`Phase B`
- 在 Go 内核层实现统一 phase runtime
- 同时保留 legacy runtime，不动旧语义

`Phase C`
- 增加脚本方言识别与加载期校验
- mixed mode 默认报错，不做 silent remap
- 增加显式 adapter 能力：`compatProfile` / `compatMap`

`Phase D`
- 逐步把现有离散能力收敛进统一 action，例如重试、短路、归档控制

`Phase E`
- 在 phase API 稳定之后，再评估是否额外暴露更底层的 `handle(ctx, next)`
- 不建议把它作为 v1 的用户主入口

## 12. 最终结论
- 不建议继续无上限追加 `beforeXxx/afterXxx/finalizeXxx`
- 推荐把 HotPatch 收敛成“固定 phase + ctx + action”的 pipeline 模型
- phase 的命名应该基于生命周期边界，而不是基于签名、加解密、解包、封包这类具体功能
- Global / Module 应该被定义为作用域，而不是被硬编码成“协议层 / 业务层”
- 当前后端代码已经具备分层执行基础，但还不具备完整 phase runtime、统一 `ctx`、统一 action

一句话概括：
> 现在最值得做的，不是围绕某个具体功能继续补 hook，而是把 HotPatch 从“固定 hook 集合”升级成“稳定的边界型 phase runtime”。
