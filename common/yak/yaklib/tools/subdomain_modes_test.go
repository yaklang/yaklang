package tools

import (
	"testing"

	"github.com/yaklang/yaklang/common/subdomain"
	"gotest.tools/v3/assert"
)

func TestWithModesSearchOnly(t *testing.T) {
	cfg := subdomain.NewSubdomainScannerConfig()
	withModes("search")(cfg)
	assert.DeepEqual(t, []int{subdomain.SEARCH}, cfg.Modes)
}

func TestWithModesCombined(t *testing.T) {
	cfg := subdomain.NewSubdomainScannerConfig()
	withModes("search", "brute", "axfr")(cfg)
	assert.DeepEqual(t, []int{subdomain.SEARCH, subdomain.BRUTE, subdomain.ZONE_TRANSFER}, cfg.Modes)
}
