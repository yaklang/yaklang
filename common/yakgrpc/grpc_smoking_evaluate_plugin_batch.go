package yakgrpc

import (
	"fmt"

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
	// names := make([]string, 0, len(req.GetScriptNames()))

	pluginSize := len(req.GetScriptNames())
	for index, name := range req.GetScriptNames() {
		progress := float64(index+1) / float64(pluginSize)
		fmt.Println("check name: ", index, name, pluginSize, progress)
		ins, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), name)
		if err != nil {
			send(progress, name, "error")
		}
		code := ins.Content
		pluginType := ins.Type
		if res, err := s.EvaluatePlugin(stream.Context(), code, pluginType); err == nil {
			if res.Score > 60 {
				send(progress, name, "success")
				// names = append(names, name)
				continue
			}
		}
		send(progress, name, "error")
	}
	// send(1, strings.Join(names, ","), "success-again")
	return nil
}
