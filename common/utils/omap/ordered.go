package omap

import (
	"fmt"
	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/utils"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type OrderedMap[T comparable, V any] struct {
	lock     *sync.RWMutex
	m        map[T]V
	indexMap map[T]int
	keyChain []T

	parent *OrderedMap[T, V]
}

func NewEmptyOrderedMap[T comparable, V any]() *OrderedMap[T, V] {
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        make(map[T]V),
		keyChain: make([]T, 0),
	}
}

func NewOrderedMap[T comparable, V any](m map[T]V, initOrder ...func(int, int) bool) *OrderedMap[T, V] {
	if m == nil {
		return NewEmptyOrderedMap[T, V]()
	}
	k := make([]T, 0)
	for key := range m {
		k = append(k, key)
	}
	for _, s := range initOrder {
		sort.SliceStable(k, s)
	}
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: k,
	}
}

func (o *OrderedMap[T, V]) Set(key T, v V) {
	o.lock.Lock()
	defer o.lock.Unlock()

	_, ok := o.m[key]
	if !ok {
		o.m[key] = v
		o.keyChain = append(o.keyChain, key)
		return
	}

	// existed
	o.m[key] = v
}

func (o *OrderedMap[T, V]) Get(key T) (V, bool) {
	o.lock.RLock()
	defer o.lock.RUnlock()

	v, ok := o.m[key]
	return v, ok
}

func (o *OrderedMap[T, V]) GetByIndex(index int) (V, bool) {
	o.lock.RLock()
	defer o.lock.RUnlock()

	if index < 0 || index >= len(o.keyChain) {
		var z V
		return z, false
	}

	return o.m[o.keyChain[index]], true
}

func (o *OrderedMap[T, V]) First() (T, V, bool) {
	o.lock.RLock()
	defer o.lock.RUnlock()

	if len(o.keyChain) == 0 {
		var z T
		var v V
		return z, v, false
	}

	return o.keyChain[0], o.m[o.keyChain[0]], true
}

func (o *OrderedMap[T, V]) Last() (T, V, bool) {
	o.lock.RLock()
	defer o.lock.RUnlock()

	if len(o.keyChain) == 0 {
		var z T
		var v V
		return z, v, false
	}

	mi := len(o.keyChain) - 1
	return o.keyChain[mi], o.m[o.keyChain[mi]], true
}

func (o *OrderedMap[T, V]) Len() int {
	o.lock.RLock()
	defer o.lock.RUnlock()

	return len(o.keyChain)
}

func (o *OrderedMap[T, V]) Delete(key T) {
	o.lock.Lock()
	defer o.lock.Unlock()

	delete(o.m, key)
	var index = -1
	for i, k := range o.keyChain {
		if k == key {
			index = i
			break
		}
	}
	if index == -1 {
		return
	}

	after := make([]T, len(o.keyChain)-1)
	copy(after, o.keyChain[:index])
	copy(after[index:], o.keyChain[index+1:])
	o.keyChain = after
}

func (o *OrderedMap[T, V]) Keys() []T {
	o.lock.RLock()
	defer o.lock.RUnlock()

	return o.keyChain
}

func (o *OrderedMap[T, V]) Values() []V {
	o.lock.RLock()
	defer o.lock.RUnlock()

	values := make([]V, len(o.keyChain))
	for i, k := range o.keyChain {
		values[i] = o.m[k]
	}
	return values
}

func (o *OrderedMap[T, V]) ValuesMap() *OrderedMap[string, V] {
	o.lock.RLock()
	defer o.lock.RUnlock()

	result := NewEmptyOrderedMap[string, V]()
	for _, v := range o.Values() {
		vv := reflect.ValueOf(v)
		vt := reflect.TypeOf(v)
		switch vt.Kind() {
		case reflect.Map:
			for _, key := range vv.MapKeys() {
				value := vv.MapIndex(key).Interface().(V)
				result.Set(utils.InterfaceToString(key.Interface()), value)
			}
		case reflect.Array:
			for i := 0; i < vv.Len(); i++ {
				result.Set(fmt.Sprint(i), vv.Index(i).Interface().(V))
			}
		case reflect.Slice:
			for i := 0; i < vv.Len(); i++ {
				result.Set(fmt.Sprint(i), vv.Index(i).Interface().(V))
			}
		default:
			key := utils.InterfaceToString(v)
			var count = 0
			var origin = key
		RETRY:
			if _, ok := result.Get(key); ok {
				count++
				key = origin + "_" + fmt.Sprint(count)
				goto RETRY
			}
			result.Set(key, v)
		}
	}
	return result
}

func (o *OrderedMap[T, V]) Have(i any) bool {
	o.lock.RLock()
	defer o.lock.RUnlock()

	switch i.(type) {
	case T:
		_, ok := o.m[i.(T)]
		return ok
	case V:
		for _, v := range o.m {
			if reflect.DeepEqual(i, v) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (o *OrderedMap[T, V]) Filter(f func(T, V) (bool, error)) *OrderedMap[T, V] {
	o.lock.Lock()
	defer o.lock.Unlock()

	m := make(map[T]V)
	k := make([]T, 0)
	for _, key := range o.keyChain {
		v, ok := o.m[key]
		if !ok {
			continue
		}
		ok, err := f(key, v)
		if err != nil {
			break
		}
		if ok {
			m[key] = v
			k = append(k, key)
		}
	}
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: k,
		parent:   o,
	}
}

func (o *OrderedMap[T, V]) GetParent() *OrderedMap[T, V] {
	return o.parent
}

func (o *OrderedMap[T, V]) GetRoot() (*OrderedMap[T, V], bool) {
	if o.parent == nil {
		return o, true
	}
	return o.parent.GetRoot()
}

func (o *OrderedMap[T, V]) Map(f func(T, V) (T, V, error)) *OrderedMap[T, V] {
	o.lock.Lock()
	defer o.lock.Unlock()

	m := make(map[T]V)
	k := make([]T, 0)
	for _, key := range o.keyChain {
		v, ok := o.m[key]
		if !ok {
			continue
		}
		nk, nv, err := f(key, v)
		if err != nil {
			break
		}
		m[nk] = nv
		k = append(k, nk)
	}
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: k,
		parent:   o,
	}
}

func (o *OrderedMap[T, V]) Flat(f func(T, V) (struct {
	Key   T
	Value V
}, error)) *OrderedMap[T, V] {
	o.lock.Lock()
	defer o.lock.Unlock()

	m := make(map[T]V)
	k := make([]T, 0)
	for _, key := range o.keyChain {
		v, ok := o.m[key]
		if !ok {
			continue
		}
		n, err := f(key, v)
		if err != nil {
			break
		}
		m[n.Key] = n.Value
		k = append(k, n.Key)
	}
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: k,
		parent:   o,
	}
}

func (s *OrderedMap[T, V]) Copy() *OrderedMap[T, V] {
	s.lock.RLock()
	defer s.lock.RUnlock()

	m := make(map[T]V)
	for k, v := range s.m {
		m[k] = v
	}
	ks := make([]T, len(s.keyChain))
	copy(ks, s.keyChain)
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: ks,
		parent:   s,
	}
}

func (s *OrderedMap[T, V]) SearchKey(i ...string) (*OrderedMap[T, V], error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	m := make(map[T]V)
	k := make([]T, 0)
	for _, key := range s.keyChain {
		v, ok := s.m[key]
		if !ok {
			continue
		}
		for _, j := range i {
			if utils.InterfaceToString(key) == j {
				m[key] = v
				k = append(k, key)
				break
			}
		}
	}
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: k,
		parent:   s,
	}, nil
}

func (s *OrderedMap[T, V]) SearchValue(i ...string) (*OrderedMap[T, V], error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	m := make(map[T]V)
	k := make([]T, 0)
	for _, key := range s.keyChain {
		v, ok := s.m[key]
		if !ok {
			continue
		}
		for _, j := range i {
			if utils.InterfaceToString(key) == j {
				m[key] = v
				k = append(k, key)
				break
			}
		}
	}
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: k,
		parent:   s,
	}, nil
}

func (s *OrderedMap[T, V]) SearchIndexKey(i ...int) (*OrderedMap[T, V], error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	m := make(map[T]V)
	k := make([]T, 0)
	var indexMap = make(map[int]struct{})
	for _, idx := range i {
		indexMap[idx] = struct{}{}
	}

	for index, key := range s.keyChain {
		v, ok := s.m[key]
		if !ok {
			continue
		}
		if _, ok := indexMap[index]; ok {
			m[key] = v
			k = append(k, key)
		}
	}
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		parent:   s,
		keyChain: k,
	}, nil
}

func (s *OrderedMap[T, V]) SearchRegexKey(i string) (*OrderedMap[T, V], error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	rule, err := regexp.Compile(i)
	if err != nil {
		return s, err
	}

	m := make(map[T]V)
	k := make([]T, 0)
	for _, key := range s.keyChain {
		v, ok := s.m[key]
		if !ok {
			continue
		}
		if rule.MatchString(utils.InterfaceToString(key)) {
			m[key] = v
			k = append(k, key)
		}
	}
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: k,
		parent:   s,
	}, nil
}

func (s *OrderedMap[T, V]) SearchGlobKey(i string, seps ...string) (*OrderedMap[T, V], error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	var sepsChar = []rune(strings.Join(seps, ""))
	rule, err := glob.Compile(i, sepsChar...)
	if err != nil {
		return s, err
	}

	m := make(map[T]V)
	k := make([]T, 0)
	for _, key := range s.keyChain {
		v, ok := s.m[key]
		if !ok {
			continue
		}
		if rule.Match(utils.InterfaceToString(key)) {
			m[key] = v
			k = append(k, key)
		}
	}
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: k,
		parent:   s,
	}, nil
}

func Merge[T comparable, V any](dicts ...*OrderedMap[T, V]) *OrderedMap[T, V] {
	m := make(map[T]V)
	k := make([]T, 0)
	for _, d := range dicts {
		for _, key := range d.keyChain {
			v, ok := d.m[key]
			if !ok {
				continue
			}
			m[key] = v
			k = append(k, key)
		}
	}
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: k,
	}
}

func (s *OrderedMap[T, V]) Merge(i ...*OrderedMap[T, V]) *OrderedMap[T, V] {
	s.lock.RLock()
	defer s.lock.RUnlock()
	r := Merge[T, V](append([]*OrderedMap[T, V]{s}, i...)...)
	r.parent = s
	return r
}
