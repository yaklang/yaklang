package sfweb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type SyntaxFlowScanRequest struct {
	Content        string `json:"content"`
	Lang           string `json:"lang"`
	ControlMessage string `json:"control_message"`
	TimeoutSecond  int    `json:"timeout_second"`
}

type SyntaxFlowScanResponse struct {
	Error    string                `json:"error,omitempty"`
	Message  string                `json:"message,omitempty"`
	Risk     []*SyntaxFlowScanRisk `json:"risk,omitempty"`
	Progress float64               `json:"progress,omitempty"`
}

type SyntaxFlowScanRisk struct {
	RuleName  string `json:"rule_name"`
	Severity  string `json:"severity"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	VarName   string `json:"var_name"`
	RiskHash  string `json:"risk_hash"`
	ResultID  uint64 `json:"result_id"`
	Timestamp int64  `json:"timestamp"`
}

func ypbToSyntaxFlowScanRisk(risk *ypb.Risk, result *ypb.SyntaxFlowResult) *SyntaxFlowScanRisk {
	if risk == nil {
		return nil
	}
	return &SyntaxFlowScanRisk{
		ResultID:  result.ResultID,
		RuleName:  result.RuleName,
		Severity:  risk.Severity,
		Timestamp: time.Now().Unix(),
		Title:     risk.Title,
		Type:      risk.RiskType,
		VarName:   risk.SyntaxFlowVariable,
		RiskHash:  risk.Hash,
	}
}

func WriteWebsocketJSON(c *websocket.Conn, data any) error {
	if SfWebLogger.Level == log.DebugLevel {
		bytes, _ := json.Marshal(data)
		SfWebLogger.Debugf("->client: %s", bytes)
	}
	if err := c.WriteJSON(data); err != nil {
		return err
	}
	return nil
}

func (s *SyntaxFlowWebServer) registerScanRoute() {
	s.router.HandleFunc("/scan", func(w http.ResponseWriter, r *http.Request) {
		// upgrade to websocket
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			writeErrorJson(w, err)
			return
		}
		defer conn.Close()
		// 读取客户端的消息
		_, msg, err := conn.ReadMessage()
		if err != nil {
			SfWebLogger.Errorf("read websocket message failed: %v", err)
			return
		}
		var req SyntaxFlowScanRequest
		if err := json.Unmarshal(msg, &req); err != nil {
			WriteWebsocketJSON(conn, &SyntaxFlowScanResponse{
				Error: fmt.Sprintf("invalid request: %v", err),
			})
			SfWebLogger.Errorf("unmarshal request failed: %v", err)
			return
		}
		if req.TimeoutSecond == 0 {
			req.TimeoutSecond = 180
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.TimeoutSecond)*time.Second)
		defer cancel()
		programName := uuid.NewString()
		_, err = ssaapi.Parse(req.Content,
			ssaapi.WithRawLanguage(req.Lang),
			ssaapi.WithProgramName(programName),
			ssaapi.WithContext(ctx),
		)
		if err != nil {
			WriteWebsocketJSON(conn, &SyntaxFlowScanResponse{
				Error: fmt.Sprintf("compile file failed: %v", err),
			})
			SfWebLogger.Errorf("compile file failed: %v", err)
			return
		}
		stream, err := s.grpcClient.SyntaxFlowScan(ctx)
		if err != nil {
			WriteWebsocketJSON(conn, &SyntaxFlowScanResponse{
				Error: fmt.Sprintf("create stream failed: %v", err),
			})
			SfWebLogger.Errorf("create stream failed: %v", err)
			return
		}
		// 发送请求
		if err := stream.Send(&ypb.SyntaxFlowScanRequest{
			ProgramName: []string{programName},
			ControlMode: req.ControlMessage,
			Filter:      &ypb.SyntaxFlowRuleFilter{},
		}); err != nil {
			WriteWebsocketJSON(conn, &SyntaxFlowScanResponse{
				Error: fmt.Sprintf("start syntaxflow scan failed: %v", err),
			})
			SfWebLogger.Errorf("start syntaxflow scan failed: %v", err)
			return
		}
		for {
			msg, err := stream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
					WriteWebsocketJSON(conn, &SyntaxFlowScanResponse{
						Error: fmt.Sprintf("syntaxflow scan failed: %v", err),
					})
					SfWebLogger.Errorf("syntaxflow scan failed: %v", err)
				}
				break
			}

			if len(msg.GetRisks()) > 0 {
				risks := lo.Map(msg.GetRisks(), func(risk *ypb.Risk, _ int) *SyntaxFlowScanRisk {
					return ypbToSyntaxFlowScanRisk(risk, msg.GetResult())
				})
				err = WriteWebsocketJSON(conn, &SyntaxFlowScanResponse{
					Risk: risks,
				})
				if err != nil {
					SfWebLogger.Errorf("write risks failed: %v", err)
					break
				}
			}

			if result := msg.GetExecResult(); result != nil && result.IsMessage {
				rawMsg := msg.ExecResult.GetMessage()
				result := gjson.ParseBytes(rawMsg)
				typ := result.Get("type").String()
				content := result.Get("content")
				if typ == "progress" {
					progress := content.Get("progress").Float()
					if progress > 0 {
						err = WriteWebsocketJSON(conn, &SyntaxFlowScanResponse{
							Progress: progress,
						})
						if err != nil {
							SfWebLogger.Errorf("write progress failed: %v", err)
							break
						}
					}
				} else if typ == "log" {
					level := content.Get("level").String()
					data := content.Get("data").String()
					if level == "error" {
						err = WriteWebsocketJSON(conn, &SyntaxFlowScanResponse{
							Error: data,
						})
						if err != nil {
							SfWebLogger.Errorf("write error message failed: %v", err)
							break
						}
					} else {
						err = WriteWebsocketJSON(conn, &SyntaxFlowScanResponse{
							Message: fmt.Sprintf("[%s] %s", level, data),
						})
						if err != nil {
							SfWebLogger.Errorf("write log message failed: %v", err)
							break
						}
					}
				}
			}
		}
	}).Name("syntaxflow scan")
}
