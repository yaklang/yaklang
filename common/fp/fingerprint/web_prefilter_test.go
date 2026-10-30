package fingerprint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"github.com/yaklang/yaklang/common/schema"
)

func TestWebPrefilterClosesScanWorkers(t *testing.T) {
	if runtime.GOMAXPROCS(0) < 2 {
		t.Skip("native parallel scans require multiple processors")
	}
	rules := []*rule.FingerPrintRule{
		webRegexpRule("merged", `[a-z]{3}`, ""),
		webRegexpRule("assertion", `\b[a-z]{2}\b`, ""),
	}
	filter := compileWebPrefilter(rules)
	require.NotNil(t, filter.db)
	t.Cleanup(func() { _ = filter.db.Close() })
	page := bytes.Repeat([]byte("abc "), 256)
	filter.allows(page, rules)
	before := runtime.NumGoroutine()
	for i := 0; i < 20; i++ {
		require.NotEmpty(t, filter.allows(page, rules))
	}
	runtime.GC()
	runtime.Gosched()
	require.LessOrEqual(t, runtime.NumGoroutine()-before, 8, "scan workers must not outlive their scratch")
}

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

func TestMatchResourceReusesPathResponses(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprintf("failed=%v", failed), func(t *testing.T) {
			var calls atomic.Int32
			var rules []*rule.FingerPrintRule
			for i := 0; i < 32; i++ {
				rules = append(rules, webRegexpRule(fmt.Sprint(i), "icon-token", "/favicon.ico"))
			}
			matcher := NewMatcher()
			got := matcher.MatchResource(context.Background(), 8, rules, func(path string) (*rule.MatchResource, error) {
				calls.Add(1)
				if path != "/favicon.ico" {
					t.Errorf("unexpected resource path: %s", path)
					return nil, fmt.Errorf("unexpected resource path: %s", path)
				}
				if failed {
					return nil, errors.New("slow or unavailable icon")
				}
				return rule.NewHttpResource([]byte("icon-token")), nil
			})
			require.Equal(t, int32(1), calls.Load(), "rules must share both successful and failed path requests")
			if failed {
				require.Empty(t, got)
			} else {
				require.Len(t, got, len(rules), "response reuse must preserve every matching product")
			}
			got = matcher.MatchResource(context.Background(), 8, rules, func(string) (*rule.MatchResource, error) {
				calls.Add(1)
				return rule.NewHttpResource([]byte("another target")), nil
			})
			require.Equal(t, int32(2), calls.Load(), "response cache must be scoped to one match")
			require.Empty(t, got)
		})
	}
}
