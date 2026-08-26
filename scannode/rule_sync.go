package scannode

import (
	"bytes"
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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	ssaconfig "github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"golang.org/x/sync/singleflight"
)

const (
	// activeRuleSnapshotEndpointPath is the node-accessible active-snapshot
	// manifest endpoint. It authenticates the node via the node_session_id
	// query parameter + Bearer session token (no platform user session needed).
	activeRuleSnapshotEndpointPath = "/v1/ssa-rule-sync-node/active-snapshot"
	ruleSnapshotBundleEndpointFmt  = "/v1/ssa-rule-sync-node/snapshots/%s"
	ruleSnapshotBundleFormatJSON   = "json"
	ruleSnapshotSchemaVersionV1    = "ssa_rule_snapshot_bundle.v1"
	ruleSnapshotExecutionV2        = "ssa.rule_snapshot.execution.v2"
	ruleSnapshotCacheReadyFile     = "READY"
	maxRuleSnapshotResponseBytes   = 256 << 20
	// nodeSessionIDQueryParam is the query parameter carrying the node session
	// id used by the server to authenticate the node.
	nodeSessionIDQueryParam = "node_session_id"
)

type ruleSyncer interface {
	PrepareSnapshot(context.Context, RuleSnapshotExpectation) (*PreparedRuleSnapshot, error)
}

type RuleSyncBundleImporter func(context.Context, RuleSnapshotBundle) (int, error)

type RuleSyncConfig struct {
	ServerURL   string `json:"server_url"`
	BearerToken string `json:"bearer_token,omitempty"`
	// NodeSessionID is the node session id sent as the node_session_id query
	// parameter so the server can authenticate the node via its session token.
	// It is populated after bootstrap completes (see UpdateCredentials).
	NodeSessionID string                 `json:"node_session_id,omitempty"`
	SyncEnabled   bool                   `json:"sync_enabled"`
	CacheDir      string                 `json:"cache_dir,omitempty"`
	Client        *http.Client           `json:"-"`
	Importer      RuleSyncBundleImporter `json:"-"`
}

type RuleSyncClient struct {
	config       *RuleSyncConfig
	httpClient   *http.Client
	cacheDir     string
	importBundle RuleSyncBundleImporter
	mu           sync.RWMutex
	downloads    singleflight.Group
}

// RuleSnapshotExpectation is the immutable snapshot identity supplied by the
// dispatching platform. SnapshotID is required. The remaining fields are
// optional for legacy commands, but when present they pin execution to the
// exact canonical bundle selected by the platform.
type RuleSnapshotExpectation struct {
	SnapshotID    string   `json:"snapshot_id"`
	ContentSHA256 string   `json:"content_sha256,omitempty"`
	BundleFormat  string   `json:"bundle_format,omitempty"`
	SchemaVersion string   `json:"schema_version,omitempty"`
	AssetIDs      []string `json:"asset_ids,omitempty"`
}

type RuleSnapshotPreparationReceipt struct {
	CapabilityKey string    `json:"capability_key"`
	State         string    `json:"state"`
	SnapshotID    string    `json:"snapshot_id"`
	ContentSHA256 string    `json:"content_sha256"`
	BundleFormat  string    `json:"bundle_format"`
	SchemaVersion string    `json:"schema_version"`
	AssetCount    int       `json:"asset_count"`
	CacheHit      bool      `json:"cache_hit"`
	PreparedAt    time.Time `json:"prepared_at"`
}

type PreparedRuleSnapshot struct {
	Bundle        RuleSnapshotBundle
	Receipt       RuleSnapshotPreparationReceipt
	taskYakitHome string
	cleanup       func()
}

func (p *PreparedRuleSnapshot) Cleanup() {
	if p != nil && p.cleanup != nil {
		p.cleanup()
		p.cleanup = nil
	}
}

type downloadedRuleSnapshot struct {
	Bundle   *RuleSnapshotBundle
	CacheHit bool
}

type RuleSnapshotManifest struct {
	SnapshotID    string    `json:"snapshot_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	AssetCount    int       `json:"asset_count"`
	IsActive      bool      `json:"is_active"`
	PublishedAt   time.Time `json:"published_at"`
	ContentSHA256 string    `json:"content_sha256"`
	BundleFormat  string    `json:"bundle_format"`
	SchemaVersion string    `json:"schema_version"`
}

type RuleSnapshotItem struct {
	AssetID       string          `json:"asset_id"`
	SourceRuleID  string          `json:"source_rule_id,omitempty"`
	Name          string          `json:"name"`
	Title         string          `json:"title,omitempty"`
	TitleZh       string          `json:"title_zh,omitempty"`
	Language      string          `json:"language,omitempty"`
	Purpose       string          `json:"purpose,omitempty"`
	Tag           string          `json:"tag,omitempty"`
	CWE           []string        `json:"cwe,omitempty"`
	CVE           string          `json:"cve,omitempty"`
	RiskType      string          `json:"risk_type,omitempty"`
	Type          string          `json:"type,omitempty"`
	Severity      string          `json:"severity,omitempty"`
	Description   string          `json:"description,omitempty"`
	Solution      string          `json:"solution,omitempty"`
	Version       string          `json:"version,omitempty"`
	ContentHash   string          `json:"content_hash,omitempty"`
	IsBuiltin     bool            `json:"is_builtin,omitempty"`
	Verified      bool            `json:"verified,omitempty"`
	AllowIncluded bool            `json:"allow_included,omitempty"`
	IncludedName  string          `json:"included_name,omitempty"`
	Groups        []string        `json:"groups,omitempty"`
	AlertDesc     json.RawMessage `json:"alert_desc,omitempty"`
	Content       string          `json:"content"`
}

type RuleSnapshotBundle struct {
	RuleSnapshotManifest
	Items []RuleSnapshotItem `json:"items"`
}

type ruleSyncHTTPError struct {
	StatusCode int
	Message    string
	Body       string
}

func (e *ruleSyncHTTPError) Error() string {
	if e == nil {
		return "rule sync transport status=0"
	}
	if e.Message != "" {
		return fmt.Sprintf("rule sync transport status=%d error=%s", e.StatusCode, e.Message)
	}
	if e.Body != "" {
		return fmt.Sprintf("rule sync transport status=%d body=%s", e.StatusCode, e.Body)
	}
	return fmt.Sprintf("rule sync transport status=%d", e.StatusCode)
}

func NewRuleSyncClient(config *RuleSyncConfig) *RuleSyncClient {
	if config == nil {
		config = &RuleSyncConfig{}
	}

	cacheDir := strings.TrimSpace(config.CacheDir)
	if cacheDir == "" {
		cacheDir = filepath.Join(utils.GetHomeDirDefault("/tmp"), ".palm-desktop", "rule_cache")
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		log.Warnf("create rule sync cache dir failed: %v", err)
	} else if err := os.Chmod(cacheDir, 0o700); err != nil {
		log.Warnf("secure rule sync cache dir failed: %v", err)
	}

	httpClient := config.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	importer := config.Importer

	return &RuleSyncClient{
		config:       config,
		httpClient:   httpClient,
		cacheDir:     cacheDir,
		importBundle: importer,
	}
}

func (c *RuleSyncClient) GetActiveSnapshot(ctx context.Context) (*RuleSnapshotManifest, error) {
	if err := c.validateConfigured(); err != nil {
		return nil, err
	}

	var manifest RuleSnapshotManifest
	if err := c.getJSON(ctx, activeRuleSnapshotEndpointPath, &manifest); err != nil {
		return nil, utils.Wrap(err, "request active snapshot manifest failed")
	}
	if err := validateRuleSnapshotManifest(manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func (c *RuleSyncClient) DownloadSnapshotBundle(
	ctx context.Context,
	snapshotID string,
) (*RuleSnapshotBundle, error) {
	downloaded, err := c.downloadSnapshotBundle(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	return downloaded.Bundle, nil
}

func (c *RuleSyncClient) downloadSnapshotBundle(
	ctx context.Context,
	snapshotID string,
) (*downloadedRuleSnapshot, error) {
	if err := c.validateConfigured(); err != nil {
		return nil, err
	}

	normalizedID := strings.TrimSpace(snapshotID)
	if normalizedID == "" {
		return nil, utils.Error("rule snapshot id is required")
	}

	if bundle, err := c.loadCachedSnapshotBundle(normalizedID); err == nil {
		log.Infof("loaded rule snapshot from cache: %s", normalizedID)
		return &downloadedRuleSnapshot{Bundle: bundle, CacheHit: true}, nil
	}

	result, err, _ := c.downloads.Do(normalizedID, func() (any, error) {
		if bundle, cacheErr := c.loadCachedSnapshotBundle(normalizedID); cacheErr == nil {
			return &downloadedRuleSnapshot{Bundle: bundle, CacheHit: true}, nil
		}

		endpoint := fmt.Sprintf(ruleSnapshotBundleEndpointFmt, url.PathEscape(normalizedID))
		raw, requestErr := c.getRaw(ctx, endpoint)
		if requestErr != nil {
			return nil, utils.Wrap(requestErr, "download snapshot bundle failed")
		}

		bundle, decodeErr := decodeRuleSnapshotBundle(raw)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if validationErr := validateRuleSnapshotBundle(bundle, normalizedID, raw); validationErr != nil {
			return nil, validationErr
		}
		if cacheErr := c.cacheSnapshotBundle(bundle, raw); cacheErr != nil {
			return nil, utils.Wrap(cacheErr, "cache snapshot bundle failed")
		}

		log.Infof(
			"downloaded rule snapshot bundle: snapshot=%s sha=%s items=%d",
			bundle.SnapshotID,
			bundle.ContentSHA256,
			len(bundle.Items),
		)
		return &downloadedRuleSnapshot{Bundle: &bundle}, nil
	})
	if err != nil {
		return nil, utils.Wrap(err, "download snapshot bundle failed")
	}
	downloaded, ok := result.(*downloadedRuleSnapshot)
	if !ok || downloaded == nil || downloaded.Bundle == nil {
		return nil, utils.Error("download snapshot bundle returned an invalid result")
	}
	return downloaded, nil
}

func (c *RuleSyncClient) HasLocalSnapshot(snapshotID string) bool {
	normalizedID := strings.TrimSpace(snapshotID)
	if normalizedID == "" {
		return false
	}
	_, err := c.loadCachedSnapshotBundle(normalizedID)
	return err == nil
}

func (c *RuleSyncClient) SyncActiveSnapshot(
	ctx context.Context,
) (ruleCount int, err error) {
	if c.config == nil || !c.config.SyncEnabled {
		return 0, utils.Error("rule sync disabled")
	}

	manifest, err := c.GetActiveSnapshot(ctx)
	if err != nil {
		return 0, utils.Wrap(err, "get active snapshot failed")
	}
	return c.SyncSnapshot(ctx, manifest.SnapshotID)
}

func (c *RuleSyncClient) SyncSnapshot(
	ctx context.Context,
	snapshotID string,
) (ruleCount int, err error) {
	if c.config == nil || !c.config.SyncEnabled {
		return 0, utils.Error("rule sync disabled")
	}

	prepared, err := c.PrepareSnapshot(ctx, RuleSnapshotExpectation{SnapshotID: snapshotID})
	if err != nil {
		return 0, err
	}
	if c.importBundle == nil {
		return prepared.Receipt.AssetCount, nil
	}

	ruleCount, err = c.importBundle(ctx, prepared.Bundle)
	if err != nil {
		return 0, utils.Wrap(err, "import snapshot bundle failed")
	}

	log.Infof(
		"successfully imported %d rules from snapshot %s (%s)",
		ruleCount,
		prepared.Bundle.SnapshotID,
		prepared.Bundle.ContentSHA256,
	)
	return ruleCount, nil
}

func (c *RuleSyncClient) PrepareSnapshot(
	ctx context.Context,
	expectation RuleSnapshotExpectation,
) (*PreparedRuleSnapshot, error) {
	normalized, err := normalizeRuleSnapshotExpectation(expectation)
	if err != nil {
		return nil, err
	}
	downloaded, err := c.downloadSnapshotBundle(ctx, normalized.SnapshotID)
	if err != nil {
		return nil, utils.Wrap(err, "prepare rule snapshot failed")
	}
	bundle := downloaded.Bundle
	if err := validatePreparedRuleSnapshot(*bundle, normalized); err != nil {
		return nil, err
	}
	return &PreparedRuleSnapshot{
		Bundle: *bundle,
		Receipt: RuleSnapshotPreparationReceipt{
			CapabilityKey: ruleSnapshotExecutionV2,
			State:         ruleSnapshotCacheReadyFile,
			SnapshotID:    bundle.SnapshotID,
			ContentSHA256: bundle.ContentSHA256,
			BundleFormat:  bundle.BundleFormat,
			SchemaVersion: bundle.SchemaVersion,
			AssetCount:    len(bundle.Items),
			CacheHit:      downloaded.CacheHit,
			PreparedAt:    time.Now().UTC(),
		},
	}, nil
}

func buildSyntaxFlowRuleFromSnapshotItem(item RuleSnapshotItem) (*schema.SyntaxFlowRule, error) {
	content := strings.TrimSpace(item.Content)
	if content == "" {
		return nil, utils.Error("snapshot rule content is required")
	}

	rule, err := sfdb.CheckSyntaxFlowRuleContent(content)
	if err != nil {
		return nil, utils.Wrapf(err, "parse syntax flow rule %s failed", item.Name)
	}
	if rule == nil {
		rule = &schema.SyntaxFlowRule{}
	}
	contentAllowIncluded := rule.AllowIncluded
	contentIncludedName := strings.TrimSpace(rule.IncludedName)
	itemIncludedName := strings.TrimSpace(item.IncludedName)
	if item.AllowIncluded != contentAllowIncluded {
		return nil, utils.Errorf(
			"snapshot rule %s allow_included does not match compiled content",
			item.Name,
		)
	}
	if item.AllowIncluded && itemIncludedName != contentIncludedName {
		return nil, utils.Errorf(
			"snapshot rule %s included_name does not match compiled content",
			item.Name,
		)
	}

	rule.Content = content
	if language := strings.TrimSpace(item.Language); language != "" {
		validated, err := ssaconfig.ValidateLanguage(language)
		if err != nil {
			return nil, utils.Wrapf(err, "validate snapshot rule %s language", item.Name)
		}
		rule.Language = validated
	}
	rule.Purpose = schema.SyntaxFlowRulePurposeType(strings.TrimSpace(item.Purpose))
	rule.Tag = strings.TrimSpace(item.Tag)
	rule.CWE = schema.StringArray(item.CWE)
	rule.CVE = strings.TrimSpace(item.CVE)
	rule.Type = schema.ValidRuleType(item.Type)
	rule.AllowIncluded = item.AllowIncluded
	rule.IncludedName = itemIncludedName
	rule.IsBuildInRule = item.IsBuiltin
	rule.Verified = item.Verified
	rule.Solution = strings.TrimSpace(item.Solution)
	rule.Version = strings.TrimSpace(item.Version)
	if name := strings.TrimSpace(item.Name); name != "" {
		rule.RuleName = name
		if strings.TrimSpace(rule.Title) == "" {
			rule.Title = name
		}
	}
	if strings.TrimSpace(rule.RuleName) == "" {
		return nil, utils.Error("snapshot rule name is required")
	}
	if description := strings.TrimSpace(item.Description); description != "" {
		rule.Description = description
	}
	if title := strings.TrimSpace(item.Title); title != "" {
		rule.Title = title
	}
	if titleZh := strings.TrimSpace(item.TitleZh); titleZh != "" {
		rule.TitleZh = titleZh
	}
	if riskType := strings.TrimSpace(item.RiskType); riskType != "" {
		rule.RiskType = riskType
	}
	if severity := strings.TrimSpace(item.Severity); severity != "" {
		rule.Severity = schema.ValidSeverityType(severity)
	}
	rule.NeedUpdate = false
	rule.NormalizeMode()
	return rule, nil
}

func validateRuleSnapshotManifest(manifest RuleSnapshotManifest) error {
	if strings.TrimSpace(manifest.SnapshotID) == "" {
		return utils.Error("snapshot manifest snapshot_id is required")
	}
	if manifest.SnapshotID != strings.TrimSpace(manifest.SnapshotID) {
		return utils.Error("snapshot manifest snapshot_id must be canonical")
	}
	if _, err := normalizeRuleSnapshotSHA256(manifest.ContentSHA256); err != nil {
		return err
	}
	bundleFormat := strings.TrimSpace(manifest.BundleFormat)
	if bundleFormat != manifest.BundleFormat {
		return utils.Error("snapshot manifest bundle_format must be canonical")
	}
	if bundleFormat != ruleSnapshotBundleFormatJSON {
		return utils.Errorf("unsupported snapshot bundle format: %s", bundleFormat)
	}
	schemaVersion := strings.TrimSpace(manifest.SchemaVersion)
	if schemaVersion != manifest.SchemaVersion {
		return utils.Error("snapshot manifest schema_version must be canonical")
	}
	if schemaVersion != ruleSnapshotSchemaVersionV1 {
		return utils.Errorf("unsupported snapshot schema version: %s", schemaVersion)
	}
	if manifest.AssetCount < 0 {
		return utils.Error("snapshot manifest asset_count must not be negative")
	}
	return nil
}

func validateRuleSnapshotBundle(bundle RuleSnapshotBundle, expectedSnapshotID string, raw []byte) error {
	if err := validateRuleSnapshotManifest(bundle.RuleSnapshotManifest); err != nil {
		return err
	}
	if strings.TrimSpace(expectedSnapshotID) != "" &&
		strings.TrimSpace(bundle.SnapshotID) != strings.TrimSpace(expectedSnapshotID) {
		return utils.Errorf(
			"snapshot bundle id mismatch: want %s got %s",
			expectedSnapshotID,
			bundle.SnapshotID,
		)
	}
	if bundle.AssetCount != len(bundle.Items) {
		return utils.Errorf(
			"snapshot bundle asset_count mismatch: manifest=%d items=%d",
			bundle.AssetCount,
			len(bundle.Items),
		)
	}
	if len(bundle.Items) == 0 {
		return utils.Error("snapshot bundle must contain at least one rule")
	}
	seenAssetIDs := make(map[string]struct{}, len(bundle.Items))
	seenNames := make(map[string]struct{}, len(bundle.Items))
	seenLibraries := make(map[string]string)
	for index, item := range bundle.Items {
		assetID := strings.TrimSpace(item.AssetID)
		name := strings.TrimSpace(item.Name)
		switch {
		case assetID == "":
			return utils.Errorf("snapshot bundle item %d asset_id is required", index)
		case assetID != item.AssetID:
			return utils.Errorf("snapshot bundle item %d asset_id must be canonical", index)
		case name == "":
			return utils.Errorf("snapshot bundle item %d name is required", index)
		case name != item.Name:
			return utils.Errorf("snapshot bundle item %d name must be canonical", index)
		case strings.TrimSpace(item.Content) == "":
			return utils.Errorf("snapshot bundle item %d content is required", index)
		}
		if _, exists := seenAssetIDs[assetID]; exists {
			return utils.Errorf("snapshot bundle contains duplicate asset_id: %s", assetID)
		}
		if _, exists := seenNames[name]; exists {
			return utils.Errorf("snapshot bundle contains duplicate rule name: %s", name)
		}
		seenAssetIDs[assetID] = struct{}{}
		seenNames[name] = struct{}{}
		rule, err := buildSyntaxFlowRuleFromSnapshotItem(item)
		if err != nil {
			return utils.Wrapf(err, "validate snapshot bundle item %d", index)
		}
		if rule.AllowIncluded {
			for _, libraryName := range []string{rule.Title, rule.IncludedName} {
				libraryName = strings.TrimSpace(libraryName)
				if libraryName == "" {
					continue
				}
				if owner, exists := seenLibraries[libraryName]; exists && owner != assetID {
					return utils.Errorf("snapshot bundle contains duplicate library name: %s", libraryName)
				}
				seenLibraries[libraryName] = assetID
			}
		}
	}
	computedHash, err := calculateRuleSnapshotItemsSHA256FromRaw(raw)
	if err != nil {
		return err
	}
	if bundle.ContentSHA256 != computedHash {
		return utils.Errorf(
			"snapshot bundle content_sha256 mismatch: want %s got %s",
			computedHash,
			bundle.ContentSHA256,
		)
	}
	return nil
}

func decodeRuleSnapshotBundle(raw []byte) (RuleSnapshotBundle, error) {
	var bundle RuleSnapshotBundle
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return RuleSnapshotBundle{}, utils.Wrap(err, "decode snapshot bundle failed")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return RuleSnapshotBundle{}, utils.Error("decode snapshot bundle failed: trailing JSON value")
	}
	return bundle, nil
}

func normalizeRuleSnapshotSHA256(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", utils.Error("snapshot manifest content_sha256 is required")
	}
	if trimmed != strings.ToLower(trimmed) {
		return "", utils.Error("snapshot manifest content_sha256 must be lowercase hexadecimal")
	}
	raw, err := hex.DecodeString(trimmed)
	if err != nil || len(raw) != sha256.Size {
		return "", utils.Error("snapshot manifest content_sha256 must be a 64-character SHA-256")
	}
	return trimmed, nil
}

func normalizeRuleSnapshotExpectation(
	expectation RuleSnapshotExpectation,
) (RuleSnapshotExpectation, error) {
	expectation.AssetIDs = append([]string(nil), expectation.AssetIDs...)
	expectation.SnapshotID = strings.TrimSpace(expectation.SnapshotID)
	if expectation.SnapshotID == "" {
		return RuleSnapshotExpectation{}, utils.Error("rule snapshot id is required")
	}
	if strings.TrimSpace(expectation.ContentSHA256) != "" {
		sha, err := normalizeRuleSnapshotSHA256(expectation.ContentSHA256)
		if err != nil {
			return RuleSnapshotExpectation{}, utils.Wrap(err, "invalid expected rule snapshot digest")
		}
		expectation.ContentSHA256 = sha
	}
	expectation.BundleFormat = strings.TrimSpace(expectation.BundleFormat)
	expectation.SchemaVersion = strings.TrimSpace(expectation.SchemaVersion)
	if expectation.BundleFormat != "" && expectation.BundleFormat != ruleSnapshotBundleFormatJSON {
		return RuleSnapshotExpectation{}, utils.Errorf(
			"unsupported expected rule snapshot bundle format: %s",
			expectation.BundleFormat,
		)
	}
	if expectation.SchemaVersion != "" && expectation.SchemaVersion != ruleSnapshotSchemaVersionV1 {
		return RuleSnapshotExpectation{}, utils.Errorf(
			"unsupported expected rule snapshot schema version: %s",
			expectation.SchemaVersion,
		)
	}
	if len(expectation.AssetIDs) > 0 {
		seen := make(map[string]struct{}, len(expectation.AssetIDs))
		for index := range expectation.AssetIDs {
			expectation.AssetIDs[index] = strings.TrimSpace(expectation.AssetIDs[index])
			if expectation.AssetIDs[index] == "" {
				return RuleSnapshotExpectation{}, utils.Error("expected rule snapshot asset_ids must not contain an empty value")
			}
			if _, exists := seen[expectation.AssetIDs[index]]; exists {
				return RuleSnapshotExpectation{}, utils.Errorf(
					"expected rule snapshot contains duplicate asset_id: %s",
					expectation.AssetIDs[index],
				)
			}
			seen[expectation.AssetIDs[index]] = struct{}{}
		}
		sort.Strings(expectation.AssetIDs)
	}
	return expectation, nil
}

func validatePreparedRuleSnapshot(
	bundle RuleSnapshotBundle,
	expectation RuleSnapshotExpectation,
) error {
	if bundle.SnapshotID != expectation.SnapshotID {
		return utils.Errorf(
			"prepared rule snapshot id mismatch: expected=%s actual=%s",
			expectation.SnapshotID,
			bundle.SnapshotID,
		)
	}
	if expectation.ContentSHA256 != "" && bundle.ContentSHA256 != expectation.ContentSHA256 {
		return utils.Errorf(
			"prepared rule snapshot content_sha256 mismatch: expected=%s actual=%s",
			expectation.ContentSHA256,
			bundle.ContentSHA256,
		)
	}
	if expectation.BundleFormat != "" && bundle.BundleFormat != expectation.BundleFormat {
		return utils.Errorf(
			"prepared rule snapshot bundle_format mismatch: expected=%s actual=%s",
			expectation.BundleFormat,
			bundle.BundleFormat,
		)
	}
	if expectation.SchemaVersion != "" && bundle.SchemaVersion != expectation.SchemaVersion {
		return utils.Errorf(
			"prepared rule snapshot schema_version mismatch: expected=%s actual=%s",
			expectation.SchemaVersion,
			bundle.SchemaVersion,
		)
	}
	if len(expectation.AssetIDs) > 0 {
		actualAssetIDs := make([]string, len(bundle.Items))
		for index := range bundle.Items {
			actualAssetIDs[index] = bundle.Items[index].AssetID
		}
		sort.Strings(actualAssetIDs)
		if len(actualAssetIDs) != len(expectation.AssetIDs) {
			return utils.Errorf(
				"prepared rule snapshot asset_ids count mismatch: expected=%d actual=%d",
				len(expectation.AssetIDs),
				len(actualAssetIDs),
			)
		}
		for index, actualAssetID := range actualAssetIDs {
			if actualAssetID != expectation.AssetIDs[index] {
				return utils.Errorf(
					"prepared rule snapshot asset_ids mismatch at index %d: expected=%s actual=%s",
					index,
					expectation.AssetIDs[index],
					actualAssetID,
				)
			}
		}
	}
	return nil
}

func calculateRuleSnapshotItemsSHA256(items []RuleSnapshotItem) (string, error) {
	payload, err := json.Marshal(canonicalRuleSnapshotItems(items))
	if err != nil {
		return "", utils.Wrap(err, "marshal snapshot items failed")
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// calculateRuleSnapshotItemsSHA256FromRaw hashes the exact items array bytes
// received over the wire. The server computes content_sha256 over the same
// canonical JSON it serializes, so re-marshaling the decoded struct would
// silently accept a payload whose wire fields differ from the hash input.
func calculateRuleSnapshotItemsSHA256FromRaw(raw []byte) (string, error) {
	var payload struct {
		Items json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", utils.Wrap(err, "decode snapshot bundle items failed")
	}
	if len(payload.Items) == 0 {
		return "", utils.Error("snapshot bundle items are required")
	}
	sum := sha256.Sum256(payload.Items)
	return hex.EncodeToString(sum[:]), nil
}

func canonicalRuleSnapshotItems(items []RuleSnapshotItem) []RuleSnapshotItem {
	canonical := make([]RuleSnapshotItem, len(items))
	copy(canonical, items)
	sort.Slice(canonical, func(left, right int) bool {
		if canonical[left].Name != canonical[right].Name {
			return canonical[left].Name < canonical[right].Name
		}
		return canonical[left].AssetID < canonical[right].AssetID
	})
	return canonical
}

func (c *RuleSyncClient) validateConfigured() error {
	if c.config == nil || strings.TrimSpace(c.config.ServerURL) == "" {
		return utils.Error("rule sync not configured")
	}
	return nil
}

func (c *RuleSyncClient) getJSON(ctx context.Context, path string, target any) error {
	raw, err := c.getRaw(ctx, path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return utils.Wrap(err, "decode rule sync response failed")
	}
	return nil
}

func (c *RuleSyncClient) getRaw(ctx context.Context, path string) ([]byte, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(c.config.ServerURL), "/")

	// Build the request URL with the node_session_id query parameter so the
	// server can authenticate the node via its session token.
	requestURL := baseURL + path
	c.mu.RLock()
	sessionID := strings.TrimSpace(c.config.NodeSessionID)
	c.mu.RUnlock()
	if sessionID != "" {
		separator := "&"
		if !strings.Contains(requestURL, "?") {
			separator = "?"
		}
		requestURL = requestURL + separator + nodeSessionIDQueryParam + "=" + url.QueryEscape(sessionID)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, utils.Wrap(err, "build rule sync request failed")
	}
	c.mu.RLock()
	token := strings.TrimSpace(c.config.BearerToken)
	c.mu.RUnlock()
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, utils.Wrap(err, "send rule sync request failed")
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		return nil, readRuleSyncHTTPError(response)
	}

	if response.ContentLength > maxRuleSnapshotResponseBytes {
		return nil, utils.Error("rule sync response exceeds the size limit")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxRuleSnapshotResponseBytes+1))
	if err != nil {
		return nil, utils.Wrap(err, "read rule sync response failed")
	}
	if len(body) > maxRuleSnapshotResponseBytes {
		return nil, utils.Error("rule sync response exceeds the size limit")
	}
	return body, nil
}

// UpdateCredentials updates the node session credentials used to authenticate
// rule sync requests. It is called after bootstrap completes so the client
// can talk to the node-accessible snapshot endpoints with a valid session.
func (c *RuleSyncClient) UpdateCredentials(nodeSessionID, sessionToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config.BearerToken = strings.TrimSpace(sessionToken)
	c.config.NodeSessionID = strings.TrimSpace(nodeSessionID)
}

func readRuleSyncHTTPError(response *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return utils.Errorf("rule sync transport status=%d read_body=%v", response.StatusCode, err)
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return &ruleSyncHTTPError{StatusCode: response.StatusCode}
	}

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.Error) != "" {
		return &ruleSyncHTTPError{
			StatusCode: response.StatusCode,
			Message:    strings.TrimSpace(payload.Error),
			Body:       trimmed,
		}
	}
	return &ruleSyncHTTPError{
		StatusCode: response.StatusCode,
		Body:       trimmed,
	}
}

func (c *RuleSyncClient) loadCachedSnapshotBundle(
	snapshotID string,
) (*RuleSnapshotBundle, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if err := secureRuleSnapshotCacheDirectory(c.cacheDir); err != nil {
		return nil, err
	}
	if err := secureRuleSnapshotCacheDirectory(c.snapshotRefsDir()); err != nil {
		return nil, err
	}
	refPath := c.snapshotRefPath(snapshotID)
	if err := secureRuleSnapshotCacheFile(refPath); err != nil {
		return nil, err
	}
	contentSHA, err := os.ReadFile(refPath)
	if err != nil {
		return nil, err
	}
	normalizedSHA, err := normalizeRuleSnapshotSHA256(string(contentSHA))
	if err != nil {
		return nil, err
	}
	if err := secureRuleSnapshotCacheDirectory(c.snapshotObjectsDir()); err != nil {
		return nil, err
	}
	if err := secureRuleSnapshotCacheDirectory(c.snapshotObjectDir(normalizedSHA)); err != nil {
		return nil, err
	}
	readyPath := c.snapshotReadyPath(normalizedSHA)
	if err := secureRuleSnapshotCacheFile(readyPath); err != nil {
		return nil, err
	}
	ready, err := os.ReadFile(readyPath)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(ready)) != normalizedSHA {
		return nil, utils.Error("snapshot cache READY marker does not match content digest")
	}
	bundlePath := c.snapshotBundlePath(normalizedSHA)
	if err := secureRuleSnapshotCacheFile(bundlePath); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(bundlePath)
	if err != nil {
		return nil, err
	}

	bundle, err := decodeRuleSnapshotBundle(raw)
	if err != nil {
		return nil, err
	}
	if err := validateRuleSnapshotBundle(bundle, "", raw); err != nil {
		return nil, err
	}
	if bundle.ContentSHA256 != normalizedSHA {
		return nil, utils.Error("snapshot cache ref does not match bundle content digest")
	}
	bundle.SnapshotID = snapshotID
	return &bundle, nil
}

func (c *RuleSyncClient) cacheSnapshotBundle(bundle RuleSnapshotBundle, raw []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := validateRuleSnapshotBundle(bundle, bundle.SnapshotID, raw); err != nil {
		return err
	}
	contentSHA := bundle.ContentSHA256
	if err := os.MkdirAll(c.snapshotObjectsDir(), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(c.snapshotObjectsDir(), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(c.snapshotRefsDir(), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(c.snapshotRefsDir(), 0o700); err != nil {
		return err
	}

	if !c.snapshotObjectReady(contentSHA) {
		tempDir, err := os.MkdirTemp(c.snapshotObjectsDir(), "."+contentSHA+".tmp-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempDir)

		if err := writeRuleSnapshotFile(filepath.Join(tempDir, "bundle.json"), raw, 0o600); err != nil {
			return err
		}
		if err := writeRuleSnapshotFile(
			filepath.Join(tempDir, ruleSnapshotCacheReadyFile),
			[]byte(contentSHA+"\n"),
			0o600,
		); err != nil {
			return err
		}
		if err := os.Rename(tempDir, c.snapshotObjectDir(contentSHA)); err != nil {
			if !c.snapshotObjectReady(contentSHA) {
				return err
			}
		}
	}
	if err := secureRuleSnapshotCacheDirectory(c.snapshotObjectDir(contentSHA)); err != nil {
		return err
	}
	if err := secureRuleSnapshotCacheFile(c.snapshotBundlePath(contentSHA)); err != nil {
		return err
	}
	if err := secureRuleSnapshotCacheFile(c.snapshotReadyPath(contentSHA)); err != nil {
		return err
	}

	if err := writeRuleSnapshotFileAtomic(
		c.snapshotRefPath(bundle.SnapshotID),
		[]byte(contentSHA+"\n"),
		0o600,
	); err != nil {
		return err
	}
	return nil
}

func (c *RuleSyncClient) snapshotRefPath(snapshotID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(snapshotID)))
	return filepath.Join(c.snapshotRefsDir(), hex.EncodeToString(digest[:])+".ref")
}

func (c *RuleSyncClient) snapshotBundlePath(contentSHA string) string {
	return filepath.Join(c.snapshotObjectDir(contentSHA), "bundle.json")
}

func (c *RuleSyncClient) snapshotObjectsDir() string {
	return filepath.Join(c.cacheDir, "objects")
}

func (c *RuleSyncClient) snapshotRefsDir() string {
	return filepath.Join(c.cacheDir, "refs")
}

func (c *RuleSyncClient) snapshotObjectDir(contentSHA string) string {
	return filepath.Join(c.snapshotObjectsDir(), contentSHA)
}

func (c *RuleSyncClient) snapshotReadyPath(contentSHA string) string {
	return filepath.Join(c.snapshotObjectDir(contentSHA), ruleSnapshotCacheReadyFile)
}

func (c *RuleSyncClient) snapshotObjectReady(contentSHA string) bool {
	ready, err := os.ReadFile(c.snapshotReadyPath(contentSHA))
	if err != nil || strings.TrimSpace(string(ready)) != contentSHA {
		return false
	}
	raw, err := os.ReadFile(c.snapshotBundlePath(contentSHA))
	if err != nil {
		return false
	}
	bundle, err := decodeRuleSnapshotBundle(raw)
	if err != nil || validateRuleSnapshotBundle(bundle, "", raw) != nil {
		return false
	}
	return bundle.ContentSHA256 == contentSHA
}

func writeRuleSnapshotFile(path string, raw []byte, mode os.FileMode) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()
	if _, err := file.Write(raw); err != nil {
		return err
	}
	return file.Sync()
}

func writeRuleSnapshotFileAtomic(path string, raw []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func secureRuleSnapshotCacheDirectory(path string) error {
	if err := os.Chmod(path, 0o700); err != nil {
		return utils.Wrapf(err, "secure rule snapshot cache directory %s", path)
	}
	return nil
}

func secureRuleSnapshotCacheFile(path string) error {
	if err := os.Chmod(path, 0o600); err != nil {
		return utils.Wrapf(err, "secure rule snapshot cache file %s", path)
	}
	return nil
}

var _ ruleSyncer = (*RuleSyncClient)(nil)
