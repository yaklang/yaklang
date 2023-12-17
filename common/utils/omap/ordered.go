package omap

import (
	"fmt"
	"github.com/gobwas/glob"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
)

func tryGetParent[V any](v any) *OrderedMap[string, V] {
	switch v.(type) {
	case *OrderedMap[string, V]:
		return v.(*OrderedMap[string, V])
	}
	return nil
}

type OrderedMap[T comparable, V any] struct {
	lock     *sync.RWMutex
	m        map[T]V
	namedKey bool
	keyChain []T

	parent       *OrderedMap[T, V]
	literalValue any
}

func (i *OrderedMap[T, V]) LiteralValue() any {
	return i.literalValue
}

func (i *OrderedMap[T, V]) HaveLiteralValue() bool {
	return i.literalValue != nil
}

func (i *OrderedMap[T, V]) SetLiteralValue(val any) {
	i.literalValue = val
}

func NewEmptyOrderedMap[T comparable, V any]() *OrderedMap[T, V] {
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        make(map[T]V),
		keyChain: make([]T, 0),
	}
}

func NewGeneralOrderedMap() *OrderedMap[string, any] {
	return NewEmptyOrderedMap[string, any]()
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
		namedKey: len(initOrder) > 0,
	}
}

func (o *OrderedMap[T, V]) Add(v V) error {
	o.lock.Lock()
	defer o.lock.Unlock()

	conv := func(i any) (T, bool) {
		res, ok := i.(T)
		if ok {
			return res, true
		}
		var z T
		return z, false
	}

	k := ksuid.New().String()
	val, ok := conv(k)
	if !ok {
		return utils.Errorf("convert failed:  cannot convert %v to %v", k, reflect.TypeOf(val))
	}
	o.m[val] = v
	o.keyChain = append(o.keyChain, val)
	return nil
}

func (o *OrderedMap[T, V]) Set(key T, v V) {
	o.lock.Lock()
	defer o.lock.Unlock()

	_, ok := o.m[key]
	if !ok {
		o.m[key] = v
		o.keyChain = append(o.keyChain, key)
		o.namedKey = true
		return
	}

	// existed
	o.m[key] = v
	o.namedKey = true
}

func (o *OrderedMap[T, V]) Get(key T) (V, bool) {
	o.lock.RLock()
	defer o.lock.RUnlock()

	v, ok := o.m[key]
	return v, ok
}

func (o *OrderedMap[T, V]) GetMust(key T) V {
	o.lock.RLock()
	defer o.lock.RUnlock()

	v, ok := o.m[key]
	if !ok {
		var z V
		return z
	}
	return v
}

func (o *OrderedMap[T, V]) Index(i int) *OrderedMap[string, V] {
	o.lock.RLock()
	defer o.lock.RUnlock()

	if i < 0 || i >= len(o.keyChain) {
		return NewEmptyOrderedMap[string, V]()
	}

	result := NewEmptyOrderedMap[string, V]()
	err := result.Add(o.m[o.keyChain[i]])
	if err != nil {
		log.Errorf("BUG: why? general map type convert failed: %v", err)
	}
	result.parent = tryGetParent[V](o)
	return result
}

func (o *OrderedMap[T, V]) Field(key T) *OrderedMap[string, V] {
	val, ok := o.Get(key)
	if !ok {
		return NewEmptyOrderedMap[string, V]()
	}
	result := BuildGeneralMap[V](val)
	result.parent = tryGetParent[V](o)
	return result
}

func BuildGeneralMap[V any](m any) *OrderedMap[string, V] {
	if m == nil {
		t := NewEmptyOrderedMap[string, V]()
		return t
	}

	ty := reflect.TypeOf(m)
	switch ty.Kind() {
	case reflect.Map:
		vv := reflect.ValueOf(m)
		result := NewEmptyOrderedMap[string, V]()
		for _, key := range vv.MapKeys() {
			value := vv.MapIndex(key).Interface().(V)
			result.Set(utils.InterfaceToString(key.Interface()), value)
		}
		return result
	case reflect.Array, reflect.Slice:
		vv := reflect.ValueOf(m)
		result := NewEmptyOrderedMap[string, V]()
		for i := 0; i < vv.Len(); i++ {
			result.Add(vv.Index(i).Interface().(V))
		}
		return result
	case reflect.Ptr:
		switch ret := m.(type) {
		case *OrderedMap[string, V]:
			return ret
		}
		vv := reflect.ValueOf(m)
		return BuildGeneralMap[V](vv.Elem().Interface())
	case reflect.Struct:
		vv := reflect.ValueOf(m)
		result := NewEmptyOrderedMap[string, V]()
		for i := 0; i < vv.NumField(); i++ {
			result.Set(vv.Type().Field(i).Name, vv.Field(i).Interface().(V))
		}
		return result
	default:
		result := NewEmptyOrderedMap[string, V]()
		result.Set(utils.InterfaceToString(m), m.(V))
		return result
	}
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
				result.Add(vv.Index(i).Interface().(V))
			}
		case reflect.Slice:
			for i := 0; i < vv.Len(); i++ {
				result.Add(vv.Index(i).Interface().(V))
			}
		default:
			result.Add(v)
		}
	}
	result.parent = tryGetParent[V](o)
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

	r := NewEmptyOrderedMap[T, V]()
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
			r.Set(key, v)
		}
	}
	r.parent = o
	return r
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

	var r = NewEmptyOrderedMap[T, V]()
	for _, key := range o.keyChain {
		v, ok := o.m[key]
		if !ok {
			continue
		}
		nk, nv, err := f(key, v)
		if err != nil {
			break
		}
		r.Set(nk, nv)
	}
	r.parent = o
	return r
}

func (o *OrderedMap[T, V]) Flat(f func(T, V) (struct {
	Key   T
	Value V
}, error)) *OrderedMap[T, V] {
	o.lock.Lock()
	defer o.lock.Unlock()

	var r = NewEmptyOrderedMap[T, V]()
	for _, key := range o.keyChain {
		v, ok := o.m[key]
		if !ok {
			continue
		}
		n, err := f(key, v)
		if err != nil {
			break
		}
		r.Set(n.Key, n.Value)
	}
	r.parent = o
	return r
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
		namedKey: s.namedKey,
	}
}

func (s *OrderedMap[T, V]) SearchKey(i ...string) (*OrderedMap[T, V], error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	r := NewEmptyOrderedMap[T, V]()
	for _, key := range s.keyChain {
		v, ok := s.m[key]
		if !ok {
			continue
		}
		for _, j := range i {
			if utils.InterfaceToString(key) == j {
				r.Set(key, v)
				break
			}
		}
	}
	r.parent = s
	return r, nil
}

func (s *OrderedMap[T, V]) SearchValue(i ...string) (*OrderedMap[T, V], error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	r := NewEmptyOrderedMap[T, V]()
	for _, key := range s.keyChain {
		v, ok := s.m[key]
		if !ok {
			continue
		}
		for _, j := range i {
			if utils.InterfaceToString(key) == j {
				r.Set(key, v)
				break
			}
		}
	}
	r.parent = s
	return r, nil
}

func (s *OrderedMap[T, V]) SearchKeyByValue(i ...string) (*OrderedMap[T, V], error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	r := NewEmptyOrderedMap[T, V]()
	for _, key := range s.keyChain {
		v, ok := s.m[key]
		if !ok {
			continue
		}
		for _, j := range i {
			if utils.InterfaceToString(key) == j {
				r.Add(v)
				break
			}
		}
	}
	r.parent = s
	return r, nil
}

func (s *OrderedMap[T, V]) SearchIndexKey(i ...int) (*OrderedMap[T, V], error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	var indexMap = make(map[int]struct{})
	for _, idx := range i {
		indexMap[idx] = struct{}{}
	}

	r := NewEmptyOrderedMap[T, V]()
	for index, key := range s.keyChain {
		v, ok := s.m[key]
		if !ok {
			continue
		}
		if _, ok := indexMap[index]; ok {
			r.Set(key, v)
		}
	}
	r.parent = s
	return r, nil
}

func (s *OrderedMap[T, V]) SearchRegexKey(i string) (*OrderedMap[T, V], error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	rule, err := regexp.Compile(i)
	if err != nil {
		return s, err
	}

	r := NewEmptyOrderedMap[T, V]()
	for _, key := range s.keyChain {
		v, ok := s.m[key]
		if !ok {
			continue
		}
		if rule.MatchString(utils.InterfaceToString(key)) {
			r.Set(key, v)
		}
	}
	r.parent = s
	return r, nil
}

func (s *OrderedMap[T, V]) WalkSearchRegexpKey(i string) (*OrderedMap[T, V], error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	rule, err := regexp.Compile(i)
	if err != nil {
		return s, err
	}

	var m = NewOrderedMap[T, V](map[T]V{})
	Walk(s, func(parent any, key any, value any) bool {
		if rule.MatchString(utils.InterfaceToString(key)) {
			v, ok := value.(V)
			if !ok {
				return true
			}
			m.Add(v)
		}
		return true
	})
	return m, nil
}

func (s *OrderedMap[T, V]) SearchGlobKey(i string, seps ...string) (*OrderedMap[T, V], error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	var sepsChar = []rune(strings.Join(seps, ""))
	rule, err := glob.Compile(i, sepsChar...)
	if err != nil {
		return s, err
	}

	r := NewEmptyOrderedMap[T, V]()
	for _, key := range s.keyChain {
		v, ok := s.m[key]
		if !ok {
			continue
		}
		if rule.Match(utils.InterfaceToString(key)) {
			r.Set(key, v)
		}
	}
	r.parent = s
	return r, nil
}

func (s *OrderedMap[T, V]) WalkSearchGlobKey(i string, seps ...string) (*OrderedMap[T, V], error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	var sepsChar = []rune(strings.Join(seps, ""))
	rule, err := glob.Compile(i, sepsChar...)
	if err != nil {
		return s, err
	}

	var m = NewOrderedMap(map[T]V{})
	Walk(s, func(parent any, key any, value any) bool {
		if rule.Match(utils.InterfaceToString(key)) {
			v, ok := value.(V)
			if !ok {
				return true
			}
			m.Add(v)
		}
		return true
	})
	return m, nil
}

func Merge[T comparable, V any](dicts ...*OrderedMap[T, V]) *OrderedMap[T, V] {
	r := NewEmptyOrderedMap[T, V]()
	for _, d := range dicts {
		for _, key := range d.keyChain {
			v, ok := d.m[key]
			if !ok {
				continue
			}
			r.Set(key, v)
		}
	}
	return r
}

func (s *OrderedMap[T, V]) Merge(i ...*OrderedMap[T, V]) *OrderedMap[T, V] {
	s.lock.RLock()
	defer s.lock.RUnlock()
	r := Merge[T, V](append([]*OrderedMap[T, V]{s}, i...)...)
	r.parent = s
	return r
}

func (s *OrderedMap[T, V]) String() string {
	s.lock.RLock()
	defer s.lock.RUnlock()

	var builder strings.Builder
	builder.WriteString("{")
	for i, k := range s.keyChain {
		builder.WriteString(fmt.Sprintf("%v: %#v", k, s.m[k]))
		if i != len(s.keyChain)-1 {
			builder.WriteString(", ")
		}
	}
	builder.WriteString("}")
	return builder.String()
}

func (s *OrderedMap[T, V]) UnsetParent() {
	if s == nil {
		return
	}
	s.parent = nil
}

func (s *OrderedMap[T, V]) CanAsList() bool {
	return !s.namedKey
}
