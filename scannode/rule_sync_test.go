package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRuleSyncClientRejectsOversizedResponseBeforeReadingBody(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.FormatInt(maxRuleSnapshotResponseBytes+1, 10))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := NewRuleSyncClient(&RuleSyncConfig{
		ServerURL: server.URL,
		CacheDir:  t.TempDir(),
		Client:    server.Client(),
	})
	_, err := client.getRaw(context.Background(), "/oversized")
	if err == nil || !strings.Contains(err.Error(), "exceeds the size limit") {
		t.Fatalf("expected oversized response rejection, got %v", err)
	}
}

func TestRuleSyncClientGetActiveSnapshot(t *testing.T) {
	t.Parallel()

	items := []RuleSnapshotItem{
		{AssetID: "asset-1", Name: "sql-injection", Content: "desc(title: 'SQL Injection');"},
	}
	contentSHA, err := calculateRuleSnapshotItemsSHA256(items)
	if err != nil {
		t.Fatalf("calculate content sha: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != activeRuleSnapshotEndpointPath {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RuleSnapshotManifest{
			SnapshotID:    "rulesnapshot-active",
			Name:          "baseline",
			AssetCount:    1,
			IsActive:      true,
			PublishedAt:   time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
			ContentSHA256: contentSHA,
			BundleFormat:  ruleSnapshotBundleFormatJSON,
			SchemaVersion: ruleSnapshotSchemaVersionV1,
		})
	}))
	defer server.Close()

	client := NewRuleSyncClient(&RuleSyncConfig{
		ServerURL:   server.URL,
		SyncEnabled: true,
		CacheDir:    t.TempDir(),
		Client:      server.Client(),
	})

	manifest, err := client.GetActiveSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetActiveSnapshot returned error: %v", err)
	}
	if manifest.SnapshotID != "rulesnapshot-active" {
		t.Fatalf("unexpected snapshot id: %q", manifest.SnapshotID)
	}
	if manifest.ContentSHA256 != contentSHA {
		t.Fatalf("unexpected content sha: %q", manifest.ContentSHA256)
	}
}

func TestRuleSyncClientDownloadSnapshotBundleUsesCache(t *testing.T) {
	t.Parallel()

	items := []RuleSnapshotItem{
		{AssetID: "asset-1", Name: "sql-injection", Content: "desc(title: 'SQL Injection');"},
	}
	contentSHA, err := calculateRuleSnapshotItemsSHA256(items)
	if err != nil {
		t.Fatalf("calculate content sha: %v", err)
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ssa-rule-sync-node/snapshots/rulesnapshot-a" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RuleSnapshotBundle{
			RuleSnapshotManifest: RuleSnapshotManifest{
				SnapshotID:    "rulesnapshot-a",
				Name:          "baseline",
				AssetCount:    1,
				IsActive:      false,
				PublishedAt:   time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
				ContentSHA256: contentSHA,
				BundleFormat:  ruleSnapshotBundleFormatJSON,
				SchemaVersion: ruleSnapshotSchemaVersionV1,
			},
			Items: items,
		})
	}))
	defer server.Close()

	client := NewRuleSyncClient(&RuleSyncConfig{
		ServerURL:   server.URL,
		SyncEnabled: true,
		CacheDir:    t.TempDir(),
		Client:      server.Client(),
	})

	first, err := client.DownloadSnapshotBundle(context.Background(), "rulesnapshot-a")
	if err != nil {
		t.Fatalf("DownloadSnapshotBundle first call error: %v", err)
	}
	second, err := client.DownloadSnapshotBundle(context.Background(), "rulesnapshot-a")
	if err != nil {
		t.Fatalf("DownloadSnapshotBundle second call error: %v", err)
	}

	if requestCount != 1 {
		t.Fatalf("expected exactly one HTTP request, got %d", requestCount)
	}
	if first.ContentSHA256 != second.ContentSHA256 {
		t.Fatalf("expected identical cached bundle hash, got %q vs %q", first.ContentSHA256, second.ContentSHA256)
	}
	if !client.HasLocalSnapshot("rulesnapshot-a") {
		t.Fatal("expected local snapshot cache to exist")
	}
}

func TestRuleSyncClientSyncSnapshotUsesImporter(t *testing.T) {
	t.Parallel()

	items := []RuleSnapshotItem{
		{AssetID: "asset-1", Name: "sql-injection", Content: "desc(title: 'SQL Injection');"},
		{AssetID: "asset-2", Name: "xss", Content: "desc(title: 'XSS');"},
	}
	contentSHA, err := calculateRuleSnapshotItemsSHA256(items)
	if err != nil {
		t.Fatalf("calculate content sha: %v", err)
	}

	importedSnapshotID := ""
	importedCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RuleSnapshotBundle{
			RuleSnapshotManifest: RuleSnapshotManifest{
				SnapshotID:    "rulesnapshot-a",
				Name:          "baseline",
				AssetCount:    len(items),
				IsActive:      false,
				PublishedAt:   time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
				ContentSHA256: contentSHA,
				BundleFormat:  ruleSnapshotBundleFormatJSON,
				SchemaVersion: ruleSnapshotSchemaVersionV1,
			},
			Items: items,
		})
	}))
	defer server.Close()

	client := NewRuleSyncClient(&RuleSyncConfig{
		ServerURL:   server.URL,
		SyncEnabled: true,
		CacheDir:    t.TempDir(),
		Client:      server.Client(),
		Importer: func(_ context.Context, bundle RuleSnapshotBundle) (int, error) {
			importedSnapshotID = bundle.SnapshotID
			importedCount = len(bundle.Items)
			return len(bundle.Items), nil
		},
	})

	count, err := client.SyncSnapshot(context.Background(), "rulesnapshot-a")
	if err != nil {
		t.Fatalf("SyncSnapshot returned error: %v", err)
	}
	if importedSnapshotID != "rulesnapshot-a" {
		t.Fatalf("unexpected imported snapshot id: %q", importedSnapshotID)
	}
	if importedCount != len(items) || count != len(items) {
		t.Fatalf("unexpected imported count: importer=%d return=%d", importedCount, count)
	}
}

func TestRuleSnapshotCanonicalDigestMatchesLegionContract(t *testing.T) {
	t.Parallel()

	items := []RuleSnapshotItem{
		{AssetID: "asset-z", Name: "z-rule", Content: `desc(title: "Z");`},
		{
			AssetID: "asset-a", SourceRuleID: "source-a", Name: "a-rule",
			Title: "A", TitleZh: "甲", Language: "java", Purpose: "audit", Tag: "cwe|owasp",
			CWE: []string{"CWE-89"}, CVE: "CVE-1", RiskType: "sqli", Type: "sf", Severity: "high",
			Description: "desc", Solution: "fix", Version: "1", ContentHash: "source-hash",
			IsBuiltin: true, Verified: true, AllowIncluded: true, IncludedName: "lib-a",
			Groups: []string{"java"}, AlertDesc: json.RawMessage(`{"risk":{"title":"A"}}`),
			Content: `desc(title: "A");`,
		},
	}
	canonicalJSON := `[{"asset_id":"asset-a","source_rule_id":"source-a","name":"a-rule","title":"A","title_zh":"甲","language":"java","purpose":"audit","tag":"cwe|owasp","cwe":["CWE-89"],"cve":"CVE-1","risk_type":"sqli","type":"sf","severity":"high","description":"desc","solution":"fix","version":"1","content_hash":"source-hash","is_builtin":true,"verified":true,"allow_included":true,"included_name":"lib-a","groups":["java"],"alert_desc":{"risk":{"title":"A"}},"content":"desc(title: \"A\");"},{"asset_id":"asset-z","name":"z-rule","is_builtin":false,"verified":false,"allow_included":false,"content":"desc(title: \"Z\");"}]`
	raw, err := json.Marshal(canonicalRuleSnapshotItems(items))
	if err != nil {
		t.Fatalf("marshal canonical items: %v", err)
	}
	if string(raw) != canonicalJSON {
		t.Fatalf("canonical JSON drifted:\n got: %s\nwant: %s", raw, canonicalJSON)
	}
	wantSum := sha256.Sum256([]byte(canonicalJSON))
	got, err := calculateRuleSnapshotItemsSHA256(items)
	if err != nil {
		t.Fatalf("calculate digest: %v", err)
	}
	if got != hex.EncodeToString(wantSum[:]) {
		t.Fatalf("canonical digest mismatch: got=%s want=%s", got, hex.EncodeToString(wantSum[:]))
	}
}

func TestRuleSnapshotBundleRejectsNonCanonicalIdentityAndUnknownFields(t *testing.T) {
	t.Parallel()

	items := []RuleSnapshotItem{{AssetID: "asset-a", Name: "a-rule", Content: `desc(title: "A");`}}
	digest, err := calculateRuleSnapshotItemsSHA256(items)
	if err != nil {
		t.Fatalf("calculate digest: %v", err)
	}
	valid := RuleSnapshotBundle{
		RuleSnapshotManifest: RuleSnapshotManifest{
			SnapshotID: "rulesnapshot-a", AssetCount: 1, ContentSHA256: digest,
			BundleFormat: ruleSnapshotBundleFormatJSON, SchemaVersion: ruleSnapshotSchemaVersionV1,
		},
		Items: items,
	}

	nonCanonicalFormat := valid
	nonCanonicalFormat.BundleFormat = " json "
	nonCanonicalFormatRaw, err := json.Marshal(nonCanonicalFormat)
	if err != nil {
		t.Fatalf("marshal non-canonical format bundle: %v", err)
	}
	if err := validateRuleSnapshotBundle(nonCanonicalFormat, "rulesnapshot-a", nonCanonicalFormatRaw); err == nil ||
		!strings.Contains(err.Error(), "bundle_format must be canonical") {
		t.Fatalf("expected non-canonical format rejection, got %v", err)
	}

	nonCanonicalAsset := valid
	nonCanonicalAsset.Items = append([]RuleSnapshotItem(nil), valid.Items...)
	nonCanonicalAsset.Items[0].AssetID = " asset-a"
	nonCanonicalAssetRaw, err := json.Marshal(nonCanonicalAsset)
	if err != nil {
		t.Fatalf("marshal non-canonical asset bundle: %v", err)
	}
	if err := validateRuleSnapshotBundle(nonCanonicalAsset, "rulesnapshot-a", nonCanonicalAssetRaw); err == nil ||
		!strings.Contains(err.Error(), "asset_id must be canonical") {
		t.Fatalf("expected non-canonical asset rejection, got %v", err)
	}

	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("marshal valid bundle: %v", err)
	}
	withUnknown := strings.TrimSuffix(string(raw), "}") + `,"unexpected":true}`
	if _, err := decodeRuleSnapshotBundle([]byte(withUnknown)); err == nil ||
		!strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown-field rejection, got %v", err)
	}
}

func TestRuleSyncClientPrepareSnapshotWritesReadyContentAddressedCache(t *testing.T) {
	t.Parallel()

	items := []RuleSnapshotItem{
		{AssetID: "asset-b", Name: "b-rule", Content: `desc(title: "B");`},
		{AssetID: "asset-a", Name: "a-rule", Content: `desc(title: "A");`},
	}
	digest, err := calculateRuleSnapshotItemsSHA256(items)
	if err != nil {
		t.Fatalf("calculate digest: %v", err)
	}
	bundle := RuleSnapshotBundle{
		RuleSnapshotManifest: RuleSnapshotManifest{
			SnapshotID: "rulesnapshot-ready", AssetCount: len(items), ContentSHA256: digest,
			BundleFormat: ruleSnapshotBundleFormatJSON, SchemaVersion: ruleSnapshotSchemaVersionV1,
		},
		Items: canonicalRuleSnapshotItems(items),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(bundle)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	client := NewRuleSyncClient(&RuleSyncConfig{
		ServerURL: server.URL, SyncEnabled: true, CacheDir: cacheDir, Client: server.Client(),
	})
	expectation := RuleSnapshotExpectation{
		SnapshotID: "rulesnapshot-ready", ContentSHA256: digest,
		BundleFormat: ruleSnapshotBundleFormatJSON, SchemaVersion: ruleSnapshotSchemaVersionV1,
		AssetIDs: []string{"asset-b", "asset-a"},
	}
	first, err := client.PrepareSnapshot(context.Background(), expectation)
	if err != nil {
		t.Fatalf("prepare first snapshot: %v", err)
	}
	if first.Receipt.CacheHit || first.Receipt.State != ruleSnapshotCacheReadyFile {
		t.Fatalf("unexpected first receipt: %#v", first.Receipt)
	}
	readyPath := filepath.Join(cacheDir, "objects", digest, ruleSnapshotCacheReadyFile)
	ready, err := os.ReadFile(readyPath)
	if err != nil || strings.TrimSpace(string(ready)) != digest {
		t.Fatalf("invalid READY marker: path=%s raw=%q err=%v", readyPath, ready, err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "objects", digest, "bundle.json")); err != nil {
		t.Fatalf("canonical bundle not cached: %v", err)
	}
	// Simulate a cache created by a pre-v2 node. A cache hit must repair the
	// legacy broad permissions before reading reusable rule source.
	for _, path := range []string{
		cacheDir,
		filepath.Join(cacheDir, "objects"),
		filepath.Join(cacheDir, "refs"),
		filepath.Join(cacheDir, "objects", digest),
	} {
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatalf("broaden legacy cache directory %s: %v", path, err)
		}
	}
	for _, path := range []string{
		readyPath,
		filepath.Join(cacheDir, "objects", digest, "bundle.json"),
		client.snapshotRefPath("rulesnapshot-ready"),
	} {
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatalf("broaden legacy cache file %s: %v", path, err)
		}
	}
	second, err := client.PrepareSnapshot(context.Background(), expectation)
	if err != nil {
		t.Fatalf("prepare cached snapshot: %v", err)
	}
	if !second.Receipt.CacheHit {
		t.Fatalf("cached receipt did not report cache hit: %#v", second.Receipt)
	}
	for _, path := range []string{
		cacheDir,
		filepath.Join(cacheDir, "objects"),
		filepath.Join(cacheDir, "refs"),
		filepath.Join(cacheDir, "objects", digest),
	} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("stat protected cache directory %s: %v", path, statErr)
		}
		if gotMode := info.Mode().Perm(); gotMode != 0o700 {
			t.Fatalf("cache directory %s mode=%#o, want 0700", path, gotMode)
		}
	}
	for _, path := range []string{
		readyPath,
		filepath.Join(cacheDir, "objects", digest, "bundle.json"),
		client.snapshotRefPath("rulesnapshot-ready"),
	} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("stat protected cache file %s: %v", path, statErr)
		}
		if gotMode := info.Mode().Perm(); gotMode != 0o600 {
			t.Fatalf("cache file %s mode=%#o, want 0600", path, gotMode)
		}
	}
}

func TestRuleSyncClientContentAddressedCacheSupportsMultipleSnapshotIDs(t *testing.T) {
	t.Parallel()

	items := []RuleSnapshotItem{{AssetID: "asset-shared", Name: "shared", Content: `desc(title: "Shared");`}}
	digest, err := calculateRuleSnapshotItemsSHA256(items)
	if err != nil {
		t.Fatalf("calculate digest: %v", err)
	}
	bundleFor := func(snapshotID string) RuleSnapshotBundle {
		return RuleSnapshotBundle{
			RuleSnapshotManifest: RuleSnapshotManifest{
				SnapshotID: snapshotID, AssetCount: 1, ContentSHA256: digest,
				BundleFormat: ruleSnapshotBundleFormatJSON, SchemaVersion: ruleSnapshotSchemaVersionV1,
			},
			Items: items,
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snapshotID := strings.TrimPrefix(r.URL.Path, "/v1/ssa-rule-sync-node/snapshots/")
		if snapshotID != "rulesnapshot-a" && snapshotID != "rulesnapshot-b" {
			t.Fatalf("unexpected snapshot path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(bundleFor(snapshotID))
	}))
	client := NewRuleSyncClient(&RuleSyncConfig{
		ServerURL: server.URL, SyncEnabled: true, CacheDir: t.TempDir(), Client: server.Client(),
	})
	expectationFor := func(snapshotID string) RuleSnapshotExpectation {
		return RuleSnapshotExpectation{
			SnapshotID: snapshotID, ContentSHA256: digest,
			BundleFormat: ruleSnapshotBundleFormatJSON, SchemaVersion: ruleSnapshotSchemaVersionV1,
			AssetIDs: []string{"asset-shared"},
		}
	}

	if _, err := client.PrepareSnapshot(context.Background(), expectationFor("rulesnapshot-a")); err != nil {
		t.Fatalf("prepare first identity: %v", err)
	}
	if _, err := client.PrepareSnapshot(context.Background(), expectationFor("rulesnapshot-b")); err != nil {
		t.Fatalf("prepare second identity sharing content: %v", err)
	}
	server.Close()

	cached, err := client.PrepareSnapshot(context.Background(), expectationFor("rulesnapshot-b"))
	if err != nil {
		t.Fatalf("prepare second identity from offline cache: %v", err)
	}
	if !cached.Receipt.CacheHit || cached.Bundle.SnapshotID != "rulesnapshot-b" ||
		cached.Receipt.SnapshotID != "rulesnapshot-b" {
		t.Fatalf("cached bundle leaked the first snapshot identity: %#v", cached)
	}
}

func TestRuleSyncClientDownloadSnapshotBundleSingleflight(t *testing.T) {
	t.Parallel()

	items := []RuleSnapshotItem{{AssetID: "asset-1", Name: "one", Content: `desc(title: "One");`}}
	digest, err := calculateRuleSnapshotItemsSHA256(items)
	if err != nil {
		t.Fatalf("calculate digest: %v", err)
	}
	bundle := RuleSnapshotBundle{
		RuleSnapshotManifest: RuleSnapshotManifest{
			SnapshotID: "rulesnapshot-singleflight", AssetCount: 1, ContentSHA256: digest,
			BundleFormat: ruleSnapshotBundleFormatJSON, SchemaVersion: ruleSnapshotSchemaVersionV1,
		},
		Items: items,
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		time.Sleep(50 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(bundle)
	}))
	defer server.Close()
	client := NewRuleSyncClient(&RuleSyncConfig{
		ServerURL: server.URL, SyncEnabled: true, CacheDir: t.TempDir(), Client: server.Client(),
	})

	const callers = 12
	start := make(chan struct{})
	errCh := make(chan error, callers)
	var wait sync.WaitGroup
	for index := 0; index < callers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, callErr := client.DownloadSnapshotBundle(context.Background(), "rulesnapshot-singleflight")
			errCh <- callErr
		}()
	}
	close(start)
	wait.Wait()
	close(errCh)
	for callErr := range errCh {
		if callErr != nil {
			t.Fatalf("concurrent download failed: %v", callErr)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("singleflight request count: got=%d want=1", got)
	}
}

func TestRuleSyncClientRejectsTamperedBundleWithoutReadyMarker(t *testing.T) {
	t.Parallel()

	items := []RuleSnapshotItem{{AssetID: "asset-1", Name: "one", Content: `desc(title: "One");`}}
	bundle := RuleSnapshotBundle{
		RuleSnapshotManifest: RuleSnapshotManifest{
			SnapshotID: "rulesnapshot-tampered", AssetCount: 1,
			ContentSHA256: strings.Repeat("0", 64), BundleFormat: ruleSnapshotBundleFormatJSON,
			SchemaVersion: ruleSnapshotSchemaVersionV1,
		},
		Items: items,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(bundle)
	}))
	defer server.Close()
	cacheDir := t.TempDir()
	client := NewRuleSyncClient(&RuleSyncConfig{
		ServerURL: server.URL, SyncEnabled: true, CacheDir: cacheDir, Client: server.Client(),
	})
	_, err := client.DownloadSnapshotBundle(context.Background(), "rulesnapshot-tampered")
	if err == nil || !strings.Contains(err.Error(), "content_sha256 mismatch") {
		t.Fatalf("expected tamper rejection, got %v", err)
	}
	entries, readErr := os.ReadDir(filepath.Join(cacheDir, "objects"))
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read cache objects: %v", readErr)
	}
	for _, entry := range entries {
		if _, statErr := os.Stat(filepath.Join(cacheDir, "objects", entry.Name(), ruleSnapshotCacheReadyFile)); statErr == nil {
			t.Fatalf("tampered bundle produced READY object: %s", entry.Name())
		}
	}
}
