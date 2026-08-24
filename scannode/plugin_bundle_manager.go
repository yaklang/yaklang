package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	commonbundle "github.com/yaklang/yaklang/common/pluginbundle"
)

const pluginBundleDownloadTimeout = 90 * time.Second

type PluginBundleManagerConfig struct {
	BaseDir string
	Client  *http.Client
}

type PluginBundleInstallInput struct {
	BundleID      string
	ArtifactURL   string
	SHA256        string
	SizeBytes     int64
	ItemCount     int
	SchemaVersion string
	NodeSessionID string
	SessionToken  string
}

type PluginBundleManager struct {
	rootDir string
	client  *http.Client
	mu      sync.Mutex
}

func NewPluginBundleManager(config PluginBundleManagerConfig) (*PluginBundleManager, error) {
	baseDir := strings.TrimSpace(config.BaseDir)
	if baseDir == "" {
		return nil, errors.New("plugin bundle manager base directory is required")
	}
	rootDir := filepath.Join(baseDir, "plugin-bundles")
	if err := os.MkdirAll(rootDir, 0o700); err != nil {
		return nil, fmt.Errorf("create plugin bundle store: %w", err)
	}
	return &PluginBundleManager{rootDir: rootDir, client: config.Client}, nil
}

// Install downloads, verifies and atomically publishes one immutable bundle.
// The returned directory is READY: every member has already passed strict
// schema, identity, size and digest validation.
func (m *PluginBundleManager) Install(ctx context.Context, input PluginBundleInstallInput) (string, error) {
	if m == nil || strings.TrimSpace(m.rootDir) == "" {
		return "", errors.New("plugin bundle manager is not configured")
	}
	normalized, err := normalizePluginBundleInstallInput(input)
	if err != nil {
		return "", err
	}
	expected := commonbundle.Expected{
		BundleID:      normalized.BundleID,
		SchemaVersion: normalized.SchemaVersion,
		ItemCount:     normalized.ItemCount,
	}
	finalDir := filepath.Join(m.rootDir, normalized.SHA256)

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := commonbundle.LoadDirectory(finalDir, expected); err == nil {
		return finalDir, nil
	}
	if err := os.RemoveAll(finalDir); err != nil {
		return "", fmt.Errorf("remove invalid plugin bundle cache: %w", err)
	}

	archivePath, err := m.download(ctx, normalized)
	if err != nil {
		return "", err
	}
	defer os.Remove(archivePath)
	installDir, err := os.MkdirTemp(m.rootDir, ".install-")
	if err != nil {
		return "", fmt.Errorf("create plugin bundle staging directory: %w", err)
	}
	defer os.RemoveAll(installDir)
	if _, err := commonbundle.ExtractArchive(archivePath, installDir, expected); err != nil {
		return "", fmt.Errorf("validate plugin bundle: %w", err)
	}
	if err := os.Rename(installDir, finalDir); err != nil {
		if _, loadErr := commonbundle.LoadDirectory(finalDir, expected); loadErr == nil {
			return finalDir, nil
		}
		return "", fmt.Errorf("publish plugin bundle installation: %w", err)
	}
	if _, err := commonbundle.LoadDirectory(finalDir, expected); err != nil {
		_ = os.RemoveAll(finalDir)
		return "", fmt.Errorf("verify published plugin bundle installation: %w", err)
	}
	return finalDir, nil
}

func (m *PluginBundleManager) download(ctx context.Context, input PluginBundleInstallInput) (localPath string, err error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, input.ArtifactURL, nil)
	if err != nil {
		return "", fmt.Errorf("build plugin bundle download request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+input.SessionToken)

	client := m.client
	if client == nil {
		client = &http.Client{Timeout: pluginBundleDownloadTimeout}
	}
	requestClient := *client
	if requestClient.Timeout <= 0 {
		requestClient.Timeout = pluginBundleDownloadTimeout
	}
	requestClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	response, err := requestClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("download plugin bundle: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download plugin bundle failed: status=%d", response.StatusCode)
	}
	if response.ContentLength > input.SizeBytes {
		return "", errors.New("plugin bundle response exceeds expected size")
	}

	file, err := os.CreateTemp(m.rootDir, ".download-*.zip")
	if err != nil {
		return "", fmt.Errorf("create plugin bundle download file: %w", err)
	}
	localPath = file.Name()
	defer func() {
		closeErr := file.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(localPath)
		}
	}()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, input.SizeBytes+1))
	if err != nil {
		return "", fmt.Errorf("write plugin bundle download: %w", err)
	}
	if written != input.SizeBytes {
		return "", fmt.Errorf("plugin bundle size mismatch: expected=%d actual=%d", input.SizeBytes, written)
	}
	actualSHA256 := hex.EncodeToString(hash.Sum(nil))
	if actualSHA256 != input.SHA256 {
		return "", fmt.Errorf("plugin bundle sha256 mismatch: expected=%s actual=%s", input.SHA256, actualSHA256)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync plugin bundle download: %w", err)
	}
	return localPath, nil
}

func normalizePluginBundleInstallInput(input PluginBundleInstallInput) (PluginBundleInstallInput, error) {
	input.BundleID = strings.TrimSpace(input.BundleID)
	input.ArtifactURL = strings.TrimSpace(input.ArtifactURL)
	input.SHA256 = strings.ToLower(strings.TrimSpace(input.SHA256))
	input.SchemaVersion = strings.TrimSpace(input.SchemaVersion)
	input.NodeSessionID = strings.TrimSpace(input.NodeSessionID)
	input.SessionToken = strings.TrimSpace(input.SessionToken)
	if input.BundleID == "" {
		return PluginBundleInstallInput{}, errors.New("plugin bundle id is required")
	}
	digest, err := hex.DecodeString(input.SHA256)
	if err != nil || len(digest) != sha256.Size {
		return PluginBundleInstallInput{}, errors.New("plugin bundle sha256 is invalid")
	}
	if input.SizeBytes <= 0 || input.SizeBytes > commonbundle.MaxArtifactSize {
		return PluginBundleInstallInput{}, errors.New("plugin bundle size is outside the allowed range")
	}
	if input.ItemCount <= 0 || input.ItemCount > commonbundle.MaxMemberCount {
		return PluginBundleInstallInput{}, errors.New("plugin bundle item count is outside the allowed range")
	}
	if input.SchemaVersion != commonbundle.ManifestSchemaVersion {
		return PluginBundleInstallInput{}, fmt.Errorf("unsupported plugin bundle schema version %q", input.SchemaVersion)
	}
	if input.NodeSessionID == "" || input.SessionToken == "" {
		return PluginBundleInstallInput{}, errors.New("node session credentials are required for plugin bundle download")
	}
	parsedURL, err := url.Parse(input.ArtifactURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" ||
		(parsedURL.Scheme != "http" && parsedURL.Scheme != "https") ||
		parsedURL.User != nil || parsedURL.Fragment != "" {
		return PluginBundleInstallInput{}, errors.New("plugin bundle artifact URL is invalid")
	}
	if parsedURL.Query().Get("node_session_id") != input.NodeSessionID {
		return PluginBundleInstallInput{}, errors.New("plugin bundle artifact URL is not bound to the node session")
	}
	return input, nil
}
