package algorithm

import "container/list"

func BFS[T any](first []T, next func(T) []T, handle func(T) bool) {
	workList := list.New()
	for _, item := range first {
		workList.PushBack(item)
	}
	for {
		if workList.Len() == 0 {
			break
		}
		elem := workList.Back()
		item := elem.Value.(T)
		workList.Remove(elem)
		if handle(item) {
			for _, nextItem := range next(item) {
				workList.PushBack(nextItem)
			}
		}
	}
}
