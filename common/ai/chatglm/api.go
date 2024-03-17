package chatglm

import (
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func init() {
	aispec.Register("chatglm", func() aispec.AIGateway {
		return &GLMClient{}
	})
}
