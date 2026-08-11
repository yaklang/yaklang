package jsonextractor

import (
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"
)

type bufStackKv struct {
	key any
	val any
}

type bufStack struct {
	isRoot bool
	// containerKindKnown is set by the streaming state machine. Older direct
	// bufStack tests/callers can omit the kind and retain the legacy generic map.
	containerKindKnown bool
	isArray            bool
	key                any
	parent             *bufStack
	kv                 func(key any, val any)
	currentStack       *vmstack.Stack
	recorders          []*bufStackKv
	// 字段流上下文，绑定到当前栈层级
	fieldStreamContexts []*fieldStreamContext
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

func (m *bufStackManager) prepareFieldStreamContexts(key string) {
	if m.callbackManager == nil {
		return
	}
	if len(m.base.fieldStreamContexts) > 0 {
		return
	}
	contexts := m.callbackManager.handleFieldStreamStart(key, m)
	if len(contexts) > 0 {
		m.base.fieldStreamContexts = contexts
	}
}

func (m *bufStackManager) PushKey(v any) {
	switch ret := v.(type) {
	case []byte:
		keyStr := string(ret)
		m.base.PushKey(keyStr)
		// 如果尚未准备字段流上下文，则现在准备
		m.prepareFieldStreamContexts(keyStr)
	case string:
		m.base.PushKey(ret)
		// 如果尚未准备字段流上下文，则现在准备
		m.prepareFieldStreamContexts(ret)
	case int:
		m.base.PushKey(ret)
		// 数组索引不需要字段流处理
		m.base.fieldStreamContexts = nil
	}
}

// activatePendingFieldWriter 激活待处理的字段写入器
func (m *bufStackManager) activatePendingFieldWriter() *fieldStreamFrame {
	if len(m.base.fieldStreamContexts) > 0 && m.callbackManager != nil {
		frame := m.callbackManager.pushFieldStreamFrame(m.base.fieldStreamContexts)
		m.base.fieldStreamContexts = nil
		return frame
	}
	return nil
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
	if len(m.base.fieldStreamContexts) > 0 {
		m.base.fieldStreamContexts = nil
	}
	m.base.PushValue(v)
}

func (m *bufStackManager) PushContainer(arrayKind ...bool) {
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
		fieldStreamContexts: m.base.fieldStreamContexts,
	}
	if len(arrayKind) > 0 {
		sub.containerKindKnown = true
		sub.isArray = arrayKind[0]
	}
	m.base = sub
	m.stack.Push(sub)
}

func (m *bufStackManager) PopContainer() {
	sub := m.stack.Pop()
	if sub != nil {
		if subSubStack, ok := sub.(*bufStack); ok {
			m.base = subSubStack.parent
			var result any
			switch {
			case subSubStack.containerKindKnown && subSubStack.isArray:
				values := make(map[int]any, len(subSubStack.recorders))
				for _, v := range subSubStack.recorders {
					if index, ok := v.key.(int); ok {
						values[index] = v.val
					}
				}
				result = values
			case subSubStack.containerKindKnown:
				values := make(map[string]any, len(subSubStack.recorders))
				for _, v := range subSubStack.recorders {
					if key, ok := v.key.(string); ok {
						values[key] = v.val
					}
				}
				result = values
			default:
				values := make(map[any]any, len(subSubStack.recorders))
				for _, v := range subSubStack.recorders {
					values[v.key] = v.val
				}
				result = values
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
