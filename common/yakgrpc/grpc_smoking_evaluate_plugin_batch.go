package yakgrpc

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SmokingEvaluatePluginBatch(*SmokingEvaluatePluginBatchRequest, Yak_SmokingEvaluatePluginBatchServer) error
// SmokingEvaluatePluginBatch
func (s *Server) SmokingEvaluatePluginBatch(req *ypb.SmokingEvaluatePluginBatchRequest, stream ypb.Yak_SmokingEvaluatePluginBatchServer) error {
	fmt.Println("in smoking evaluate plugin batch!!")
	send := func(progress float64, message, messageType string) {
		fmt.Println("progress: ", progress, " message: ", message, " messageType: ", messageType)
		stream.Send(&ypb.SmokingEvaluatePluginBatchResponse{
			Progress:    progress,
			Message:     message,
			MessageType: messageType,
		})
	}
	names := make([]string, 0, len(req.GetScriptNames()))

	pluginSize := len(req.GetScriptNames())
	for index, name := range req.GetScriptNames() {
		progress := float64(index+1) / float64(pluginSize)
		// fmt.Println("check name: ", index, name, pluginSize, progress)
		ins, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), name)
		if err != nil {
			msg := fmt.Sprintf("%s: 无法获取该插件", name)
			send(progress, msg, "error")
		}
		code := ins.Content
		pluginType := ins.Type
		res, err := s.EvaluatePlugin(stream.Context(), code, pluginType)
		if err != nil {
			msg := fmt.Sprintf("%s 启动插件检测失败", name)
			send(progress, msg, "error")
		}
		if res.Score >= 60 {
			msg := fmt.Sprintf("%s 插件得分: %d", name, res.Score)
			send(progress, msg, "success")
			names = append(names, name)
			continue
		} else {
			msg := fmt.Sprintf("%s 插件得分: %d (<60)", name, res.Score)
			send(progress, msg, "error")
		}
	}
	send(2, strings.Join(names, ","), "success-again")
	return nil
}
