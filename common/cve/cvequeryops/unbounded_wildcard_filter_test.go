package cvequeryops

import (
	"testing"

	"github.com/yaklang/yaklang/common/cve/cveresources"
)

func TestShouldSkipUnboundedWildcardOnly(t *testing.T) {
	queryCPE := []cveresources.CPE{
		{Part: "*", Vendor: "*", Product: "internet_information_server", Version: "8.5", Edition: ""},
	}

	t.Run("wildcard-only", func(t *testing.T) {
		cfg := cveresources.Configurations{
			Nodes: []cveresources.Nodes{
				{
					Operator: "OR",
					CpeMatch: []cveresources.CpeMatch{
						{
							Vulnerable: true,
							Cpe23URI:   "cpe:2.3:a:microsoft:internet_information_server:*:*:*:*:*:*:*:*",
						},
					},
				},
			},
		}
		if !shouldSkipUnboundedWildcardOnly(cfg, queryCPE) {
			t.Fatalf("expected skip")
		}
	})

	t.Run("wildcard-with-bounds", func(t *testing.T) {
		cfg := cveresources.Configurations{
			Nodes: []cveresources.Nodes{
				{
					Operator: "OR",
					CpeMatch: []cveresources.CpeMatch{
						{
							Vulnerable:          true,
							Cpe23URI:            "cpe:2.3:a:microsoft:internet_information_server:*:*:*:*:*:*:*:*",
							VersionEndIncluding: "8.5",
						},
					},
				},
			},
		}
		if shouldSkipUnboundedWildcardOnly(cfg, queryCPE) {
			t.Fatalf("expected not skip")
		}
	})

	t.Run("specific-version", func(t *testing.T) {
		cfg := cveresources.Configurations{
			Nodes: []cveresources.Nodes{
				{
					Operator: "OR",
					CpeMatch: []cveresources.CpeMatch{
						{
							Vulnerable: true,
							Cpe23URI:   "cpe:2.3:a:microsoft:internet_information_server:8.5:*:*:*:*:*:*:*",
						},
					},
				},
			},
		}
		if shouldSkipUnboundedWildcardOnly(cfg, queryCPE) {
			t.Fatalf("expected not skip")
		}
	})

	t.Run("different-product", func(t *testing.T) {
		cfg := cveresources.Configurations{
			Nodes: []cveresources.Nodes{
				{
					Operator: "OR",
					CpeMatch: []cveresources.CpeMatch{
						{
							Vulnerable: true,
							Cpe23URI:   "cpe:2.3:a:apache:tomcat:*:*:*:*:*:*:*:*",
						},
					},
				},
			},
		}
		if shouldSkipUnboundedWildcardOnly(cfg, queryCPE) {
			t.Fatalf("expected not skip")
		}
	})

	t.Run("multi-product-other-prevents-skip", func(t *testing.T) {
		multiQueryCPE := []cveresources.CPE{
			{Part: "*", Vendor: "*", Product: "internet_information_server", Version: "8.5", Edition: ""},
			{Part: "*", Vendor: "*", Product: "tomcat", Version: "8.5.84", Edition: ""},
		}
		cfg := cveresources.Configurations{
			Nodes: []cveresources.Nodes{
				{
					Operator: "OR",
					CpeMatch: []cveresources.CpeMatch{
						{
							Vulnerable: true,
							Cpe23URI:   "cpe:2.3:a:microsoft:internet_information_server:*:*:*:*:*:*:*:*",
						},
						{
							Vulnerable: true,
							Cpe23URI:   "cpe:2.3:a:apache:tomcat:8.5.84:*:*:*:*:*:*:*",
						},
					},
				},
			},
		}
		if shouldSkipUnboundedWildcardOnly(cfg, multiQueryCPE) {
			t.Fatalf("expected not skip")
		}
	})
}
