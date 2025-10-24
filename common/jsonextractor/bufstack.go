package jsonextractor

import (
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"
)

type bufStackKv struct {
	key any
	val any
}

type bufStack struct {
	isRoot       bool
	key          any
	parent       *bufStack
	kv           func(key any, val any)
	currentStack *vmstack.Stack
	recorders    []*bufStackKv
	// 字段流写入器，绑定到当前栈层级
	fieldWriters []io.WriteCloser
}

type bufStackManager struct {
	stack           *vmstack.Stack
	base            *bufStack
	callbackManager *callbackManager
}

func newBufStackManager(kv func(key any, val any, parents []string)) *bufStackManager {
	manager := &bufStackManager{
		stack: vmstack.New(),
	}
	manager.base = &bufStack{
		isRoot: true,
		kv: func(key any, val any) {
			kv(key, val, manager.getParentPath())
		},
		currentStack: vmstack.New(),
		recorders:    []*bufStackKv{},
	}
	manager.stack.Push(manager.base)
	return manager
}

func (m *bufStackManager) setCallbackManager(cm *callbackManager) {
	m.callbackManager = cm
}

func (m *bufStackManager) getCurrentKey() any {
	if m.base != nil && m.base.currentStack != nil {
		return m.base.currentStack.PeekN(1)
	}
	return nil
}

func (m *bufStackManager) PushKey(v any) {
	switch ret := v.(type) {
	case []byte:
		keyStr := string(ret)
		m.base.PushKey(keyStr)
		// 检查是否需要开始字段流处理，将写入器绑定到当前栈
		if m.callbackManager != nil {
			writers := m.callbackManager.handleFieldStreamStart(keyStr, m)
			m.base.fieldWriters = writers
		}
	case string:
		m.base.PushKey(ret)
		// 检查是否需要开始字段流处理，将写入器绑定到当前栈
		if m.callbackManager != nil {
			writers := m.callbackManager.handleFieldStreamStart(ret, m)
			m.base.fieldWriters = writers
		}
	case int:
		m.base.PushKey(ret)
		// 数组索引不需要字段流处理
		m.base.fieldWriters = nil
	}
}

// activatePendingFieldWriter 激活待处理的字段写入器
func (m *bufStackManager) activatePendingFieldWriter() {
	if len(m.base.fieldWriters) > 0 && m.callbackManager != nil {
		m.callbackManager.activeWriters = make([]io.WriteCloser, len(m.base.fieldWriters))
		copy(m.callbackManager.activeWriters, m.base.fieldWriters)
	}
}

// getParentPath 从stack中获取父路径
func (m *bufStackManager) getParentPath() []string {
	parents := make([]string, 0)

	// 从stack遍历父路径
	current := m.base
	for current != nil && !current.isRoot {
		if current.key != nil {
			if keyStr, ok := current.key.(string); ok {
				// 清理键名中的引号和空格
				cleanKey := strings.Trim(strings.TrimSpace(keyStr), `"`)
				// 将父路径插入到开头，保持正确的顺序
				parents = append([]string{cleanKey}, parents...)
			}
		}
		current = current.parent
	}

	return parents
}

func (m *bufStackManager) getPrefixKey() []string { // get parent path and current path prefix key
	prefix := m.getParentPath()

	// 需要检查当前正在处理的键
	if m.base != nil && m.base.currentStack != nil {
		// 获取stack中的所有键，除了最后一个（当前正在处理的值）
		size := m.base.currentStack.Len()
		for i := 0; i < size-1; i++ {
			if key := m.base.currentStack.PeekN(size - i); key != nil {
				if keyStr, ok := key.(string); ok {
					// 清理键名中的引号和空格
					cleanKey := strings.Trim(strings.TrimSpace(keyStr), `"`)
					prefix = append(prefix, cleanKey)
				}
			}
		}
	}

	return prefix
}

func (m *bufStackManager) PushValue(v string) {
	// 字符级流式写入现在在状态机中处理，这里不再写入
	// 清理当前栈的字段写入器（如果有的话）
	if len(m.base.fieldWriters) > 0 {
		m.base.fieldWriters = nil
	}
	m.base.PushValue(v)
}

func (m *bufStackManager) PushContainer() {
	var keyRaw any
	if ret := m.base.currentStack.Peek(); ret != nil {
		keyRaw = ret
	}
	sub := &bufStack{
		isRoot:       false,
		key:          keyRaw,
		parent:       m.base,
		kv:           m.base.kv,
		currentStack: vmstack.New(),
		recorders:    []*bufStackKv{},
		// 继承父栈的字段写入器
		fieldWriters: m.base.fieldWriters,
	}
	m.base = sub
	m.stack.Push(sub)
}

func (m *bufStackManager) PopContainer() {
	sub := m.stack.Pop()
	if sub != nil {
		if subSubStack, ok := sub.(*bufStack); ok {
			m.base = subSubStack.parent
			result := make(map[any]any)
			for _, v := range subSubStack.recorders {
				result[v.key] = v.val
			}
			m.base.emit(subSubStack.key, result)
			m.base.recorders = append(m.base.recorders, &bufStackKv{
				key: subSubStack.key,
				val: result,
			})
		}
	}
}

func (b *bufStack) emit(k any, v any) {
	if b.kv != nil {
		b.kv(k, v)
		return
	}
	log.Infof("emit: %v, %v", k, v)
}

func (b *bufStack) PushKey(v any) {
	b.currentStack.Push(v)
}

func (b *bufStack) PushValue(v string) {
	defer func() {
		keyRaw := b.currentStack.PeekN(1)
		b.emit(keyRaw, v)
		b.recorders = append(b.recorders, &bufStackKv{
			key: keyRaw,
			val: v,
		})
	}()
	b.currentStack.Push(v)
}

func (m *bufStackManager) TriggerEmit() {
	b := m.base
	for {
		if b.isRoot {
			break
		}
		b = b.parent
	}
	finalResult := make(map[any]any)
	for _, item := range b.recorders {
		finalResult[item.key] = item.val
	}
	b.emit("", finalResult)
}
