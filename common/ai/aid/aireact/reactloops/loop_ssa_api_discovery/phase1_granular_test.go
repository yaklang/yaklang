package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestClassifyProbeResponse_InterfaceNotFound(t *testing.T) {
	fs := DefaultFailureSemantics()
	kind, verdict := ClassifyProbeResponse(fs, 200, "application/json", `{"error":"interfaceNotFound"}`)
	require.Equal(t, failureKindWrongPath, kind)
	require.Equal(t, "wrong_route", verdict)
}

func TestApplyFailureSemanticsToProbeResult_RejectsFalsePositive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))
	fs := DefaultFailureSemantics()
	require.NoError(t, persistFailureSemantics(&Runtime{WorkDir: dir}, fs))

	rt := &Runtime{WorkDir: dir}
	pr := &ProbeResult{
		Verified:        true,
		Method:          "GET",
		PathPattern:     "/cmsCategory/save",
		ProbeStatusCode: 200,
		ContentType:     "application/json",
		ResponseExcerpt: `{"error":"interfaceNotFound"}`,
		VerdictReason:   "HTTP 200 JSON",
	}
	ApplyFailureSemanticsToProbeResult(rt, pr)
	require.False(t, pr.Verified)
	require.Contains(t, pr.RejectReason, "failure_semantics_override")
}

func TestValidateAuthCalibration_RequiresTwoProbes(t *testing.T) {
	surface := &AuthSurfaceMapV1{
		Surfaces: []AuthSurfaceEntry{{AuthRealm: "admin"}},
	}
	cal := &AuthCalibrationV1{
		Realms: []AuthCalibrationRealm{{
			AuthRealm:  "admin",
			Calibrated: true,
			Probes: []AuthCalibrationProbe{
				{Passed: true}, {Passed: true},
			},
		}},
	}
	require.NoError(t, validateAuthCalibration(cal, surface))
	require.True(t, cal.AllCalibrated)
}

func TestValidateAuthCalibration_PartialOneRealmOK(t *testing.T) {
	surface := &AuthSurfaceMapV1{
		Surfaces: []AuthSurfaceEntry{
			{AuthRealm: "admin"},
			{AuthRealm: "api"},
		},
	}
	cal := &AuthCalibrationV1{
		Realms: []AuthCalibrationRealm{{
			AuthRealm:  "admin",
			Calibrated: true,
			Probes: []AuthCalibrationProbe{
				{Passed: true}, {Passed: true},
			},
		}},
	}
	require.NoError(t, validateAuthCalibration(cal, surface))
	require.False(t, cal.AllCalibrated)
}

func TestValidateAuthCalibration_FailsWhenNoneCalibrated(t *testing.T) {
	surface := &AuthSurfaceMapV1{
		Surfaces: []AuthSurfaceEntry{{AuthRealm: "admin"}, {AuthRealm: "api"}},
	}
	cal := &AuthCalibrationV1{Realms: []AuthCalibrationRealm{}}
	require.Error(t, validateAuthCalibration(cal, surface))
}

func TestValidateAuthCalibration_FailsIncomplete(t *testing.T) {
	surface := &AuthSurfaceMapV1{
		Surfaces: []AuthSurfaceEntry{{AuthRealm: "admin"}},
	}
	cal := &AuthCalibrationV1{
		Realms: []AuthCalibrationRealm{{
			AuthRealm: "admin",
			Probes:    []AuthCalibrationProbe{{Passed: true}},
		}},
	}
	require.Error(t, validateAuthCalibration(cal, surface))
}

func TestValidateRoutingProfileMountPrefixes(t *testing.T) {
	p := &RoutingProfileV1{
		SchemaVersion:    1,
		ValidationStatus: "confirmed",
		URLSpaces:        []RoutingURLSpace{{ID: "admin", MountPrefix: "/admin"}},
		EffectiveBases:   []RoutingEffectiveBase{{SpaceID: "admin", BaseURL: "http://host/admin"}},
	}
	require.NoError(t, validateRoutingProfileMountPrefixes(p))

	empty := &RoutingProfileV1{SchemaVersion: 1, ValidationStatus: "confirmed"}
	require.Error(t, validateRoutingProfileMountPrefixes(empty))
}

func TestResolveAuthRealmForHandler(t *testing.T) {
	surface := &AuthSurfaceMapV1{
		Surfaces: []AuthSurfaceEntry{{
			AuthRealm:       "admin",
			PackagePatterns: []string{"*.controller.admin.*"},
		}},
	}
	realm := ResolveAuthRealmForHandler(surface, "com.publiccms.controller.admin.cms.CmsCategoryAdminController")
	require.Equal(t, "admin", realm)
}

func TestValidateFailureSemantics(t *testing.T) {
	require.NoError(t, validateFailureSemantics(DefaultFailureSemantics()))
	bad := &FailureSemanticsV1{Categories: []FailureSemanticsCategory{}}
	require.Error(t, validateFailureSemantics(bad))
}

func TestStripAITaggedJSONPayload(t *testing.T) {
	raw := `<|JSON_START|>{"schema_version":1,"features":[]}<|JSON_END|>`
	got := stripAITaggedJSONPayload(raw)
	require.JSONEq(t, `{"schema_version":1,"features":[]}`, got)
}

func TestParseAgentJSONObject_StripsTags(t *testing.T) {
	var inv FeatureInventoryV1
	err := parseAgentJSONObject(`<|OUT|>{"schema_version":1,"features":[{"feature_id":"x","package_patterns":["*.controller.admin.*"]}]}`, &inv)
	require.NoError(t, err)
	require.Len(t, inv.Features, 1)
	require.Equal(t, "x", inv.Features[0].FeatureID)
}

func TestFeaturePatternCoversScopeUnit_JavaPackage(t *testing.T) {
	unit := "src/main/java/com/publiccms/controller/admin/cms"
	require.True(t, featurePatternCoversScopeUnit(unit, "com.publiccms.controller.admin.cms.*"))
	require.True(t, featurePatternCoversScopeUnit(unit, "*.controller.admin.cms.*"))
	require.False(t, featurePatternCoversScopeUnit(unit, "com.publiccms.controller.admin.sys.*"))
	require.False(t, featurePatternCoversScopeUnit(unit, "com.publiccms.controller.admin.cms.CmsCategoryAdminController"))
}

func TestParseRoutingProfileFromAgentJSON_FlexibleEffectiveBases(t *testing.T) {
	rt := &Runtime{Session: &store.DiscoverySession{TargetRaw: "http://127.0.0.1:8080"}}
	raw := `{
		"validation_status": "confirmed",
		"url_spaces": [
			{"id": "admin", "mount_prefix": "/admin", "confidence": "high"},
			{"id": "api", "mount_prefix": "/api", "confidence": "medium"}
		],
		"effective_bases": ["/admin", "/api", "/"]
	}`
	p, err := parseRoutingProfileFromAgentJSON(raw, rt)
	require.NoError(t, err)
	require.Len(t, p.URLSpaces, 2)
	require.Len(t, p.EffectiveBases, 3)
	require.Equal(t, "http://127.0.0.1:8080/admin", p.EffectiveBases[0].BaseURL)
}

func TestBootstrapRoutingProfileFromComponentMap(t *testing.T) {
	dir := t.TempDir()
	rt := &Runtime{
		WorkDir: dir,
		Session: &store.DiscoverySession{TargetRaw: "http://host/", Language: "java"},
	}
	comp := &ComponentPackageMapV1{
		SchemaVersion: 1,
		Components: []ComponentPackageEntry{
			{ID: "admin", ControllerLayer: "admin", PackagePatterns: []string{"*.controller.admin.*"}},
			{ID: "api", ControllerLayer: "api", PackagePatterns: []string{"*.controller.api.*"}},
			{ID: "web", ControllerLayer: "web", PackagePatterns: []string{"*.controller.web.*"}},
		},
	}
	require.NoError(t, persistComponentPackageMap(rt, comp))
	require.NoError(t, bootstrapRoutingProfileFromComponentMap(rt))
	require.FileExists(t, store.RoutingProfilePath(dir))
	raw, err := os.ReadFile(store.RoutingProfilePath(dir))
	require.NoError(t, err)
	var rp RoutingProfileV1
	require.NoError(t, json.Unmarshal(raw, &rp))
	require.Len(t, rp.URLSpaces, 3)
	prefixes := map[string]bool{}
	for _, sp := range rp.URLSpaces {
		prefixes[sp.MountPrefix] = true
	}
	require.True(t, prefixes["/admin"])
	require.True(t, prefixes["/api"])
	require.True(t, prefixes["/"])
}

func TestEvaluateFeatureInventoryCoverage_ControllerPackages(t *testing.T) {
	dir := t.TempDir()
	rt := &Runtime{WorkDir: dir, Session: &store.DiscoverySession{Language: "java"}}
	inv := &JavaBusinessScopeInventory{
		Modules: []JavaModuleScope{{
			ScopeUnits: []JavaScopeUnit{
				{Kind: scopeUnitKindJavaPackage, Path: "src/main/java/com/app/controller/admin/cms", DomainSegment: "controller"},
				{Kind: scopeUnitKindJavaPackage, Path: "src/main/java/com/app/controller/admin/sys", DomainSegment: "controller"},
				{Kind: scopeUnitKindJavaPackage, Path: "src/main/java/com/app/service/foo", DomainSegment: "service"},
			},
		}},
	}
	require.NoError(t, persistJavaBusinessScopeInventory(rt, inv))

	featureInv := &FeatureInventoryV1{
		Features: []FeatureInventoryEntry{
			{FeatureID: "cms", PackagePatterns: []string{"*.controller.admin.cms.*"}},
			{FeatureID: "sys", PackagePatterns: []string{"com.app.controller.admin.sys.*"}},
		},
	}
	cov := evaluateFeatureInventoryCoverage(rt, featureInv)
	require.True(t, cov.Complete)
	require.Equal(t, 2, cov.Covered)
	require.Equal(t, 2, cov.TotalRequired)
}

func TestEvaluateFeatureEntryFilesCoverage_Registry(t *testing.T) {
	registry := &CodeUnitRegistryV1{
		Units: []CodeUnitEntry{
			{RelPath: "mod/AController.java"},
			{RelPath: "mod/BController.java"},
		},
	}
	inv := &FeatureInventoryV1{
		Features: []FeatureInventoryEntry{
			{FeatureID: "f1", SurfaceKind: SurfaceKindHTTPAPI, EntryFiles: []string{"mod/AController.java"}},
			{FeatureID: "f2", SurfaceKind: SurfaceKindCodeOnly, EntryFiles: []string{"mod/BController.java"}, NoHttpReason: "service only"},
		},
	}
	cov := evaluateFeatureEntryFilesCoverage(registry, inv)
	require.True(t, cov.Complete)
	require.Equal(t, 2, cov.Covered)
	require.Equal(t, 2, cov.TotalRequired)
}

func TestValidateFeatureInventory_RequiresRegistryCoverage(t *testing.T) {
	dir := t.TempDir()
	rt := &Runtime{WorkDir: dir, Session: &store.DiscoverySession{Language: "java", CodePathOK: true}}
	reg := &CodeUnitRegistryV1{
		SchemaVersion: 1,
		Units:         []CodeUnitEntry{{RelPath: "mod/A.java"}},
	}
	require.NoError(t, persistCodeUnitRegistry(rt, reg))
	inv := &FeatureInventoryV1{
		Features: []FeatureInventoryEntry{
			{FeatureID: "f1", SurfaceKind: SurfaceKindHTTPAPI, EntryFiles: []string{"mod/A.java"}},
		},
	}
	require.NoError(t, validateFeatureInventory(inv, rt))
}

func TestValidateFeatureInventory_RejectsMissingSurfaceKind(t *testing.T) {
	dir := t.TempDir()
	rt := &Runtime{WorkDir: dir, Session: &store.DiscoverySession{Language: "java", CodePathOK: true}}
	reg := &CodeUnitRegistryV1{Units: []CodeUnitEntry{{RelPath: "mod/A.java"}}}
	require.NoError(t, persistCodeUnitRegistry(rt, reg))
	inv := &FeatureInventoryV1{
		Features: []FeatureInventoryEntry{
			{FeatureID: "f1", EntryFiles: []string{"mod/A.java"}},
		},
	}
	require.Error(t, validateFeatureInventory(inv, rt))
}
func TestValidateFeatureVerifyEntry(t *testing.T) {
	require.NoError(t, validateFeatureVerifyEntry(nil, &FeatureApiMapEntry{
		FeatureID: "cms",
		Apis:      []FeatureApiEntry{{Method: "GET", PathPattern: "/foo"}},
	}))
	require.Error(t, validateFeatureVerifyEntry(nil, &FeatureApiMapEntry{
		FeatureID: "cms",
		Apis:      []FeatureApiEntry{{Method: "GET", PathPattern: "foo"}},
	}))
	require.Error(t, validateFeatureVerifyEntry(nil, &FeatureApiMapEntry{
		FeatureID: "cms",
		Apis:      []FeatureApiEntry{},
	}))
	require.NoError(t, validateFeatureVerifyEntry(nil, &FeatureApiMapEntry{
		FeatureID:   "cms",
		NoApiReason: "directive only",
	}))
	rt := &Runtime{Session: &store.DiscoverySession{TargetReachable: true}}
	require.Error(t, validateFeatureVerifyEntry(rt, &FeatureApiMapEntry{
		FeatureID: "cms",
		Apis: []FeatureApiEntry{{
			Method: "GET", PathPattern: "/foo", Verified: true,
		}},
	}))
	require.NoError(t, validateFeatureVerifyEntry(rt, &FeatureApiMapEntry{
		FeatureID: "cms",
		Apis: []FeatureApiEntry{{
			Method: "GET", PathPattern: "/foo", Verified: true,
			FullSampleURL: "http://127.0.0.1/foo", VerdictReason: "hit",
		}},
	}))
}
