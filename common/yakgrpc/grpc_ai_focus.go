package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/reactloops_yak"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryAIFocus(ctx context.Context, _ *ypb.QueryAIFocusRequest) (*ypb.QueryAIFocusResponse, error) {
	// 在返回 focus mode 列表之前，懒扫描 ~/yakit-projects/ai-focus/
	// 把用户自定义 yak focus mode 注册到全局表。
	// 内部冷却（默认 5s），失败只 log 不影响主流程。
	// 关键词: query ai focus ensure user dir loaded
	if err := reactloops_yak.EnsureUserFocusModesLoaded(); err != nil {
		log.Warnf("ensure user yak focus modes failed: %v", err)
	}

	metas := reactloops.GetAllLoopMetadata()

	resp := &ypb.QueryAIFocusResponse{
		Data: make([]*ypb.AIFocus, 0, len(metas)),
	}
	for _, meta := range metas {
		if meta == nil {
			continue
		}
		if meta.IsHidden {
			continue
		}
		resp.Data = append(resp.Data, &ypb.AIFocus{
			Name:                meta.Name,
			Description:         meta.Description,
			DescriptionZh:       meta.GetDescriptionZh(),
			OutputExamplePrompt: meta.OutputExamplePrompt,
			UsagePrompt:         meta.UsagePrompt,
			VerboseName:         meta.VerboseName,
			VerboseNameZh:       meta.VerboseNameZh,
		})
	}

	return resp, nil
}
