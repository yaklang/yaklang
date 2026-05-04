package aicommon

import "io"

func WithGeneralConfigStreamableFieldWithNodeId(nodeId string, fieldKey string) GeneralKVConfigOption {
	return func(c *GeneralKVConfig) {
		result, ok := c.config.Get("streamFields")
		if !ok {
			result = []map[string]string{}
			c.config.Set("streamFields", result)
		}
		streamFields, ok := result.([]map[string]string)
		if !ok {
			streamFields = []map[string]string{}
		}
		streamFields = append(streamFields, map[string]string{
			"aiNodeId": nodeId,
			"fieldKey": fieldKey,
		})
		c.config.Set("streamFields", streamFields)
	}
}

func WithGeneralConfigStreamableField(fieldKey string) GeneralKVConfigOption {
	return WithGeneralConfigStreamableFieldWithNodeId("re-act-loop-thought", fieldKey)
}

// WithLiteForgeStaticInstruction 携带 LiteForge 的系统侧静态指令到 GeneralKVConfig.
// 该指令最终会被 invoke_liteforge.go 解析并通过 aiforge.WithLiteForge_StaticInstruction
// 传给 LiteForge. P0-B1 之后进入 semi-dynamic 段 (历史曾在 high-static 段, 因
// schema/instruction 按 forge 维度变化导致跨 forge cache miss, 已下移),
// 跨同一 forge 多次调用稳定哈希.
// 关键词: aicache, PROMPT_SECTION_semi-dynamic, StaticInstruction,
//
//	WithLiteForgeStaticInstruction
func WithLiteForgeStaticInstruction(s string) GeneralKVConfigOption {
	return func(c *GeneralKVConfig) {
		c.config.Set("liteForgeStaticInstruction", s)
	}
}

// StreamableFieldCallback is a callback function that handles streaming field data during LiteForge execution.
// key: the field key that matches one of the monitored fields
// r: io.Reader containing the streaming data for that field
type StreamableFieldCallback func(key string, r io.Reader)

// StreamableFieldEmitterCallback is like StreamableFieldCallback, but also
// receives the emitter that has already been bound to the current AI response.
type StreamableFieldEmitterCallback func(key string, r io.Reader, emitter *Emitter)

// StreamableFieldCallbackItem stores the field keys and callback pair
type StreamableFieldCallbackItem struct {
	FieldKeys []string
	Callback  StreamableFieldEmitterCallback
}

// WithGeneralConfigStreamableFieldCallback registers a callback for streaming field data during LiteForge execution.
// fieldKeys: array of field names to monitor for streaming data
// callback: function called when data streams into any of the monitored fields
// This enables extensibility for processing streaming JSON data in real-time.
func WithGeneralConfigStreamableFieldCallback(fieldKeys []string, callback StreamableFieldCallback) GeneralKVConfigOption {
	return WithGeneralConfigStreamableFieldEmitterCallback(fieldKeys, func(key string, r io.Reader, _ *Emitter) {
		callback(key, r)
	})
}

// WithGeneralConfigStreamableFieldEmitterCallback registers a callback that
// receives the response-bound emitter for correctly scoped AI event metadata.
func WithGeneralConfigStreamableFieldEmitterCallback(fieldKeys []string, callback StreamableFieldEmitterCallback) GeneralKVConfigOption {
	return func(c *GeneralKVConfig) {
		result, ok := c.config.Get("streamFieldCallbacks")
		if !ok {
			result = []*StreamableFieldCallbackItem{}
		}
		callbacks, ok := result.([]*StreamableFieldCallbackItem)
		if !ok {
			callbacks = []*StreamableFieldCallbackItem{}
		}
		callbacks = append(callbacks, &StreamableFieldCallbackItem{
			FieldKeys: fieldKeys,
			Callback:  callback,
		})
		c.config.Set("streamFieldCallbacks", callbacks)
	}
}
