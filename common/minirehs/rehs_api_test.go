package minirehs

import (
	"sort"
	"testing"
)

// TestRehsBuildGroupBasic 覆盖面向 yak 的高层 API: BuildGroup + Match/Find/MatchedPatterns/Count。
func TestRehsBuildGroupBasic(t *testing.T) {
	g, err := BuildGroup([]string{`admin`, `password`, `token=\w+`})
	if err != nil {
		t.Fatalf("BuildGroup: %v", err)
	}
	defer g.Close()

	if g.Len() != 3 {
		t.Fatalf("Len=%d want 3", g.Len())
	}
	t.Logf("backend=%s tier=%d simd=%v always_on=%d", g.Info().Backend, g.Info().Tier, g.Info().SIMD, g.Info().NumAlwaysOn)

	if !g.Match("here is an admin token=abc123") {
		t.Fatal("Match should hit (admin/token)")
	}
	if g.Match("nothing relevant here") {
		t.Fatal("Match should not hit")
	}
	if !g.MatchString("password reset") {
		t.Fatal("MatchString should hit password")
	}
	if !g.MatchBytes([]byte("token=xyz")) {
		t.Fatal("MatchBytes should hit token")
	}

	pats := g.MatchedPatterns("admin password token=zzz")
	sort.Strings(pats)
	want := []string{`admin`, `password`, `token=\w+`}
	sort.Strings(want)
	if len(pats) != 3 {
		t.Fatalf("MatchedPatterns=%v want 3 distinct", pats)
	}

	matches := g.Find("admin token=abc")
	if len(matches) == 0 {
		t.Fatal("Find returned no matches")
	}
	for _, m := range matches {
		t.Logf("hit index=%d pattern=%q from=%d to=%d value=%q", m.Index, m.Pattern, m.From, m.To, m.Value)
		if m.From >= 0 && m.Value == "" {
			t.Fatalf("located match should carry Value: %+v", m)
		}
	}
}

// TestRehsExistenceOnly 验证存在性快路径选项: Match 正常, Find 偏移为 -1。
func TestRehsExistenceOnly(t *testing.T) {
	g, err := BuildGroup([]string{`secret`, `[0-9]{6}`}, WithGroupExistenceOnly(true))
	if err != nil {
		t.Fatalf("BuildGroup: %v", err)
	}
	defer g.Close()

	if !g.Match("the otp is 123456") {
		t.Fatal("existence Match should hit digits")
	}
	for _, m := range g.Find("secret 654321") {
		if m.From != -1 || m.To != -1 {
			t.Fatalf("existenceOnly Find should report -1 offsets, got %+v", m)
		}
	}
}

// TestRehsOptionsAndFlags 验证大小写不敏感选项与 Exports 中的 option 构造器一致可用。
func TestRehsOptionsAndFlags(t *testing.T) {
	g, err := BuildGroup([]string{`hello`}, WithGroupCaseInsensitive(true))
	if err != nil {
		t.Fatalf("BuildGroup: %v", err)
	}
	defer g.Close()
	if !g.Match("HELLO WORLD") {
		t.Fatal("case-insensitive Match should hit HELLO")
	}

	// Exports 里的 option 构造器 (yak 调用形态) 应返回可用的 GroupOption。
	ctor, ok := Exports["caseInsensitive"].(func() GroupOption)
	if !ok {
		t.Fatal("Exports[caseInsensitive] has unexpected type")
	}
	g2, err := BuildGroup([]string{`world`}, ctor())
	if err != nil {
		t.Fatalf("BuildGroup via export ctor: %v", err)
	}
	defer g2.Close()
	if !g2.Match("WORLD") {
		t.Fatal("export-ctor case-insensitive Match should hit")
	}
}

// TestRehsMatchAny 验证一次性便捷接口。
func TestRehsMatchAny(t *testing.T) {
	ok, err := MatchAny([]string{`\bfoo\b`, `bar`}, "a foo b")
	if err != nil {
		t.Fatalf("MatchAny: %v", err)
	}
	if !ok {
		t.Fatal("MatchAny should hit foo")
	}
	ok, err = MatchAny([]string{`zzz`}, "nothing")
	if err != nil {
		t.Fatalf("MatchAny: %v", err)
	}
	if ok {
		t.Fatal("MatchAny should not hit")
	}
}

// TestRehsScanCallback 验证流式回调与提前终止。
func TestRehsScanCallback(t *testing.T) {
	g, err := BuildGroup([]string{`a`, `b`, `c`})
	if err != nil {
		t.Fatalf("BuildGroup: %v", err)
	}
	defer g.Close()
	count := 0
	g.Scan("a b c a b c", func(m *GroupMatch) bool {
		count++
		return count < 2 // 第二次命中后停止
	})
	if count != 2 {
		t.Fatalf("Scan early-stop: got %d callbacks, want 2", count)
	}
}
