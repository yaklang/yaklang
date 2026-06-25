package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
)

func TestMergeAuthSurfaceIntoRoutingProfile(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, store.SubDirName())
	require.NoError(t, os.MkdirAll(sub, 0o755))
	surface := &AuthSurfaceMapV1{
		SchemaVersion: 1,
		Surfaces: []AuthSurfaceEntry{
			{
				AuthRealm:      "admin",
				PathPrefixes:   []string{"/admin"},
				MountPrefix:    "/",
			},
		},
	}
	require.NoError(t, writeArtifactJSON(store.AuthSurfaceMapPath(dir), surface))
	rt := &Runtime{
		WorkDir: dir,
		Session: &store.DiscoverySession{
			UUID:            uuid.NewString(),
			TargetRaw:       "http://127.0.0.1:8080",
			TargetReachable: true,
		},
	}
	require.NoError(t, MergeAuthSurfaceIntoRoutingProfile(rt))
	rp, err := loadRoutingProfileFromWorkDir(dir)
	require.NoError(t, err)
	require.NotEmpty(t, rp.URLSpaces)
	found := false
	for _, sp := range rp.URLSpaces {
		if sp.MountPrefix == "/admin" {
			found = true
			break
		}
	}
	require.True(t, found, "expected /admin url_space")
}

func TestEnsureAuthReadyBeforeFeatureWork_NoCreds(t *testing.T) {
	dir := t.TempDir()
	rt := &Runtime{
		WorkDir: dir,
		Session: &store.DiscoverySession{
			UUID:            uuid.NewString(),
			TargetReachable: true,
		},
		UserAuthPassword: "test",
	}
	require.NoError(t, writeArtifactJSON(store.AuthSurfaceMapPath(dir), &AuthSurfaceMapV1{
		SchemaVersion: 1,
		Surfaces: []AuthSurfaceEntry{
			{AuthRealm: "admin", LoginPostPath: "/admin/login"},
		},
	}))
	err := EnsureAuthReadyBeforeFeatureWork(rt)
	require.Error(t, err)
}
