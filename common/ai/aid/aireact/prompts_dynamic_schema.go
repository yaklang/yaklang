package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func getDirectlyAnswer() string {
	return aitool.NewObjectSchemaWithActionName(
		"directly_answer",
		aitool.WithStringParam(
			"answer_payload",
			aitool.WithParam_Description(
				`USE THIS FIELD ONLY IF @action is 'directly_answer' AND answer is short (≤200 chars). For long answers, leave this empty and use '<|FINAL_ANSWER_...|>' tags after JSON. CRITICAL: answer_payload and <|FINAL_ANSWER_...|> are STRICTLY MUTUALLY EXCLUSIVE - never use both simultaneously.`,
			),
		),
	)
}

func getChangeAIBlueprintSchema() string {
	return aitool.NewObjectSchema(
		aitool.WithStringParam(
			"@action",
			aitool.WithParam_Const("change-ai-blueprint"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"reasoning",
			aitool.WithParam_Description("切换这个新的 Blueprint/Forge 的原因"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"new_blueprint",
			aitool.WithParam_Description("切换成新的 Blueprint 的名称"),
			aitool.WithParam_Required(true),
		),
	)
}
