package utils

import (
	"container/list"
	"fmt"
)

type entry struct {
	key   string
	value int
}

type OrderedMap struct {
	dict map[string]*list.Element
	list *list.List
}

func NewOrderedMap() *OrderedMap {
	return &OrderedMap{
		dict: make(map[string]*list.Element),
		list: list.New(),
	}
}

func (om *OrderedMap) Set(key string, value int) {
	// If key is found in the map
	if el, ok := om.dict[key]; ok {
		om.list.Remove(el)
		delete(om.dict, key)
	}

	// Add new value
	el := om.list.PushBack(&entry{key, value})
	om.dict[key] = el
}

func (om *OrderedMap) Get(key string) (int, bool) {
	if el, ok := om.dict[key]; ok {
		return el.Value.(*entry).value, true
	}
	return 0, false
}

func (om *OrderedMap) Delete(key string) {
	if el, ok := om.dict[key]; ok {
		om.list.Remove(el)
		delete(om.dict, key)
	}
}

func (om *OrderedMap) Print() {
	for e := om.list.Front(); e != nil; e = e.Next() {
		fmt.Println(e.Value.(*entry).key, e.Value.(*entry).value)
	}
}
