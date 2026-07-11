package imcontrol

import "testing"

func TestShouldShowStreamIncludesAIError(t *testing.T) {
	e := New(Config{})

	if !e.shouldShowStream("feishu", "re-act-loop-answer-payload") {
		t.Fatal("answer payload stream should be visible")
	}
	if !e.shouldShowStream("feishu", "ai-error") {
		t.Fatal("ai-error stream should be visible so IM cards do not stay in loading state")
	}
	if e.shouldShowStream("feishu", "thought") {
		t.Fatal("thought stream should stay hidden in standard mode")
	}
}
