package yak

import "testing"

// TestRehsYakIntegration 通过真实 yak 引擎执行脚本, 端到端验证 rehs 库已注册且
// BuildGroup()~ / Match / Find / MatchedPatterns 在 yak 语言层可用。
func TestRehsYakIntegration(t *testing.T) {
	code := `
group = rehs.BuildGroup(["admin", "(?i)password", "token=\\w+"])~
assert group.Match("see admin token=abc"), "should match admin/token"
assert group.Match("nothing relevant here") == false, "should not match"
assert group.MatchString("Password reset"), "case-insensitive should match password"

matches = group.Find("admin token=xyz")
assert len(matches) > 0, "Find should return matches"
m = matches[0]
log.info("rehs hit: pattern=%v from=%v to=%v value=%v", m.Pattern, m.From, m.To, m.Value)
assert m.Value != "", "located match should carry value"

pats = group.MatchedPatterns("admin password token=zzz")
assert len(pats) == 3, "should match all three distinct patterns"

info = group.Info()
log.info("rehs backend=%v tier=%v simd=%v patterns=%v", info.Backend, info.Tier, info.SIMD, info.NumPatterns)

group.Close()
`
	if _, err := Execute(code); err != nil {
		t.Fatalf("yak rehs integration script failed: %v", err)
	}
}

// TestRehsYakExistenceOnly 验证存在性快路径选项 (rehs.existenceOnly()) 在 yak 层可用。
func TestRehsYakExistenceOnly(t *testing.T) {
	code := `
group = rehs.BuildGroup(["secret", "[0-9]{6}"], rehs.existenceOnly())~
assert group.Match("the otp is 123456"), "existence match should hit digits"
group.Close()

ok = rehs.MatchAny(["\\bfoo\\b", "bar"], "a foo b")~
assert ok, "MatchAny should hit foo"
`
	if _, err := Execute(code); err != nil {
		t.Fatalf("yak rehs existence script failed: %v", err)
	}
}
