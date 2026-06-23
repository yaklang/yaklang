package ssautil

import (
	"github.com/yaklang/yaklang/common/utils/omap"
)

type node[T versionedValue] struct {
	next  *node[T]
	value VersionedIF[T]
	ID    int64
}

func (n *node[T]) Append(val VersionedIF[T]) *node[T] {
	node := &node[T]{
		next:  nil,
		value: val,
		ID:    0,
	}
	if n != nil {
		n.next = node
		node.ID = n.ID + 1
	}
	val.SetVersion(node.ID)
	return node
}

type linkNodeCallback[T versionedValue] func(i VersionedIF[T])

type LinkNode[T versionedValue] struct {
	last   *node[T]
	header *node[T]
}

func (n *LinkNode[T]) Append(val VersionedIF[T]) {
	n.last = n.last.Append(val)
	if n.header == nil {
		n.header = n.last
	}
}

func (n *LinkNode[T]) Last() VersionedIF[T] {
	return n.last.value
}

func (n *LinkNode[T]) First() VersionedIF[T] {
	return n.header.value
}

func (n *LinkNode[T]) All() []VersionedIF[T] {
	if n == nil || n.header == nil {
		return nil
	}

	length := 0
	if n.last != nil && n.last.ID >= 0 {
		length = int(n.last.ID + 1)
	}
	if length <= 0 {
		for current := n.header; current != nil; current = current.next {
			length++
		}
	}

	ret := make([]VersionedIF[T], length)
	i := length - 1
	for current := n.header; current != nil; current = current.next {
		ret[i] = current.value
		i--
	}
	return ret
}

type linkNodeMap[T versionedValue] struct {
	val      *omap.OrderedMap[string, *LinkNode[T]]
	callBack linkNodeCallback[T]
}

func newLinkNodeMap[T versionedValue](callback ...linkNodeCallback[T]) linkNodeMap[T] {
	cb := func(i VersionedIF[T]) {}
	if len(callback) > 0 {
		cb = callback[0]
	}
	return linkNodeMap[T]{
		val:      omap.NewEmptyOrderedMap[string, *LinkNode[T]](),
		callBack: cb,
	}
}

func (m linkNodeMap[T]) Get(key string) VersionedIF[T] {
	if v, ok := m.val.Get(key); ok {
		return v.Last()
	}
	return nil
}

func (m linkNodeMap[T]) GetAll(key string) []VersionedIF[T] {
	if v, ok := m.val.Get(key); ok {
		return v.All()
	}
	return nil
}

func (m linkNodeMap[T]) GetHead(key string) VersionedIF[T] {
	if v, ok := m.val.Get(key); ok {
		return v.First()
	}
	return nil
}

func (m linkNodeMap[T]) ForEach(handler VariableHandler[T]) {
	m.val.ForEach(func(k string, v *LinkNode[T]) bool {
		handler(k, v.Last())
		return true
	})
}

func (m linkNodeMap[T]) Append(key string, val VersionedIF[T]) {
	ln, ok := m.val.Get(key)
	if !ok {
		ln = &LinkNode[T]{}
		m.val.Set(key, ln)
	}
	ln.Append(val)
	m.callBack(val)
}
