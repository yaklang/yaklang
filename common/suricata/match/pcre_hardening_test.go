package match

import (
	"errors"
	"strings"
	"testing"

	pcre2lite "github.com/VillanCh/go-pcre2-lite"
	"github.com/yaklang/yaklang/common/suricata/data/modifier"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

func TestPCREMatchLimitFailsClosed(t *testing.T) {
	rules, err := rule.Parse(`alert tcp any any -> any any (pcre:"/(a+)+$/"; sid:1;)`)
	if err != nil {
		t.Fatal(err)
	}
	matcher, err := CompileRule(rules[0])
	if err != nil {
		t.Fatal(err)
	}
	packet := buildTCPPacket(t, []byte(strings.Repeat("a", 64)+"!"))

	err, matched := matcher.matcher.Match(packet)
	if err == nil {
		t.Fatal("expected PCRE match limit error")
	}
	if matched {
		t.Fatal("match workflow must fail closed on PCRE execution errors")
	}
	if !strings.Contains(err.Error(), pcre2lite.ErrMatchLimit.Error()) {
		t.Fatalf("match limit error was not propagated through workflow: %v", err)
	}
	if matcher.MatchPackage(packet) {
		t.Fatal("public matcher must fail closed on PCRE execution errors")
	}
}

func TestNewPCREMatchPreservesLimitError(t *testing.T) {
	rules, err := rule.Parse(`alert tcp any any -> any any (pcre:"/(a+)+$/"; sid:1;)`)
	if err != nil {
		t.Fatal(err)
	}
	contentRule := rules[0].ContentRuleConfig.ContentRules[0]
	handler, err := newPCREMatch(contentRule)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &matchContext{
		Rule:  rules[0],
		Value: make(map[string]any),
		buffer: map[modifier.Modifier][]byte{
			modifier.Default: []byte(strings.Repeat("a", 64) + "!"),
		},
	}

	err = handler(ctx)
	if !errors.Is(err, pcre2lite.ErrMatchLimit) {
		t.Fatalf("expected wrapped match limit error, got %v", err)
	}
	if !ctx.IsRejected() {
		t.Fatal("PCRE execution error must reject the match context")
	}
}
