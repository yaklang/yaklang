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
func (r *ReAct) SelectKnowledgeBase(ctx context.Context, originQuery string) (result *aicommon.SelectedKnowledgeBaseResult, err error) {
	r.GetCurrentTask()
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}

	var pr *io.PipeReader
	var pw *io.PipeWriter
	defer func() {
		if pw != nil {
			pw.Close()
		}
	}()
	firstExec := utils.NewOnce()
	// Use the Config-owned auxiliary scheduler so single-model policy can govern
	// this lightweight selection step without coupling it to ReAct.
	r.config.ScheduleAuxiliaryTask(ctx,
		aicommon.CallerLabelSelectKnowledgeBase,
		func() string {
			// Get all available knowledge base names from the database
			allKBNames, loadErr := yakit.GetKnowledgeBaseNameList(consts.GetGormProfileDatabase())
			if loadErr != nil {
				err = utils.Errorf("failed to load all knowledge base names: %v", loadErr)
				return ""
			}

			if len(allKBNames) == 0 {
				err = utils.Error("no knowledge bases available in the system")
				return ""
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

			pr, pw = io.Pipe()
			return prompt
		},
		func(action *aicommon.Action) {
			knowledgeBases := action.GetStringSlice("knowledge_bases")
			reason := action.GetString("reason")
			if reason == "" {
				reason = "AI 根据问题相关性自动选择"
			}

			log.Infof("selected %d knowledge bases: %v, reason: %s", len(knowledgeBases), knowledgeBases, reason)

			result = aicommon.NewSelectedKnowledgeBaseResult(reason, knowledgeBases)
		},
		aicommon.WithAuxiliaryOnError(func(cause error) { err = utils.Wrap(cause, "failed to select knowledge bases via liteforge") }),
		aicommon.WithAuxiliaryOutputs(
			aitool.WithStringArrayParam("knowledge_bases",
				aitool.WithParam_Description("要搜索的知识库名称列表，必须指定至少一个知识库"),
				aitool.WithParam_Required(true),
			),
			aitool.WithStringParam("reason",
				aitool.WithParam_Description("选择这些知识库的理由"),
				aitool.WithParam_Required(true),
			),
		),
		aicommon.WithAuxiliaryOpts(
			aicommon.WithGeneralConfigStreamableFieldEmitterCallback([]string{
				"reason", "knowledge_bases",
			}, func(key string, rd io.Reader, emitter *aicommon.Emitter) {
				if emitter == nil {
					io.Copy(io.Discard, rd)
					return
				}
				firstExec.DoOr(func() {
					emitter.EmitDefaultStreamEvent(
						"search-relative-knowledge-base", pr,
						r.GetCurrentTaskId(),
					)
				}, func() {
					pw.Write([]byte("\n ... "))
				})
				io.Copy(pw, rd)
			}),
		),
	)
	if result == nil && err == nil {
		err = utils.Error("knowledge base selection returned no result")
	}
	return result, err
}
