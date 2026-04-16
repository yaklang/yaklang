package scannode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
		if r.URL.Path != "/v1/ssa-rule-sync/snapshots/rulesnapshot-a" {
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
