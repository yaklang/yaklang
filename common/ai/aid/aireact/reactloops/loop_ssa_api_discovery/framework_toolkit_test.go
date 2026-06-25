package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestDetectFrameworkToolkit_PublicCMSMiniFixture(t *testing.T) {
	dir := t.TempDir()
	codeRoot := filepath.Join(dir, "repo")
	javaDir := filepath.Join(codeRoot, "publiccms-core/src/main/java/com/publiccms/controller/admin")
	require.NoError(t, os.MkdirAll(javaDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "repo/pom.xml"), []byte(`<groupId>com.publiccms</groupId><artifactId>publiccms-core</artifactId>`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(javaDir, "AdminInitializer.java"), []byte(`class AdminInitializer {}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(codeRoot, "publiccms-core/src/main/java/com/publiccms/ApiInitializer.java"), []byte(`class ApiInitializer {}`), 0o644))

	rt := &Runtime{
		Session: storeDiscoverySession(codeRoot),
	}
	tk := PublicCMSToolkit{}
	score, evidence := tk.Detect(rt)
	require.GreaterOrEqual(t, score, 0.8, "evidence=%v", evidence)
}

func TestDetectFrameworkToolkit_OtherWhenNoMatch(t *testing.T) {
	dir := t.TempDir()
	codeRoot := filepath.Join(dir, "repo")
	require.NoError(t, os.MkdirAll(codeRoot, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(codeRoot, "go.mod"), []byte("module example.com/foo"), 0o644))
	rt := &Runtime{Session: storeDiscoverySession(codeRoot)}
	_, ok := DetectFrameworkToolkit(rt)
	require.False(t, ok)
}

func storeDiscoverySession(codeRoot string) *store.DiscoverySession {
	return &store.DiscoverySession{CodeRootPath: codeRoot, CodePathOK: true, Language: "java"}
}

func TestFrameworkToolkitSelectionPersist(t *testing.T) {
	dir := t.TempDir()
	workDir := filepath.Join(dir, "work")
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, store.SubDirName()), 0o755))
	rt := &Runtime{WorkDir: workDir}
	sel := &FrameworkToolkitSelectionV1{FrameworkID: "publiccms", Confidence: 0.9, Rationale: "test"}
	require.NoError(t, persistFrameworkToolkitSelection(rt, sel))
	require.Equal(t, "publiccms", rt.SelectedFrameworkID)
	loaded, err := loadFrameworkToolkitSelection(workDir)
	require.NoError(t, err)
	require.Equal(t, "publiccms", loaded.FrameworkID)
}

func TestShouldSkipVPhaseForToolkit(t *testing.T) {
	rt := &Runtime{FrameworkToolkitEnabled: true, FrameworkToolkitMode: FrameworkToolkitModeFast}
	require.True(t, shouldSkipVPhaseForToolkit(rt))
	rt.FrameworkToolkitMode = FrameworkToolkitModeFallbackAI
	require.False(t, shouldSkipVPhaseForToolkit(rt))
}

func TestRouterFallback_Other(t *testing.T) {
	rt := &Runtime{
		FrameworkToolkitEnabled: true,
		SelectedFrameworkID:     FrameworkToolkitIDOther,
		FrameworkToolkitMode:    FrameworkToolkitModeFallbackAI,
	}
	require.False(t, shouldSkipVPhaseForToolkit(rt))
}

func TestParseHTTPStatusFromToolOutput(t *testing.T) {
	require.Equal(t, 302, parseHTTPStatusFromToolOutput("HTTP/1.1 302 Found\r\nSet-Cookie: a=b"))
	require.Equal(t, 404, parseHTTPStatusFromToolOutput("response: HTTP/1.0 404"))
	require.Equal(t, 0, parseHTTPStatusFromToolOutput("no status"))
}

func TestIsProbeDestructivePathToolkit(t *testing.T) {
	yes, _ := isProbeDestructivePath("/admin/logout")
	require.True(t, yes)
}
