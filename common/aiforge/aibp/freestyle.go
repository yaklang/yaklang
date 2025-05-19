package aibp

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
)

func init() {
	var lfopts []aiforge.LiteForgeOption
	lfopts = append(lfopts,
		aiforge.WithLiteForge_Prompt(`# 目的
你作为一个工具助手，目的是减轻用户输入的负担。
你会知道用户输入了什么内容，你需要在用户输入的位置后增加一些补全，用户可以通过Tab和方向键选用你生成的补全信息。
在生成选项的时候，可以猜测3-5条选项，每一条选项生成的时候尽量在5-30字节以内，确保精炼好修改。
`))
	lfopts = append(lfopts, aiforge.WithLiteForge_OutputSchema(
		aitool.WithStructArrayParam(
			"suggestions",
			[]aitool.PropertyOption{
				aitool.WithParam_Min(1),
			},
			nil,
			aitool.WithStringParam("text", aitool.WithParam_Required(true), aitool.WithParam_Description("在用户输入文本后插入的内容")),
		),
	))
	err := aiforge.RegisterLiteForge("freestyle", lfopts...)
	if err != nil {
		log.Errorf("register freestyle chat completion failed: %v", err)
	}
}
