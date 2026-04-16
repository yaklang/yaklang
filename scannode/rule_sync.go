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
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	activeRuleSnapshotEndpointPath = "/v1/ssa-rule-sync/active-snapshot"
	ruleSnapshotBundleEndpointFmt  = "/v1/ssa-rule-sync/snapshots/%s"
	ruleSnapshotBundleFormatJSON   = "json"
	ruleSnapshotSchemaVersionV1    = "ssa_rule_snapshot_bundle.v1"
)

type ruleSyncer interface {
	HasLocalSnapshot(string) bool
	SyncSnapshot(context.Context, string) (int, error)
}

type RuleSyncBundleImporter func(context.Context, RuleSnapshotBundle) (int, error)

type RuleSyncConfig struct {
	ServerURL   string                 `json:"server_url"`
	BearerToken string                 `json:"bearer_token,omitempty"`
	SyncEnabled bool                   `json:"sync_enabled"`
	CacheDir    string                 `json:"cache_dir,omitempty"`
	Client      *http.Client           `json:"-"`
	Importer    RuleSyncBundleImporter `json:"-"`
}

type RuleSyncClient struct {
	config       *RuleSyncConfig
	httpClient   *http.Client
	cacheDir     string
	importBundle RuleSyncBundleImporter
	mu           sync.RWMutex
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
	AssetID     string `json:"asset_id"`
	Name        string `json:"name"`
	RiskType    string `json:"risk_type,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Description string `json:"description,omitempty"`
	Content     string `json:"content"`
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
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		log.Warnf("create rule sync cache dir failed: %v", err)
	}

	httpClient := config.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	importer := config.Importer
	if importer == nil {
		importer = defaultRuleSnapshotBundleImporter
	}

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
	if err := c.validateConfigured(); err != nil {
		return nil, err
	}

	normalizedID := strings.TrimSpace(snapshotID)
	if normalizedID == "" {
		return nil, utils.Error("rule snapshot id is required")
	}

	if bundle, err := c.loadCachedSnapshotBundle(normalizedID); err == nil {
		log.Infof("loaded rule snapshot from cache: %s", normalizedID)
		return bundle, nil
	}

	endpoint := fmt.Sprintf(ruleSnapshotBundleEndpointFmt, url.PathEscape(normalizedID))
	raw, err := c.getRaw(ctx, endpoint)
	if err != nil {
		return nil, utils.Wrap(err, "download snapshot bundle failed")
	}

	var bundle RuleSnapshotBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return nil, utils.Wrap(err, "decode snapshot bundle failed")
	}
	if err := validateRuleSnapshotBundle(bundle, normalizedID); err != nil {
		return nil, err
	}

	if err := c.cacheSnapshotBundle(bundle, raw); err != nil {
		log.Warnf("cache snapshot bundle failed: %v", err)
	}

	log.Infof(
		"downloaded rule snapshot bundle: snapshot=%s sha=%s items=%d",
		bundle.SnapshotID,
		bundle.ContentSHA256,
		len(bundle.Items),
	)
	return &bundle, nil
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

	bundle, err := c.DownloadSnapshotBundle(ctx, snapshotID)
	if err != nil {
		return 0, utils.Wrap(err, "download snapshot bundle failed")
	}

	ruleCount, err = c.importBundle(ctx, *bundle)
	if err != nil {
		return 0, utils.Wrap(err, "import snapshot bundle failed")
	}

	log.Infof(
		"successfully imported %d rules from snapshot %s (%s)",
		ruleCount,
		bundle.SnapshotID,
		bundle.ContentSHA256,
	)
	return ruleCount, nil
}

func defaultRuleSnapshotBundleImporter(
	ctx context.Context,
	bundle RuleSnapshotBundle,
) (int, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return 0, utils.Error("local database not available")
	}

	count := 0
	for _, item := range bundle.Items {
		if err := ctx.Err(); err != nil {
			return count, err
		}

		rule, err := buildSyntaxFlowRuleFromSnapshotItem(item)
		if err != nil {
			return count, err
		}

		if _, err := sfdb.CreateOrUpdateRuleWithGroup(rule); err != nil {
			return count, utils.Wrapf(err, "upsert syntax flow rule %s failed", rule.RuleName)
		}
		count++
	}
	return count, nil
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

	rule.Content = content
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
	if riskType := strings.TrimSpace(item.RiskType); riskType != "" {
		rule.RiskType = riskType
	}
	if severity := strings.TrimSpace(item.Severity); severity != "" {
		rule.Severity = schema.ValidSeverityType(severity)
	}
	rule.IsBuildInRule = false
	rule.NeedUpdate = false
	return rule, nil
}

func validateRuleSnapshotManifest(manifest RuleSnapshotManifest) error {
	if strings.TrimSpace(manifest.SnapshotID) == "" {
		return utils.Error("snapshot manifest snapshot_id is required")
	}
	if strings.TrimSpace(manifest.ContentSHA256) == "" {
		return utils.Error("snapshot manifest content_sha256 is required")
	}
	if bundleFormat := strings.TrimSpace(manifest.BundleFormat); bundleFormat != "" &&
		bundleFormat != ruleSnapshotBundleFormatJSON {
		return utils.Errorf("unsupported snapshot bundle format: %s", bundleFormat)
	}
	if schemaVersion := strings.TrimSpace(manifest.SchemaVersion); schemaVersion != "" &&
		schemaVersion != ruleSnapshotSchemaVersionV1 {
		return utils.Errorf("unsupported snapshot schema version: %s", schemaVersion)
	}
	return nil
}

func validateRuleSnapshotBundle(bundle RuleSnapshotBundle, expectedSnapshotID string) error {
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
	computedHash, err := calculateRuleSnapshotItemsSHA256(bundle.Items)
	if err != nil {
		return err
	}
	if strings.TrimSpace(bundle.ContentSHA256) != computedHash {
		return utils.Errorf(
			"snapshot bundle content_sha256 mismatch: want %s got %s",
			computedHash,
			bundle.ContentSHA256,
		)
	}
	return nil
}

func calculateRuleSnapshotItemsSHA256(items []RuleSnapshotItem) (string, error) {
	payload, err := json.Marshal(items)
	if err != nil {
		return "", utils.Wrap(err, "marshal snapshot items failed")
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
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
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, utils.Wrap(err, "build rule sync request failed")
	}
	if token := strings.TrimSpace(c.config.BearerToken); token != "" {
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

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, utils.Wrap(err, "read rule sync response failed")
	}
	return body, nil
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

	contentSHA, err := os.ReadFile(c.snapshotRefPath(snapshotID))
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(c.snapshotBundlePath(strings.TrimSpace(string(contentSHA))))
	if err != nil {
		return nil, err
	}

	var bundle RuleSnapshotBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return nil, err
	}
	if err := validateRuleSnapshotBundle(bundle, snapshotID); err != nil {
		return nil, err
	}
	return &bundle, nil
}

func (c *RuleSyncClient) cacheSnapshotBundle(bundle RuleSnapshotBundle, raw []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	contentSHA := strings.TrimSpace(bundle.ContentSHA256)
	if contentSHA == "" {
		return utils.Error("snapshot bundle content_sha256 is required")
	}
	if err := os.WriteFile(c.snapshotBundlePath(contentSHA), raw, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(c.snapshotRefPath(bundle.SnapshotID), []byte(contentSHA), 0o644); err != nil {
		return err
	}
	return nil
}

func (c *RuleSyncClient) snapshotRefPath(snapshotID string) string {
	return filepath.Join(c.cacheDir, fmt.Sprintf("%s.ref", sanitizeRuleSnapshotFilePart(snapshotID)))
}

func (c *RuleSyncClient) snapshotBundlePath(contentSHA string) string {
	return filepath.Join(c.cacheDir, fmt.Sprintf("%s.bundle.json", sanitizeRuleSnapshotFilePart(contentSHA)))
}

func sanitizeRuleSnapshotFilePart(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return replacer.Replace(trimmed)
}

var _ ruleSyncer = (*RuleSyncClient)(nil)
