package java

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
)

func loadSecurityContextSaveGatedRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("java/cwe-287-improper-authentication/java-security-context-save-gated-by-contextsaved-flag.sf")
	if !ok {
		t.Skip("java-security-context-save-gated-by-contextsaved-flag.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "java-security-context-save-gated-by-contextsaved-flag.sf 内容为空")
	return content
}

func TestSecurityContextSaveGatedRule_Positive(t *testing.T) {
	rule := loadSecurityContextSaveGatedRule(t)
	code := `
class SecurityContext {
}

class SaveContextOnUpdateOrErrorResponseWrapper {
    boolean isContextSaved() { return false; }
    void saveContext(SecurityContext context) {
    }
}

class Repository {
    void saveContext(SecurityContext context, SaveContextOnUpdateOrErrorResponseWrapper responseWrapper) {
        if (!responseWrapper.isContextSaved()) {
            responseWrapper.saveContext(context);
        }
    }
}
`

	counts := runJavaRule(t, rule, "Repository.java", code)
	assert.Greater(t, counts["risk"], 0, "saveContext 被 isContextSaved() 门控时应触发告警")
}

func TestSecurityContextSaveGatedRule_Negative_DirectSave(t *testing.T) {
	rule := loadSecurityContextSaveGatedRule(t)
	code := `
class SecurityContext {
}

class SaveContextOnUpdateOrErrorResponseWrapper {
    void saveContext(SecurityContext context) {
    }
}

class Repository {
    void saveContext(SecurityContext context, SaveContextOnUpdateOrErrorResponseWrapper responseWrapper) {
        responseWrapper.saveContext(context);
    }
}
`

	counts := runJavaRule(t, rule, "RepositorySafe.java", code)
	assert.Equal(t, 0, totalAlerts(counts), "直接保存 SecurityContext 时不应由该规则报出")
}
