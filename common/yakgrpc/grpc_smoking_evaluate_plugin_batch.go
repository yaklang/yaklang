package yakgrpc

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
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

	all := utils.NewSet(req.GetScriptNames())
	exist := utils.NewSet[string]()

	send(0, "开始检测", "success")
	pluginSize := len(req.GetScriptNames())
	index := 0
	// define check with interface ScriptOrRule
	check := func(ins schema.ScriptOrRule) {
		progress := float64(index+1) / float64(pluginSize)
		index++
		name := ins.GetScriptName()
		exist.Add(name)
		code := ins.GetContent()
		pluginType := ins.GetType()
		res, err := s.EvaluatePlugin(stream.Context(), code, pluginType, pluginTestingServer)
		if err != nil {
			msg := fmt.Sprintf("%s 启动插件检测失败", name)
			send(progress, msg, "error")
			errorNum++
			return
		}
		if res.Score >= 60 {
			msg := fmt.Sprintf("%s 插件得分: %d", name, res.Score)
			send(progress, msg, "success")
			names = append(names, name)
			successNum++
			return
		} else {
			msg := fmt.Sprintf("%s 插件得分: %d (<60)", name, res.Score)
			send(progress, msg, "error")
			errorNum++
			return
		}
	}
	// check
	switch req.PluginType {
	case "syntaxflow":
		ch := sfdb.YieldSyntaxFlowRules(
			bizhelper.ExactOrQueryStringArrayOr(s.GetSSADatabase(), "rule_name", req.GetScriptNames()),
			stream.Context(),
		)
		for rule := range ch {
			check(rule)
		}
	default:
		ch := yakit.YieldYakScripts(
			bizhelper.ExactQueryStringArrayOr(s.GetProfileDatabase(), "script_name", req.GetScriptNames()),
			stream.Context(),
		)
		for ins := range ch {
			check(ins)
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
