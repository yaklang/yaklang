package aibp

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
)

//go:embed sf_test_cases_completion_prompts/test_cases_init.txt
var sf_test_cases_completion_prompt string

func init() {
	err := aiforge.RegisterLiteForge("sf_test_cases_completion",
		aiforge.WithLiteForge_Prompt(sf_test_cases_completion_prompt),
		aiforge.WithLiteForge_OutputSchema(
			aitool.WithStructArrayParam("positive_test_cases", []aitool.PropertyOption{
				aitool.WithParam_Description("正向测试用例列表"),
				aitool.WithParam_Required(false),
			}, []aitool.PropertyOption{},
				aitool.WithStringParam("filename", aitool.WithParam_Required(true), aitool.WithParam_Description("测试用例文件名")),
				aitool.WithStringParam("content", aitool.WithParam_Required(true), aitool.WithParam_Description("测试用例代码内容")),
				aitool.WithStringParam("description", aitool.WithParam_Required(false), aitool.WithParam_Description("测试用例描述")),
			),
			aitool.WithStructArrayParam("negative_test_cases", []aitool.PropertyOption{
				aitool.WithParam_Description("反向测试用例列表"),
				aitool.WithParam_Required(false),
			}, []aitool.PropertyOption{},
				aitool.WithStringParam("filename", aitool.WithParam_Required(true), aitool.WithParam_Description("测试用例文件名")),
				aitool.WithStringParam("content", aitool.WithParam_Required(true), aitool.WithParam_Description("测试用例代码内容")),
				aitool.WithStringParam("description", aitool.WithParam_Required(false), aitool.WithParam_Description("测试用例描述")),
			),
			aitool.WithStringParam("test_case_summary", aitool.WithParam_Required(true), aitool.WithParam_Description("测试用例补全的简要说明")),
			aitool.WithBoolParam("has_positive_tests", aitool.WithParam_Description("规则是否已包含正向测试用例")),
			aitool.WithBoolParam("has_negative_tests", aitool.WithParam_Description("规则是否已包含反向测试用例")),
		))
	if err != nil {
		log.Errorf("register sf_test_cases_completion failed: %v", err)
		return
	}
}
