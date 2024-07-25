package subdomain

import (
	"testing"
)

func TestNewSubdomainScanner(t *testing.T) {
	scanner, err := NewSubdomainScanner(NewSubdomainScannerConfig(
		// WithDNSServers([]string{""}),
		WithModes(SEARCH),
	))
	if err != nil {
		t.Logf("build subdomain scanner failed: %s", err)
		t.FailNow()
	}

	scanner.Feed("vulhub.org")

	flag := false
	scanner.OnResult(func(result *SubdomainResult) {
		flag = true
	})

	err = scanner.Run()
	if err != nil {
		t.Fatalf("scan subdomain failed: %s", err)
	}

	if !flag {
		t.Log("subdomain scanner execute failed")
		t.FailNow()
	}
}
