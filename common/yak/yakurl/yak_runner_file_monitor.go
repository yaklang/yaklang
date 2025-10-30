package yakurl

import (
	"context"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var OP_NEW_MONITOR = "new"
var OP_STOP_MONITOR = "stop"

var YakRunnerMonitor *utils.SafeMap[*filesys.YakFileMonitor]

func init() {
	YakRunnerMonitor = utils.NewSafeMap[*filesys.YakFileMonitor]()
	yakit.YakitDuplexConnectionServer.RegisterHandler("file_monitor", handlerFileMonitor)
}

func handlerFileMonitor(ctx context.Context, request *ypb.DuplexConnectionRequest) error {
	eventsHandler := func(eventSet *filesys.EventSet) {
		yakit.BroadcastData(yakit.ServerPushType_File_Monitor, eventSet)
	}
	data := request.GetData()
	op := gjson.Get(string(data), "operate").String()
	id := gjson.Get(string(data), "id").String()
	switch op {
	case OP_NEW_MONITOR:
		path := gjson.Get(string(data), "path").String()
		m, err := filesys.WatchPath(ctx, path, eventsHandler)
		if err != nil {
			return err
		}
		if oldMonitor, ok := YakRunnerMonitor.Get(id); ok {
			oldMonitor.CancelFunc()
		}
		YakRunnerMonitor.Set(id, m)
		log.Infof("Start monitor path: %v", path)
	case OP_STOP_MONITOR:
		if monitor, ok := YakRunnerMonitor.Get(id); ok {
			monitor.CancelFunc()
			YakRunnerMonitor.Delete(id)
			log.Infof("Stop monitor path: %v", monitor.WatchPatch)
		}
	default:
	}
	return nil
}

func CheckUpdateFileMonitors(absPath string) error {
	var e error
	YakRunnerMonitor.ForEach(func(key string, value *filesys.YakFileMonitor) bool {
		if utils.IsSubPath(absPath, value.WatchPatch) {
			err := value.UpdateFileTree()
			if err != nil {
				e = utils.JoinErrors(e, err)
				log.Errorf("failed to update file tree: %s", err)
			}
		}
		return true
	})
	return e
}
