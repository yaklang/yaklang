package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
)

func loadSpelArrayConstructionRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("java/cwe-400-uncontrolled-resource-consumption/java-spel-array-construction-without-size-limit.sf")
	if !ok {
		t.Skip("java-spel-array-construction-without-size-limit.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "java-spel-array-construction-without-size-limit.sf 内容为空")
	return content
}

func TestSpelArrayConstructionRule_Positive(t *testing.T) {
	rule := loadSpelArrayConstructionRule(t)
	code := `
import java.lang.reflect.Array;

class ExpressionUtils {
    static int toInt(Object converter, Object value) {
        return 0;
    }
}

class State {
    Class<?> findType(String type) {
        return Object.class;
    }
}

class UnsafeArrayConstructor {
    Object createArray(State state, Object converter, Object value, String type) {
        int arraySize = ExpressionUtils.toInt(converter, value);
        Class<?> componentType = state.findType(type);
        return Array.newInstance(componentType, arraySize);
    }
}
`

	counts := runJavaRule(t, rule, "UnsafeArrayConstructor.java", code)
	assert.Greater(t, counts["risk"], 0, "表达式驱动的数组构造缺少上限检查时应触发告警")
}

func TestSpelArrayConstructionRule_Negative_Checked(t *testing.T) {
	rule := loadSpelArrayConstructionRule(t)
	code := `
import java.lang.reflect.Array;

class ExpressionUtils {
    static int toInt(Object converter, Object value) {
        return 0;
    }
}

class State {
    Class<?> findType(String type) {
        return Object.class;
    }
}

class SafeArrayConstructor {
    Object createArray(State state, Object converter, Object value, String type) {
        int arraySize = ExpressionUtils.toInt(converter, value);
        Class<?> componentType = state.findType(type);
        checkNumElements(arraySize);
        return Array.newInstance(componentType, arraySize);
    }

    void checkNumElements(int arraySize) {
    }
}
`

	counts := runJavaRule(t, rule, "SafeArrayConstructor.java", code)
	assert.Equal(t, 0, totalAlerts(counts), "同一函数内存在 checkNumElements 时不应再报出")
}
