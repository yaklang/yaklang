package resources_monitor

import (
	"errors"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/builtinmetadata"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestLazyMetadataFailureDoesNotAdvanceMarker(t *testing.T) {
	if !builtinmetadata.Enabled {
		t.Skip("gzip runtime")
	}
	const key = "test-lazy-metadata-failure-marker"
	old := yakit.Get(key)
	defer yakit.Set(key, old)
	yakit.Set(key, "previous")
	monitor := NewEmbedResourcesMonitor(key, "prepared")
	for i := 0; i < 2; i++ {
		if err := monitor.MonitorModifiedWithAction(func() string { return "" }, func() error { return nil }); err != nil {
			t.Fatal(err)
		}
		if got := yakit.Get(key); got != "previous" {
			t.Fatal("lazy metadata failure advanced the marker", got)
		}
	}
}

func TestGzipMonitorUsesPreparedHashInDevBuild(t *testing.T) {
	old := consts.GetYakVersion()
	consts.SetYakVersion("dev")
	defer consts.SetYakVersion(old)
	calls := 0
	got := NewEmbedResourcesMonitor("unused", "prepared").getCurrentHash(func() string { calls++; return "dynamic" })
	if builtinmetadata.Enabled {
		if got != "prepared" || calls != 0 {
			t.Fatal("gzip startup must not hash resources")
		}
	} else if got != "dynamic" || calls != 1 {
		t.Fatal("plain development hashing changed")
	}
}

func TestFailedResourceSyncRetriesWithoutAdvancingHash(t *testing.T) {
	const key = "builtin-metadata-test-retry"
	old := yakit.Get(key)
	defer yakit.Set(key, old)
	yakit.Set(key, "old")
	monitor := NewEmbedResourcesMonitor(key, "new")
	if err := monitor.MonitorModifiedWithAction(func() string { return "new" }, func() error { return errors.New("failed import") }); err == nil {
		t.Fatal("sync failure was hidden")
	}
	if yakit.Get(key) != "old" {
		t.Fatal("failed synchronization advanced the marker")
	}
	calls := 0
	if err := monitor.MonitorModifiedWithAction(func() string { return "new" }, func() error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if yakit.Get(key) != "new" || calls != 1 {
		t.Fatal("failed synchronization was not retried")
	}
	if err := monitor.MonitorModifiedWithAction(func() string { return "new" }, func() error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("successful synchronization must not repeat")
	}
}

func TestInvalidMetadataDoesNotAdvanceMarker(t *testing.T) {
	const key = "metadata-invalid-marker-test"
	old := yakit.Get(key)
	defer yakit.Set(key, old)
	yakit.Set(key, "last-valid-metadata")
	monitor := NewEmbedResourcesMonitor(key, "")
	calls := 0
	for i := 0; i < 2; i++ {
		if err := monitor.MonitorModifiedWithAction(func() string { return "" }, func() error { calls++; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 || yakit.Get(key) != "last-valid-metadata" {
		t.Fatal("invalid metadata must retry dynamically without storing a success marker")
	}
}
