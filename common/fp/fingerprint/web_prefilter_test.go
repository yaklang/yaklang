package fingerprint

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"github.com/yaklang/yaklang/common/schema"
)

func webRegexpRule(product, pattern, path string) *rule.FingerPrintRule {
	r := rule.NewEmptyFingerPrintRule()
	r.Method = "regexp"
	r.WebPath = path
	r.MatchParam.RegexpPattern = pattern
	r.MatchParam.Keyword = &webfingerprint.KeywordMatcher{VersionIndex: 1}
	r.MatchParam.Info = &schema.CPE{Product: product}
	return r
}

func TestWebPrefilterDropsMissesAndKeepsVersion(t *testing.T) {
	page := []byte("HTTP/1.0 200 OK\r\nServer: nginx/1.24.0\r\n\r\n")
	rules := []*rule.FingerPrintRule{
		webRegexpRule("nginx", `nginx/([\d.]+)`, ""),
		webRegexpRule("apache", `Apache/([\d.]+)`, ""),
	}
	for i := 0; i < 200; i++ {
		rules = append(rules, webRegexpRule("other", `this-product-is-absent-[0-9]{8}`, ""))
	}
	rules = append(rules, webRegexpRule("admin", `secret-admin`, "/admin"))

	before := webPrefilterBuilds.Load()
	filtered := filterRootWebRules(rules, map[string][]byte{"": page}, nil)
	require.NotEmpty(t, filtered)
	var products []string
	for _, r := range filtered {
		products = append(products, r.MatchParam.Info.Product)
	}
	require.Contains(t, products, "nginx")
	require.NotContains(t, products, "apache")
	require.NotContains(t, products, "other")
	require.Contains(t, products, "admin")

	var adminCalls atomic.Int32
	var bad atomic.Int32
	getter := func(path string) (*rule.MatchResource, error) {
		if path == "/admin" {
			adminCalls.Add(1)
		}
		return rule.NewHttpResource(page), nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := NewMatcher().MatchResource(context.Background(), 4, rules, getter)
			var version string
			seenNginx := false
			for _, cpe := range got {
				if cpe.Product == "apache" {
					bad.Add(1)
				}
				if cpe.Product == "nginx" {
					seenNginx = true
					version = cpe.Version
				}
			}
			if !seenNginx || version != "1.24.0" {
				bad.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Zero(t, bad.Load())
	require.Equal(t, int32(1), webPrefilterBuilds.Load()-before)
	require.Greater(t, adminCalls.Load(), int32(0))

	again := filterRootWebRules(rules, map[string][]byte{"": page}, nil)
	require.Equal(t, len(filtered), len(again))
	require.Equal(t, int32(1), webPrefilterBuilds.Load()-before)

	kept := filterRootWebRules(rules, nil, map[string]bool{"": true})
	require.Len(t, kept, len(rules))
}
