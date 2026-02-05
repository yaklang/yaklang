package aireact

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// SelectKnowledgeBase selects appropriate knowledge bases based on the user query
// It uses AI to analyze the query and match it with available knowledge bases
func (r *ReAct) SelectKnowledgeBase(ctx context.Context, originQuery string) (*aicommon.SelectedKnowledgeBaseResult, error) {
	r.GetCurrentTask()
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}

	// Get all available knowledge base names from the database
	allKBNames, err := yakit.GetKnowledgeBaseNameList(consts.GetGormProfileDatabase())
	if err != nil {
		return nil, utils.Errorf("failed to load all knowledge base names: %v", err)
	}

	if len(allKBNames) == 0 {
		return nil, utils.Errorf("no knowledge bases available in the system")
	}

	// Build the knowledge base list for the prompt
	buf := bytes.NewBufferString("")
	for _, kb := range allKBNames {
		buf.WriteString(fmt.Sprintf("- %#v\n", kb))
	}

	log.Info("start to select knowledge bases via liteforge")
	nonce := utils.RandStringBytes(4)
	prompt := utils.MustRenderTemplate(`
<|INSTRUCT_{{ .nonce }}|>
你是一个知识库选择助手。你的任务是根据用户的问题，从可用的知识库列表中选择最相关的知识库。
请分析用户的问题，选择最可能包含相关信息的知识库。
<|INSTRUCT_END{{ .nonce }}|>

<|ALL_EXISTED_KNOWLEDGE_BASES_{{ .nonce }}|>
{{ .knowledgeBases }}
<|ALL_EXISTED_KNOWLEDGE_BASES_END_{{ .nonce }}|>

<|USER_QUERY_{{ .nonce }}|>
{{ .userQuery }}
<|USER_QUERY_END_{{ .nonce }}|>
`, map[string]any{
		"nonce":          nonce,
		"knowledgeBases": buf.String(),
		"userQuery":      originQuery,
	})
	log.Infof("start to select knowledge bases from: \n%v", prompt)

	pr, pw := io.Pipe()
	defer pw.Close()
	firstExec := utils.NewOnce()
	// Use liteforge to select knowledge bases
	action, err := r.InvokeLiteForge(ctx, "select_knowledge_base", prompt, []aitool.ToolOption{
		aitool.WithStringArrayParam("knowledge_bases",
			aitool.WithParam_Description("要搜索的知识库名称列表，必须指定至少一个知识库"),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam("reason",
			aitool.WithParam_Description("选择这些知识库的理由"),
			aitool.WithParam_Required(true),
		),
	}, aicommon.WithGeneralConfigStreamableFieldCallback([]string{
		"reason", "knowledge_bases",
	}, func(key string, rd io.Reader) {
		firstExec.DoOr(func() {
			r.GetConfig().GetEmitter().EmitDefaultStreamEvent(
				"search-relative-knowledge-base", pr,
				r.GetCurrentTaskId(),
			)
		}, func() {
			pw.Write([]byte("\n ... "))
		})
		io.Copy(pw, rd)
	}))
	if err != nil {
		return nil, utils.Errorf("failed to select knowledge bases via liteforge: %v", err)
	}

	knowledgeBases := action.GetStringSlice("knowledge_bases")
	reason := action.GetString("reason")
	if reason == "" {
		reason = "AI 根据问题相关性自动选择"
	}

	log.Infof("selected %d knowledge bases: %v, reason: %s", len(knowledgeBases), knowledgeBases, reason)

	return aicommon.NewSelectedKnowledgeBaseResult(reason, knowledgeBases), nil
}
