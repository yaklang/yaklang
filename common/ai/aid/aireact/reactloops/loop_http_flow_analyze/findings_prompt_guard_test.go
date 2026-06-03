package loop_http_flow_analyze

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOutputFindingsDescription_RestrictsFindingsFieldScope(t *testing.T) {
	require.Contains(t, outputFindingsActionDescription, "the `FINDINGS` AITag block")
	require.Contains(t, outputFindingsActionDescription, "ONLY when `@action=\"output_findings\"`")
	require.Contains(t, outputFindingsActionDescription, "NEVER attach `findings` to `directly_answer`")
	require.Contains(t, outputFindingsParamDescription, "USE THIS FIELD ONLY IF `@action` IS `output_findings`")
}

func TestFindingsPrompts_RequireDedicatedActionAndDirectAnswerPayload(t *testing.T) {
	require.Contains(t, persistentInstruction, "`findings` **只允许**出现在 `@action=output_findings` 时")
	require.Contains(t, persistentInstruction, "`directly_answer` 只负责回答用户")
	require.Contains(t, reactiveData, "Record them with the dedicated `output_findings` action only.")
	require.Contains(t, reactiveData, "do not attach a `findings` field to `directly_answer` or any other action")
	require.Contains(t, outputExample, "### 示例 A - 普通查询，不带 findings")
	require.Contains(t, outputExample, "### 示例 B - 发现可复用结论时，单独调用 output_findings")
	require.Contains(t, outputExample, "### 示例 C - findings 内容较长时，用 FINDINGS AITag")
	require.Contains(t, outputExample, "<|FINDINGS_CURRENT_NONCE|>")
	require.Contains(t, outputExample, "### 示例 D - 最终答复时必须给 answer_payload，不带 findings")
	require.Contains(t, outputExample, "### 示例 E - 最终答复较长时，用 FINAL_ANSWER AITag，不带 findings")
	require.Contains(t, outputExample, "<|FINAL_ANSWER_CURRENT_NONCE|>")
	require.Contains(t, outputExample, "\"@action\": \"directly_answer\"")
	require.NotContains(t, outputExample, "\"@action\": \"directly_answer\",\n  \"findings\"")
	require.NotContains(t, outputExample, "\"@action\": \"dispatch_fuzz_test\",\n  \"findings\"")
}
