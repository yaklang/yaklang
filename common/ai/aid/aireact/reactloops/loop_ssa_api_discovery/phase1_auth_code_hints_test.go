package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestFormatAuthBackendCodeHintsForRealm_Admin(t *testing.T) {
	dir := t.TempDir()
	discoveryDir := filepath.Join(dir, "ssa_discovery")
	require.NoError(t, os.MkdirAll(discoveryDir, 0o755))

	realmInv := AuthRealmInventoryV1{
		SchemaVersion: 1,
		MultiAuth:     true,
		Realms: []AuthRealmSummary{
			{AuthRealm: AuthRealmAdmin, MountPrefix: "/admin"},
			{AuthRealm: AuthRealmWeb, MountPrefix: "/"},
		},
	}
	rb, _ := json.MarshalIndent(realmInv, "", "  ")
	require.NoError(t, os.WriteFile(store.AuthRealmInventoryPath(dir), rb, 0o644))

	scope := BackendScopeReport{
		BackendRoots: []string{"publiccms-core/src/main/java"},
		ControllerFileCandidates: []struct {
			RelPath string `json:"rel_path"`
			Reason  string `json:"reason"`
		}{
			{RelPath: "publiccms-core/src/main/java/com/publiccms/controller/admin/LoginAdminController.java", Reason: "controller_admin"},
			{RelPath: "publiccms-core/src/main/java/com/publiccms/controller/web/LoginController.java", Reason: "controller_web"},
		},
		ApiRouteFiles: []string{
			"publiccms-core/src/main/java/com/publiccms/controller/admin/LoginAdminController.java",
			"publiccms-core/src/main/java/com/publiccms/controller/web/LoginController.java",
		},
	}
	sb, _ := json.MarshalIndent(scope, "", "  ")
	require.NoError(t, os.WriteFile(store.BackendScopePath(dir), sb, 0o644))

	comp := ComponentPackageMapV1{
		Components: []ComponentPackageEntry{
			{ID: "admin", ControllerLayer: "admin", PackagePatterns: []string{"*.controller.admin.*"}},
			{ID: "web", ControllerLayer: "web", PackagePatterns: []string{"*.controller.web.*"}},
		},
	}
	cb, _ := json.MarshalIndent(comp, "", "  ")
	require.NoError(t, os.WriteFile(store.ComponentPackageMapPath(dir), cb, 0o644))

	rt := &Runtime{WorkDir: dir}
	hints := FormatAuthBackendCodeHintsForRealm(rt, AuthRealmAdmin)
	require.Contains(t, hints, "Backend code locations")
	require.Contains(t, hints, "LoginAdminController.java")
	require.NotContains(t, hints, "controller/web/LoginController.java")
}

func TestAuthPathMatchesRealm(t *testing.T) {
	require.True(t, authPathMatchesRealm("com/foo/controller/admin/Login.java", AuthRealmAdmin, "/admin"))
	require.False(t, authPathMatchesRealm("com/foo/controller/web/Login.java", AuthRealmAdmin, "/admin"))
	require.True(t, authPathMatchesRealm("com/foo/controller/web/Login.java", AuthRealmWeb, "/"))
	require.False(t, authPathMatchesRealm("com/foo/controller/admin/Login.java", AuthRealmWeb, "/"))
}

func TestBuildAuthMechanismExtraContext_IncludesLoginPlaybook(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ssa_discovery"), 0o755))
	rt := &Runtime{WorkDir: dir}
	ctx := buildAuthMechanismExtraContext(rt, AuthRealmAdmin)
	require.Contains(t, ctx, "login_page_kind")
	require.Contains(t, ctx, "Backend code locations")
}
