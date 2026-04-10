package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
)

func loadDeserializationAllowlistBypassRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("java/cwe-502-untrusted-unserialization/java-empty-deserialization-allowlist-bypass.sf")
	if !ok {
		t.Skip("java-empty-deserialization-allowlist-bypass.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "java-empty-deserialization-allowlist-bypass.sf 内容为空")
	return content
}

func TestDeserializationAllowlistBypassRule_Positive(t *testing.T) {
	rule := loadDeserializationAllowlistBypassRule(t)
	code := `
import java.util.Set;

class ObjectUtils {
    static boolean isEmpty(Object value) { return value == null; }
}

class PatternMatchUtils {
    static boolean simpleMatch(String pattern, String className) { return false; }
}

class SecurityException extends RuntimeException {
    SecurityException(String message) { super(message); }
}

class AllowlistVerifier {
    static void checkAllowedList(Class<?> clazz, Set<String> patterns) {
        if (ObjectUtils.isEmpty(patterns)) {
            return;
        }
        String className = clazz.getName();
        for (String pattern : patterns) {
            if (PatternMatchUtils.simpleMatch(pattern, className)) {
                return;
            }
        }
        throw new SecurityException("Attempt to deserialize unauthorized " + clazz);
    }
}
`

	counts := runJavaRule(t, rule, "AllowlistVerifier.java", code)
	assert.Greater(t, counts["risk"], 0, "空 allowlist 直接放行时应触发告警")
}

func TestDeserializationAllowlistBypassRule_Negative_Gated(t *testing.T) {
	rule := loadDeserializationAllowlistBypassRule(t)
	code := `
import java.util.Set;

class ObjectUtils {
    static boolean isEmpty(Object value) { return value == null; }
}

class PatternMatchUtils {
    static boolean simpleMatch(String pattern, String className) { return false; }
}

class SecurityException extends RuntimeException {
    SecurityException(String message) { super(message); }
}

class AllowlistVerifier {
    static void checkAllowedList(Class<?> clazz, Set<String> patterns) {
        String className = clazz.getName();
        for (String pattern : patterns) {
            if (PatternMatchUtils.simpleMatch(pattern, className)) {
                return;
            }
        }
        throw new SecurityException("Attempt to deserialize unauthorized " + clazz);
    }
}
`

	counts := runJavaRule(t, rule, "AllowlistVerifierSafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts), "空 allowlist 只有在额外显式 gate 下放行时不应由该规则报出")
}
