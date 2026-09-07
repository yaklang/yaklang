package scannode

import "testing"

func TestScriptFailureFromResultPrefersStructuredCode(t *testing.T) {
	t.Parallel()

	got := scriptFailureFromResult(&ScriptExecutionResult{
		Data: map[string]any{
			"error":      "没有可编译的源码文件",
			"error_code": "notFoundFileCanCompile",
		},
	})
	if got == nil {
		t.Fatal("expected structured failure")
	}
	if got.Code != "notFoundFileCanCompile" {
		t.Fatalf("code = %q", got.Code)
	}
	if got.Message != "没有可编译的源码文件" {
		t.Fatalf("message = %q", got.Message)
	}

	clone := scriptFailureFromResult(&ScriptExecutionResult{
		Data: map[string]any{
			"error":      "克隆源码失败: EOF",
			"error_code": "gitCloneError",
		},
	})
	if clone == nil || clone.Code != "gitCloneError" {
		t.Fatalf("clone failure = %#v", clone)
	}
}
