package reactloops

import "io"

type LoopStreamFieldHandler func(fieldReader io.Reader, emitWriter io.Writer)

type LoopStreamField struct {
	FieldName     string
	AINodeId      string
	Prefix        string
	ContentType   string
	StreamHandler LoopStreamFieldHandler
}

// LoopAITagField 描述一个被注册到 ActionMaker 的 AITAG 字段.
//
// 字段语义:
//   - TagName: <|TagName_<nonce>|> 的 tagName 部分, 不含 nonce
//   - VariableName: 解析到的内容写入 action.params[VariableName]
//   - AINodeId: 用于 emitter 流事件的归属节点 id (与 streamFields 共用语义)
//   - ContentType: 流事件 content type, 默认空
//   - ExtraNonces: 给该 tag 追加的 nonce 候选 (与 ActionMaker turn nonce 并列,
//     不替代). 解析时 ActionMaker 会同时给 turn nonce + 每个 extra nonce 都
//     注册 callback, LLM 用任一 nonce 输出 AITAG 都能命中并写入 VariableName.
//
// 典型用例: CACHE_TOOL_CALL 块内 TOOL_PARAM_xxx 在 prompt 中用占位符字面量
// nonce "[current-nonce]" (aicommon.RecentToolCacheStableNonce) 渲染, 保持
// prompt 字节稳定, 进入 prefix cache; 同时 LLM 也可能将占位符替换为 turn
// nonce 输出. 通过 ExtraNonces=["[current-nonce]"], 两种行为都能解析成功.
//
// 该候选追加是字段级精准的, 不会扩散到其他 LoopAITagField (USER_QUERY 等
// 仍然只走 turn nonce).
//
// 关键词: LoopAITagField ExtraNonces, 双注册兜底, [current-nonce] 占位符,
//        精准覆盖工具缓存, prefix cache 字节稳定
type LoopAITagField struct {
	TagName      string
	VariableName string
	AINodeId     string
	ContentType  string
	ExtraNonces  []string
}
