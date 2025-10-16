package aimem

import (
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// AddRawText 从原始文本生成记忆条目
func (r *AIMemoryTriage) AddRawText(i string) ([]*MemoryEntity, error) {
	temp, infos, err := r.invoker.GetBasicPromptInfo(nil)
	if err != nil {
		return nil, utils.Errorf("GetBasicPromptInfo failed: %v", err)
	}
	basic, err := utils.RenderTemplate(temp, infos)
	if err != nil {
		return nil, utils.Errorf("RenderTemplate failed: %v", err)
	}

	nonce := utils.RandStringBytes(4)

	var dynContext string
	if r.contextProvider != nil {
		dynContext, err = r.contextProvider()
		if err != nil {
			return nil, utils.Errorf("contextProvider failed: %v", err)
		}
	}

	existedTag, err := r.GetDynamicContextWithTags()
	if err != nil {
		return nil, utils.Errorf("GetDynamicContextWithTags failed: %v", err)
	}
	dynContext += existedTag

	promptResult, err := utils.RenderTemplate(memoryTriagePrompt, map[string]any{
		"Basic":              basic,
		"Nonce":              nonce,
		"Query":              i,
		"HaveDynamicContext": dynContext != "",
		"DynamicContext":     dynContext,
		"CorePactPrinciple":  corepactPrinciplesPrompt,
	})
	if err != nil {
		return nil, err
	}

	ac, err := r.invoker.InvokeLiteForge(r.ctx, "memory-triage", promptResult, []aitool.ToolOption{
		aitool.WithStructArrayParam(
			"memory_entities",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("根据用户的输入的内容，分析用户行为，生成一个或多个记忆条目"),
			},
			[]aitool.PropertyOption{
				aitool.WithParam_Description("记忆实体，Content 为要记忆的东西，Tags 为要记忆的标签（领域），"),
			},
			aitool.WithStringParam("content", aitool.WithParam_Description("需要作为记忆内容，你需要摘出来，尽量包含原文，尊重原意，去掉一些不和谐的东西或者乱七八糟的符号，如果有代码的话，保持代码原文")),
			aitool.WithStringArrayParam("tags", aitool.WithParam_Description("要记忆的标签（领域）,不要太多，尊重备选内容，如果万不得已你再创建新的，如果已经有了记忆标签，请你使用已有的")),
			aitool.WithStringArrayParam("potential_questions", aitool.WithParam_Description("这个记忆可能引发的问题，你可以根据这个记忆生成一些问题，用于后续的RAG搜索")),
			aitool.WithNumberParam("t", aitool.WithParam_Description("时效评分，核心问题：这个记忆应该如何被保留?"), aitool.WithParam_Min(0.0), aitool.WithParam_Max(1.0)),
			aitool.WithNumberParam("a", aitool.WithParam_Description("可操作性评分，是否可以从学习中改进未来行为？"), aitool.WithParam_Min(0.0), aitool.WithParam_Max(1.0)),
			aitool.WithNumberParam("p", aitool.WithParam_Description("个人偏好评分，这个行为或者问题是否绑定了用户个人风格，品味？"), aitool.WithParam_Min(0.0), aitool.WithParam_Max(1.0)),
			aitool.WithNumberParam("o", aitool.WithParam_Description("来源与确定性评分，这个信息从哪来？有多可信？"), aitool.WithParam_Min(0.0), aitool.WithParam_Max(1.0)),
			aitool.WithNumberParam("e", aitool.WithParam_Description("情感评分，用户在表达这个信息时的情绪如何？越低越消极，消极评分时一般伴随信息源不可信"), aitool.WithParam_Min(0.0), aitool.WithParam_Max(1.0)),
			aitool.WithNumberParam("r", aitool.WithParam_Description("相关性评分，这个信息对用户的目的有多关键？无关紧要？锦上添花？还是成败在此一举？"), aitool.WithParam_Min(0.0), aitool.WithParam_Max(1.0)),
			aitool.WithNumberParam("c", aitool.WithParam_Description("关联度评分，这个记忆与其他记忆如何关联？这是一个一次性事实，几乎与其他事实没有什么关联程度"), aitool.WithParam_Min(0.0), aitool.WithParam_Max(1.0)),
		),
	})
	if err != nil {
		return nil, utils.Errorf("InvokeLiteForge failed: %v", err)
	}
	result := ac.GetInvokeParamsArray("memory_entities")
	if len(result) == 0 {
		return nil, utils.Errorf("no memory entities found")
	}

	var entities []*MemoryEntity
	for _, item := range result {
		id := uuid.New().String()
		entity := &MemoryEntity{
			Id:                 id,
			CreatedAt:          time.Now(),
			Content:            item.GetString("content"),
			Tags:               item.GetStringSlice("tags"),
			PotentialQuestions: item.GetStringSlice("potential_questions"),
			T_Score:            item.GetFloat("t"),
			A_Score:            item.GetFloat("a"),
			P_Score:            item.GetFloat("p"),
			O_Score:            item.GetFloat("o"),
			E_Score:            item.GetFloat("e"),
			R_Score:            item.GetFloat("r"),
			C_Score:            item.GetFloat("c"),
		}
		entity.CorePactVector = []float32{
			float32(entity.C_Score),
			float32(entity.O_Score),
			float32(entity.R_Score),
			float32(entity.E_Score),
			float32(entity.P_Score),
			float32(entity.A_Score),
			float32(entity.T_Score),
		}
		entities = append(entities, entity)
	}
	return entities, nil
}
