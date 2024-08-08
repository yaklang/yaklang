package yakgrpc

import (
	"context"
	uuid "github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
	"time"
)

func (s *Server) DuplexConnection(stream ypb.Yak_DuplexConnectionServer) error {
	id := uuid.New().String()
	yakit.RegisterServerPushCallback(id, stream)
	defer yakit.UnRegisterServerPushCallback(id)

	yakit.BroadcastData(yakit.ServerPushType_Global, map[string]any{
		"config": map[string]any{
			"enableServerPush": true,
		},
	})

	yakit.YakitDuplexConnectionServer.Server(stream.Context(), stream)

	startOnce := new(sync.Once)
	startOnce.Do(func() {
		var httpFlowsSeq int64
		var changed bool
		go func() {
			for {
				select {
				case <-stream.Context().Done():
					return
				default:
					if httpFlowsSeq == 0 {
						httpFlowsSeq, _ = WatchDatabaseTableMeta(nil, 0, stream.Context(), "http_flows")
						time.Sleep(time.Second)
						continue
					}

					httpFlowsSeq, changed = WatchDatabaseTableMeta(nil, httpFlowsSeq, stream.Context(), "http_flows")
					if changed {
						yakit.BroadcastData(yakit.ServerPushType_HttpFlow, "create")
					}
					time.Sleep(time.Second)
				}
			}
		}()
	})

	<-stream.Context().Done()
	return stream.Context().Err()
}

func WatchDatabaseTableMeta(db *gorm.DB, last int64, streamCtx context.Context, tableName string) (_ int64, changed bool) {
	if db == nil {
		db = consts.GetGormProjectDatabase()
	}
	var result struct {
		Count int64
	}
	db = db.Raw(`select seq as count from SQLITE_SEQUENCE where name = ?`, tableName)
	if db.Scan(&result).Error != nil {
		return last, false
	}
	if result.Count != last {
		return result.Count, true
	}
	return result.Count, false
}
