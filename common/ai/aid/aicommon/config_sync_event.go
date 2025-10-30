package aicommon

import (
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)


const (
	SYNC_TYPE_PLAN          string = "plan"
	SYNC_TYPE_CONSUMPTION   string = "consumption"
	SYNC_TYPE_PING          string = "ping"
	SYNC_TYPE_SET_CONFIG    string = "set_config"
	SYNC_TYPE_PROCESS_EVENT string = "sync_process_event"

	ProcessID           string = "process_id"
	SyncProcessEeventID        = "sync_process_event_id"
)

func (c *Config) HandleSyncConsumptionEvent(_ *ypb.AIInputEvent) error {
	c.EmitJSON(
		schema.EVENT_TYPE_CONSUMPTION,
		"system",
		map[string]any{
			"input_consumption":  c.GetInputConsumption(),
			"output_consumption": c.GetOutputConsumption(),
			"consumption_uuid":   c.consumptionUUID,
		},
	)
	return nil
}

func (c *Config) HandleSyncPongEvent(_ *ypb.AIInputEvent)  error {
	c.EmitJSON(schema.EVENT_TYPE_PONG, "system", map[string]any{
		"now":         time.Now().Format(time.RFC3339),
		"now_unix":    time.Now().Unix(),
		"now_unix_ms": time.Now().UnixMilli(),
	})
	return nil
}

func (c *Config) RegisterBasicSyncHandlers() {
	c.InputEventManager.RegisterSyncCallback(SYNC_TYPE_CONSUMPTION, c.HandleSyncConsumptionEvent)
	c.InputEventManager.RegisterSyncCallback(SYNC_TYPE_PING, c.HandleSyncPongEvent)
}