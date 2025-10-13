package aimem

import "testing"

func TestNewAIMemory(t *testing.T) {
	mem, err := NewAIMemory("test", WithContextProvider(func() (string, error) {
		return "获取一些背景知识，用户根据这些内容在实现一些复杂功能", nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	_ = mem
	// mem.AddRawMemory("AI 决定执行 write_code, Code 的内容为 [....] 其中有点问题，reflection 出了点问题。")
}
