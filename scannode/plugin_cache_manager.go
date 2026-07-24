package scannode

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/yaklang/yaklang/common/consts"
)

const (
	pluginCacheStatusReady      = "ready"
	pluginCacheManifestFileName = "manifest.json"
	pluginCacheArtifactFileName = "script.yak"
	pluginCacheDownloadTimeout  = 30 * time.Second
)

var (
	ErrInvalidPluginCacheReleaseID = errors.New("plugin_release_id is required")
	ErrInvalidPluginArtifactURI    = errors.New("artifact_uri is required")
	ErrInvalidPluginArtifactSHA256 = errors.New("artifact_sha256 is required")
	ErrInvalidPluginEntryKind      = errors.New("entry_kind is required")
	ErrUnsupportedPluginEntryKind  = errors.New("unsupported entry_kind")
	ErrUnsupportedArtifactScheme   = errors.New("unsupported artifact uri scheme")
	ErrArtifactHashMismatch        = errors.New("artifact sha256 mismatch")
	ErrArtifactSizeMismatch        = errors.New("artifact size mismatch")
	ErrPluginCacheMiss             = errors.New("plugin release is not cached")
)

type PluginCacheManagerConfig struct {
	NodeID     string
	BaseDir    string
	HTTPClient *http.Client
}

type PluginSyncInput struct {
	PluginID          string
	ReleaseID         string
	Version           string
	EntryKind         string
	ArtifactURI       string
	ArtifactSHA256    string
	ArtifactSizeBytes int64
}

type PluginSyncResult struct {
	PluginID        string
	ReleaseID       string
	Version         string
	EntryKind       string
	Status          string
	LocalPath       string
	CachedSizeBytes int64
	ObservedAt      time.Time
}

type PluginCacheManager struct {
	nodeID     string
	storeDir   string
	httpClient *http.Client
}

type pluginReleaseDocument struct {
	NodeID          string    `json:"node_id"`
	PluginID        string    `json:"plugin_id"`
	ReleaseID       string    `json:"release_id"`
	Version         string    `json:"version"`
	EntryKind       string    `json:"entry_kind"`
	ArtifactURI     string    `json:"artifact_uri"`
	ArtifactSHA256  string    `json:"artifact_sha256"`
	CachedSizeBytes int64     `json:"cached_size_bytes"`
	LocalPath       string    `json:"local_path"`
	SyncedAt        time.Time `json:"synced_at"`
}

type downloadedArtifact struct {
	tempPath string
	sha256   string
	size     int64
}

func newPluginCacheManager(cfg PluginCacheManagerConfig) *PluginCacheManager {
	baseDir := strings.TrimSpace(cfg.BaseDir)
	if baseDir == "" {
		baseDir = consts.GetDefaultYakitBaseDir()
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: pluginCacheDownloadTimeout}
	}
	return &PluginCacheManager{
		nodeID:     strings.TrimSpace(cfg.NodeID),
		storeDir:   filepath.Join(baseDir, "legion", "plugins"),
		httpClient: client,
	}
}

func (m *PluginCacheManager) Sync(
	ctx context.Context,
	input PluginSyncInput,
) (PluginSyncResult, error) {
	normalized, err := normalizePluginSyncInput(input)
	if err != nil {
		return PluginSyncResult{}, err
	}

	artifact, err := m.downloadArtifact(ctx, normalized.ArtifactURI)
	if err != nil {
		return PluginSyncResult{}, err
	}
	defer os.Remove(artifact.tempPath)

	if err := validateDownloadedArtifact(artifact, normalized); err != nil {
		return PluginSyncResult{}, err
	}
	localPath, err := m.persistArtifact(normalized.ReleaseID, artifact.tempPath)
	if err != nil {
		return PluginSyncResult{}, err
	}

	result := PluginSyncResult{
		PluginID:        normalized.PluginID,
		ReleaseID:       normalized.ReleaseID,
		Version:         normalized.Version,
		EntryKind:       normalized.EntryKind,
		Status:          pluginCacheStatusReady,
		LocalPath:       localPath,
		CachedSizeBytes: artifact.size,
		ObservedAt:      time.Now().UTC(),
	}
	if err := m.saveDocument(normalized, result); err != nil {
		return PluginSyncResult{}, err
	}
	return result, nil
}

func (m *PluginCacheManager) LoadScriptContent(releaseID string) (string, error) {
	normalized, err := normalizePluginReleaseID(releaseID)
	if err != nil {
		return "", err
	}
	document, err := m.loadDocument(normalized)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(document.LocalPath)
	if os.IsNotExist(err) {
		return "", ErrPluginCacheMiss
	}
	if err != nil {
		return "", fmt.Errorf("read cached plugin release: %w", err)
	}
	if len(raw) == 0 {
		return "", fmt.Errorf("cached plugin release is empty: %s", normalized)
	}
	return string(raw), nil
}

func normalizePluginSyncInput(input PluginSyncInput) (PluginSyncInput, error) {
	releaseID, err := normalizePluginReleaseID(input.ReleaseID)
	if err != nil {
		return PluginSyncInput{}, err
	}
	entryKind, err := normalizePluginEntryKind(input.EntryKind)
	if err != nil {
		return PluginSyncInput{}, err
	}
	artifactURI := strings.TrimSpace(input.ArtifactURI)
	if artifactURI == "" {
		return PluginSyncInput{}, ErrInvalidPluginArtifactURI
	}
	artifactSHA256 := strings.ToLower(strings.TrimSpace(input.ArtifactSHA256))
	if artifactSHA256 == "" {
		return PluginSyncInput{}, ErrInvalidPluginArtifactSHA256
	}
	return PluginSyncInput{
		PluginID:          strings.TrimSpace(input.PluginID),
		ReleaseID:         releaseID,
		Version:           strings.TrimSpace(input.Version),
		EntryKind:         entryKind,
		ArtifactURI:       artifactURI,
		ArtifactSHA256:    artifactSHA256,
		ArtifactSizeBytes: input.ArtifactSizeBytes,
	}, nil
}

func normalizePluginReleaseID(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", ErrInvalidPluginCacheReleaseID
	}
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '.', '-', '_':
			continue
		default:
			return "", ErrInvalidPluginCacheReleaseID
		}
	}
	return trimmed, nil
}

func normalizePluginEntryKind(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "":
		return "", ErrInvalidPluginEntryKind
	case "yak_script":
		return strings.TrimSpace(value), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedPluginEntryKind, strings.TrimSpace(value))
	}
}

func (m *PluginCacheManager) downloadArtifact(
	ctx context.Context,
	artifactURI string,
) (downloadedArtifact, error) {
	return downloadArtifact(ctx, m.httpClient, m.tempDir(), artifactURI)
}
