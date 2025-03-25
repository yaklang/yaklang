package yakvm

import (
	"container/list"
	"context"
	"fmt"
	"reflect"
	"sort"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
	"github.com/yaklang/yaklang/common/yak/yaklib/container"
)

type IteratorType int

const (
	SliceIteratorType IteratorType = iota
	MapIteratorType
	ChannelIteratorType
	RepeatIteratorType
)

type IteratorInterface interface {
	IsEnd() bool
	Next() (data []interface{}, hadEnd bool)
	Type() IteratorType
}

type BaseIterator struct {
	Current, N int
	typ        IteratorType
}

func (i *BaseIterator) IsEnd() bool {
	return i.Current >= i.N
}

func (i *BaseIterator) Type() IteratorType {
	return i.typ
}

func (i *BaseIterator) nextStep() (current int, hadEnd bool) {
	current = i.Current
	hadEnd = i.IsEnd()
	i.Current++
	return current, hadEnd
}

func (i *BaseIterator) Next() (data []interface{}, hadEnd bool) {
	_, hadEnd = i.nextStep()
	return
}

type SliceIterator struct {
	BaseIterator

	p reflect.Value
}

func newSliceIterator(i interface{}) *SliceIterator {
	p := reflect.ValueOf(i)
	kind := p.Kind()
	if kind != reflect.Slice && kind != reflect.Array {
		panic("sliceIterator error: i must be slice or array")
	}
	sliceLen := p.Len()
	if sliceLen == 0 {
		return nil
	}

	return &SliceIterator{
		BaseIterator: BaseIterator{
			Current: 0,
			N:       sliceLen,
			typ:     SliceIteratorType,
		},
		p: p,
	}
}

func (i *SliceIterator) Next() (data []interface{}, hadEnd bool) {
	var current int
	current, hadEnd = i.nextStep()
	if hadEnd {
		data = []interface{}{current, nil}
	} else {
		data = []interface{}{current, i.p.Index(current).Interface()}
	}
	return
}

type MapIterator struct {
	BaseIterator

	p       reflect.Value
	mapKeys []reflect.Value
}

func newMapIterator(i interface{}) *MapIterator {
	p := reflect.ValueOf(i)
	kind := p.Kind()
	if kind != reflect.Map {
		panic("mapIterator error: i must be map")
	}
	mapLen := p.Len()
	if mapLen == 0 {
		return nil
	}

	mapKeys := p.MapKeys()
	sort.SliceStable(mapKeys, func(i, j int) bool {
		return fmt.Sprint(mapKeys[i].Interface()) < fmt.Sprint(mapKeys[j].Interface())
	})

	return &MapIterator{
		BaseIterator: BaseIterator{
			Current: 0,
			N:       p.Len(),
			typ:     MapIteratorType,
		},
		p:       p,
		mapKeys: mapKeys,
	}
}

func (i *MapIterator) Next() (data []interface{}, hadEnd bool) {
	var current int
	current, hadEnd = i.nextStep()
	if hadEnd {
		data = []interface{}{nil, nil}
	} else {
		key := i.mapKeys[current]
		data = []interface{}{key.Interface(), i.p.MapIndex(key).Interface()}
	}
	return
}

type OrderedMapIterator struct {
	BaseIterator

	orderedMap *orderedmap.OrderedMap
	mapKeys    []string
}

func newOrderedMapIterator(i *orderedmap.OrderedMap) *OrderedMapIterator {
	mapLen := i.Len()
	if mapLen == 0 {
		return nil
	}

	return &OrderedMapIterator{
		BaseIterator: BaseIterator{
			Current: 0,
			N:       mapLen,
			typ:     MapIteratorType,
		},
		orderedMap: i,
		mapKeys:    i.Keys(),
	}
}

func (i *OrderedMapIterator) Next() (data []interface{}, hadEnd bool) {
	var current int
	current, hadEnd = i.nextStep()
	if hadEnd {
		data = []interface{}{nil, nil}
	} else {
		key := i.mapKeys[current]
		value, ok := i.orderedMap.Get(key)
		if !ok {
			data = []interface{}{nil, nil}
		} else {
			data = []interface{}{key, value}
		}
	}
	return
}

type ChannelIterator struct {
	BaseIterator

	ctx context.Context
	p   reflect.Value
}

func newChannelIterator(ctx context.Context, i interface{}) *ChannelIterator {
	p := reflect.ValueOf(i)
	kind := p.Kind()
	if kind != reflect.Chan {
		panic("channelIterator error: i must be channel")
	}

	return &ChannelIterator{
		BaseIterator: BaseIterator{
			Current: 0,
			N:       2,
			typ:     ChannelIteratorType,
		},
		ctx: ctx,
		p:   p,
	}
}

func (i *ChannelIterator) Next() (data []interface{}, hadEnd bool) {
	_, hadEnd = i.nextStep()
	chosen, cv, ok := reflect.Select([]reflect.SelectCase{
		{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(i.ctx.Done()),
		}, {
			Dir:  reflect.SelectRecv,
			Chan: i.p,
		},
	})
	if chosen == 0 {
		ok = false
	}

	if ok {
		i.Current = 0
		data = []interface{}{cv.Interface()}
	} else {
		i.N = 2
		hadEnd = true
		data = []interface{}{nil}
	}

	return
}

type RepeatIterator struct {
	BaseIterator
}

func newRepeatIterator(i int64) *RepeatIterator {
	if i == 0 {
		return nil
	}
	return &RepeatIterator{
		BaseIterator: BaseIterator{
			Current: 0,
			N:       int(i),
			typ:     RepeatIteratorType,
		},
	}
}

func (i *RepeatIterator) Next() (data []interface{}, hadEnd bool) {
	var current int
	current, hadEnd = i.nextStep()
	data = []interface{}{current}
	return
}

type SetIterator struct {
	BaseIterator

	iter *mapset.Iterator[any]
}

func newSetIterator(s mapset.Set[any]) *SetIterator {
	setLen := s.Cardinality()
	if setLen == 0 {
		return nil
	}

	return &SetIterator{
		BaseIterator: BaseIterator{
			Current: 0,
			N:       setLen,
			typ:     SliceIteratorType,
		},
		iter: s.Iterator(),
	}
}

func (i *SetIterator) Next() (data []interface{}, hadEnd bool) {
	var current int
	current, hadEnd = i.nextStep()
	if hadEnd {
		data = []interface{}{nil, nil}
	} else {
		v, ok := <-i.iter.C
		if !ok {
			data = []interface{}{nil, nil}
		} else {
			data = []interface{}{current, v}
		}
	}
	return
}

type LinkedListIterator struct {
	BaseIterator

	list    *list.List
	element *list.Element
}

func newLinkedListIterator(l *list.List) *LinkedListIterator {
	return &LinkedListIterator{
		BaseIterator: BaseIterator{
			Current: 0,
			N:       2,
			typ:     SliceIteratorType,
		},
		list: l,
	}
}

func (i *LinkedListIterator) Next() (data []interface{}, hadEnd bool) {
	current, hadEnd := i.nextStep()
	var ele *list.Element
	if i.element == nil {
		i.element = i.list.Front()
	} else {
		i.element = i.element.Next()
	}
	ele = i.element
	if ele != nil {
		i.N++
		data = []interface{}{current, ele.Value}
	} else {
		i.N = i.Current
		hadEnd = true
		data = []interface{}{nil, nil}
	}
	return
}

func NewIterator(ctx context.Context, i interface{}) (IteratorInterface, error) {
	if i == nil {
		return nil, nil
	}

	// OrderedMap iterator
	switch iter := i.(type) {
	case *orderedmap.OrderedMap:
		return newOrderedMapIterator(iter), nil
	case *container.Set:
		return newSetIterator(iter.Set), nil
	case *list.List:
		return newLinkedListIterator(iter), nil
	case *container.LinkedList:
		return newLinkedListIterator(iter.List), nil
	}

	kind := reflect.TypeOf(i).Kind()
	switch kind {
	case reflect.String:
		runes := []rune(i.(string))
		strArr := make([]string, len(runes))
		for i, r := range runes {
			strArr[i] = string(r)
		}
		return newSliceIterator(strArr), nil
	case reflect.Slice, reflect.Array:
		return newSliceIterator(i), nil
	case reflect.Map:
		return newMapIterator(i), nil
	case reflect.Chan:
		return newChannelIterator(ctx, i), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return newRepeatIterator(reflect.ValueOf(i).Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return newRepeatIterator(int64(reflect.ValueOf(i).Uint())), nil
	default:
	}

	return nil, utils.Errorf("is not rangeable")
}
