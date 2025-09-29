package aicommon

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils/yakgit/yakdiff"
)

// TimelineDiffer 用于计算 Timeline 的差异
// 每次 Diff 都会保存上一次的内容，下次 Diff 的结果是和上一次的对比
type TimelineDiffer struct {
	timeline    *Timeline
	lastDump    string // 上一次的 Timeline 转储内容
	lastDumpMux sync.RWMutex
}

// NewTimelineDiffer 创建一个新的 TimelineDiffer 实例
func NewTimelineDiffer(timeline *Timeline) *TimelineDiffer {
	return &TimelineDiffer{
		timeline: timeline,
		lastDump: "", // 初始为空，表示第一次比较
	}
}

// Diff 计算当前 Timeline 和上一次状态的差异
// 返回差异描述字符串，描述了 Timeline 的变化增量
// 第一次调用时返回当前 Timeline 和空状态的对比（即完整的当前状态）
// 调用后自动更新基准状态为当前状态
func (d *TimelineDiffer) Diff() (string, error) {
	d.lastDumpMux.Lock()
	defer d.lastDumpMux.Unlock()

	// 获取当前 Timeline 的转储内容
	currentDump := d.timeline.Dump()

	// 使用 yakdiff 计算差异
	diff, err := yakdiff.Diff(d.lastDump, currentDump)
	if err != nil {
		return "", err
	}

	// 更新上一次的状态为当前状态
	d.lastDump = currentDump

	return diff, nil
}

// GetCurrentDump 获取当前 Timeline 的转储内容（不更新状态）
func (d *TimelineDiffer) GetCurrentDump() string {
	return d.timeline.Dump()
}

// GetLastDump 获取上一次保存的 Timeline 转储内容
func (d *TimelineDiffer) GetLastDump() string {
	d.lastDumpMux.RLock()
	defer d.lastDumpMux.RUnlock()
	return d.lastDump
}

// Reset 重置差异计算器，清空上一次的状态
// 下次调用 Diff() 将返回当前状态和空状态的对比
func (d *TimelineDiffer) Reset() {
	d.lastDumpMux.Lock()
	defer d.lastDumpMux.Unlock()
	d.lastDump = ""
}

// SetBaseline 手动设置基准状态为当前 Timeline 的状态
// 下次调用 Diff() 将返回当前状态和此基准状态的对比
func (d *TimelineDiffer) SetBaseline() {
	d.lastDumpMux.Lock()
	defer d.lastDumpMux.Unlock()
	d.lastDump = d.timeline.Dump()
}
