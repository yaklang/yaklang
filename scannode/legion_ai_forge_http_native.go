package scannode

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/yaklang/yaklang/common/fp/fingerprint"
	"github.com/yaklang/yaklang/common/fp/fingerprint/parsers"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule_resources"
	"github.com/yaklang/yaklang/common/schema"
	"sort"
	"strings"
	"sync"
	"time"
)

var legionNativeFingerprintRules struct {
	sync.Once
	rules   []*rule.FingerPrintRule
	digest  string
	err     error
	skipped int
}

func legionForgePassiveFingerprint(ctx context.Context, packet string) (map[string]any, error) {
	legionNativeFingerprintRules.Do(func() {
		raw, err := rule_resources.FS.ReadFile("exp_rule.txt")
		if err != nil {
			legionNativeFingerprintRules.err = err
			return
		}
		if len(raw) > 16<<20 {
			legionNativeFingerprintRules.err = fmt.Errorf("embedded fingerprint rules exceed bounded size")
			return
		}
		digest := sha256.Sum256(raw)
		legionNativeFingerprintRules.digest = fmt.Sprintf("%x", digest)
		definitions := []*schema.GeneralRule{}
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			parts := strings.SplitN(line, "\x00", 2)
			if len(parts) != 2 {
				legionNativeFingerprintRules.err = fmt.Errorf("invalid embedded fingerprint rule")
				return
			}
			definitions = append(definitions, &schema.GeneralRule{MatchExpression: parts[1], CPE: &schema.CPE{Product: parts[0]}})
			if len(definitions) > 20000 {
				legionNativeFingerprintRules.err = fmt.Errorf("embedded fingerprint rule count exceeded")
				return
			}
		}
		for _, definition := range definitions {
			compiled, err := parsers.ParseExpRule(definition)
			if err != nil {
				legionNativeFingerprintRules.skipped++
				continue
			}
			legionNativeFingerprintRules.rules = append(legionNativeFingerprintRules.rules, compiled...)
		}
	})
	if legionNativeFingerprintRules.err != nil {
		return nil, legionNativeFingerprintRules.err
	}
	if len(legionNativeFingerprintRules.rules) == 0 {
		return nil, fmt.Errorf("native embedded fingerprint rules unavailable")
	}
	matcher := fingerprint.NewMatcher()
	matches := matcher.MatchResource(ctx, 1, legionNativeFingerprintRules.rules, func(path string) (*rule.MatchResource, error) {
		if path != "" && path != "/" {
			return nil, fmt.Errorf("active fingerprint path is disabled")
		}
		return rule.NewHttpResource([]byte(packet)), nil
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	products := []string{}
	seen := map[string]bool{}
	for _, match := range matches {
		if match != nil && match.Product != "" && !seen[match.Product] {
			seen[match.Product] = true
			products = append(products, match.Product)
		}
	}
	sort.Strings(products)
	truncated := len(products) > 128
	if truncated {
		products = products[:128]
	}
	return map[string]any{"products": products, "source": "yaklang_embedded_expression_rules", "rules_sha256": legionNativeFingerprintRules.digest, "rule_count": len(legionNativeFingerprintRules.rules), "unsupported_rule_count": legionNativeFingerprintRules.skipped, "passive": true, "matches_truncated": truncated}, nil
}

func legionForgeNativeHTTPCall(callCtx, parent context.Context, runtime *legionServerFocusRuntime, name string, request map[string]any) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(callCtx, 20*time.Second)
	defer cancel()
	stop := context.AfterFunc(parent, cancel)
	defer stop()
	for key := range request {
		if key != "runtime_id" && key != "url" && key != "method" {
			return nil, fmt.Errorf("undeclared HTTP argument %q", key)
		}
	}
	if name == "simple_crawler" {
		method := strings.ToUpper(focusRuntimeString(request, "method"))
		if method != "" && method != "GET" {
			return nil, fmt.Errorf("native crawler permits only GET")
		}
		request["method"] = "GET"
	}
	result, err := runtime.executeHTTPRequestContext(ctx, request)
	if err != nil {
		return nil, err
	}
	switch name {
	case "web_fingerprint":
		fp, err := legionForgePassiveFingerprint(ctx, focusRuntimeRawString(result, "response_evidence"))
		if err != nil {
			return nil, err
		}
		result["fingerprints"] = fp
		delete(result, "body")
	case "simple_crawler":
		refs, err := runtime.extractReferences(map[string]any{"base_url": result["url"], "document": result["body"]})
		if err != nil {
			return nil, err
		}
		pages := []map[string]any{result}
		candidates, _ := refs["pages"].([]string)
		visited := map[string]bool{focusRuntimeString(result, "url"): true}
		failures := []map[string]string{}
		attempted := 1
		skipped := 0
		for _, target := range candidates {
			if visited[target] {
				continue
			}
			visited[target] = true
			if attempted >= 4 {
				skipped++
				continue
			}
			attempted++
			page, err := runtime.executeHTTPRequestContext(ctx, map[string]any{"url": target, "method": "GET"})
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				failures = append(failures, map[string]string{"url": target, "error": err.Error()})
				continue
			}
			pages = append(pages, page)
		}
		return map[string]any{"url": result["url"], "pages": pages, "failures": failures, "references": refs, "attempted_pages": attempted, "skipped_pages": skipped, "max_pages": 4, "max_depth": 1, "exhaustive": false}, nil
	}
	return result, nil
}
