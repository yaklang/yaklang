package java

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func initHardcodeRule(t *testing.T) string {
	t.Helper()
	yakit.InitialDatabase()
	require.NoError(t, sfbuildin.SyncEmbedRule())
	rule, err := sfdb.GetRulePure("检测通用硬编码凭据")
	require.NoError(t, err, "buildin rule '检测通用硬编码凭据' not found")
	return rule.Content
}

// TestHardcodeCredentials_NoFalsePositive 验证规则对反编译 Java 伪代码中的普通变量不误报。
// 这些代码行来自 spring-context JAR 反编译结果，历史上因旧正则 access[(token)|(key)] 触发误报。
func TestHardcodeCredentials_NoFalsePositive(t *testing.T) {
	ruleContent := initHardcodeRule(t)

	vfs := filesys.NewVirtualFs()
	vfs.AddFile("App.java", `
public class App {
    private final Class temporalAccessorType = var1;
    IllegalAccessException var3 = Exception;
    void init() {
        this.directFieldAccessor = this.createDirectFieldAccessor();
        if ((this.directFieldAccessor) == (null)) {}
        if (((this.messageSourceAccessor) == (null)) && (this.isContextRequired())) {}
        this.messageSourceAccessor = new MessageSourceAccessor(var1);
        this.messageSourceAccessor = null;
    }
}
`)

	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		result, err := programs.SyntaxFlowWithError(ruleContent)
		require.NoError(t, err)

		total := 0
		for _, name := range result.GetAlertVariables() {
			vals := result.GetValues(name)
			total += len(vals)
			for _, v := range vals {
				t.Logf("误报: %s = %v", name, v)
			}
		}
		if total > 0 {
			t.Errorf("期望零告警（不应误报），实际命中 %d 处", total)
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

// TestHardcodeCredentials_DetectRealSecrets 验证规则能正确命中真实的硬编码凭据。
func TestHardcodeCredentials_DetectRealSecrets(t *testing.T) {
	ruleContent := initHardcodeRule(t)

	vfs := filesys.NewVirtualFs()
	content, err := codec.DecodeBase64("cHVibGljIGNsYXNzIENvbmZpZyB7CiAgICAvLyBBV1MgQWNjZXNzIEtleSBJRAogICAgc3RhdGljIFN0cmluZyBhd3NLZXlJZCA9ICJBS0lBSU9TRk9ETk43RVhBTVBMRTEyMzQiOwogICAgLy8gR2l0SHViIFBlcnNvbmFsIEFjY2VzcyBUb2tlbgogICAgc3RhdGljIFN0cmluZyBnaXRodWJUb2tlbiA9ICJnaHBfYUJjRGVGZ0hpSmtMbU5vUHFSc1R1VndYeVoxMjM0NTY3ODkiOwogICAgLy8gR29vZ2xlIENsb3VkIEFQSSBLZXkKICAgIHN0YXRpYyBTdHJpbmcgZ2NwQXBpS2V5ID0gIkFJemFTeUQtOXRTcmtlNzJJNmUwRFZ3ZE9QekM2ZXhhbXBsZTEyIjsKICAgIC8vIFN0cmlwZSBzZWNyZXQga2V5CiAgICBzdGF0aWMgU3RyaW5nIHN0cmlwZUtleSA9ICJza19saXZlX2FiY2RlZmdoaWprbG1ub3BxcnN0dXZ3eCI7CiAgICAvLyBhY2Nlc3Nfa2V5IOaYjuaWh+i1i+WAvAogICAgc3RhdGljIFN0cmluZyBhY2Nlc3Nfa2V5ID0gIm15LWhhcmRjb2RlZC1zZWNyZXQtdmFsdWUiOwp9")
	require.NoError(t, err)
	vfs.AddFile("Config.java", string(content))

	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		result, err := programs.SyntaxFlowWithError(ruleContent)
		require.NoError(t, err)

		total := 0
		for _, name := range result.GetAlertVariables() {
			total += len(result.GetValues(name))
		}
		if total == 0 {
			t.Error("期望命中硬编码凭据，但规则未报告任何告警")
		} else {
			t.Logf("命中 %d 处硬编码凭据，符合预期", total)
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
