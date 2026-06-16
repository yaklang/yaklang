package loop_http_flow_analyze

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPFlowEvidenceDescription_RestrictsEvidenceFieldScope(t *testing.T) {
	require.Contains(t, recordHTTPFlowEvidenceActionDescription, "the `HTTP_FLOW_EVIDENCE` AITag block")
	require.Contains(t, recordHTTPFlowEvidenceActionDescription, "ONLY when `@action=\"record_http_flow_evidence\"`")
	require.Contains(t, recordHTTPFlowEvidenceActionDescription, "NEVER attach `http_flow_evidence` to `directly_answer`")
	require.Contains(t, httpFlowEvidenceParamDescription, "USE THIS FIELD ONLY IF `@action` IS `record_http_flow_evidence`")
}

func TestHTTPFlowEvidencePrompts_RequireDedicatedActionAndDirectAnswerPayload(t *testing.T) {
	require.Contains(t, persistentInstruction, "`http_flow_evidence` **只允许**出现在 `@action=record_http_flow_evidence` 时")
	require.Contains(t, persistentInstruction, "`directly_answer` 只负责回答用户")
	require.Contains(t, reactiveData, "Record them with the dedicated `record_http_flow_evidence` action only.")
	require.Contains(t, reactiveData, "do not attach an `http_flow_evidence` field to `directly_answer` or any other action")
	// 检查核心示例内容，不依赖具体编号
	require.Contains(t, outputExample, "普通查询，不带 http_flow_evidence")
	require.Contains(t, outputExample, "发现可复用证据时，单独调用 record_http_flow_evidence")
	require.Contains(t, outputExample, "http_flow_evidence 内容较长时，用 HTTP_FLOW_EVIDENCE AITag")
	require.Contains(t, outputExample, "<|HTTP_FLOW_EVIDENCE_CURRENT_NONCE|>")
	require.Contains(t, outputExample, "最终答复时必须给 answer_payload，不带 http_flow_evidence")
	require.Contains(t, outputExample, "最终答复较长时，用 FINAL_ANSWER AITag，不带 http_flow_evidence")
	require.Contains(t, outputExample, "<|FINAL_ANSWER_CURRENT_NONCE|>")
	require.Contains(t, outputExample, "\"@action\": \"directly_answer\"")
	require.NotContains(t, outputExample, "\"@action\": \"directly_answer\",\n  \"http_flow_evidence\"")
	require.NotContains(t, outputExample, "\"@action\": \"dispatch_fuzz_test\",\n  \"http_flow_evidence\"")
}
