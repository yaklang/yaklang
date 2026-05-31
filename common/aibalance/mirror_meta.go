// mirror_meta.go - aibalance Mirror Rules 元信息接口
//
// 这里集中维护两类前端/用户可见的"自描述"信息:
//   1. DefaultMirrorScript: 默认脚本模板, 包含 if YAK_MAIN { ... } 自测块,
//      用户复制到本地 (`yak xxx.yak`) 即可直接跑.
//   2. MirrorDataSpec:     handle(data) 的 data 字段表 (字段名 / 类型 /
//      含义 / 示例), 给 portal 表单做帮助面板用.
//
// 修改原则:
//   - 字段顺序应与 MirrorSnapshot 保持一致 (易读)
//   - 新增字段时务必同步更新 spec, 否则 UI 文档失真
//   - 不要在 spec 里展示任何敏感字段 (例如曾经的 api_key, 现在用 api_key_fp)
//
// 关键词: aibalance mirror metadata, default script template, data spec,
//        handle(data) self-describe API, YAK_MAIN local test entry

package aibalance

// ==================== Data Spec ====================

// MirrorDataField 描述 handle(data) 中 data 的一个字段.
//
// 关键词: MirrorDataField, mirror data spec entry
type MirrorDataField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Example     string `json:"example"`
}

// MirrorDataSpec 返回 handle(data) 入参的字段定义清单.
// 顺序与 MirrorSnapshot 保持一致.
//
// 关键词: MirrorDataSpec, mirror snapshot 字段表
func MirrorDataSpec() []MirrorDataField {
	return []MirrorDataField{
		{
			Name:        "req_id",
			Type:        "string",
			Description: "本次请求的唯一 ID, 与 aibalance 日志中的 reqID 一致, 可用于日志关联",
			Example:     `"a1b2c3d4e5"`,
		},
		{
			Name:        "timestamp_ms",
			Type:        "int64 (ms since epoch)",
			Description: "请求开始时间, 毫秒 Unix 时间戳",
			Example:     "1748321400000",
		},
		{
			Name:        "model",
			Type:        "string",
			Description: "客户端请求的对外模型名 (透传给上游可能被改写)",
			Example:     `"gpt-4o-mini"`,
		},
		{
			Name:        "type_name",
			Type:        "string",
			Description: "实际命中的 provider 类型, 如 openai / deepseek / kimi 等",
			Example:     `"openai"`,
		},
		{
			Name:        "domain",
			Type:        "string",
			Description: "上游 provider 的域名 (或完整 URL)",
			Example:     `"api.openai.com"`,
		},
		{
			Name:        "api_key_fp",
			Type:        "string",
			Description: "API Key 的不可逆指纹 (SHA256[:16] hex). 仅用于区分不同 key, 不可反推原 key. free 用户为字面量 \"free-user\"",
			Example:     `"3f8a9c1d2e0b4f56"`,
		},
		{
			Name:        "is_free_model",
			Type:        "bool",
			Description: "是否走 free-model 免费通道",
			Example:     "false",
		},
		{
			Name:        "stream",
			Type:        "bool",
			Description: "客户端是否请求了 SSE 流式输出",
			Example:     "true",
		},
		{
			Name:        "request_messages",
			Type:        "[]ChatDetail",
			Description: "OpenAI chat/completions 请求体里的 messages 数组 (含 role / content / tool_calls 等)",
			Example:     `[{"role":"user","content":"hi"}]`,
		},
		{
			Name:        "response_text",
			Type:        "string",
			Description: "最终响应的主体文本 (多个 choices/delta 拼接). 流式与非流式都会拼成完整字符串",
			Example:     `"{\"@action\":\"directly_answer\",\"answer_payload\":\"hello\"}"`,
		},
		{
			Name:        "response_reason",
			Type:        "string",
			Description: "OpenAI 模型返回的 reasoning_content (思维链) 拼接结果, 多数模型为空",
			Example:     `""`,
		},
		{
			Name:        "tool_calls",
			Type:        "[]ToolCall",
			Description: "OpenAI 原生 tool_calls 数组 (function calling). 没有则为空数组",
			Example:     `[{"id":"call_1","function":{"name":"read_file","arguments":"{...}"}}]`,
		},
		{
			Name:        "action",
			Type:        "string",
			Description: "yaklang JSON 协议解析得到的 @action 字段. 解析失败为空串. 常见值: directly_answer / call-tool / require_tool / ...",
			Example:     `"call-tool"`,
		},
		{
			Name:        "action_payload",
			Type:        "object (map[string]any)",
			Description: "完整的 @action JSON payload, 含 tool / next_action / answer_payload 等所有字段. 解析失败为 null",
			Example:     `{"@action":"call-tool","tool":"read_file","params":{...}}`,
		},
		{
			Name:        "duration_ms",
			Type:        "int64",
			Description: "本次请求从入口到响应完成的总耗时 (毫秒)",
			Example:     "1234",
		},
		{
			Name:        "input_bytes",
			Type:        "int64",
			Description: "请求体大小 (字节)",
			Example:     "512",
		},
		{
			Name:        "output_bytes",
			Type:        "int64",
			Description: "响应体大小 (字节, 流式累加)",
			Example:     "2048",
		},
		{
			Name:        "usage",
			Type:        "object | null",
			Description: "OpenAI ChatUsage. 含 prompt_tokens / completion_tokens / total_tokens. 上游未返回则为 null",
			Example:     `{"prompt_tokens":40,"completion_tokens":85,"total_tokens":125}`,
		},
	}
}

// ==================== Default Script Template ====================

// DefaultMirrorScript 返回新建规则时默认填入的 yak 脚本模板.
//
// 设计:
//   - 必须包含 func handle(data) { ... } 入口 (aibalance 调度时自动调用)
//   - 末尾给一段 if YAK_MAIN { handle({...}) } 本地自测块: 用户直接 `yak xxx.yak`
//     就能跑通, 无需启动 aibalance. YAK_MAIN 默认为 false, aibalance 走的是
//     ScriptEngine.ExecuteEx, 因此该块在 aibalance 触发时不会执行, 不会
//     污染统计 (双重防呆: 加 if 防止用户复制粘贴本地代码到生产).
//   - 示例字段都用 redacted / hash 形态, 不暴露任何敏感数据.
//
// 关键词: DefaultMirrorScript, mirror default template, YAK_MAIN guarded test,
//
//	handle(data) entrypoint, 本地测试 yak xxx.yak
func DefaultMirrorScript() string {
	return defaultMirrorScriptTemplate
}

const defaultMirrorScriptTemplate = `// aibalance mirror callback.
// 必须定义 func handle(data) { ... }, aibalance worker 命中规则时会自动调用.
// 关键词: aibalance mirror callback, handle(data) entry

func handle(data) {
    // data 是一个 map, 字段见右侧 "data 字段说明" 面板.
    // 关键词: handle data map, model/action/duration_ms/tool_calls 字段读取
    toolCallCount = 0
    if data.tool_calls != nil {
        toolCallCount = len(data.tool_calls)
    }
    log.info(f"mirror got req: model=${data.model} action=${data.action} duration=${data["duration_ms"]}ms tool_calls=${toolCallCount}")

    // 示例: 调用内置 save 把本次镜像数据落盘归档 (容量受限, 超限自动清理旧数据).
    //   save()        // 落盘当前 data
    //   save(data)    // 等价写法, 也可传入自定义对象 save({"k": "v"})

    // 示例: 把命中条件的请求落到本地文件 (按需打开).
    //   file.SaveAndAppend("/tmp/mirror.log", json.dumps(data) + "\n")

    // 示例: 落到 KV 存储 (yakit 内置 db).
    //   db.SetKey("mirror:" + data.req_id, json.dumps(data))

    // 示例: 推到外部 webhook.
    //   poc.Post("https://example.com/hook",
    //       poc.body(json.dumps(data)),
    //       poc.timeout(5),
    //   )
}

// ------------------------------------------------------------------
// 本地自测入口: 复制本脚本到任意 .yak 文件, 用 ` + "`yak xxx.yak`" + ` 直接跑.
// YAK_MAIN 默认 false, aibalance 调度时不会进入此块 (双保险).
// 关键词: YAK_MAIN local test, mirror callback 本地复测
// ------------------------------------------------------------------
if YAK_MAIN {
    sample = {
        "req_id":         "test-local-001",
        "timestamp_ms":   1748321400000,
        "model":          "test-model",
        "type_name":      "openai",
        "domain":         "api.example.com",
        "api_key_fp":     "deadbeefcafef00d",
        "is_free_model":  false,
        "stream":         true,
        "request_messages": [
            {"role": "user", "content": "hi"},
        ],
        "response_text":   ` + "`{\"@action\":\"directly_answer\",\"answer_payload\":\"hello world\"}`" + `,
        "response_reason": "",
        "tool_calls":      [],
        "action":          "directly_answer",
        "action_payload": {
            "@action":        "directly_answer",
            "answer_payload": "hello world",
        },
        "duration_ms":  1234,
        "input_bytes":  100,
        "output_bytes": 200,
        "usage": {
            "prompt_tokens":     40,
            "completion_tokens": 85,
            "total_tokens":      125,
        },
    }
    handle(sample)
}
`
