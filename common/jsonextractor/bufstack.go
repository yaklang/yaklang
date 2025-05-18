package jsonextractor

import (
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
}

type bufStackManager struct {
	stack *vmstack.Stack
	base  *bufStack
}

func newBufStackManager(kv func(key any, val any)) *bufStackManager {
	manager := &bufStackManager{
		base: &bufStack{
			isRoot:       true,
			kv:           kv,
			currentStack: vmstack.New(),
			recorders:    []*bufStackKv{},
		},
		stack: vmstack.New(),
	}
	manager.stack.Push(manager.base)
	return manager
}

func (m *bufStackManager) PushKey(v any) {
	switch ret := v.(type) {
	case []byte:
		m.base.PushKey(string(ret))
	case string:
		m.base.PushKey(ret)
	case int:
		m.base.PushKey(ret)
	}

}

func (m *bufStackManager) PushValue(v string) {
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
