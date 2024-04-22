package yakgrpc

import (
	_ "embed"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed grpc_creat_report.yak
var creat_report []byte

func (s *Server) SimpleDetectCreatReport(req *ypb.CreatReportRequest, stream ypb.Yak_SimpleDetectCreatReportServer) error {
	reportMetaInfo := yakit.Get("simple_detect_" + req.GetRuntimeId())

	execRequest := &ypb.DebugPluginRequest{
		Code: string(creat_report),
		ExecParams: []*ypb.KVPair{
			{Key: "host_total", Value: gjson.Get(reportMetaInfo, "hostTotal").String()},
			{Key: "port_total", Value: gjson.Get(reportMetaInfo, "portTotal").String()},
			{Key: "ping_alive_host_total", Value: gjson.Get(reportMetaInfo, "pingAliveHostTotal").String()},
			{Key: "plugins", Value: gjson.Get(reportMetaInfo, "plugins").String()},
			{Key: "task_name", Value: req.GetRuntimeId()},
			{Key: "report_name", Value: req.GetReportName()},
			{Key: "runtime_id", Value: req.GetRuntimeId()},
		},
	}

	return s.DebugPlugin(execRequest, stream)
}
