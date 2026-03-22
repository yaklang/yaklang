package reactloops

import (
	"strings"
	"testing"
)

func TestCompactIntentSummary_RemovesNarrationAndTruncates(t *testing.T) {
	input := "用户说「执行全面的主机健康状态扫描」，目的是：系统化评估主机健康状态，识别性能瓶颈与资源占用问题。通过搜索得到后续可用能力。"
	output := CompactIntentSummary(input)
	if output == "" {
		t.Fatal("expected non-empty compact summary")
	}
	if strings.Contains(output, "用户说") || strings.Contains(output, "通过搜索") {
		t.Fatalf("unexpected narration kept in compact summary: %s", output)
	}
	if len([]rune(output)) > IntentSummaryMaxRunes {
		t.Fatalf("compact summary should be <= %d runes, got %d: %s", IntentSummaryMaxRunes, len([]rune(output)), output)
	}
}

func TestCompactCapabilityNames_LimitsItems(t *testing.T) {
	output := CompactCapabilityNames("synscan, servicescan, nuclei_scan, report_gen", 2)
	if output != "synscan, servicescan ..." {
		t.Fatalf("unexpected compact names output: %s", output)
	}
}
