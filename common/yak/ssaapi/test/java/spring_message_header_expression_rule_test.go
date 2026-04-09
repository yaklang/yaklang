package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
)

func loadSpringMessageHeaderExpressionRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("java/cwe-94-code-injection/java-spring-message-header-expression-evaluation.sf")
	if !ok {
		t.Skip("java-spring-message-header-expression-evaluation.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "java-spring-message-header-expression-evaluation.sf 内容为空")
	return content
}

func TestSpringMessageHeaderExpressionRule_Positive_NativeHeader(t *testing.T) {
	rule := loadSpringMessageHeaderExpressionRule(t)
	code := `
import org.springframework.expression.Expression;
import org.springframework.expression.spel.standard.SpelExpressionParser;

class MessageHeaders {
}

class NativeMessageHeaderAccessor {
    static String getFirstNativeHeader(String name, MessageHeaders headers) {
        return null;
    }
}

class SelectorExpressionRegistry {
    private final SpelExpressionParser expressionParser = new SpelExpressionParser();

    Expression bad(MessageHeaders headers, String selectorHeaderName) {
        String selector = NativeMessageHeaderAccessor.getFirstNativeHeader(selectorHeaderName, headers);
        return this.expressionParser.parseExpression(selector);
    }
}
`

	counts := runJavaRule(t, rule, "SelectorExpressionRegistry.java", code)
	assert.Greater(t, counts["risk"], 0, "native header 派生字符串进入 parseExpression 时应触发告警")
}

func TestSpringMessageHeaderExpressionRule_Positive_NativeHeaderForwarding(t *testing.T) {
	rule := loadSpringMessageHeaderExpressionRule(t)
	code := `
import org.springframework.expression.Expression;
import org.springframework.expression.ExpressionParser;
import org.springframework.expression.spel.standard.SpelExpressionParser;

class MessageHeaders {
}

class NativeMessageHeaderAccessor {
    static String getFirstNativeHeader(String name, MessageHeaders headers) {
        return null;
    }
}

class NativeHeaderExpressionController {
    private final ExpressionParser parser = new SpelExpressionParser();

    Expression bad(MessageHeaders headers, String selectorHeaderName) {
        String expression = NativeMessageHeaderAccessor.getFirstNativeHeader(selectorHeaderName, headers);
        return this.parser.parseExpression(expression);
    }
}
`

	counts := runJavaRule(t, rule, "NativeHeaderExpressionController.java", code)
	assert.Greater(t, counts["risk"], 0, "native header 转发到 parseExpression 时应触发告警")
}

func TestSpringMessageHeaderExpressionRule_Negative_FixedExpression(t *testing.T) {
	rule := loadSpringMessageHeaderExpressionRule(t)
	code := `
import org.springframework.expression.Expression;
import org.springframework.expression.spel.standard.SpelExpressionParser;

class FixedExpressionRegistry {
    private final SpelExpressionParser expressionParser = new SpelExpressionParser();

    Expression safe() {
        return this.expressionParser.parseExpression("headers.foo == 'bar'");
    }
}
`

	counts := runJavaRule(t, rule, "FixedExpressionRegistry.java", code)
	assert.Equal(t, 0, totalAlerts(counts), "固定表达式字符串不应由 header 表达式规则报出")
}

func TestSpringMessageHeaderExpressionRule_Negative_HeaderReadOnly(t *testing.T) {
	rule := loadSpringMessageHeaderExpressionRule(t)
	code := `
class MessageHeaders {
}

class HeaderReadOnly {
    String safe(MessageHeaders headers) {
        return null;
    }
}
`

	counts := runJavaRule(t, rule, "HeaderReadOnly.java", code)
	assert.Equal(t, 0, totalAlerts(counts), "仅仅读取 header 但未交给 parseExpression 时不应触发告警")
}
