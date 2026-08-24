package scannode

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/utils"
	pluginv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/plugin/v1"
)

const pluginBundleArtifactPathPrefix = "/v1/plugin-bundles/"

var pluginBundleIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type pluginBundleInstaller interface {
	Install(context.Context, PluginBundleInstallInput) (string, error)
}

func (s *ScanNode) preparePluginBundle(ctx context.Context, ref *pluginv1.PluginBundleRef) (string, error) {
	if ref == nil {
		return "", nil
	}
	if s == nil || s.node == nil || s.pluginBundles == nil {
		return "", errors.New("plugin bundle runtime is not ready")
	}
	if err := validatePluginBundleRef(ref); err != nil {
		return "", err
	}
	session, ok := s.node.GetSessionState()
	if !ok {
		return "", errors.New("node session is unavailable for plugin bundle download")
	}
	return installPluginBundle(
		ctx,
		s.pluginBundles,
		s.resolvePlatformAPIBaseURL(),
		session,
		ref,
	)
}

func installPluginBundle(
	ctx context.Context,
	installer pluginBundleInstaller,
	platformAPIBaseURL string,
	session node.SessionState,
	ref *pluginv1.PluginBundleRef,
) (string, error) {
	if installer == nil || ref == nil {
		return "", errors.New("plugin bundle installer and reference are required")
	}
	if strings.TrimSpace(session.SessionID) == "" || strings.TrimSpace(session.SessionToken) == "" {
		return "", errors.New("node session credentials are unavailable for plugin bundle download")
	}
	artifactURL, err := pluginBundleArtifactURL(platformAPIBaseURL, ref.GetBundleId(), session.SessionID)
	if err != nil {
		return "", err
	}
	installedPath, err := installer.Install(ctx, PluginBundleInstallInput{
		BundleID:      ref.GetBundleId(),
		ArtifactURL:   artifactURL,
		SHA256:        ref.GetArtifactSha256(),
		SizeBytes:     ref.GetArtifactSizeBytes(),
		ItemCount:     int(ref.GetItemCount()),
		SchemaVersion: ref.GetSchemaVersion(),
		NodeSessionID: session.SessionID,
		SessionToken:  session.SessionToken,
	})
	if err != nil {
		return "", utils.Errorf("prepare plugin bundle %s: %v", ref.GetBundleId(), err)
	}
	if strings.TrimSpace(installedPath) == "" {
		return "", errors.New("plugin bundle installer returned an empty path")
	}
	return installedPath, nil
}

func pluginBundleArtifactURL(platformAPIBaseURL, bundleID, sessionID string) (string, error) {
	baseURL, err := url.Parse(strings.TrimSpace(platformAPIBaseURL))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" ||
		(baseURL.Scheme != "http" && baseURL.Scheme != "https") ||
		baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return "", errors.New("invalid platform API base URL for plugin bundle download")
	}
	bundleID = strings.TrimSpace(bundleID)
	sessionID = strings.TrimSpace(sessionID)
	if !pluginBundleIDPattern.MatchString(bundleID) || sessionID == "" {
		return "", errors.New("plugin bundle id and node session id are required")
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + pluginBundleArtifactPathPrefix +
		bundleID + "/artifact"
	baseURL.RawPath = ""
	query := baseURL.Query()
	query.Set("node_session_id", sessionID)
	baseURL.RawQuery = query.Encode()
	return baseURL.String(), nil
}
