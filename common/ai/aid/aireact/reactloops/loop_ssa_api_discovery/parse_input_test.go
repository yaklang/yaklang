package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

func TestParseUserInput_LabeledLines(t *testing.T) {
	in := "Code path: /tmp/proj\nTarget: http://127.0.0.1:8080\n"
	p, err := ParseUserInput(in)
	require.NoError(t, err)
	require.Equal(t, "/tmp/proj", p.CodePath)
	require.Equal(t, "http://127.0.0.1:8080", p.TargetRaw)
	require.Equal(t, 0, p.PipelineMaxStage)
}

func TestParseUserInput_PipelineMaxStage(t *testing.T) {
	cases := []struct {
		snippets string
		want     int
	}{
		{"Code path: /tmp/p\nPipeline max stage: 4\n", 4},
		{"Code path: /tmp/p\n跑到第3阶段\n", 3},
		{"阶段上限: 2\n/var/repo\n", 2},
		{"阶段: 6\nCode path: /tmp/p\n", 6},
		{"跑到阶段 5\nCode path: /tmp/p\n", 5},
	}
	for _, tc := range cases {
		p, err := ParseUserInput(tc.snippets)
		require.NoError(t, err, tc.snippets)
		require.Equal(t, tc.want, p.PipelineMaxStage, tc.snippets)
	}
}

func TestNormalizePipelineMaxStage(t *testing.T) {
	require.Equal(t, PipelineStageFullMax, NormalizePipelineMaxStage(0))
	require.Equal(t, 1, NormalizePipelineMaxStage(1))
	require.Equal(t, 5, NormalizePipelineMaxStage(6))
	require.Equal(t, 5, NormalizePipelineMaxStage(99))
}

func TestParseUserInputLenient_AllowsMissingCodePath(t *testing.T) {
	in := "Target: http://127.0.0.1:8080\n请继续分析\n"
	p, err := ParseUserInputLenient(in)
	require.NoError(t, err)
	require.Empty(t, p.CodePath)
	require.Equal(t, "http://127.0.0.1:8080", p.TargetRaw)

	_, err = ParseUserInput(in)
	require.Error(t, err)
}

func TestParseUserInput_AbsPathGuess(t *testing.T) {
	in := "/var/repo/src\nTarget: 10.0.0.1:9090\n"
	p, err := ParseUserInput(in)
	require.NoError(t, err)
	require.Equal(t, "/var/repo/src", p.CodePath)
	require.Equal(t, "http://10.0.0.1:9090", p.TargetRaw)
}

func TestParseUserInput_ApiArchTestFlag(t *testing.T) {
	base := "Code path: /tmp/p\nTarget: http://127.0.0.1:8080\n"
	cases := []struct {
		line string
	}{
		{"api-arch-test: yes\n"},
		{"api_arch_test: yes\n"},
		{`api\_arch\_test: yes` + "\n"},
		{"API-ARCH-TEST: true\n"},
	}
	for _, tc := range cases {
		p, err := ParseUserInput(base + tc.line)
		require.NoError(t, err, tc.line)
		require.True(t, p.ApiArchTest, tc.line)
	}
	p, err := ParseUserInput(base + "language: java\n")
	require.NoError(t, err)
	require.False(t, p.ApiArchTest)
}

func TestParseUserInput_ApiArchTestAuthPassword(t *testing.T) {
	in := "Code path: /tmp/p\nTarget: http://127.0.0.1:8080\napi-arch-test: yes\nauth-password: admin123\n"
	p, err := ParseUserInput(in)
	require.NoError(t, err)
	require.True(t, p.ApiArchTest)
	require.Equal(t, "admin123", p.AuthLine)

	in2 := "Code path: /tmp/p\nTarget: http://127.0.0.1:8080\napi-arch-test: yes\nauth: admin123\n"
	p2, err := ParseUserInput(in2)
	require.NoError(t, err)
	require.Equal(t, "admin123", p2.AuthLine)
}

func TestParseUserInput_MarkdownEscapedUnderscoresInPath(t *testing.T) {
	bad := "Code path: /home/murkfox/yaklang-main/common/ai/aid/aireact/reactloops/loop\\_ssa\\_api\\_discovery/testfixtures/minimal\\_java\\_webapp\n"
	p, err := ParseUserInput(bad)
	require.NoError(t, err)
	want := "/home/murkfox/yaklang-main/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/testfixtures/minimal_java_webapp"
	require.Equal(t, want, p.CodePath)
}

func TestAbsCodeDir_EscapedUnderscoresFixture(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	pkgDir := filepath.Dir(thisFile)
	real := filepath.Join(pkgDir, "testfixtures", "minimal_java_webapp")
	st, err := os.Stat(real)
	if err != nil || !st.IsDir() {
		t.Skip("fixture dir not present:", real)
	}
	escaped := strings.ReplaceAll(real, "_", `\_`)
	got, err := AbsCodeDir(escaped)
	require.NoError(t, err)
	require.Equal(t, filepath.Clean(real), filepath.Clean(got))
}

func TestParseUserInput_AuthEscapedColon(t *testing.T) {
	in := "Code path: /tmp/p\nTarget: http://127.0.0.1:8080\nauth\\:admin/potian123\n"
	p, err := ParseUserInput(in)
	require.NoError(t, err)
	require.Equal(t, "admin/potian123", p.AuthLine)
	u, pass := ResolveUserCredentials(p)
	require.Equal(t, "admin", u)
	require.Equal(t, "potian123", pass)
}

func TestResolveUserCredentials_AuthPasswordFields(t *testing.T) {
	p := &ParsedUserInput{AuthUsername: "root", AuthPassword: "secret"}
	u, pass := ResolveUserCredentials(p)
	require.Equal(t, "root", u)
	require.Equal(t, "secret", pass)
}

func TestParseUserInput_MultiAuthGroups(t *testing.T) {
	in := `Code path: /tmp/p
Target: http://127.0.0.1:8080
admin_auth: admin1/potian123, admin2/backup
user_auth: user1/test123
`
	p, err := ParseUserInput(in)
	require.NoError(t, err)
	require.Len(t, p.AuthCredentialGroups, 2)
	require.Equal(t, "admin", p.AuthCredentialGroups[0].GroupID)
	require.Len(t, p.AuthCredentialGroups[0].Accounts, 2)
	require.Equal(t, "user", p.AuthCredentialGroups[1].GroupID)
}

func TestLoopRegistered(t *testing.T) {
	_, ok := reactloops.GetLoopFactory(schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY)
	require.True(t, ok, "import loop_ssa_api_discovery to register")
}
