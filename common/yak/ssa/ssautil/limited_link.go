package ssautil

type node[T versionedValue] struct {
	next  *node[T]
	value VersionedIF[T]
	ID    int64
}

func (n *node[T]) Append(val VersionedIF[T]) *node[T] {
	// first node
	node := &node[T]{
		next:  nil,
		value: val,
		ID:    0,
	}
	if n != nil {
		// append
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
	return
}

func (n *LinkNode[T]) Last() VersionedIF[T] {
	return n.last.value
}

func (n *LinkNode[T]) First() VersionedIF[T] {
	return n.header.value
}

func (n *LinkNode[T]) All() []VersionedIF[T] {
	var ret []VersionedIF[T]
	for current := n.header; current != nil; current = current.next {
		ret = append([]VersionedIF[T]{current.value}, ret...)
	}
	return ret
}

type linkNodeMap[T versionedValue] struct {
	val      map[string]*LinkNode[T]
	callBack linkNodeCallback[T]
}

func newLinkNodeMap[T versionedValue](callback ...linkNodeCallback[T]) linkNodeMap[T] {
	// return make(map[string]*LinkNode[T])
	cb := func(i VersionedIF[T]) {}
	if len(callback) > 0 {
		cb = callback[0]
	}
	return linkNodeMap[T]{
		val:      make(map[string]*LinkNode[T]),
		callBack: cb,
	}
}

func (m linkNodeMap[T]) Get(key string) VersionedIF[T] {
	if v, ok := m.val[key]; ok {
		return v.Last()
	}
	return nil
}

func (m linkNodeMap[T]) GetAll(key string) []VersionedIF[T] {
	if v, ok := m.val[key]; ok {
		return v.All()
	}
	return nil
}

func (m linkNodeMap[T]) GetHead(key string) VersionedIF[T] {
	if v, ok := m.val[key]; ok {
		return v.First()
	}
	return nil
}

func (m linkNodeMap[T]) ForEach(handler VariableHandler[T]) {
	for k, v := range m.val {
		handler(k, v.Last())
	}
}

func (m linkNodeMap[T]) Append(key string, val VersionedIF[T]) {
	if _, ok := m.val[key]; !ok {
		m.val[key] = &LinkNode[T]{
			last:   nil,
			header: nil,
		}
	}
	m.val[key].Append(val)
	m.callBack(val)
	return
}
