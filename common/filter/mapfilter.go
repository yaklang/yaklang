package filter

import "sync"

type MapFilter struct {
	sync.Mutex
	m map[string]struct{}
}

func NewMapFilter() Filterable {
	return &MapFilter{
		m: make(map[string]struct{}),
	}
}

func (f *MapFilter) Exist(str string) bool {
	f.Lock()
	defer f.Unlock()

	_, ok := f.m[str]
	return ok
}

func (f *MapFilter) Insert(str string) bool {
	f.Lock()
	defer f.Unlock()

	_, ok := f.m[str]
	if !ok {
		f.m[str] = struct{}{}
	}
	return !ok
}

func (f *MapFilter) Close() {
	// 在这个例子中，Close 方法可能不需要做任何事情，因为 Go 的垃圾收集器会自动回收不再使用的内存。
	// 如果你有需要在关闭 MapFilter 时做的清理工作，你可以在这里添加。
}

func (f *MapFilter) Clear() {
	f.Lock()
	defer f.Unlock()

	f.m = make(map[string]struct{})
}
