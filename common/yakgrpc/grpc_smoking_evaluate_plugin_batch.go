package yakgrpc

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SmokingEvaluatePluginBatch(*SmokingEvaluatePluginBatchRequest, Yak_SmokingEvaluatePluginBatchServer) error
// SmokingEvaluatePluginBatch
func (s *Server) SmokingEvaluatePluginBatch(req *ypb.SmokingEvaluatePluginBatchRequest, stream ypb.Yak_SmokingEvaluatePluginBatchServer) error {
	// fmt.Println("in smoking evaluate plugin batch!!")
	send := func(progress float64, message, messageType string) {
		// fmt.Println("progress: ", progress, " message: ", message, " messageType: ", messageType)
		stream.Send(&ypb.SmokingEvaluatePluginBatchResponse{
			Progress:    progress,
			Message:     message,
			MessageType: messageType,
		})
	}
	names := make([]string, 0, len(req.GetScriptNames()))
	successNum := 0
	errorNum := 0

	pluginTestingServer := NewPluginTestingEchoServer(stream.Context())

	ch := yakit.YieldYakScripts(
		bizhelper.ExactQueryStringArrayOr(s.GetProfileDatabase(), "script_name", req.GetScriptNames()),
		stream.Context(),
	)

	all := utils.NewSet(req.GetScriptNames())
	exist := utils.NewSet[string]()

	send(0, "开始检测", "success")
	pluginSize := len(req.GetScriptNames())
	index := 0
	for ins := range ch {
		progress := float64(index+1) / float64(pluginSize)
		index++
		exist.Add(ins.ScriptName)
		code := ins.Content
		pluginType := ins.Type
		res, err := s.EvaluatePlugin(stream.Context(), code, pluginType, pluginTestingServer)
		if err != nil {
			msg := fmt.Sprintf("%s 启动插件检测失败", ins.ScriptName)
			send(progress, msg, "error")
			errorNum++
			continue
		}
		if res.Score >= 60 {
			msg := fmt.Sprintf("%s 插件得分: %d", ins.ScriptName, res.Score)
			send(progress, msg, "success")
			names = append(names, ins.ScriptName)
			successNum++
			continue
		} else {
			msg := fmt.Sprintf("%s 插件得分: %d (<60)", ins.ScriptName, res.Score)
			send(progress, msg, "error")
			errorNum++
			continue
		}
	}

	all.Diff(exist).ForEach(func(name string) {
		progress := float64(index+1) / float64(pluginSize)
		index++
		msg := fmt.Sprintf("%s: 无法获取该插件", name)
		send(progress, msg, "error")
		errorNum++
	})

	{
		msg := ""
		if successNum > 0 {
			msg += fmt.Sprintf("检测通过%d个", successNum)
		}
		if errorNum > 0 {
			msg += fmt.Sprintf(", 检测失败%d个", errorNum)
		}
		if msg == "" {
			msg += "检测结束"
		}
		send(1, msg, "success")
	}
	msg, err := json.Marshal(names)
	if err != nil {
		return err
	}
	send(2, string(msg), "success-again")
	// send(2, strings.Join(names, ","), "success-again")
	return nil
}
