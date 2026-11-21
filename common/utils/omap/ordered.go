package omap

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func tryGetParent[V any](v any) *OrderedMap[string, V] {
	if p, ok := v.(*OrderedMap[string, V]); ok {
		return p
	}
	return nil
}

type OrderedMap[T comparable, V any] struct {
	initOnce sync.Once
	lock     *sync.RWMutex
	m        map[T]V
	namedKey bool
	keyChain []T

	parent       *OrderedMap[T, V]
	literalValue any
}

func (o *OrderedMap[T, V]) init() {
	if o == nil {
		return
	}
	o.initOnce.Do(func() {
		if o.lock == nil {
			o.lock = new(sync.RWMutex)
		}
		if o.m == nil {
			o.m = make(map[T]V)
		}
		if o.keyChain == nil {
			o.keyChain = make([]T, 0)
		}
	})
}

func (o *OrderedMap[T, V]) snapshotAny() ([]any, []any) {
	if o == nil {
		return nil, nil
	}
	o.init()
	o.lock.RLock()
	keys := make([]any, 0, len(o.keyChain))
	vals := make([]any, 0, len(o.keyChain))
	for _, k := range o.keyChain {
		v, ok := o.m[k]
		if !ok {
			continue
		}
		keys = append(keys, k)
		vals = append(vals, v)
	}
	o.lock.RUnlock()
	return keys, vals
}

func (o *OrderedMap[T, V]) snapshotKeysVals() ([]T, []V) {
	if o == nil {
		return nil, nil
	}
	o.init()
	o.lock.RLock()
	keys := make([]T, 0, len(o.keyChain))
	vals := make([]V, 0, len(o.keyChain))
	for _, k := range o.keyChain {
		v, ok := o.m[k]
		if !ok {
			continue
		}
		keys = append(keys, k)
		vals = append(vals, v)
	}
	o.lock.RUnlock()
	return keys, vals
}

func LiteralValue[T comparable, V any](i *OrderedMap[T, V]) any {
	if i == nil {
		return nil
	}
	return i.literalValue
}

func (i *OrderedMap[T, V]) haveLiteralValue() bool {
	if i == nil {
		return false
	}
	return i.literalValue != nil
}

func SetLiteralValue[T comparable, V any](i *OrderedMap[T, V], val any) {
	if i == nil {
		return
	}
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
	k := make([]T, 0, len(m))
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

var nonnamedkeyconst = `[[-non-named-key-]]`

func (o *OrderedMap[T, V]) Add(v V) error {
	if o == nil {
		return fmt.Errorf("cannot add to a nil OrderedMap")
	}
	o.init()
	k := newNonNamedKey()
	val, ok := any(k).(T)
	if !ok {
		return utils.Errorf("convert failed:  cannot convert %v to %v", k, reflect.TypeOf(val))
	}
	o.lock.Lock()
	o.m[val] = v
	o.keyChain = append(o.keyChain, val)
	o.lock.Unlock()
	return nil
}

func (o *OrderedMap[T, V]) Set(key T, v V) {
	if o == nil {
		return
	}
	o.init()
	o.lock.Lock()
	if _, ok := o.m[key]; !ok {
		o.keyChain = append(o.keyChain, key)
	}
	o.m[key] = v
	o.namedKey = true
	o.lock.Unlock()
}

// OrderInsert inserts (key, v) into the map and places the key in keyChain according to less.
// less(a, b) should return true if a should be ordered before b.
// If key already exists, it will be moved to the new ordered position and its value updated.
// If less is nil, it falls back to Set (append-on-new).
func OrderInsert[T comparable, V any](o *OrderedMap[T, V], key T, v V, less func(a, b T) bool) {
	if o == nil {
		return
	}
	if less == nil {
		o.Set(key, v)
		return
	}
	o.init()
	o.lock.Lock()
	defer o.lock.Unlock()

	// Upsert value first.
	o.m[key] = v
	o.namedKey = true

	// Remove existing key from keyChain if present (to allow repositioning).
	for i, k := range o.keyChain {
		if k == key {
			o.keyChain = append(o.keyChain[:i], o.keyChain[i+1:]...)
			break
		}
	}

	// Find insertion index (stable: insert after existing equals if less is strict).
	insertAt := sort.Search(len(o.keyChain), func(i int) bool {
		return less(key, o.keyChain[i])
	})

	// Insert key into keyChain at insertAt.
	var zero T
	o.keyChain = append(o.keyChain, zero)
	copy(o.keyChain[insertAt+1:], o.keyChain[insertAt:])
	o.keyChain[insertAt] = key
}

func BringKeyToLastOne[T comparable, V any](o *OrderedMap[T, V], target T) {
	if o == nil {
		return
	}
	o.init()
	o.lock.Lock()
	for key, instance := range o.keyChain {
		if instance == target {
			o.keyChain = append(o.keyChain[:key], o.keyChain[key+1:]...)
			break
		}
	}
	o.keyChain = append(o.keyChain, target)
	o.lock.Unlock()
}

func (o *OrderedMap[T, V]) Get(key T) (V, bool) {
	if o == nil {
		var zero V
		return zero, false
	}
	o.init()
	o.lock.RLock()
	v, ok := o.m[key]
	o.lock.RUnlock()
	return v, ok
}

func GetOrSet[T comparable, V any](o *OrderedMap[T, V], key T, value V) V {
	if o == nil {
		return value
	}
	o.init()
	o.lock.Lock()
	existValue, ok := o.m[key]
	if !ok {
		o.m[key] = value
		o.keyChain = append(o.keyChain, key)
		o.namedKey = true
		o.lock.Unlock()
		return value
	}
	o.namedKey = true
	o.lock.Unlock()
	return existValue
}

func GetMust[T comparable, V any](o *OrderedMap[T, V], key T) V {
	if o == nil {
		var zero V
		return zero
	}
	o.init()
	o.lock.RLock()
	v := o.m[key]
	o.lock.RUnlock()
	return v
}

func Index[T comparable, V any](o *OrderedMap[T, V], i int) *OrderedMap[string, V] {
	if o == nil {
		return nil
	}
	o.init()
	o.lock.RLock()
	if i < 0 || i >= len(o.keyChain) {
		o.lock.RUnlock()
		return NewEmptyOrderedMap[string, V]()
	}
	v := o.m[o.keyChain[i]]
	o.lock.RUnlock()

	result := NewEmptyOrderedMap[string, V]()
	if err := result.Add(v); err != nil {
		log.Errorf("BUG: why? general map type convert failed: %v", err)
	}
	result.parent = tryGetParent[V](o)
	return result
}

func Field[T comparable, V any](o *OrderedMap[T, V], key T) *OrderedMap[string, V] {
	if o == nil {
		return nil
	}
	o.init()
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
		return NewEmptyOrderedMap[string, V]()
	}
	if ret, ok := m.(*OrderedMap[string, V]); ok {
		return ret
	}
	result := NewEmptyOrderedMap[string, V]()
	for _, entry := range extractGeneralMapEntries(m) {
		if entry.named {
			result.Set(entry.key, entry.val.(V))
		} else {
			result.Add(entry.val.(V))
		}
	}
	return result
}

func GetByIndex[T comparable, V any](o *OrderedMap[T, V], index int) (V, bool) {
	if o == nil {
		var zero V
		return zero, false
	}
	o.init()
	o.lock.RLock()
	if index < 0 || index >= len(o.keyChain) {
		o.lock.RUnlock()
		var z V
		return z, false
	}
	v := o.m[o.keyChain[index]]
	o.lock.RUnlock()
	return v, true
}

func GetByIndexMust[T comparable, V any](o *OrderedMap[T, V], index int) V {
	if o == nil {
		var zero V
		return zero
	}
	o.init()
	r, _ := GetByIndex(o, index)
	return r
}

func First[T comparable, V any](o *OrderedMap[T, V]) (T, V, bool) {
	if o == nil {
		var zt T
		var zv V
		return zt, zv, false
	}
	o.init()
	o.lock.RLock()
	if len(o.keyChain) == 0 {
		o.lock.RUnlock()
		var z T
		var v V
		return z, v, false
	}
	k := o.keyChain[0]
	v := o.m[k]
	o.lock.RUnlock()
	return k, v, true
}

func Last[T comparable, V any](o *OrderedMap[T, V]) (T, V, bool) {
	if o == nil {
		var zt T
		var zv V
		return zt, zv, false
	}
	o.init()
	o.lock.RLock()
	if len(o.keyChain) == 0 {
		o.lock.RUnlock()
		var z T
		var v V
		return z, v, false
	}
	mi := len(o.keyChain) - 1
	k := o.keyChain[mi]
	v := o.m[k]
	o.lock.RUnlock()
	return k, v, true
}

func (o *OrderedMap[T, V]) Len() int {
	if o == nil {
		return 0
	}
	o.init()
	o.lock.RLock()
	n := len(o.keyChain)
	o.lock.RUnlock()
	return n
}

func (o *OrderedMap[T, V]) Delete(key T) {
	if o == nil {
		return
	}
	o.init()
	o.lock.Lock()
	delete(o.m, key)
	index := -1
	for i, k := range o.keyChain {
		if k == key {
			index = i
			break
		}
	}
	if index != -1 {
		// The DFS-style stack uses Pop/Shift, which delete the last or first key.
		// Truncating either end preserves the old Keys() snapshot because it never
		// writes through the old slice; only a middle delete needs a new slice.
		if index == len(o.keyChain)-1 {
			o.keyChain = o.keyChain[:index:index]
		} else if index == 0 {
			o.keyChain = o.keyChain[1:len(o.keyChain):len(o.keyChain)]
		} else {
			after := make([]T, len(o.keyChain)-1)
			copy(after, o.keyChain[:index])
			copy(after[index:], o.keyChain[index+1:])
			o.keyChain = after
		}
	}
	o.lock.Unlock()
}

func (o *OrderedMap[T, V]) Keys() []T {
	if o == nil {
		return nil
	}
	o.init()
	o.lock.RLock()
	ks := o.keyChain
	o.lock.RUnlock()
	return ks
}

func (o *OrderedMap[T, V]) Values() []V {
	if o == nil {
		return nil
	}
	o.init()
	o.lock.RLock()
	values := make([]V, len(o.keyChain))
	for i, k := range o.keyChain {
		values[i] = o.m[k]
	}
	o.lock.RUnlock()
	return values
}

func ValuesMap[T comparable, V any](o *OrderedMap[T, V]) *OrderedMap[string, V] {
	if o == nil {
		return nil
	}
	_, vals := o.snapshotAny()
	result := NewEmptyOrderedMap[string, V]()
	for _, entry := range flattenValuesForMap(vals) {
		if entry.named {
			result.Set(entry.key, entry.val.(V))
		} else {
			result.Add(entry.val.(V))
		}
	}
	result.parent = tryGetParent[V](o)
	return result
}

func (o *OrderedMap[T, V]) Have(i any) bool {
	if o == nil {
		return false
	}
	o.init()
	switch key := i.(type) {
	case T:
		o.lock.RLock()
		_, ok := o.m[key]
		o.lock.RUnlock()
		return ok
	case V:
		_, vals := o.snapshotAny()
		return containsDeepEqual(vals, i)
	default:
		return false
	}
}

func Filter[T comparable, V any](o *OrderedMap[T, V], f func(T, V) (bool, error)) *OrderedMap[T, V] {
	if o == nil {
		return nil
	}
	o.init()
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
			if sk, ok := any(key).(string); ok && strings.HasSuffix(sk, nonnamedkeyconst) {
				r.Add(v)
			} else {
				r.Set(key, v)
			}
		}
	}
	r.parent = o
	return r
}

func GetParent[T comparable, V any](o *OrderedMap[T, V]) *OrderedMap[T, V] {
	if o == nil {
		return nil
	}
	o.init()
	return o.parent
}

func GetRoot[T comparable, V any](o *OrderedMap[T, V]) (*OrderedMap[T, V], bool) {
	if o == nil {
		return nil, false
	}
	o.init()
	if o.parent == nil {
		return o, true
	}
	return GetRoot(o.parent)
}

func GetMap[T comparable, V any](o *OrderedMap[T, V]) map[T]V {
	if o == nil {
		return nil
	}
	o.init()
	return o.m
}

// SnapshotMap returns a copy of the map. Use it when the caller iterates the
// result while other goroutines may add entries: GetMap exposes the internal
// map and ranging it during a Set is a concurrent map read and write.
func SnapshotMap[T comparable, V any](o *OrderedMap[T, V]) map[T]V {
	if o == nil {
		return nil
	}
	o.init()
	o.lock.RLock()
	snapshot := make(map[T]V, len(o.m))
	for key, value := range o.m {
		snapshot[key] = value
	}
	o.lock.RUnlock()
	return snapshot
}

func Map[T comparable, V any](o *OrderedMap[T, V], f func(T, V) (T, V, error)) *OrderedMap[T, V] {
	if o == nil {
		return nil
	}
	o.init()
	o.lock.Lock()
	defer o.lock.Unlock()

	r := NewEmptyOrderedMap[T, V]()
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

func (o *OrderedMap[T, V]) ForEach(handler func(i T, v V) bool) {
	if o == nil {
		return
	}
	o.init()
	o.lock.RLock()
	keys := append([]T(nil), o.keyChain...)
	values := make([]V, len(keys))
	for i, key := range keys {
		values[i] = o.m[key]
	}
	o.lock.RUnlock()

	for i, key := range keys {
		if !handler(key, values[i]) {
			break
		}
	}
}

func Flat[T comparable, V any](o *OrderedMap[T, V], f func(T, V) (struct {
	Key   T
	Value V
}, error)) *OrderedMap[T, V] {
	if o == nil {
		return nil
	}
	o.init()
	o.lock.Lock()
	defer o.lock.Unlock()

	r := NewEmptyOrderedMap[T, V]()
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
	if s == nil {
		return nil
	}
	s.init()
	s.lock.RLock()
	m := make(map[T]V, len(s.m))
	for k, v := range s.m {
		m[k] = v
	}
	ks := make([]T, len(s.keyChain))
	copy(ks, s.keyChain)
	namedKey := s.namedKey
	s.lock.RUnlock()

	newMap := &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: ks,
		parent:   nil,
		namedKey: namedKey,
	}
	newMap.initOnce.Do(func() {})
	return newMap
}

func (s *OrderedMap[T, V]) buildMatched(indices []int, addOnly bool) *OrderedMap[T, V] {
	keys, vals := s.snapshotKeysVals()
	r := NewEmptyOrderedMap[T, V]()
	for _, idx := range indices {
		if idx < 0 || idx >= len(keys) {
			continue
		}
		if addOnly {
			r.Add(vals[idx])
		} else {
			r.Set(keys[idx], vals[idx])
		}
	}
	r.parent = s
	return r
}

func SearchKey[T comparable, V any](s *OrderedMap[T, V], i ...string) (*OrderedMap[T, V], error) {
	if s == nil {
		return nil, nil
	}
	keys, _ := s.snapshotAny()
	return s.buildMatched(matchExactStrings(keys, i), false), nil
}

func SearchValue[T comparable, V any](s *OrderedMap[T, V], i ...string) (*OrderedMap[T, V], error) {
	return SearchKey(s, i...)
}

func SearchKeyByValue[T comparable, V any](s *OrderedMap[T, V], i ...string) (*OrderedMap[T, V], error) {
	if s == nil {
		return nil, nil
	}
	keys, _ := s.snapshotAny()
	return s.buildMatched(matchExactStrings(keys, i), true), nil
}

func SearchIndexKey[T comparable, V any](s *OrderedMap[T, V], i ...int) (*OrderedMap[T, V], error) {
	if s == nil {
		return nil, nil
	}
	return s.buildMatched(matchIndexSet(s.Len(), i), false), nil
}

func SearchRegexKey[T comparable, V any](s *OrderedMap[T, V], i string) (*OrderedMap[T, V], error) {
	if s == nil {
		return nil, nil
	}
	keys, _ := s.snapshotAny()
	matched, err := matchRegexKeys(keys, i)
	if err != nil {
		return s, err
	}
	return s.buildMatched(matched, false), nil
}

func WalkSearchRegexpKey[T comparable, V any](s *OrderedMap[T, V], i string) (*OrderedMap[T, V], error) {
	if s == nil {
		return nil, nil
	}
	matched, err := walkMatchRegex(s, i)
	if err != nil {
		return s, err
	}
	m := NewOrderedMap[T, V](map[T]V{})
	for _, raw := range matched {
		if v, ok := raw.(V); ok {
			m.Add(v)
		}
	}
	return m, nil
}

func SearchGlobKey[T comparable, V any](s *OrderedMap[T, V], i string, seps ...string) (*OrderedMap[T, V], error) {
	if s == nil {
		return nil, nil
	}
	keys, _ := s.snapshotAny()
	matched, err := matchGlobKeys(keys, i, seps)
	if err != nil {
		return s, err
	}
	return s.buildMatched(matched, false), nil
}

func WalkSearchGlobKey[T comparable, V any](s *OrderedMap[T, V], i string, seps ...string) (*OrderedMap[T, V], error) {
	if s == nil {
		return nil, nil
	}
	matched, err := walkMatchGlob(s, i, seps)
	if err != nil {
		return s, err
	}
	m := NewOrderedMap(map[T]V{})
	for _, raw := range matched {
		if v, ok := raw.(V); ok {
			m.Add(v)
		}
	}
	return m, nil
}

func Merge[T comparable, V any](dicts ...*OrderedMap[T, V]) *OrderedMap[T, V] {
	r := NewEmptyOrderedMap[T, V]()
	for _, d := range dicts {
		keys, vals := d.snapshotKeysVals()
		for i, key := range keys {
			r.Set(key, vals[i])
		}
	}
	return r
}

func (s *OrderedMap[T, V]) String() string {
	if s == nil {
		return "<nil>"
	}
	keys, vals := s.snapshotAny()
	return formatOrderedMapString(keys, vals)
}

func (s *OrderedMap[T, V]) canAsList() bool {
	if s == nil {
		return true
	}
	return !s.namedKey
}

func Push[T comparable, V any](s *OrderedMap[T, V], a V) error {
	if s == nil {
		return fmt.Errorf("cannot push to a nil OrderedMap")
	}
	return s.Add(a)
}

func Pop[T comparable, V any](s *OrderedMap[T, V]) V {
	if s == nil {
		var zero V
		return zero
	}
	key, v, ok := Last(s)
	if ok {
		s.Delete(key)
		return v
	}
	var ret V
	return ret
}

func Shift[T comparable, V any](s *OrderedMap[T, V]) V {
	if s == nil {
		var zero V
		return zero
	}
	key, v, ok := First(s)
	if ok {
		s.Delete(key)
		return v
	}
	var ret V
	return ret
}
