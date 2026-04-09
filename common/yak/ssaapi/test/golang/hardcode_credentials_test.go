package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func TestGeneralHardcodeCredentials_NoFalsePositive_CommentedJWTExampleInGo(t *testing.T) {
	rule := loadGolangBuiltinRule(t, "general/cew-798-Hardcoded-Credentials/general-hardcode-credentials.sf")

	total := runGolangBuiltinRule(t, rule, "jwt_comment_negative.go", `
package demo

// tokenString = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJmb28iOiJiYXIiLCJleHAiOjE1MDAwLCJpc3MiOiJ0ZXN0In0.HE7fK0xOQwFEr4WDgRWj4teRPZ6i3GLwD5YCm6Pwu_c"
// token = "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJkb3VibGVzIn0.cBDFVLuQZV2B7J76kk17LE2hmni_3RbzTBzIH_OsriE"

func parse() {}
`)

	assert.Equal(t, 0, total, "注释中的 JWT 示例字符串不应被识别为硬编码凭据")
}

func TestGeneralHardcodeCredentials_Positive_SingleJWTAssignmentInGo(t *testing.T) {
	rule := loadGolangBuiltinRule(t, "general/cew-798-Hardcoded-Credentials/general-hardcode-credentials.sf")

	vfsCode := `
package demo

func parse() string {
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJmb28iOiJiYXIiLCJleHAiOjE1MDAwLCJpc3MiOiJ0ZXN0In0.HE7fK0xOQwFEr4WDgRWj4teRPZ6i3GLwD5YCm6Pwu_c"
	return token
}
`

	vfs := filesys.NewVirtualFs()
	vfs.AddFile("jwt_assignment_positive.go", vfsCode)

	total := 0
	unique := map[string]struct{}{}
	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		result, err := programs.SyntaxFlowWithError(rule)
		if err != nil {
			return err
		}

		for _, name := range result.GetAlertVariables() {
			vals := result.GetValues(name)
			total += len(vals)
			for i, v := range vals {
				t.Logf("alert[%s][%d]=%v", name, i, v)
				unique[codec.AnyToString(v)] = struct{}{}
			}
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))

	assert.Equal(t, 1, len(unique), "真实硬编码 JWT 赋值应只产生一类唯一告警值")
	assert.GreaterOrEqual(t, total, 1, "真实硬编码 JWT 赋值应产生告警")
}
