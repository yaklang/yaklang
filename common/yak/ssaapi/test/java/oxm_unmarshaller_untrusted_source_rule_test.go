package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
)

func loadOxmUnmarshallerUntrustedSourceRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("java/cwe-611-xxe/java-oxm-unmarshaller-untrusted-source.sf")
	if !ok {
		t.Skip("java-oxm-unmarshaller-untrusted-source.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "java-oxm-unmarshaller-untrusted-source.sf 内容为空")
	return content
}

func TestOxmUnmarshallerUntrustedSourceRule_Positive(t *testing.T) {
	rule := loadOxmUnmarshallerUntrustedSourceRule(t)
	code := `
class StringSource {
    StringSource(String payload) {
    }
}

interface Unmarshaller {
    Object unmarshal(Object source) throws java.io.IOException;
}

class UnmarshallingTransformer {
    private final Unmarshaller unmarshaller = null;

    Object transformPayload(String payload) throws Exception {
        Object source = new StringSource(payload);
        return this.unmarshaller.unmarshal(source);
    }
}
`

	counts := runJavaRule(t, rule, "UnmarshallingTransformer.java", code)
	assert.Greater(t, counts["risk"], 0, "不可信 XML Source 进入 unmarshal 时应触发告警")
}

func TestOxmUnmarshallerUntrustedSourceRule_Positive_SourceFactory(t *testing.T) {
	rule := loadOxmUnmarshallerUntrustedSourceRule(t)
	code := `
interface SourceFactory {
    Object createSource(Object payload);
}

interface Unmarshaller {
    Object unmarshal(Object source) throws java.io.IOException;
}

class RuntimeSourceTransformer {
    private final Unmarshaller unmarshaller = null;
    private final SourceFactory sourceFactory = null;

    Object transformPayload(Object payload) throws Exception {
        Object source = this.sourceFactory.createSource(payload);
        return this.unmarshaller.unmarshal(source);
    }
}
`

	counts := runJavaRule(t, rule, "RuntimeSourceTransformer.java", code)
	assert.Greater(t, counts["risk"], 0, "SourceFactory 创建的运行时 Source 进入 unmarshal 时应触发告警")
}

func TestOxmUnmarshallerUntrustedSourceRule_Negative_DomSource(t *testing.T) {
	rule := loadOxmUnmarshallerUntrustedSourceRule(t)
	code := `
class DOMSource {
    DOMSource(Object document) {
    }
}

interface Unmarshaller {
    Object unmarshal(Object source) throws java.io.IOException;
}

class SafeTransformer {
    private final Unmarshaller unmarshaller = null;

    Object transformPayload() throws Exception {
        Object source = new DOMSource(new Object());
        return this.unmarshaller.unmarshal(source);
    }
}
`

	counts := runJavaRule(t, rule, "SafeTransformer.java", code)
	assert.Equal(t, 0, totalAlerts(counts), "本地 DOMSource 不应由这条规则报出")
}
