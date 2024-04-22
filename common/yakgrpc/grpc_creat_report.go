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
			{Key: "hostTotal", Value: gjson.Get(reportMetaInfo, "hostTotal").String()},
			{Key: "portTotal", Value: gjson.Get(reportMetaInfo, "portTotal").String()},
			{Key: "pingAliveHostTotal", Value: gjson.Get(reportMetaInfo, "pingAliveHostTotal").String()},
			{Key: "plugins", Value: gjson.Get(reportMetaInfo, "plugins").String()},
		},
	}

	return s.DebugPlugin(execRequest, stream)
}
