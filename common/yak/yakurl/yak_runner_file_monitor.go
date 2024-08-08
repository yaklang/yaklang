package yakurl

import (
	"context"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var OP_NEW_MONITOR = "new"
var OP_STOP_MONITOR = "stop"

var YakRunnerMonitor *filesys.YakFileMonitor

func init() {
	yakit.YakitDuplexConnectionServer.RegisterHandler("file_monitor", func(ctx context.Context, request *ypb.DuplexConnectionRequest) error {
		eventsHandler := func(eventSet *filesys.EventSet) {
			yakit.BroadcastData(yakit.ServerPushType_File_Monitor, eventSet)
		}
		data := request.GetData()
		op := gjson.Get(string(data), "operate").String()
		switch op {
		case OP_NEW_MONITOR:
			path := gjson.Get(string(data), "path").String()
			m, err := filesys.WatchPath(ctx, path, eventsHandler)
			if err != nil {
				return err
			}
			if YakRunnerMonitor != nil { // stop the old monitor. keep just watch one
				YakRunnerMonitor.CancelFunc()
			}
			YakRunnerMonitor = m
			log.Infof("Start monitor path: %v", path)
		case OP_STOP_MONITOR:
			if YakRunnerMonitor != nil {
				YakRunnerMonitor.CancelFunc()
				YakRunnerMonitor = nil
				log.Infof("Stop monitor path")
			}
		default:
		}
		return nil
	})
}
