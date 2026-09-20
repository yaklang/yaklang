//go:build !irify_exclude

package sfdb

import (
	"bytes"
	"encoding/json"
	"os"
	"sync"
	"testing"
)

func TestCompressedRuleVersions(t *testing.T) {
	want, err := os.ReadFile("rule_versions.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := readRuleVersions()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Fatal("rule versions differ; run go generate ./common/syntaxflow/sfdb")
	}
	var rules []RuleInfo
	if err = json.Unmarshal(want, &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) == 0 {
		t.Fatal("no rules")
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			info, err := GetRuleInfo(rules[0].RuleID)
			if err != nil || info == nil || *info != rules[0] {
				t.Errorf("unexpected rule: %v %v", info, err)
			}
		}()
	}
	wg.Wait()
}
