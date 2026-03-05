package diagnostics

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// BuildTreeTracker 追踪 LazyBuild 调用栈，输出树形结构
type BuildTreeTracker interface {
	PushLazyBuild(id string)
	PopLazyBuild(duration time.Duration, hadTasks bool)
	// PopLazyBuildProgramLevel 程序级 @main 结束时调用，会检测 stack 遗留并 warning
	PopLazyBuildProgramLevel(duration time.Duration, hadTasks bool)
	PrintTree(label string)
}

type buildTreeNode struct {
	id       string
	start    time.Time
	total    time.Duration
	children []*buildTreeNode
}

// buildTreeEvent 记录进入/退出事件，用于打印时还原 > 和 <
type buildTreeEvent struct {
	kind     string // "enter" 或 "exit"
	id       string
	depth    int
	duration time.Duration // 仅 exit 有效
	hadTasks bool          // 仅 exit 有效：是否曾 AddLazyBuilder
}

type buildTreeTrackerImpl struct {
	mu    sync.Mutex
	stack []*buildTreeNode
	root  *buildTreeNode
	// events 记录进入/退出事件序列，与 stack 分离，用于最终打印
	events []buildTreeEvent
}

// NewBuildTreeTracker 创建 BuildTreeTracker 实例
func NewBuildTreeTracker() BuildTreeTracker {
	return &buildTreeTrackerImpl{
		stack:  make([]*buildTreeNode, 0),
		root:   &buildTreeNode{id: "root", children: make([]*buildTreeNode, 0)},
		events: make([]buildTreeEvent, 0),
	}
}

func (t *buildTreeTrackerImpl) PushLazyBuild(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	depth := len(t.stack)
	n := &buildTreeNode{
		id:       id,
		start:    time.Now(),
		children: make([]*buildTreeNode, 0),
	}
	if len(t.stack) == 0 {
		t.root.children = append(t.root.children, n)
		t.stack = append(t.stack, n)
	} else {
		parent := t.stack[len(t.stack)-1]
		parent.children = append(parent.children, n)
		t.stack = append(t.stack, n)
	}
	t.events = append(t.events, buildTreeEvent{kind: "enter", id: id, depth: depth})
}

func (t *buildTreeTrackerImpl) PopLazyBuild(duration time.Duration, hadTasks bool) {
	t.popLazyBuildInternal(duration, hadTasks, false)
}

// PopLazyBuildProgramLevel 程序级 @main 结束时调用，会检测 stack 是否有遗留
func (t *buildTreeTrackerImpl) PopLazyBuildProgramLevel(duration time.Duration, hadTasks bool) {
	t.popLazyBuildInternal(duration, hadTasks, true)
}

func (t *buildTreeTrackerImpl) popLazyBuildInternal(duration time.Duration, hadTasks bool, isProgramLevel bool) {
	t.mu.Lock()
	if len(t.stack) == 0 {
		t.mu.Unlock()
		return
	}
	n := t.stack[len(t.stack)-1]
	depth := len(t.stack) - 1
	t.stack = t.stack[:len(t.stack)-1]
	n.total = duration
	t.events = append(t.events, buildTreeEvent{kind: "exit", id: n.id, depth: depth, duration: duration, hadTasks: hadTasks})
	id := n.id
	// @main 结束时检测 stack 是否有遗留（应已全部 pop）
	leftoverIds := make([]string, 0, len(t.stack))
	if isProgramLevel && len(t.stack) > 0 {
		for _, item := range t.stack {
			leftoverIds = append(leftoverIds, item.id)
		}
	}
	t.mu.Unlock()

	if len(leftoverIds) > 0 {
		log.Warnf("[BuildTreeTracker] @main 结束时 LazyBuild stack 仍有遗留（未完成 build）: %v", leftoverIds)
	}

	// LazyBuild 结束时直接输出 SSA_PERF 日志（tracker 仅在 file-perf-log 开启时存在）
	if duration >= time.Microsecond {
		LogPerfLineIf(true, "%s  %s", id, formatDurationShort(duration))
	}
}

func (t *buildTreeTrackerImpl) PrintTree(label string) {
	t.mu.Lock()
	rootCopy := t.root
	stackLen := len(t.stack)
	unfinishedIds := make([]string, 0, stackLen)
	for _, n := range t.stack {
		unfinishedIds = append(unfinishedIds, n.id)
	}
	t.mu.Unlock()

	if len(rootCopy.children) == 0 {
		return
	}

	title := "Build Phase Performance Summary"
	const titleWidth = 67 // 与 AST Phase Performance Summary 对齐
	titleBorder := strings.Repeat("=", titleWidth)
	var sb strings.Builder
	sb.WriteString("\n" + titleBorder + "\n")
	sb.WriteString(fmt.Sprintf(" %s\n", title))
	sb.WriteString(titleBorder + "\n")
	printTreeNode(rootCopy.children, "", &sb)
	sb.WriteString(titleBorder + "\n")
	fmt.Println(sb.String())

	if stackLen > 0 {
		log.Warnf("[BuildTreeTracker] LazyBuild 已进入但未退出（未完成 build）: %v", unfinishedIds)
	}
}

// printTreeNode 递归输出树形结构，风格：├─ └─ │
func printTreeNode(nodes []*buildTreeNode, prefix string, sb *strings.Builder) {
	// 过滤耗时 <1µs 的节点
	visible := make([]*buildTreeNode, 0, len(nodes))
	for _, n := range nodes {
		if n.total >= time.Microsecond {
			visible = append(visible, n)
		}
	}
	for i, n := range visible {
		isLast := i == len(visible)-1
		connector := "├─"
		if isLast {
			connector = "└─"
		}
		line := fmt.Sprintf("%s%s%s", prefix, connector, n.id)
		if n.total >= time.Microsecond {
			line += "  " + formatDurationShort(n.total)
		}
		sb.WriteString(line + "\n")

		childPrefix := prefix
		if isLast {
			childPrefix += "   "
		} else {
			childPrefix += "│  "
		}
		if len(n.children) > 0 {
			printTreeNode(n.children, childPrefix, sb)
		}
	}
}

func formatDurationShort(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%.2fms", float64(d.Nanoseconds())/1e6)
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}
