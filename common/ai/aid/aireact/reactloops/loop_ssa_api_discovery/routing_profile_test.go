package loop_ssa_api_discovery

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestPackageGlobMatch(t *testing.T) {
	require.True(t, packageGlobMatch("com.publiccms.controller.admin.Foo", "com.publiccms.*.admin.*"))
	require.True(t, packageGlobMatch("com.a.b.Admin", "com.a.b.Admin"))
	require.False(t, packageGlobMatch("com.publiccms.controller.web.Foo", "com.publiccms.*.admin.*"))
}

func TestEffectiveProbeBaseForHandler(t *testing.T) {
	raw := `{
  "schema_version": 1,
  "validation_status": "confirmed",
  "url_spaces": [
    {"id": "public", "default_for_packages": ["com.app.web.*"]},
    {"id": "admin", "default_for_packages": ["com.app.admin.*"]}
  ],
  "default_space_id": "public",
  "effective_bases": [
    {"space_id": "public", "base_url": "http://127.0.0.1:8080/app"},
    {"space_id": "admin", "base_url": "http://127.0.0.1:8080/app/admin"}
  ]
}`
	p, err := ParseRoutingProfileJSON(raw)
	require.NoError(t, err)
	sess := &store.DiscoverySession{TargetRaw: "http://127.0.0.1:8080/app"}
	require.Equal(t, "http://127.0.0.1:8080/app/admin", EffectiveProbeBaseForHandler(p, sess, "com.app.admin.X"))
	require.Equal(t, "http://127.0.0.1:8080/app", EffectiveProbeBaseForHandler(p, sess, "com.app.web.X"))
}

func TestValidateRoutingProfileForCommit_FailedAllowsEmptyBases(t *testing.T) {
	p := &RoutingProfileV1{SchemaVersion: 1, ValidationStatus: "failed"}
	require.NoError(t, ValidateRoutingProfileForCommit(p))
}
