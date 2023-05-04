package comparer

import (
	"fmt"
	"net/http"
	"reflect"
)

type Sortable []interface{}

func (a Sortable) Len() int { // 重写 Len() 方法
	return len(a)
}
func (a Sortable) Swap(i, j int) { // 重写 Swap() 方法
	a[i], a[j] = a[j], a[i]
}
func (a Sortable) Less(i, j int) bool { // 重写 Less() 方法， 从大到小排序
	return fmt.Sprintf("%#v", a[i]) < fmt.Sprintf("%#v", a[j])
}

type mapIterItem struct {
	Key   reflect.Value
	Value reflect.Value
}

type MapItemSortable []*mapIterItem

func (a MapItemSortable) Len() int { // 重写 Len() 方法
	return len(a)
}
func (a MapItemSortable) Swap(i, j int) { // 重写 Swap() 方法
	a[i], a[j] = a[j], a[i]
}
func (a MapItemSortable) Less(i, j int) bool { // 重写 Less() 方法， 从大到小排序
	return fmt.Sprintf("%#v", a[i].Key.Interface()) < fmt.Sprintf("%#v", a[j].Key.Interface())
}

type ReflectValueSortable []reflect.Value

func (a ReflectValueSortable) Len() int { // 重写 Len() 方法
	return len(a)
}
func (a ReflectValueSortable) Swap(i, j int) { // 重写 Swap() 方法
	a[i], a[j] = a[j], a[i]
}
func (a ReflectValueSortable) Less(i, j int) bool { // 重写 Less() 方法， 从大到小排序
	return fmt.Sprintf("%#v", a[i].Interface()) < fmt.Sprintf("%#v", a[j].Interface())
}

type CookieSortable []*http.Cookie

func (a CookieSortable) Len() int { // 重写 Len() 方法
	return len(a)
}
func (a CookieSortable) Swap(i, j int) { // 重写 Swap() 方法
	a[i], a[j] = a[j], a[i]
}
func (a CookieSortable) Less(i, j int) bool { // 重写 Less() 方法， 从大到小排序
	return fmt.Sprintf("%#v", a[i].Name) < fmt.Sprintf("%#v", a[j].Name)
}
