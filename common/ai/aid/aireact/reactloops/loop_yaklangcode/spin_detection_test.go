package loop_yaklangcode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockLoop 模拟 ReActLoop 的存储接口
type MockLoop struct {
	storage    map[string]string
	lastAction *MockAction
}

type MockAction struct {
	actionName string
}

func (m *MockAction) GetActionName() string {
	return m.actionName
}

func NewMockLoop() *MockLoop {
	return &MockLoop{
		storage: make(map[string]string),
	}
}

func (m *MockLoop) Get(key string) string {
	return m.storage[key]
}

func (m *MockLoop) Set(key string, value any) {
	m.storage[key] = value.(string)
}

func (m *MockLoop) GetLastAction() interface{ GetActionName() string } {
	return m.lastAction
}

func (m *MockLoop) SetLastAction(actionName string) {
	m.lastAction = &MockAction{actionName: actionName}
}

// TestIsInSameRegion 测试区域判断
func TestIsInSameRegion(t *testing.T) {
	tests := []struct {
		name     string
		r1       ModifyRecord
		r2       ModifyRecord
		expected bool
	}{
		{
			name:     "完全相同的行",
			r1:       ModifyRecord{StartLine: 10, EndLine: 15},
			r2:       ModifyRecord{StartLine: 10, EndLine: 15},
			expected: true,
		},
		{
			name:     "起始行相近（3行差距）",
			r1:       ModifyRecord{StartLine: 10, EndLine: 15},
			r2:       ModifyRecord{StartLine: 13, EndLine: 18},
			expected: true,
		},
		{
			name:     "起始行边界（5行差距）",
			r1:       ModifyRecord{StartLine: 10, EndLine: 15},
			r2:       ModifyRecord{StartLine: 15, EndLine: 20},
			expected: true,
		},
		{
			name:     "起始行超出范围（6行差距）",
			r1:       ModifyRecord{StartLine: 10, EndLine: 15},
			r2:       ModifyRecord{StartLine: 16, EndLine: 20},
			expected: false,
		},
		{
			name:     "结束行相近但起始行差距大",
			r1:       ModifyRecord{StartLine: 10, EndLine: 15},
			r2:       ModifyRecord{StartLine: 20, EndLine: 18},
			expected: false, // 起始行差距10行，虽然结束行相近但不算同一区域
		},
		{
			name:     "完全不同区域",
			r1:       ModifyRecord{StartLine: 10, EndLine: 15},
			r2:       ModifyRecord{StartLine: 50, EndLine: 55},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInSameRegion(tt.r1, tt.r2)
			assert.Equal(t, tt.expected, result, "区域判断结果不符合预期")
		})
	}
}

// TestIsSmallEdit 测试小幅修改判断
func TestIsSmallEdit(t *testing.T) {
	tests := []struct {
		name     string
		record   ModifyRecord
		expected bool
	}{
		{
			name:     "单行修改",
			record:   ModifyRecord{StartLine: 10, EndLine: 10},
			expected: true,
		},
		{
			name:     "2行修改",
			record:   ModifyRecord{StartLine: 10, EndLine: 11},
			expected: true,
		},
		{
			name:     "3行修改（边界）",
			record:   ModifyRecord{StartLine: 10, EndLine: 12},
			expected: true,
		},
		{
			name:     "4行修改",
			record:   ModifyRecord{StartLine: 10, EndLine: 13},
			expected: false,
		},
		{
			name:     "大量行修改",
			record:   ModifyRecord{StartLine: 10, EndLine: 50},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSmallEdit(tt.record)
			assert.Equal(t, tt.expected, result, "小幅修改判断结果不符合预期")
		})
	}
}

// TestDetectSpinning_NoHistory 测试没有历史记录的情况
func TestDetectSpinning_NoHistory(t *testing.T) {
	loop := NewMockLoop()
	loop.SetLastAction("modify_code")

	record := ModifyRecord{StartLine: 10, EndLine: 12}
	isSpinning, reason := detectSpinning(loop, record)

	assert.False(t, isSpinning, "首次修改不应该被判定为打转")
	assert.Empty(t, reason, "首次修改不应该有打转原因")
	assert.Equal(t, "10-12", loop.Get("modify_history"), "应该记录修改历史")
	assert.Equal(t, "1", loop.Get("modify_spin_count"), "应该记录计数为1")
}

// TestDetectSpinning_ResetAfterTrigger 测试触发后重置
func TestDetectSpinning_ResetAfterTrigger(t *testing.T) {
	loop := NewMockLoop()
	loop.SetLastAction("modify_code")

	// 连续3次修改同一区域触发打转检测
	r1 := ModifyRecord{StartLine: 10, EndLine: 12}
	detectSpinning(loop, r1)

	loop.SetLastAction("modify_code")
	r2 := ModifyRecord{StartLine: 11, EndLine: 13}
	detectSpinning(loop, r2)

	loop.SetLastAction("modify_code")
	r3 := ModifyRecord{StartLine: 12, EndLine: 14}
	isSpinning, _ := detectSpinning(loop, r3)
	assert.True(t, isSpinning, "第三次应该触发")
	assert.Equal(t, "0", loop.Get("modify_spin_count"), "触发后重置为0")

	// 再次修改，应该重新开始计数
	loop.SetLastAction("modify_code")
	r4 := ModifyRecord{StartLine: 13, EndLine: 15}
	isSpinning, _ = detectSpinning(loop, r4)
	assert.False(t, isSpinning, "重置后再次修改不应触发")
	assert.Equal(t, "1", loop.Get("modify_spin_count"), "应该重新计数为1")
}

// TestDetectSpinning_SameRegionSmallEdits 测试相同区域的小幅修改
func TestDetectSpinning_SameRegionSmallEdits(t *testing.T) {
	loop := NewMockLoop()
	loop.SetLastAction("modify_code")

	// 第一次修改
	record1 := ModifyRecord{StartLine: 10, EndLine: 12}
	isSpinning, _ := detectSpinning(loop, record1)
	assert.False(t, isSpinning, "第一次不应该打转")

	// 第二次修改（相同区域，小幅修改）
	loop.SetLastAction("modify_code")
	record2 := ModifyRecord{StartLine: 11, EndLine: 13}
	isSpinning, _ = detectSpinning(loop, record2)
	assert.False(t, isSpinning, "第二次不应该打转")
	assert.Equal(t, "2", loop.Get("modify_spin_count"), "计数应该为2")

	// 第三次修改（相同区域，小幅修改）- 应该触发
	loop.SetLastAction("modify_code")
	record3 := ModifyRecord{StartLine: 12, EndLine: 14}
	isSpinning, reason := detectSpinning(loop, record3)
	assert.True(t, isSpinning, "第三次应该触发打转检测")
	assert.Contains(t, reason, "连续 3 次", "原因应该包含连续次数")
	assert.Equal(t, "0", loop.Get("modify_spin_count"), "触发后计数应该重置")
}

// TestDetectSpinning_DifferentRegion 测试不同区域的修改
func TestDetectSpinning_DifferentRegion(t *testing.T) {
	loop := NewMockLoop()
	loop.SetLastAction("modify_code")

	// 第一次修改
	record1 := ModifyRecord{StartLine: 10, EndLine: 12}
	detectSpinning(loop, record1)
	assert.Equal(t, "1", loop.Get("modify_spin_count"), "第一次计数应该为1")

	// 第二次修改（不同区域）
	loop.SetLastAction("modify_code")
	record2 := ModifyRecord{StartLine: 50, EndLine: 52}
	isSpinning, _ := detectSpinning(loop, record2)

	assert.False(t, isSpinning, "不同区域不应该打转")
	assert.Equal(t, "0", loop.Get("modify_spin_count"), "不同区域应该完全重置计数为0")
}

// TestDetectSpinning_LargeEdit 测试大幅修改
func TestDetectSpinning_LargeEdit(t *testing.T) {
	loop := NewMockLoop()
	loop.SetLastAction("modify_code")

	// 第一次小幅修改
	record1 := ModifyRecord{StartLine: 10, EndLine: 12}
	detectSpinning(loop, record1)
	assert.Equal(t, "1", loop.Get("modify_spin_count"))

	// 第二次大幅修改（相同区域但改动大）
	// 起始行和结束行都在5行内，但修改行数>3行
	loop.SetLastAction("modify_code")
	record2 := ModifyRecord{StartLine: 11, EndLine: 17} // 7行修改，起始行差1，结束行差5
	isSpinning, _ := detectSpinning(loop, record2)

	assert.False(t, isSpinning, "大幅修改不应该被判定为打转")
	assert.Equal(t, "1", loop.Get("modify_spin_count"), "相同区域但大幅修改，计数应该重置为1")
}

// TestDetectSpinning_CompleteScenario 完整场景测试
func TestDetectSpinning_CompleteScenario(t *testing.T) {
	loop := NewMockLoop()

	// 场景1：第一次修改
	loop.SetLastAction("modify_code")
	r1 := ModifyRecord{StartLine: 10, EndLine: 12}
	isSpinning, _ := detectSpinning(loop, r1)
	assert.False(t, isSpinning)
	assert.Equal(t, "1", loop.Get("modify_spin_count"))

	// 场景2：连续第二次修改相同区域
	loop.SetLastAction("modify_code")
	r2 := ModifyRecord{StartLine: 11, EndLine: 13}
	isSpinning, _ = detectSpinning(loop, r2)
	assert.False(t, isSpinning)
	assert.Equal(t, "2", loop.Get("modify_spin_count"))

	// 场景3：连续第三次修改相同区域 - 应该触发
	loop.SetLastAction("modify_code")
	r3 := ModifyRecord{StartLine: 12, EndLine: 14}
	isSpinning, reason := detectSpinning(loop, r3)
	assert.True(t, isSpinning)
	assert.Contains(t, reason, "第 12-14 行")
	assert.Contains(t, reason, "连续 3 次")
	assert.Equal(t, "0", loop.Get("modify_spin_count"), "触发后应该重置为0")
}

// TestDetectSpinning_HistoryLimit 测试历史记录限制
func TestDetectSpinning_HistoryLimit(t *testing.T) {
	loop := NewMockLoop()
	loop.SetLastAction("modify_code")

	// 添加多次修改
	records := []ModifyRecord{
		{StartLine: 10, EndLine: 12},
		{StartLine: 20, EndLine: 22},
		{StartLine: 30, EndLine: 32},
		{StartLine: 40, EndLine: 42},
	}

	for _, record := range records {
		loop.SetLastAction("modify_code")
		detectSpinning(loop, record)
	}

	// 验证只保留最近3条
	history := loop.Get("modify_history")
	parts := len(splitHistory(history))
	assert.LessOrEqual(t, parts, 3, "历史记录应该不超过3条")
}

// TestGenerateReflectionPrompt 测试反思提示生成
func TestGenerateReflectionPrompt(t *testing.T) {
	record := ModifyRecord{StartLine: 10, EndLine: 15}
	reason := "检测到连续修改"

	prompt := generateReflectionPrompt(record, reason)

	// 验证提示包含关键信息
	assert.Contains(t, prompt, "代码修改空转警告", "应该包含警告标题")
	assert.Contains(t, prompt, reason, "应该包含原因")
	assert.Contains(t, prompt, "第 10-15 行", "应该包含行号")
	assert.Contains(t, prompt, "问题1：改动价值", "应该包含反思问题")
	assert.Contains(t, prompt, "问题2：备选路径", "应该包含备选方案提示")
	assert.Contains(t, prompt, "问题3：搜索建议", "应该包含搜索建议")
	assert.Contains(t, prompt, "grep_yaklang_samples", "应该包含grep搜索")
	assert.Contains(t, prompt, "semantic_search_yaklang_samples", "应该包含语义搜索")
	assert.Contains(t, prompt, "不要再继续在同一位置反复尝试", "应该包含停止建议")
}

// 辅助函数：分割历史记录
func splitHistory(history string) []string {
	if history == "" {
		return []string{}
	}
	// 简单按分号分割
	count := 1 // 至少有一条记录
	for i := 0; i < len(history); i++ {
		if history[i] == ';' {
			count++
		}
	}
	return make([]string, count)
}
