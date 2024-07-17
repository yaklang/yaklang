package ssautil

type linkNodeCallback func(i ...any)

type LinkNode[T versionedValue] struct {
	Next  *LinkNode[T]
	_last *LinkNode[T]
	Value VersionedIF[T]
	Id    int

	createCallback linkNodeCallback
}

func NewInitLinkNode[T versionedValue](val VersionedIF[T], callback linkNodeCallback) *LinkNode[T] {
	i := &LinkNode[T]{
		Value:          val,
		Next:           nil,
		createCallback: callback,
		Id:             1,
	}
	i._last = i
	return i
}

func (n *LinkNode[T]) Append(val VersionedIF[T]) *LinkNode[T] {
	if n.Next == nil {
		n.Next = NewInitLinkNode[T](val, n.createCallback)
		n.Next.Id = n.Id + 1
		return n.Next
	}
	n._last = n.Next.Append(val)
	return n._last
}

func (n *LinkNode[T]) Last() *LinkNode[T] {
	return n._last
}

type linkNodeMap[T versionedValue] map[string]*LinkNode[T]

func newLinkNodeMap[T versionedValue]() linkNodeMap[T] {
	return make(map[string]*LinkNode[T])
}

func (m linkNodeMap[T]) Append(key string, val VersionedIF[T], callback linkNodeCallback) *LinkNode[T] {
	if _, ok := m[key]; !ok {
		m[key] = NewInitLinkNode[T](val, callback)
		return m[key]
	}
	m[key] = m[key].Append(val)
	return m[key]
}

type linkNodeTMap[T versionedValue] map[T]*LinkNode[T]

func newLinkNodeTMap[T versionedValue]() linkNodeTMap[T] {
	return make(map[T]*LinkNode[T])
}

func (m linkNodeTMap[T]) Append(key T, val VersionedIF[T], callback linkNodeCallback) *LinkNode[T] {
	if _, ok := m[key]; !ok {
		m[key] = NewInitLinkNode[T](val, callback)
		return m[key]
	}
	m[key] = m[key].Append(val)
	return m[key]
}
