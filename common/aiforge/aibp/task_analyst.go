package aibp

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
)

func init() {
	lfopts := []aiforge.LiteForgeOption{
		aiforge.WithLiteForge_Prompt(`# 
请分析此任务的目标、范围和执行步骤，解释其在整体规划中的作用及与其他任务的关联性。从任务描述中提取反映核心内容的关键词（包括动作、对象、条件和预期结果），并按重要性排序。同时标注可能影响任务执行的约束条件和风险因素。
## 注意
1. 你运行在一个由外部思维链约束的任务中，尽量保持输出简短，保留任务相关元素，避免冗长描述`),
		aiforge.WithLiteForge_OutputSchema(
			aitool.WithStringParam(
				"description",
				aitool.WithParam_Required(true),
				aitool.WithParam_MaxLength(100),
				aitool.WithParam_Description("description in Chinese"),
			),
			aitool.WithStringArrayParam(
				"keywords",
				aitool.WithParam_Required(true),
				aitool.WithParam_MaxLength(20),
				aitool.WithParam_Description("task keyword in Chinese"),
			),
		),
	}

	err := aiforge.RegisterAIDBuildInForge("task-analyst", lfopts...)
	if err != nil {
		log.Errorf("register task-analyst forge failed: %s", err)
	}
	err = aiforge.RegisterLiteForge("task-analyst", lfopts...)
	if err != nil {
		log.Errorf("register task-analyst forge failed: %s", err)
	}
}
