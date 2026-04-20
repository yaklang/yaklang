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
