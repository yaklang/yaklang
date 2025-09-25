package yakgrpc

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	sfscan "github.com/yaklang/yaklang/common/yak/syntaxflow_scan"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) SyntaxFlowScan(stream ypb.Yak_SyntaxFlowScanServer) error {
	scanRequest, err := stream.Recv()
	if err != nil {
		return err
	}

	switch strings.ToLower(scanRequest.ControlMode) {
	case "start":
		config, err := createSSAConfigByRequest(scanRequest)
		if err != nil {
			return err
		}
		opts := []sfscan.Option{
			sfscan.WithSyntaxFlowScanConfig(config),
		}
		_, err = sfscan.StartScan(stream.Context(), opts...)
		if err != nil {
			return err
		}
		//TODO: 暂停与停止
	}
	return nil
}

func createSSAConfigByRequest(req *ypb.SyntaxFlowScanRequest) (*ssaconfig.Config, error) {
	config, err := ssaconfig.NewSyntaxFlowScanConfig(
		ssaconfig.WithScanConcurrency(req.Concurrency),
		ssaconfig.WithScanMemory(req.Memory),
		ssaconfig.WithScanIgnoreLanguage(req.IgnoreLanguage),
		ssaconfig.WithRuleFilter(req.GetFilter()),
		ssaconfig.WithRuleInput(req.GetRuleInput()),
	)
	if err != nil {
		return nil, err
	}
	return config, nil
}
