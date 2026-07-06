//go:build irify_exclude

package yakgrpc

import "github.com/yaklang/yaklang/common/yakgrpc/ypb"

func SyntaxFlowServer(req *ypb.YaklangLanguageSuggestionRequest) (*ypb.YaklangLanguageSuggestionResponse, bool) {
	return nil, false
}
