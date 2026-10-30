# AI 网络诊断

网络记录默认关闭。它是 HTTP 层的本地字节镜像，不属于模型输出解析、SSE 事件解释或 Agent 的 action 校验。

## 启用

桌面入口：**设置 → AI 配置 → 自动化配置 → 网络请求 Debug**，与“无限重试”相邻。默认关闭；保存到当前引擎的 `AIGlobalConfig.NetworkDebug`，开启后作用于该引擎后续的主模型、辅助模型及重试 HTTP 请求，已运行的会话也在下一次模型调用读取该设置。不记录 Agent 工具本身的业务 HTTP 流量，不改变模型输出解析。关闭后停止全局采集；已经开始的请求完成当前记录。会话“导出日志”自动附带网络记录。

需要整个会话的主、辅助模型调用都记录时：gRPC `AIStartParams.NetworkDebug`（optional bool）或 Go `aicommon.WithNetworkDebug(true)` / Yak `aiagent.networkDebug(true)`。该选项只在模型 callback 中启用，Agent 工具的业务 HTTP 请求不会自动被抓入模型日志。子 Agent 继承会话选项。缓存会话参数支持显式 false 关闭，省略则保留缓存值。桌面提供上述全局开关；这里的会话参数仅用于单独指定会话的 API 调用。

单独调用模型也支持 `aispec.WithNetworkDebug(true)` / `ai.networkDebug(true)`；普通模型与 Provider 的 `ExtraParams[network_debug]` 也支持 true/false；yak-script 配置只读取 Script，不读取这些扩展项。没有会话的测试调用按 runtime/call 隔离，不会混入其他会话导出。

## 数据与导出

路径：引擎机器的 `consts.GetDefaultAISpaceDir()/.model-network-debug/<会话身份哈希>/<HTTP记录UUID>/`，通常为 `$YAKIT_HOME/aispace/.model-network-debug/`（未设置 YAKIT_HOME 时为 `~/yakit-projects/aispace/.model-network-debug/`）。不同会话不共享目录。

使用 aispace 下独立的隐藏诊断目录，不混入某个工具工作目录，也不自动加入模型上下文。网络报文只增量写文件，既不新建数据库记录，也不调用通用 HTTPFlow 保存回调：即使调试上下文中的脚本显式 poc.save(true)，lowhttp 也会抑制该请求的流量入库。普通工具流量没有该调试上下文，维持原有行为。配置开关本身仍保存在 profile DB。

每条记录包含：

- `request.http`：传输分发前已规范化的 HTTP 请求头和完整请求体。
- `response.http`：实际收到的完整响应字节，包括响应头及未解析的正文；断开时保留已收到的部分，不补造剩余内容。
- `metadata.json`：session、runtime、checkpoint sequence、逻辑 call ID、模型/Provider、脚本尝试次数、开始/结束时刻、总耗时毫秒、首字节延迟、HTTP 状态、Content-Type、Retry-After、字节数、传输/捕获/报文完整性错误。

`duration_ms` 从进入该次传输分发到返回，包含建连和接收响应；不包含此前配额排队/冷却，也不是一次逻辑 LLM call 多次重试的合计时间。`first_byte_ms` 缺省表示没有收到响应字节。

`transport_complete` 只说明 HTTP 传输及正文边界检查通过，不能证明 SSE 有业务最终消息、上游业务成功或模型返回了有效 action。HTTP 200 的业务错误仍会完整记录。`capture_error` 表示磁盘记录失败，此时文件不能被当作完整证据。记录通过逐块写盘避免在内存保留另一份长响应；不截断到 4 KB，不覆盖旧尝试。

HTTP/1 保存接收侧报文，可能仍有 chunked 和压缩编码；查看正文时需按头部解码。HTTP/2 保存重建的最终响应头与 DATA 内容，临时响应头与 trailers 另存 metadata.additional_headers；不是 TLS 密文或 HTTP/2 帧抓包；HTTP/3 使用底层返回的响应报文。普通 transport 重定向、状态重试、重连以及 HTTP/2 内部拒绝流后的重试分别记录。自定义脚本若绕过 lowhttp/poc，或不传 req.Context，则不在本机制覆盖内。

现有“导出日志”选择 `output_event` 时自动附带该会话的 `network/`，旧前端也可使用；API 也可单独指定 `network`。仅导出选定 session 或 runtime，不遍历收集其他会话的数据。正在运行的请求可导出当前文件快照，`ended_at` 缺省，不能视为最终响应。没有提前打开开关就没有历史网络报文，不能事后补录。

原来的 AICheckpoints、AIOutputEvent、timeline、memory 继续导出；AIOutputEvent 改为直接导出数据库记录，保留 session、模型显示名、流来源、恢复索引及数据库时间等原先转换时丢失的字段。

## 保留与职责

完整抓包包含 Authorization/Cookie、提示词、模型思考及业务数据。仅在排查时启用；导出包应作为敏感诊断材料处理。Unix 创建权限为目录 0700 / 文件 0600，Windows 使用用户目录的 ACL。删除 AI 会话时，先停止相关运行，再删除对应诊断目录。清理 aispace 孤立工作目录时会保留诊断根目录，避免误删其他会话。没有自动 TTL / 容量轮换；独立模型测试没有 session 时的记录需手动清理。长时间持续故障仍会增加文件系统占用，复现后应关闭调试并按需清理。

记录失败写入引擎警告，并尽可能在 metadata 中标记，不把磁盘错误变成模型内容或打断网络请求。网络调试不改写上游业务请求，不把原始响应自动放进提示词。直接回答缺少 answer_payload 的格式失败同样纳入已开启的格式恢复；已经开始交付回答片段的解析错误不扩大到该路径。持续恢复参数及执行边界见 [Yak Provider 规范](yakscript/README.md#无限重试和限频后的恢复)。
