package stream_parser

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

// Diagnostic only; not part of throughput timing. Count external identifiers
// in rules which still require fresh decoding, excluding direct-plan programs.
func TestPreparedOperatorRemainingInventory(t *testing.T) {
	if os.Getenv("BIN_PARSER_PREPARED_INVENTORY") != "1" {
		t.Skip("opt-in remaining VM inventory")
	}
	counts := map[string]int{}
	prepared, remaining := 0, 0
	for source := range embeddedOperatorSources() {
		plan, _ := loadEmbeddedOperatorPlan(source)
		if plan != nil {
			continue
		}
		p, _ := loadEmbeddedPreparedOperator(source)
		if p != nil {
			prepared++
			continue
		}
		remaining++
		artifact, err := loadOperatorProgram(source)
		if err != nil {
			continue
		}
		_, codes, err := yakvm.NewCodesMarshaller().Unmarshal(artifact)
		if err != nil {
			continue
		}
		seen := map[string]bool{}
		for _, code := range codes {
			if code.Opcode == yakvm.OpPushId {
				seen[code.Op1.String()] = true
			}
			if code.Op1 != nil {
				if _, ok := code.Op1.Value.(*yakvm.Function); ok {
					seen["<function>"] = true
				}
			}
		}
		for name := range seen {
			counts[name]++
		}
		if strings.Contains(source, "identification must start") || strings.Contains(source, "ftp: invalid reply") || strings.Contains(source, "http: incomplete first word") {
			t.Logf("remaining example identifiers=%v source=%s", seen, source)
		}
	}
	t.Logf("prepared=%d remaining=%d", prepared, remaining)
	var names []string
	for name := range counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return counts[names[i]] > counts[names[j]] })
	for _, name := range names {
		t.Log(fmt.Sprintf("%d %s", counts[name], name))
	}
}
