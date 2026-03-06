package aibp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/thirdparty_bin"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSFRuleFromTestCases_ForgeRegistered(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skip in CI: ExecuteForge may hang without AI config")
	}
	// 验证 forge 已注册：调用 ExecuteForge 不应返回 "forge not found"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := aiforge.ExecuteForge(
		"sf_rule_from_test_cases",
		ctx,
		[]*ypb.ExecParamItem{
			{Key: "positive_test_cases", Value: `[{"filename":"test.java","content":"String x = request.getParameter(\"id\");"}]`},
			{Key: "language", Value: "java"},
		},
		aicommon.WithAgreeYOLO(true),
	)
	if err != nil && strings.Contains(err.Error(), "forge not found") {
		t.Fatalf("forge sf_rule_from_test_cases should be registered: %v", err)
	}
}

func TestValidateSFRule(t *testing.T) {
	tests := []struct {
		name        string
		ruleContent string
		expectPass  bool
		expectErr   string
	}{
		{"compile_success", `desc(
	title: "Test"
	title_zh: "测试"
)
a as $x;
check $x;
alert $x;
`, true, ""},
		{"compile_fail", `desc(
invalid syntax here [
`, false, "compile failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateSFRule(tt.ruleContent, false)
			require.Equal(t, tt.expectPass, result.Passed, "ValidateSFRule(%s): %v", tt.name, result.Errors)
			if tt.expectErr != "" && len(result.Errors) > 0 {
				require.Contains(t, result.Errors[0], tt.expectErr)
			}
		})
	}
}

func TestValidateSFRule_StrictMode(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skip strict mode test in CI (may require DB/long runtime)")
	}
	// 使用 yaklang 的最小规则，与 buildin-rule-test 中的 TestBuildInRule_Verify_Positive_AlertMin2 类似
	ruleContent := `
desc(
	alert_min: 1,
	language: yaklang,
	'file://a.yak': <<<EOF
b = () => {
	a = 1;
}
EOF
)

a as $output;
check $output;
alert $output;
`
	result := ValidateSFRule(ruleContent, true)
	require.True(t, result.Passed, "minimal yaklang rule should pass strict validation: %v", result.Errors)
}

func TestSFRuleFromTestCases_ExecuteWithAICallback(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skip in CI: requires MockAICallback and no openrouter.txt")
	}
	// 使用 Mock 模拟 AI 返回，无需 openrouter.txt
	// LiteForge 期望 @action/tool/params 包装格式；Mock 按 prompt-response 对返回，需多对以支持 retry
	paramsObj := map[string]interface{}{
		"rule_content": "desc(\n\ttitle: \"Test Java SQL Injection\"\n\ttitle_zh: \"测试 Java SQL 注入\"\n)\na as $x;\ncheck $x;\nalert $x;\n",
		"title":        "Test Java SQL Injection",
		"title_zh":     "测试 Java SQL 注入",
		"cwe":          89,
		"summary":      "检测 Java 中 SQL 拼接导致的注入",
	}
	callToolResp := map[string]interface{}{
		"@action": "call-tool",
		"tool":    "output",
		"params":  paramsObj,
	}
	respBytes, _ := json.MarshalIndent(callToolResp, "", "  ")
	// 提供多对以支持 retry 时 Mock 不越界
	mockContent, _ := json.Marshal([]string{"*", string(respBytes), "*", string(respBytes)})

	positiveCases := `[{"filename":"Vuln.java","content":"String id = request.getParameter(\\"id\\");\\nString sql = \\"SELECT * FROM users WHERE id=\\" + id;\\nstmt.executeQuery(sql);","description":"SQL concatenation"}]`
	results, err := aiforge.ExecuteForge(
		"sf_rule_from_test_cases",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "positive_test_cases", Value: positiveCases},
			{Key: "language", Value: "java"},
			{Key: "vulnerability_type", Value: "SQL注入"},
		},
		aicommon.WithAgreeYOLO(true),
		aicommon.WithAICallback(aiforge.MockAICallbackByRecord(mockContent)),
	)
	require.NoError(t, err)
	require.NotNil(t, results)
	require.NotNil(t, results.Action)
	params := results.Action.GetInvokeParams("params")
	require.NotNil(t, params)
	compileResult := ValidateSFRule(params.GetString("rule_content"), false)
	require.True(t, compileResult.Passed, "generated rule should compile: %v", compileResult.Errors)
}

// TestSyntaxFlowAIKB_RAGQuery 测试 syntaxflow-aikb.rag 的导入与语义检索
func TestSyntaxFlowAIKB_RAGQuery(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("skip RAG test in CI: requires syntaxflow-aikb.rag and embedding service")
	}

	ragPath, err := thirdparty_bin.GetBinaryPath("syntaxflow-aikb-rag")
	if err != nil {
		t.Skipf("syntaxflow-aikb-rag not installed (run: yak thirdparty install syntaxflow-aikb-rag or scripts/build-syntaxflow-aikb-to-libs.cmd)")
	}

	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	db.AutoMigrate(
		&schema.VectorStoreCollection{},
		&schema.VectorStoreDocument{},
		&schema.KnowledgeBaseInfo{},
		&schema.KnowledgeBaseEntry{},
		&schema.EntityRepository{},
		&schema.ERModelEntity{},
		&schema.ERModelRelationship{},
	)

	// 导入 RAG（使用临时 DB，避免污染 profile）
	err = rag.ImportRAG(ragPath, rag.WithDB(db), rag.WithExportOverwriteExisting(true))
	require.NoError(t, err, "import syntaxflow-aikb.rag failed")

	// 语义检索（需指定 DB 与集合名；禁用 enhance 以加快测试）
	results, err := rag.SimpleQuery(db, "dataflow 数据流追踪", 5,
		rag.WithDB(db),
		rag.WithRAGCollectionNames("yaklang-syntaxflow-aikb"),
		rag.WithRAGEnhance(""))
	require.NoError(t, err)
	require.NotEmpty(t, results, "query should return at least one result for SyntaxFlow-related question")
	t.Logf("RAG query returned %d results, first score=%.3f", len(results), results[0].Score)
}
