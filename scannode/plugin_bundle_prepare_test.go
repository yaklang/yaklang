package scannode

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/node"
	pluginv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/plugin/v1"
)

type pluginBundleInstallerStub struct {
	input PluginBundleInstallInput
	path  string
	err   error
}

func (s *pluginBundleInstallerStub) Install(_ context.Context, input PluginBundleInstallInput) (string, error) {
	s.input = input
	return s.path, s.err
}

func TestInstallPluginBundleUsesTrustedPlatformURLAndSession(t *testing.T) {
	installer := &pluginBundleInstallerStub{path: "/cache/bundle"}
	path, err := installPluginBundle(
		context.Background(),
		installer,
		"https://legion.example/base",
		node.SessionState{SessionID: "session 1", SessionToken: "secret"},
		&pluginv1.PluginBundleRef{
			BundleId: "bundle-one", ArtifactSha256: strings.Repeat("ab", 32),
			ArtifactSizeBytes: 4096, ItemCount: 3,
			SchemaVersion: pluginBundleManifestSchemaVersion, ArchiveFormat: "zip",
		},
	)
	if err != nil {
		t.Fatalf("installPluginBundle() error = %v", err)
	}
	if path != "/cache/bundle" || installer.input.SessionToken != "secret" {
		t.Fatalf("path/input = %q %#v", path, installer.input)
	}
	parsed, err := url.Parse(installer.input.ArtifactURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != "legion.example" || parsed.Path != "/base/v1/plugin-bundles/bundle-one/artifact" ||
		parsed.Query().Get("node_session_id") != "session 1" {
		t.Fatalf("artifact URL = %s", installer.input.ArtifactURL)
	}
}

func TestInstallPluginBundleFailsClosed(t *testing.T) {
	installer := &pluginBundleInstallerStub{err: errors.New("digest mismatch")}
	_, err := installPluginBundle(
		context.Background(), installer, "https://legion.example", node.SessionState{
			SessionID: "session-1", SessionToken: "secret",
		}, validPluginBundleRef(),
	)
	if err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("error = %v, want installer failure", err)
	}
}
