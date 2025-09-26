package yakgrpc

import (
	"fmt"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yak/yaklib"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) SyntaxFlowScan(stream ypb.Yak_SyntaxFlowScanServer) error {
	rawConfig, err := stream.Recv()
	if err != nil {
		return err
	}

	pause := atomic.Bool{}
	go func() {
		for {
			rsp, err := stream.Recv()
			if err != nil {
				pause.Store(true)
				return
			}
			if rsp.GetControlMode() == "pause" {
				pause.Store(true)
				return
			}
		}
	}()

	var taskID string
	sendExecResult := func(execResult *ypb.ExecResult) error {
		return stream.Send(&ypb.SyntaxFlowScanResponse{
			TaskID:     taskID,
			ExecResult: execResult,
		})
	}

	return syntaxflow_scan.Scan(stream.Context(),
		syntaxflow_scan.WithRawConfig(rawConfig),
		syntaxflow_scan.WithPauseFunc(func() bool {
			return pause.Load()
		}),
		syntaxflow_scan.WithProcessCallback(func(progress float64, info *syntaxflow_scan.RuleProcessInfoList) {
			// update rule info
			sendExecResult(yaklib.NewYakitLogExecResult("code", info)) // 发送 rules info

			// update progress
			sendExecResult(yaklib.NewYakitProgressExecResult("main", progress))
			// status card
			sendExecResult(yaklib.NewYakitStatusCardExecResult("已执行规则", fmt.Sprintf("%d/%d", info.FinishedQuery, info.TotalQuery), "规则执行状态"))
			sendExecResult(yaklib.NewYakitStatusCardExecResult("已跳过规则", info.SkippedQuery, "规则执行状态"))
			sendExecResult(yaklib.NewYakitStatusCardExecResult("执行成功个数", info.SuccessQuery, "规则执行状态"))
			sendExecResult(yaklib.NewYakitStatusCardExecResult("执行失败个数", info.FailedQuery, "规则执行状态"))
			sendExecResult(yaklib.NewYakitStatusCardExecResult("检出漏洞/风险个数", info.RiskCount, "漏洞/风险状态"))
		}),
		syntaxflow_scan.WithScanResultCallback(func(sr *syntaxflow_scan.ScanResult) {
			taskID = sr.TaskID
			// 发送扫描结果
			stream.Send(&ypb.SyntaxFlowScanResponse{
				TaskID:   sr.TaskID,
				Status:   sr.Status,
				Result:   sr.Result.GetGRPCModelResult(),
				SSARisks: sr.Result.GetGRPCModelRisk(),
			})
		}),
		syntaxflow_scan.WithErrorCallback(func(format string, args ...any) {
			// 发送错误信息
			sendExecResult(yaklib.NewYakitLogExecResult("error", format, args...))
		}),
	)
}
