package aicommon

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"time"
)

const (
	SYNC_TYPE_PING string = "ping"
)

type SyncEventGuardian struct {
	*Emitter
	syncDataMap      *omap.OrderedMap[string, func() any]
	syncEventTypeMap *omap.OrderedMap[string, schema.EventType]
}

func NewSyncGuardian(emitter *Emitter) *SyncEventGuardian {
	sg := &SyncEventGuardian{
		Emitter:          emitter,
		syncDataMap:      omap.NewOrderedMap[string, func() any](map[string]func() any{}),
		syncEventTypeMap: omap.NewOrderedMap[string, schema.EventType](map[string]schema.EventType{}),
	}
	sg.basic()
	return sg
}

func (s *SyncEventGuardian) basic() {
	s.RegisterSyncFunc(SYNC_TYPE_PING, schema.EVENT_TYPE_PONG)
	s.SetSyncData(SYNC_TYPE_PING, func() any {
		return map[string]any{
			"now":         time.Now().Format(time.RFC3339),
			"now_unix":    time.Now().Unix(),
			"now_unix_ms": time.Now().UnixMilli(),
		}
	})
}

func (s *SyncEventGuardian) RegisterSyncFunc(syncInputType string, syncOutputType schema.EventType) {
	s.syncEventTypeMap.Set(syncInputType, syncOutputType)
}

func (s *SyncEventGuardian) SetSyncData(syncType string, dataFunc func() any) {
	s.syncDataMap.Set(syncType, dataFunc)
}

func (s *SyncEventGuardian) Process(syncType string, data aitool.InvokeParams) error {
	outputType, ok := s.syncEventTypeMap.Get(syncType)
	if !ok {
		return utils.Errorf("not register sync type %s", syncType)
	}

	dataFunc, ok := s.syncDataMap.Get(syncType)
	if !ok || dataFunc == nil {
		return utils.Errorf("no sync data for type %s", syncType)
	}
	s.EmitJSON(outputType, "system", dataFunc())
	return nil
}
