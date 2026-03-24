package reactloops

import (
	"strings"
	"testing"
)

func TestCompactIntentSummary_RemovesNarrationAndPreservesMeaning(t *testing.T) {
	input := "用户说「执行全面的主机健康状态扫描」，目的是：系统化评估主机健康状态，识别性能瓶颈与资源占用问题。通过搜索得到后续可用能力。"
	output := CompactIntentSummary(input)
	if output == "" {
		t.Fatal("expected non-empty compact summary")
	}
	if strings.Contains(output, "用户说") || strings.Contains(output, "通过搜索") {
		t.Fatalf("unexpected narration kept in compact summary: %s", output)
	}
	if !strings.Contains(output, "系统化评估主机健康状态") {
		t.Fatalf("expected core intent to be preserved, got: %s", output)
	}
}

func TestCompactIntentSummary_DoesNotCutOffLastMeaningfulRune(t *testing.T) {
	input := "执行僵尸进程清理与Memfit进程根因分析"
	output := CompactIntentSummary(input)
	if output != input {
		t.Fatalf("expected summary to keep complete label, got: %s", output)
	}
}

func TestCompactCapabilityNames_LimitsItems(t *testing.T) {
	output := CompactCapabilityNames("synscan, servicescan, nuclei_scan, report_gen", 2)
	if output != "synscan; servicescan ..." {
		t.Fatalf("unexpected compact names output: %s", output)
	}
}

func TestCompactCapabilityNames_HidesEmptyJSONArray(t *testing.T) {
	output := CompactCapabilityNames("[]", 3)
	if output != "" {
		t.Fatalf("expected empty output for empty json array, got: %s", output)
	}
}

func TestCompactCapabilityNames_ParsesJSONArrayAndSemicolonString(t *testing.T) {
	output := CompactCapabilityNames(`["xss_tool; load_file_tool;"]`, 5)
	if output != "xss_tool; load_file_tool" {
		t.Fatalf("unexpected compact names output: %s", output)
	}
}

func TestCompactCapabilityNames_FiltersDefaultMarker(t *testing.T) {
	output := CompactCapabilityNames(`["__DEFAULT__", "xss_tool", "load_file_tool"]`, 5)
	if output != "xss_tool; load_file_tool" {
		t.Fatalf("unexpected compact names output: %s", output)
	}
}
