package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"strings"
	"yaklang.io/yaklang/common/mutate"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

func (s *Server) HTTPRequestAnalyzer(ctx context.Context, req *ypb.HTTPRequestAnalysisMaterial) (*ypb.HTTPRequestAnalysis, error) {
	resp := &ypb.HTTPRequestAnalysis{
		Params: nil,
	}

	randomTrace := utils.RandStringBytes(20)

	fReq, err := mutate.NewFuzzHTTPRequest(req.Request)
	if err != nil {
		return nil, err
	}

	// 利用 Fuzz 分析出参数
	var params []*ypb.HTTPRequestParamItem
	var testableRequest []string
	for _, p := range fReq.GetCommonParams() {
		item := &ypb.HTTPRequestParamItem{
			TypePosition:        p.Position(),
			ParamOriginValue:    spew.Sdump(p.Value()),
			ParamName:           p.Name(),
			TypePositionVerbose: p.PositionVerbose(),
		}
		res, _ := p.Fuzz(randomTrace).Results()
		for _, r := range res {
			raw, _ := utils.HttpDumpWithBody(r, true)
			if raw != nil {
				testableRequest = append(testableRequest, strings.ReplaceAll(
					string(raw), randomTrace, "{{param}}",
				))
			}
		}
		params = append(params, item)
	}
	resp.Requests = testableRequest
	return resp, nil
}
