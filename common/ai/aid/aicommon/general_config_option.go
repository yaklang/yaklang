package aicommon

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
