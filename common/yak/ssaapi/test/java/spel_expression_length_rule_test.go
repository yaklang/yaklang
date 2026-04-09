package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
)

func loadSpelExpressionLengthRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("java/cwe-400-uncontrolled-resource-consumption/java-spel-expression-length-without-limit.sf")
	if !ok {
		t.Skip("java-spel-expression-length-without-limit.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "java-spel-expression-length-without-limit.sf 内容为空")
	return content
}

func TestSpelExpressionLengthRule_Positive(t *testing.T) {
	rule := loadSpelExpressionLengthRule(t)
	code := `
class Tokenizer {
    Tokenizer(String expressionString) {
    }
}

class InternalSpelExpressionParser {
    Object doParseExpression(String expressionString, Object context) {
        Tokenizer tokenizer = new Tokenizer(expressionString);
        return tokenizer;
    }
}
`

	counts := runJavaRule(t, rule, "InternalSpelExpressionParser.java", code)
	assert.Greater(t, counts["risk"], 0, "直接进入 tokenizer 且缺少长度检查时应触发告警")
}

func TestSpelExpressionLengthRule_Negative_Checked(t *testing.T) {
	rule := loadSpelExpressionLengthRule(t)
	code := `
class Tokenizer {
    Tokenizer(String expressionString) {
    }
}

class InternalSpelExpressionParser {
    Object doParseExpression(String expressionString, Object context) {
        checkExpressionLength(expressionString);
        Tokenizer tokenizer = new Tokenizer(expressionString);
        return tokenizer;
    }

    void checkExpressionLength(String expressionString) {
    }
}
`

	counts := runJavaRule(t, rule, "InternalSpelExpressionParserSafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts), "存在长度检查时不应再报出")
}
