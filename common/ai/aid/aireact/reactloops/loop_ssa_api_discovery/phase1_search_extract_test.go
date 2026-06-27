package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestSearchFilesUnderCodeRoot_MinimalJavaWebapp(t *testing.T) {
	fixture := filepath.Join("testfixtures", "minimal_java_webapp")
	abs, err := filepath.Abs(fixture)
	require.NoError(t, err)

	paths, err := searchFilesUnderCodeRoot(abs, fileSearchOpts{Suffix: ".java", MaxResults: 20})
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	require.Contains(t, paths, "src/main/java/com/example/discoverydemo/web/HelloController.java")

	poms, err := searchFilesUnderCodeRoot(abs, fileSearchOpts{NameContains: "pom", MaxResults: 5})
	require.NoError(t, err)
	require.Contains(t, poms, "pom.xml")
}

func TestGrepFilesUnderCodeRoot_RestController(t *testing.T) {
	fixture := filepath.Join("testfixtures", "minimal_java_webapp")
	abs, err := filepath.Abs(fixture)
	require.NoError(t, err)

	matches, err := grepFilesUnderCodeRoot(abs, `@RestController|@Controller`, "*.java", 20)
	require.NoError(t, err)
	require.NotEmpty(t, matches)
}

func TestExtractSpringRoutesFromFixtureController(t *testing.T) {
	fixture := filepath.Join("testfixtures", "minimal_java_webapp")
	rel := "src/main/java/com/example/discoverydemo/web/HelloController.java"
	b, err := os.ReadFile(filepath.Join(fixture, rel))
	require.NoError(t, err)

	routes := extractSpringRoutesFromBytes(b, rel)
	require.NotEmpty(t, routes)
	paths := map[string]string{}
	for _, r := range routes {
		paths[r.PathPattern] = r.Method
	}
	require.Equal(t, "GET", paths["/api/health"])
}

func TestExtractMavenPomFromFixture(t *testing.T) {
	fixture := filepath.Join("testfixtures", "minimal_java_webapp")
	b, err := os.ReadFile(filepath.Join(fixture, "pom.xml"))
	require.NoError(t, err)

	deps := extractMavenPomFromBytes(b)
	require.NotEmpty(t, deps)
	names := map[string]struct{}{}
	for _, d := range deps {
		names[d.Name] = struct{}{}
	}
	require.Contains(t, names, "org.springframework.boot:spring-boot-starter-web")
}

func TestExtractSpringYamlFromFixture(t *testing.T) {
	fixture := filepath.Join("testfixtures", "minimal_java_webapp")
	b, err := os.ReadFile(filepath.Join(fixture, "src/main/resources/application.yml"))
	require.NoError(t, err)

	res := extractSpringYamlFromBytes(b, "src/main/resources/application.yml")
	require.Equal(t, "18080", res.ServerPort)
}

func TestUpsertExtractedRoutes_WritesDB(t *testing.T) {
	workDir := t.TempDir()
	db, err := store.OpenSessionDB(workDir)
	require.NoError(t, err)
	defer func() { _ = db.DB().Close() }()
	repo := store.NewRepository(db)
	sess := &store.DiscoverySession{UUID: uuid.NewString(), CodeRootPath: "/tmp", CodePathOK: true}
	require.NoError(t, repo.CreateSession(sess))
	rt := &Runtime{WorkDir: workDir, SQLitePath: store.DBPath(workDir), Repo: repo, Session: sess}

	n, err := upsertExtractedRoutes(rt, []ExtractedRoute{
		{Method: "GET", PathPattern: "/api/ping", FileRelPath: "Foo.java"},
	}, SourceExtractSpring)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	eps, err := repo.ListHttpEndpoints(sess.ID)
	require.NoError(t, err)
	require.Len(t, eps, 1)
	require.Equal(t, SourceExtractSpring, eps[0].Source)
}

func TestTransformCredentialGo_Sha512(t *testing.T) {
	res, err := transformCredentialGoParams("sha512", "secret", "", "", "", "hex", false)
	require.NoError(t, err)
	require.Len(t, res.Output, 128)
}

func TestTransformCredentialGo_WithSalt(t *testing.T) {
	plain, err := transformCredentialGoParams("md5", "pass", "salt", "suffix", "", "hex", false)
	require.NoError(t, err)
	salted, err := transformCredentialGoParams("md5", "pass", "salt", "suffix", "", "hex", false)
	require.NoError(t, err)
	require.Equal(t, plain.Output, salted.Output)
}
