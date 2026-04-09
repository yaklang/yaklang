package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
)

func loadPropertyPathDepthLimitRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("java/cwe-400-uncontrolled-resource-consumption/java-recursive-property-path-parser-without-depth-limit.sf")
	if !ok {
		t.Skip("java-recursive-property-path-parser-without-depth-limit.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "java-recursive-property-path-parser-without-depth-limit.sf 内容为空")
	return content
}

func TestPropertyPathDepthLimitRule_Positive(t *testing.T) {
	rule := loadPropertyPathDepthLimitRule(t)
	code := `
import java.util.List;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

class PropertyPathParser {
    static Object create(String source, Object type, String addTail, List<Object> base) {
        Pattern pattern = Pattern.compile("\\p{Lu}+\\p{Ll}*$");
        Matcher matcher = pattern.matcher(source);
        if (matcher.find()) {
            return create("head", type, "tail" + addTail, base);
        }
        return source;
    }
}
`

	counts := runJavaRule(t, rule, "PropertyPathParser.java", code)
	assert.Greater(t, counts["risk"], 0, "递归属性路径解析缺少深度限制时应触发告警")
}

func TestPropertyPathDepthLimitRule_Negative_Guarded(t *testing.T) {
	rule := loadPropertyPathDepthLimitRule(t)
	code := `
import java.util.List;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

class PropertyPathParser {
    static Object create(String source, Object type, String addTail, List<Object> base) {
        if (base.size() > 1000) {
            throw new IllegalArgumentException("depth exceeded");
        }
        Pattern pattern = Pattern.compile("\\p{Lu}+\\p{Ll}*$");
        Matcher matcher = pattern.matcher(source);
        if (matcher.find()) {
            return create("head", type, "tail" + addTail, base);
        }
        return source;
    }
}
`

	counts := runJavaRule(t, rule, "PropertyPathParserSafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts), "存在显式深度限制时不应再报出")
}
