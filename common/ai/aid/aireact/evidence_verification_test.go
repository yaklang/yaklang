package aireact

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	aicommon_testutil "github.com/yaklang/yaklang/common/ai/aid/aicommon/testutil"
)

func TestVerifyUserSatisfaction_AcceptsEvidenceAITag(t *testing.T) {
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			nonce := aicommon_testutil.MustExtractPromptNonce(t, req.GetPrompt(), "INPUT")
			rawResponse := `{"@action":"verify-satisfaction","user_satisfied":false,"reasoning":"still verifying","evidence":[]}

<|EVIDENCE_` + nonce + `|>
## 某一个事实发现
1. 主体: 接口 GET /api/users
发现: 当前返回 401，需要补充认证头后复测。
<|EVIDENCE_END_` + nonce + `|>`
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(rawResponse))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	result, err := ins.VerifyUserSatisfaction(context.Background(), "verify evidence aitag", true, "tool executed: continue")
	require.NoError(t, err)
	require.False(t, result.Satisfied)
	require.Contains(t, result.Evidence, "主体: 接口 GET /api/users")
	require.Contains(t, result.Evidence, "发现: 当前返回 401")
}

func TestVerifyUserSatisfaction_AcceptsEvidenceJSONArray(t *testing.T) {
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rawResponse := `{"@action":"verify-satisfaction","user_satisfied":false,"reasoning":"still verifying","evidence":[{"op":"add","id":"api_401","content":"主体: GET /api/users\n动作: 发送无认证请求\n观测: 401\n控制含义: 需补认证头"},{"op":"add","id":"report_md","content":"主体: /tmp/report.md\n动作: 读取\n观测: 已生成但缺SSRF章节\n控制含义: 后续补充"}]}`
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(rawResponse))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	result, err := ins.VerifyUserSatisfaction(context.Background(), "verify evidence json array", true, "tool executed: continue")
	require.NoError(t, err)
	require.False(t, result.Satisfied)
	require.NotEmpty(t, result.EvidenceOps)
	require.Len(t, result.EvidenceOps, 2)
	require.Equal(t, "add", result.EvidenceOps[0].Op)
	require.Equal(t, "api_401", result.EvidenceOps[0].ID)
	require.Contains(t, result.EvidenceOps[0].Content, "GET /api/users")
	require.Equal(t, "report_md", result.EvidenceOps[1].ID)
}
