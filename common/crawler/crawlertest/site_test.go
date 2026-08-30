package crawlertest

import (
	"net/url"
	"testing"
)

func TestLayeredSiteGroundTruth(t *testing.T) {
	site := New(t)

	expectedSizes := map[string]int{
		SmallJSPath:  SmallJSSize,
		MediumJSPath: MediumJSSize,
		LargeJSPath:  LargeJSSize,
		HugeJSPath:   HugeJSSize,
	}
	for path, expected := range expectedSizes {
		asset, ok := site.GroundTruth.Asset(path)
		if !ok {
			t.Fatalf("missing ground-truth asset %s", path)
		}
		if asset.Size != expected {
			t.Fatalf("asset %s size=%d, want %d", path, asset.Size, expected)
		}
	}
	if site.GroundTruth.HugeTargetOffset <= HugeMinimumOffset {
		t.Fatalf("huge target offset=%d must be beyond %d", site.GroundTruth.HugeTargetOffset, HugeMinimumOffset)
	}
	huge, ok := site.GroundTruth.Finding(site.GroundTruth.HugeTargetID)
	if !ok || huge.Value != HugeTargetPath || huge.SourceOffset != site.GroundTruth.HugeTargetOffset {
		t.Fatalf("invalid huge finding: %#v", huge)
	}

	seedURL, err := url.Parse(site.URL)
	if err != nil {
		t.Fatal(err)
	}
	externalURL, err := url.Parse(site.ExternalScriptURL)
	if err != nil {
		t.Fatal(err)
	}
	if seedURL.Hostname() == externalURL.Hostname() {
		t.Fatalf("scope fixture hostnames must differ: seed=%s external=%s", seedURL.Hostname(), externalURL.Hostname())
	}
}
