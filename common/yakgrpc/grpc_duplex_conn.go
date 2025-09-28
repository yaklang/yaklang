package yakgrpc

import (
	"context"
	uuid "github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)

func (s *Server) DuplexConnection(stream ypb.Yak_DuplexConnectionServer) error {
	consts.WaitDatabasePostInitYakitCorePlugins()

	id := uuid.New().String()
	yakit.RegisterServerPushCallback(id, stream)
	defer yakit.UnRegisterServerPushCallback(id)

	yakit.BroadcastData(yakit.ServerPushType_Global, map[string]any{
		"config": map[string]any{
			"enableServerPush": true,
		},
	})

	// http flow  server push
	{
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
	}

	// rps cps server push
	{
		var lastRPS int64 //
		var rpsTicker = time.NewTicker(time.Second)
		go func() {
			for {
				select {
				case <-stream.Context().Done():
					return
				case <-rpsTicker.C:
					if currentRPS := lowhttp.GetLowhttpRPS(); currentRPS != lastRPS {
						if currentRPS > 5 {
							log.Infof("current lowhttp rps:%d", currentRPS)
						}
						yakit.BroadcastData(yakit.ServerPushType_RPS, currentRPS)
						lastRPS = currentRPS
					}
				}
			}
		}()

		var lastCPS int64
		var cpsTicker = time.NewTicker(time.Second)
		go func() {
			for {
				select {
				case <-stream.Context().Done():
					return
				case <-cpsTicker.C:
					if currentCPS := netx.GetDialxCPS(); currentCPS != lastCPS {
						if currentCPS > 5 {
							log.Infof("current dialx cps:%d", currentCPS)
						}
						yakit.BroadcastData(yakit.ServerPushType_CPS, currentCPS)
						lastCPS = currentCPS
					}
				}
			}
		}()
	}

	yakit.YakitDuplexConnectionServer.Server(stream.Context(), stream)
	return stream.Context().Err()
}

func WatchDatabaseTableMeta(db *gorm.DB, last int64, streamCtx context.Context, tableName string) (_ int64, changed bool) {
	if db == nil {
		db = consts.GetGormProjectDatabase()
	}

	current, err := bizhelper.GetTableCurrentId(db, tableName)
	if err != nil {
		return last, false
	}
	if current != last {
		return current, true
	}
	return current, false
}
