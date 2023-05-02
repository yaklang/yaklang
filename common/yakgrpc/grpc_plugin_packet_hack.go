package yakgrpc

import (
	uuid "github.com/satori/go.uuid"
	"io"
	"time"
	"yaklang/common/log"
	"yaklang/common/mutate"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib"
	"yaklang/common/yak/yaklib/codec"
	"yaklang/common/yakgrpc/yakit"
	"yaklang/common/yakgrpc/ypb"
)

const EXECUTEPACKET_CODE = `yakit.AutoInitYakit()
log.setLevel("info")
log.info("packet is loading: %v", "...")
scriptName = cli.String("script-name")
yakit.Info("Loading Script Name: %v", scriptName)
if scriptName == "" {
    yakit.Error("no script name set")
    return
}

log.Info("Start to Load HTTP Request Packet")
yakit.Info("Start to Load HTTP Request Packet")
isHttps = cli.Have("https")
packet = codec.DecodeHex("{{params(request)}}")[0]
if len(packet) <= 0 {
    yakit.Error("packet is empty")
    return
}
log.Info("HTTP Request Packet Length[%v]", len(packet))
yakit.Info("HTTP Request Packet Length[%v]", len(packet))

response = codec.DecodeHex("{{params(response)}}")[0]

log.Info("Start to Load 'func handle(https, req, rsp)' from %v", scriptName)
yakit.Info("Start to Load 'func handle(https, req, rsp)' from %v", scriptName)
core, err = hook.CallYakitPluginFunc(scriptName, "handle")
if err != nil {
    yakit.Error("load yakit plugin func error: %v", err)
    return
}

log.Info("Start to call core packet func")
yakit.Info("Start to call core packet func")
core(packet, response, isHttps)`

func (s *Server) generatePacketHackParams(req *ypb.ExecutePacketYakScriptParams) ([]*ypb.ExecParamItem, string, error) {
	scripts, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), req.GetScriptName())
	if err != nil {
		return nil, "", utils.Errorf("load yak script [%v] failed: %s", req.GetScriptName(), err)
	}
	codes, err := mutate.QuickMutate(EXECUTEPACKET_CODE, s.GetProfileDatabase(), mutate.MutateWithExtraParams(map[string][]string{
		"request":  {codec.EncodeToHex(req.GetRequest())},
		"response": {codec.EncodeToHex(req.GetResponse())},
	}))
	if err != nil {
		return nil, "", utils.Errorf("build code failed: %s", err)
	}
	if len(codes) <= 0 {
		return nil, "", utils.Errorf("build code failed... render params error")
	}

	var params []*ypb.ExecParamItem
	if req.GetIsHttps() {
		params = append(params, &ypb.ExecParamItem{Key: "https", Value: ""})
	}
	params = append(params, &ypb.ExecParamItem{Key: "script-name", Value: scripts.ScriptName})
	return params, codes[0], nil
}

func (s *Server) ExecutePacketYakScript(req *ypb.ExecutePacketYakScriptParams, stream ypb.Yak_ExecutePacketYakScriptServer) error {
	params, code, err := s.generatePacketHackParams(req)
	if err != nil {
		return err
	}
	return s.execRequest(
		&ypb.ExecRequest{
			Params: params,
			Script: code,
		},
		"exec-packet",
		stream.Context(),
		func(result *ypb.ExecResult, _ *yaklib.YakitLog) error {
			return stream.Send(result)
		}, &YakOutputStreamerHelperWC{
			stream: stream,
		})
}

func (s *Server) ExecuteBatchPacketYakScript(req *ypb.ExecuteBatchPacketYakScriptParams, stream ypb.Yak_ExecuteBatchPacketYakScriptServer) error {
	if len(req.GetScriptName()) <= 0 {
		return utils.Error("empty plugin is selected")
	}

	var concurrent int = int(req.GetConcurrent())
	if concurrent <= 0 {
		concurrent = 5
	}
	swg := utils.NewSizedWaitGroup(concurrent)
	for _, name := range req.GetScriptName() {
		script, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), name)
		if err != nil {
			log.Errorf("query script %s failed: %s", name, err)
			continue
		}

		swg.Add()
		go func() {
			swg.Done()

			params, code, err := s.generatePacketHackParams(&ypb.ExecutePacketYakScriptParams{
				ScriptName: script.ScriptName,
				IsHttps:    req.GetIsHttps(),
				Request:    req.GetRequest(),
				Response:   req.GetResponse(),
			})
			if err != nil {
				log.Errorf("generate packet-hack task failed: %s", err)
				return
			}

			taskId := uuid.NewV4().String()
			err = s.execRequest(
				&ypb.ExecRequest{Params: params, Script: code},
				"exec-module",
				stream.Context(),
				func(result *ypb.ExecResult, logInfo *yaklib.YakitLog) error {
					if logInfo == nil {
						return nil
					}

					stream.Send(&ypb.ExecBatchYakScriptResult{
						Id:        script.ScriptName,
						Status:    "data",
						PoC:       script.ToGRPCModel(),
						Result:    result,
						TaskId:    taskId,
						Timestamp: time.Now().Unix(),
					})
					return nil
				}, io.Discard,
			)
			defer stream.Send(&ypb.ExecBatchYakScriptResult{
				Id:        script.ScriptName,
				Status:    "end",
				PoC:       script.ToGRPCModel(),
				TaskId:    taskId,
				Timestamp: time.Now().Unix(),
			})
			if err != nil {
				stream.Send(&ypb.ExecBatchYakScriptResult{
					Id:        script.ScriptName,
					Timestamp: time.Now().Unix(),
					TaskId:    taskId,
					Status:    "data",
					Ok:        false,
					Reason:    err.Error(),
					PoC:       script.ToGRPCModel(),
					Result:    yaklib.NewYakitLogExecResult("error", err.Error()),
				})
				return
			}
		}()
	}
	swg.Wait()
	return nil
}
