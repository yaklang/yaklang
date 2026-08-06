package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func downloadArtifact(
	ctx context.Context,
	client *http.Client,
	tempDir string,
	artifactURI string,
) (downloadedArtifact, error) {
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return downloadedArtifact{}, fmt.Errorf("create plugin temp dir: %w", err)
	}
	tempFile, err := os.CreateTemp(tempDir, "release-*.tmp")
	if err != nil {
		return downloadedArtifact{}, fmt.Errorf("create plugin temp file: %w", err)
	}
	defer tempFile.Close()

	source, err := openArtifactSource(ctx, client, artifactURI)
	if err != nil {
		os.Remove(tempFile.Name())
		return downloadedArtifact{}, err
	}
	defer source.Close()

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(tempFile, hasher), source)
	if err != nil {
		os.Remove(tempFile.Name())
		return downloadedArtifact{}, fmt.Errorf("download plugin artifact: %w", err)
	}
	return downloadedArtifact{
		tempPath: tempFile.Name(),
		sha256:   hex.EncodeToString(hasher.Sum(nil)),
		size:     size,
	}, nil
}

func openArtifactSource(
	ctx context.Context,
	client *http.Client,
	artifactURI string,
) (io.ReadCloser, error) {
	parsed, err := url.Parse(strings.TrimSpace(artifactURI))
	if err != nil {
		return nil, fmt.Errorf("parse artifact uri: %w", err)
	}
	switch parsed.Scheme {
	case "":
		return os.Open(strings.TrimSpace(artifactURI))
	case "file":
		return os.Open(localPathFromFileURI(parsed))
	case "http", "https":
		return openHTTPArtifact(ctx, client, artifactURI)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedArtifactScheme, parsed.Scheme)
	}
}

func openHTTPArtifact(
	ctx context.Context,
	client *http.Client,
	artifactURI string,
) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, artifactURI, nil)
	if err != nil {
		return nil, fmt.Errorf("build artifact request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch artifact: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("fetch artifact: unexpected status %d", response.StatusCode)
	}
	return response.Body, nil
}

func localPathFromFileURI(uri *url.URL) string {
	if uri == nil {
		return ""
	}
	if uri.Host == "" || uri.Host == "localhost" {
		return uri.Path
	}
	return uri.Host + uri.Path
}

func validateDownloadedArtifact(
	artifact downloadedArtifact,
	input PluginSyncInput,
) error {
	if !strings.EqualFold(artifact.sha256, input.ArtifactSHA256) {
		return fmt.Errorf(
			"%w: expected %s got %s",
			ErrArtifactHashMismatch,
			input.ArtifactSHA256,
			artifact.sha256,
		)
	}
	if input.ArtifactSizeBytes > 0 && artifact.size != input.ArtifactSizeBytes {
		return fmt.Errorf(
			"%w: expected %d got %d",
			ErrArtifactSizeMismatch,
			input.ArtifactSizeBytes,
			artifact.size,
		)
	}
	return nil
}

func (m *PluginCacheManager) persistArtifact(
	releaseID string,
	tempPath string,
) (string, error) {
	releaseDir := m.releaseDir(releaseID)
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		return "", fmt.Errorf("create plugin release dir: %w", err)
	}
	localPath := filepath.Join(releaseDir, pluginCacheArtifactFileName)
	if err := os.Remove(localPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("cleanup cached artifact: %w", err)
	}
	if err := os.Rename(tempPath, localPath); err != nil {
		return "", fmt.Errorf("persist cached artifact: %w", err)
	}
	return localPath, nil
}

func (m *PluginCacheManager) saveDocument(
	input PluginSyncInput,
	result PluginSyncResult,
) error {
	document := pluginReleaseDocument{
		NodeID:          m.nodeID,
		PluginID:        input.PluginID,
		ReleaseID:       input.ReleaseID,
		Version:         input.Version,
		EntryKind:       input.EntryKind,
		ArtifactURI:     input.ArtifactURI,
		ArtifactSHA256:  input.ArtifactSHA256,
		CachedSizeBytes: result.CachedSizeBytes,
		LocalPath:       result.LocalPath,
		SyncedAt:        result.ObservedAt,
	}
	raw, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plugin manifest: %w", err)
	}
	if err := os.WriteFile(m.manifestPath(input.ReleaseID), raw, 0o644); err != nil {
		return fmt.Errorf("write plugin manifest: %w", err)
	}
	return nil
}

func (m *PluginCacheManager) loadDocument(
	releaseID string,
) (pluginReleaseDocument, error) {
	raw, err := os.ReadFile(m.manifestPath(releaseID))
	if os.IsNotExist(err) {
		return pluginReleaseDocument{}, ErrPluginCacheMiss
	}
	if err != nil {
		return pluginReleaseDocument{}, fmt.Errorf("read plugin manifest: %w", err)
	}
	var document pluginReleaseDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return pluginReleaseDocument{}, fmt.Errorf("unmarshal plugin manifest: %w", err)
	}
	if strings.TrimSpace(document.LocalPath) == "" {
		return pluginReleaseDocument{}, ErrPluginCacheMiss
	}
	return document, nil
}

func (m *PluginCacheManager) tempDir() string {
	return filepath.Join(m.storeDir, "tmp")
}

func (m *PluginCacheManager) releaseDir(releaseID string) string {
	return filepath.Join(m.storeDir, "releases", releaseID)
}

func (m *PluginCacheManager) manifestPath(releaseID string) string {
	return filepath.Join(m.releaseDir(releaseID), pluginCacheManifestFileName)
}
