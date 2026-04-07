package loop_http_fuzztest

import (
	"strings"
	"testing"
)

func TestLoopHTTPFuzztestPersistentInstruction_CoversDirectAnswerAndPacketRepair(t *testing.T) {
	checks := []string{
		"已测试方面、结果与发现、下一步建议",
		"IDOR",
		"信息泄漏",
		"先修复再测",
		"User-Agent",
		"FINAL_ANSWER AITAG",
		"answer_payload 与 FINAL_ANSWER 互斥",
	}

	for _, needle := range checks {
		if !strings.Contains(instruction, needle) {
			t.Fatalf("expected persistent instruction to contain %q", needle)
		}
	}
}

func TestLoopHTTPFuzztestOutputExample_CoversStructuredDirectAnswerFewShot(t *testing.T) {
	checks := []string{
		"已测试方面：",
		"结果与发现：",
		"下一步建议：",
		"IDOR 或权限校验缺失",
		"信息泄漏线索",
		"<|FINAL_ANSWER_{{ .Nonce }}|>",
		"| 观察项 | 当前结论 | 对后续测试的价值 |",
	}

	for _, needle := range checks {
		if !strings.Contains(outputExample, needle) {
			t.Fatalf("expected output example to contain %q", needle)
		}
	}
}