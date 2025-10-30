package aicommon

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)


func (c *Config) ReleaseInteractiveEvent(eventID string, invoke aitool.InvokeParams) {
	c.EmitInteractiveRelease(eventID, invoke)
	c.CallAfterInteractiveEventReleased(eventID, invoke)
}

func (c *Config) EmitCurrentConfigInfo() {
	c.EmitJSON(schema.EVENT_TYPE_AID_CONFIG, "system", c.SimpleInfoMap())
}