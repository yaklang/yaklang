package dashscopebase

import "github.com/yaklang/yaklang/common/ai/aispec"

func CreateDashScopeGateway(appId string) aispec.AIClient {
	return &DashScopeGateway{
		dashscopeAppId: appId,
	}
}
