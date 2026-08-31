package sfrisk

import (
	"encoding/json"
	"testing"
)

func TestRiskTypeTaxonomyAliases(t *testing.T) {
	for _, raw := range []string{"SQL注入", "SQL 注入", "sqli-inject", "sql-injection", " sqli "} {
		definition, ok := Lookup(raw)
		if !ok || definition.CanonicalName != "sql-injection" || definition.DisplayName != "SQL 注入" || definition.CategoryID != "injection" {
			t.Fatalf("SQL alias %q: %#v, found=%v", raw, definition, ok)
		}
	}
	for _, pair := range [][2]string{
		{"命令注入", "代码注入"}, {"rce", "命令注入"},
		{"硬编码密码", "硬编码凭据"}, {"弱密码", "硬编码密码"},
		{"信息", "信息泄露"}, {"认证绕过", "授权绕过"},
		{"不安全加密", "弱随机数"}, {"疑似目录穿越", "路径遍历"},
		{"路径遍历 / 任意文件读取", "路径遍历"},
	} {
		left, leftOK := Lookup(pair[0])
		right, rightOK := Lookup(pair[1])
		if !leftOK || !rightOK || left.CanonicalName == right.CanonicalName {
			t.Errorf("different semantics must stay distinct: %q / %q", pair[0], pair[1])
		}
	}
	if definition, ok := Lookup("Moderate"); !ok || !definition.ReviewRequired || definition.CategoryID != "review-required" {
		t.Fatalf("misplaced severity must be marked for review: %#v", definition)
	}
	for _, raw := range []string{"", "customer-custom-risk", "SQL注入-extra", "CWE-89"} {
		if _, ok := Lookup(raw); ok {
			t.Errorf("unexpected inferred classification for %q", raw)
		}
	}
}

func TestRiskTypeTaxonomyValidation(t *testing.T) {
	for name, mutate := range map[string]func(*Taxonomy){
		"unknown version":       func(v *Taxonomy) { v.SchemaVersion = "v999" },
		"missing label":         func(v *Taxonomy) { v.Categories[0].RiskTypes[0].DisplayName = "" },
		"invalid canonical key": func(v *Taxonomy) { v.Categories[0].RiskTypes[0].Name = "SQL 注入" },
		"duplicate alias": func(v *Taxonomy) {
			v.Categories[0].RiskTypes[1].Aliases = append(v.Categories[0].RiskTypes[1].Aliases, "SQL注入")
		},
		"duplicate category": func(v *Taxonomy) { v.Categories = append(v.Categories, v.Categories[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			value := GetTaxonomy()
			mutate(&value)
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseTaxonomy(data); err == nil {
				t.Fatal("invalid taxonomy was accepted")
			}
		})
	}
	if _, err := ParseTaxonomy([]byte(`{`)); err == nil {
		t.Fatal("malformed JSON was accepted")
	}
}

func TestRiskTypeTaxonomyIsDetached(t *testing.T) {
	value := GetTaxonomy()
	value.Categories[0].DisplayName = "changed"
	value.Categories[0].RiskTypes[0].DisplayName = "changed"
	value.Categories[0].RiskTypes[0].Aliases[0] = "changed"
	fresh := GetTaxonomy()
	if fresh.Categories[0].DisplayName != "注入攻击" || fresh.Categories[0].RiskTypes[0].DisplayName != "SQL 注入" || fresh.Categories[0].RiskTypes[0].Aliases[0] != "SQL注入" {
		t.Fatal("caller mutated producer taxonomy")
	}
}
