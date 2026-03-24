package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryAIFocus(ctx context.Context, _ *ypb.QueryAIFocusRequest) (*ypb.QueryAIFocusResponse, error) {
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
