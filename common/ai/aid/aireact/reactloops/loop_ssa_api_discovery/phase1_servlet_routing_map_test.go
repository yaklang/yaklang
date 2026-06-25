package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestExtractServletMappingPatterns_PublicCMSStyle(t *testing.T) {
	adminInit := `
public class AdminInitializer extends BaseServletInitializer {
    protected Class<?>[] getServletConfigClasses() {
        return new Class[] { AdminConfig.class };
    }
    protected String[] getServletMappings() {
        return new String[] { CommonUtils.joinString(AdminConfig.ADMIN_CONTEXT_PATH, "/*") };
    }
}`
	adminConfig := `
public class AdminConfig {
    public static final String ADMIN_CONTEXT_PATH = "/admin";
    @ComponentScan(basePackages = "com.publiccms.controller.admin")
}
`
	root := t.TempDir()
	initPath := filepath.Join(root, "config", "initializer", "AdminInitializer.java")
	cfgPath := filepath.Join(root, "config", "spring", "AdminConfig.java")
	require.NoError(t, os.MkdirAll(filepath.Dir(initPath), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgPath), 0o755))
	require.NoError(t, os.WriteFile(initPath, []byte(adminInit), 0o644))
	require.NoError(t, os.WriteFile(cfgPath, []byte(adminConfig), 0o644))

	patterns := extractServletMappingPatterns(adminInit, root)
	require.Contains(t, patterns, "/admin/*")
}

func TestBuildServletRoutingMap_AdminPrefix(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))

	initJava := `package config.initializer;
import config.spring.AdminConfig;
public class AdminInitializer {
    protected Class<?>[] getServletConfigClasses() { return new Class[] { AdminConfig.class }; }
    protected String[] getServletMappings() { return new String[] { "/admin/*" }; }
}`
	cfgJava := `package config.spring;
import org.springframework.context.annotation.ComponentScan;
public class AdminConfig {
    public static final String ADMIN_CONTEXT_PATH = "/admin";
    @ComponentScan(basePackages = "com.publiccms.controller.admin")
}`
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "config", "initializer"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "config", "spring"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config", "initializer", "AdminInitializer.java"), []byte(initJava), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config", "spring", "AdminConfig.java"), []byte(cfgJava), 0o644))

	rt := &Runtime{
		WorkDir: dir,
		Session: &store.DiscoverySession{CodePathOK: true, CodeRootPath: dir},
	}
	m, err := BuildServletRoutingMap(rt)
	require.NoError(t, err)
	require.NotEmpty(t, m.Dispatchers)
	prefix := resolveURLPrefixFromServletMap(m, "publiccms-core/src/main/java/com/publiccms/controller/admin/cms/CmsDiyAdminController.java", "", []string{"com.publiccms.controller.admin.cms.*"})
	require.Equal(t, "/admin", prefix)
}

func TestSanitizeRoutingProfileURLSpaces_DropsControllerMounts(t *testing.T) {
	rp := &RoutingProfileV1{
		URLSpaces: []RoutingURLSpace{
			{ID: "admin", MountPrefix: "/admin"},
			{ID: "cmsCategory", MountPrefix: "/cmsCategory"},
			{ID: "dict", MountPrefix: "/dict"},
		},
	}
	servletMap := &ServletRoutingMapV1{
		Dispatchers: []ServletDispatcherEntry{
			{URLPrefix: "/admin"},
			{URLPrefix: "/api"},
			{URLPrefix: "/"},
		},
	}
	SanitizeRoutingProfileURLSpaces(rp, servletMap)
	mounts := map[string]bool{}
	for _, sp := range rp.URLSpaces {
		mounts[sp.MountPrefix] = true
	}
	require.True(t, mounts["/admin"])
	require.False(t, mounts["/cmsCategory"])
	require.False(t, mounts["/dict"])
}

func TestMergeServletMapIntoAuthSurface_AddsAdmin(t *testing.T) {
	surface := &AuthSurfaceMapV1{
		Surfaces: []AuthSurfaceEntry{
			{AuthRealm: "web", PathPrefixes: []string{"/"}},
		},
	}
	m := &ServletRoutingMapV1{
		Dispatchers: []ServletDispatcherEntry{
			{URLPrefix: "/admin", PackagePatterns: []string{"*.controller.admin.*"}},
		},
	}
	mergeServletMapIntoAuthSurface(m, surface)
	require.Len(t, surface.Surfaces, 2)
	require.Equal(t, AuthRealmAdmin, surface.Surfaces[1].AuthRealm)
	require.Equal(t, []string{"/admin"}, surface.Surfaces[1].PathPrefixes)
}

func TestLoadAnalyzedEntrySet_ProcessedWithoutHandlerFile(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))
	inv := &FeatureInventoryV1{
		Features: []FeatureInventoryEntry{
			{FeatureID: "f1", EntryFiles: []string{"mod/AController.java"}},
		},
	}
	apiMap := &FeatureApiMapV1{
		Features: []FeatureApiMapEntry{
			{FeatureID: "f1", Processed: true, Apis: []FeatureApiEntry{{Method: "GET", PathPattern: "/admin/a"}}},
		},
	}
	require.NoError(t, writeArtifactJSON(store.FeatureInventoryPath(dir), inv))
	require.NoError(t, writeArtifactJSON(store.FeatureApiMapPath(dir), apiMap))
	rt := &Runtime{WorkDir: dir}
	set := loadAnalyzedEntrySet(rt)
	require.True(t, set["mod/AController.java"])
}

func TestCoverageBatchSizeFromEnv(t *testing.T) {
	t.Setenv("YAK_SSA_API_DISCOVERY_FEATURE_BATCH_SIZE", "3")
	require.Equal(t, 3, coverageBatchSize())
}
