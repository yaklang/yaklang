// Package tools
// @Author bcy2007  2025/5/12 14:54
package tools

type DynamicQueue struct {
	items []string
	check map[string]bool
}

func NewDynamicQueue() *DynamicQueue {
	return &DynamicQueue{
		items: make([]string, 0),
		check: make(map[string]bool),
	}
}

// Enqueue 向队列添加元素
func (q *DynamicQueue) Enqueue(items ...string) {
	q.items = append(q.items, q.PreCheck(items...)...)
}

// Prepend 在队列最前面插入元素
func (q *DynamicQueue) Prepend(pos int, items ...string) {
	// q.items = append(item, q.items...)
	realItems := q.PreCheck(items...)
	result := make([]string, 0, len(q.items)+len(realItems))
	result = append(result, q.items[:pos+1]...)
	result = append(result, realItems...)
	result = append(result, q.items[pos+1:]...)
	q.items = result
}

func (q *DynamicQueue) PreCheck(items ...string) []string {
	var result []string
	for _, item := range items {
		if !q.check[item] {
			result = append(result, item)
			q.check[item] = true
		}
	}
	return result
}

func (q *DynamicQueue) Dequeue() (string, bool) {
	if len(q.items) == 0 {
		return "", false
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item, true
}

func (q *DynamicQueue) Size() int {
	return len(q.items)
}

func (q *DynamicQueue) IsEmpty() bool {
	return len(q.items) == 0
}

// Range 遍历队列，支持在遍历过程中添加新元素
func (q *DynamicQueue) Range(f func(item string, pos int) bool) {
	for i := 0; i < len(q.items); i++ {
		if !f(q.items[i], i) {
			break
		}
	}
}

func (q *DynamicQueue) ToList() []string {
	return q.items
}
