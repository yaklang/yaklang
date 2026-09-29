package scannode

import (
	"errors"
	"github.com/yaklang/yaklang/common/ai/aid"
	"strings"
	"testing"
)

// Adapt a deterministic response source to the real adapter wrapper.
func runLegionResultGenerator(prompt string, call func(string) (string, error)) (string, error) {
	return legionForgeResultGenerator(func(_ *aid.Coordinator, value string) (string, error) { return call(value) })(nil, prompt)
}
func TestRetryEmptyForgeResult(t *testing.T) {
	calls := 0
	result, err := runLegionResultGenerator("original report instructions", func(prompt string) (string, error) {
		calls++
		if calls == 1 {
			if prompt != "original report instructions" {
				t.Fatalf("first prompt changed: %q", prompt)
			}
			return " \n", nil
		}
		if !strings.Contains(prompt, "最终输出通道") {
			t.Fatalf("retry did not request a final output: %q", prompt)
		}
		return "# Report", nil
	})
	if err != nil || result != "# Report" || calls != 2 {
		t.Fatalf("result=%q err=%v calls=%d", result, err, calls)
	}
}

func TestRetryEmptyForgeResultNeverPublishesReasoningOrLoops(t *testing.T) {
	calls := 0
	result, err := runLegionResultGenerator("report", func(string) (string, error) {
		calls++
		return "", nil
	})
	if err == nil || result != "" || calls != 2 {
		t.Fatalf("result=%q err=%v calls=%d", result, err, calls)
	}
	calls = 0
	want := errors.New("provider failed")
	result, err = runLegionResultGenerator("report", func(string) (string, error) {
		calls++
		return "", want
	})
	if !errors.Is(err, want) || result != "" || calls != 1 {
		t.Fatalf("result=%q err=%v calls=%d", result, err, calls)
	}
}

func TestLegionForgeResultRetryPreservesPartialFailure(t *testing.T) {
	calls := 0
	failure := errors.New("interrupted stream")
	result, err := runLegionResultGenerator("report", func(string) (string, error) { calls++; return "partial", failure })
	if result != "partial" || !errors.Is(err, failure) || calls != 1 {
		t.Fatalf("result=%q err=%v calls=%d", result, err, calls)
	}
}
