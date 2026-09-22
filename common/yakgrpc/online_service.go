package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// onlineService is the remote boundary. Database queries and persistence stay
// in the handlers and use the server's database, including in tests.
type onlineService interface {
	UploadHotPatchTemplateToOnline(context.Context, string, []byte) error
	DownloadHotPatchTemplate(string, string, string) (*yaklib.HotPatchTemplate, error)
	UploadPayloadsToOnline(context.Context, string, []byte, []byte) error
	DownloadBatchPayloads(context.Context, string, string, string) *yaklib.OnlineDownloadPayloadStream
	UploadToOnline(context.Context, string, []byte, string) error
	UploadHTTPFlowToOnline(context.Context, *ypb.HTTPFlowsToOnlineRequest, []byte) error
	GetAIApiKeyByOnline(context.Context, string) (string, error)
	DownloadOnlineSyntaxFlowRule(context.Context, string, *ypb.DownloadSyntaxFlowRuleRequest) *yaklib.OnlineDownloadFlowRuleStream
}

func (s *Server) getOnlineClient() onlineService {
	if s != nil && s.onlineClient != nil {
		return s.onlineClient
	}
	return yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
}
