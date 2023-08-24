package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
	grpcMetadata "google.golang.org/grpc/metadata"
	"time"
)

type YakExecServerWrapper struct {
	sendHandle func(result *ypb.ExecResult) error
	grpc.ServerStream
}

func NewYakExecServerWrapper(stream grpc.ServerStream, handle func(result *ypb.ExecResult) error) *YakExecServerWrapper {
	return &YakExecServerWrapper{ServerStream: stream, sendHandle: handle}
}

type VAttachCombinedOutputServer struct {
	send    func(msg *ypb.ExecResult) error
	ctx     context.Context
	cancel  func()
	isValid bool
}

func (v *VAttachCombinedOutputServer) SetHeader(md grpcMetadata.MD) error {
	//TODO implement me
	panic("implement me")
}

func (v *VAttachCombinedOutputServer) SendHeader(md grpcMetadata.MD) error {
	//TODO implement me
	panic("implement me")
}

func (v *VAttachCombinedOutputServer) SetTrailer(md grpcMetadata.MD) {
	//TODO implement me
	panic("implement me")
}

func (v *VAttachCombinedOutputServer) Context() context.Context {
	return v.ctx
}

func (v *VAttachCombinedOutputServer) SendMsg(m interface{}) error {
	//TODO implement me
	panic("implement me")
}

func (v *VAttachCombinedOutputServer) RecvMsg(m interface{}) error {
	//TODO implement me
	panic("implement me")
}

func (v *VAttachCombinedOutputServer) Send(result *ypb.ExecResult) error {
	return v.send(result)
}
func (v *VAttachCombinedOutputServer) Cancel() {
	v.isValid = false
	v.cancel()
}
func (v *VAttachCombinedOutputServer) IsValid() bool {
	return v.isValid
}

func NewVAttachCombinedOutputServer(send func(msg *ypb.ExecResult) error) *VAttachCombinedOutputServer {
	ctx, cancel := context.WithCancel(context.Background())
	s := &VAttachCombinedOutputServer{send: send, ctx: ctx, cancel: cancel, isValid: true}
	return s
}
func (y *YakExecServerWrapper) Send(result *ypb.ExecResult) error {
	if y.sendHandle != nil {
		return y.sendHandle(result)
	}
	return nil
}
func (s *Server) CreateYaklangShell(server ypb.Yak_CreateYaklangShellServer) error {
	var engine *antlr4yak.Engine
	sendToClient := func(msg []byte, isMsg bool) {
		server.Send(&ypb.YaklangShellResponse{
			RawResult: &ypb.ExecResult{
				IsMessage: isMsg,
				Message:   msg,
			},
		})
	}
	sendError := func(e error) {
		log.Error(e)
		msgIns := &yaklib.YakitLog{
			Level: "error",
			Data:  fmt.Sprintf("%v", e),
		}
		msg, err := yaklib.YakitMessageGenerator(msgIns)
		if err != nil {
			log.Errorf("generate yakit log error: %v", err)
		}
		sendToClient(msg, true)
	}

	//defer func() {
	//	vAttach.Cancel()
	//}()
	timer := time.NewTimer(10 * time.Second)
	startOutputAttachInfo := utils.NewBool(false)
	vAttach := NewVAttachCombinedOutputServer(func(msg *ypb.ExecResult) error {
		if !startOutputAttachInfo.IsSet() {
			return nil
		}
		newMsg := *msg
		newMsg.Raw = append(msg.Raw, '\n')
		server.Send(&ypb.YaklangShellResponse{
			RawResult: &newMsg,
		})
		return nil
	})
	go func() {
		err := s.AttachCombinedOutput(nil, vAttach)
		if err != nil {
			sendError(err)
			return
		}
	}()
	defer func() {
		vAttach.Cancel()
	}()
	for {
		req, err := server.Recv()
		if err != nil {
			sendError(err)
			return err
		}
		inputData := req.GetInput()
		inputDataMap := make(map[string]string)
		if err := json.Unmarshal([]byte(inputData), &inputDataMap); err != nil {
			sendError(err)
			continue
		}

		var script, mode string
		if v, ok := inputDataMap["mode"]; !ok {
			sendError(utils.Error("interactive exec error: not found mode param"))
			continue
		} else {
			mode = v
		}
		if v, ok := inputDataMap["script"]; !ok {
			sendError(utils.Error("interactive exec error: not found script param"))
			continue
		} else {
			script = v
		}

		//execError := engine.SafeEvalInline(req.GetInput())
		//inspects, _ := engine.GetScopeInspects()
		//var scopes []*ypb.YaklangShellKVPair
		//for _, i := range inspects {
		//	scopes = append(scopes, &ypb.YaklangShellKVPair{
		//		Key:          i.Name,
		//		Value:        utils.InterfaceToBytes(i.Value),
		//		ValueVerbose: i.ValueVerbose,
		//		SymbolId:     int64(i.Id),
		//	})
		//}
		switch mode {
		case "static":
			s.Exec(&ypb.ExecRequest{
				Script: script,
			}, NewYakExecServerWrapper(server, func(result *ypb.ExecResult) error {
				if result.IsMessage {
					msgIns := &yaklib.YakitMessage{}
					if err := json.Unmarshal(result.Message, msgIns); err != nil {
						return server.Send(&ypb.YaklangShellResponse{
							RawResult: result,
						})
					}
					if msgIns.Type == "scope-values-info" {
						valuesInspects := []*antlr4yak.ScopeValue{}
						if err := json.Unmarshal(msgIns.Content, &valuesInspects); err != nil {
							return err
						}
						var scopes []*ypb.YaklangShellKVPair
						for _, i := range valuesInspects {
							if i.Id == 0 {
								continue
							}
							scopes = append(scopes, &ypb.YaklangShellKVPair{
								Key:          i.Name,
								Value:        utils.InterfaceToBytes(i.Value),
								ValueVerbose: i.ValueVerbose,
								SymbolId:     int64(i.Id),
							})
						}
						return server.Send(&ypb.YaklangShellResponse{
							Scope: scopes,
						})
					} else {
						return server.Send(&ypb.YaklangShellResponse{
							RawResult: result,
						})
					}
				}
				return server.Send(&ypb.YaklangShellResponse{
					RawResult: result,
				})
			}))
		case "interactive":
			startOutputAttachInfo.Set()
			timer.Reset(10 * time.Second)
			if engine == nil {
				engine = yaklang.NewAntlrEngine()
				yaklib.SetEngineClient(engine, yaklib.NewVirtualYakitClient(func(result *ypb.ExecResult) error {
					return server.Send(&ypb.YaklangShellResponse{
						RawResult: result,
					})
				}))
			}
			if err := engine.SafeEvalInline(server.Context(), script); err != nil {
				sendError(err)
				continue
			}

			//vAttach.Cancel()
			var scopes []*ypb.YaklangShellKVPair
			lastStackValue, err := engine.GetLastStackValue()
			if err == nil {
				scopes = append(scopes, &ypb.YaklangShellKVPair{
					Key:          "__last_stack_value__",
					Value:        utils.InterfaceToBytes(lastStackValue.Value),
					ValueVerbose: lastStackValue.TypeVerbose,
					SymbolId:     int64(lastStackValue.SymbolId),
				})
			}
			variableInspects, err := engine.GetScopeInspects()
			if err != nil {
				sendError(err)
				continue
			}
			for _, i := range variableInspects {
				if i.Id == 0 {
					continue
				}
				scopes = append(scopes, &ypb.YaklangShellKVPair{
					Key:          i.Name,
					Value:        utils.InterfaceToBytes(i.Value),
					ValueVerbose: i.ValueVerbose,
					SymbolId:     int64(i.Id),
				})
			}
			server.Send(&ypb.YaklangShellResponse{
				Scope: scopes,
			})
			timer.Reset(10 * time.Second)
			go func() {
				<-timer.C
				startOutputAttachInfo.UnSet()
			}()
		}
		sendToClient([]byte("signal-interactive-exec-end"), true)
	}
}
