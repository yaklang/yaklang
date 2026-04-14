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
		"fuzztag 使用规则",
		"{{fuzz:password}}",
		"{{payload(pass_top25)}}",
		"不要手写几十上百个 payload",
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
		"{{fuzz:password(admin)}}",
		"{{fuzz:username(admin)}}",
		"这里优先使用 fuzztag 表达批量生成规则",
	}

	for _, needle := range checks {
		if !strings.Contains(outputExample, needle) {
			t.Fatalf("expected output example to contain %q", needle)
		}
	}
}

func TestLoopHTTPFuzztestReactiveData_CoversFuzztagReferenceBlocks(t *testing.T) {
	checks := []string{
		"FUZZTAG_REFERENCE",
		"AVAILABLE_PAYLOAD_GROUPS",
		"fuzztag 手册",
		"`payload(...)` 系列 fuzztag",
	}

	for _, needle := range checks {
		if !strings.Contains(reactiveData, needle) {
			t.Fatalf("expected reactive data to contain %q", needle)
		}
	}
}
