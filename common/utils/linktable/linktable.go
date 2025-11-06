package linktable

import (
	"fmt"
)

// Node 定义了链表中的一个节点
// 使用泛型 [T comparable]，T 代表任何可比较的类型（如 int, string, struct 等）
type Node[T any] struct {
	Value T
	Next  *Node[T]
}

// LinkedList 定义了链表结构
// 它包含指向头节点、尾节点的指针以及链表的长度
type LinkedList[T any] struct {
	head   *Node[T]
	tail   *Node[T]
	length int
}

// New 创建并返回一个新的空链表
func New[T any]() *LinkedList[T] {
	return &LinkedList[T]{
		head:   nil,
		tail:   nil,
		length: 0,
	}
}

// Len 返回链表的长度
func (l *LinkedList[T]) Len() int {
	return l.length
}

// IsEmpty 检查链表是否为空
func (l *LinkedList[T]) IsEmpty() bool {
	return l.length == 0
}

// PushBack 在链表尾部添加一个新元素
func (l *LinkedList[T]) PushBack(value T) {
	newNode := &Node[T]{Value: value}
	if l.IsEmpty() {
		l.head = newNode
		l.tail = newNode
	} else {
		l.tail.Next = newNode
		l.tail = newNode
	}
	l.length++
}

func (l *LinkedList[T]) Get(index int) (T, bool) {
	// 1. 边界检查：确保索引在有效范围内 [0, length-1]
	if index < 0 || index >= l.length {
		var zero T // 声明一个 T 类型的零值变量 (e.g., 0 for int, "" for string, nil for pointers)
		return zero, false
	}
	// 2. 遍历链表：从头节点开始，向后移动 index 次
	current := l.head
	for i := 0; i < index; i++ {
		current = current.Next
	}

	// 3. 返回找到的节点的值
	return current.Value, true
}

// PushFront 在链表头部添加一个新元素
func (l *LinkedList[T]) PushFront(value T) {
	newNode := &Node[T]{Value: value}
	if l.IsEmpty() {
		l.head = newNode
		l.tail = newNode
	} else {
		newNode.Next = l.head
		l.head = newNode
	}
	l.length++
}

// --- 批量操作 ---

// PushBackBatch 在链表尾部批量添加一组元素
func (l *LinkedList[T]) PushBackBatch(values []T) {
	for _, v := range values {
		l.PushBack(v)
	}
}

// PushFrontBatch 在链表头部批量添加一组元素
// 注意：为了保持传入切片的顺序，我们反向遍历切片进行添加
// 例如：PushFrontBatch([1, 2, 3]) 后，链表顺序为 1 -> 2 -> 3
func (l *LinkedList[T]) PushFrontBatch(values []T) {
	for i := len(values) - 1; i >= 0; i-- {
		l.PushFront(values[i])
	}
}

// ToSlice 将链表转换为一个切片，方便打印和调试
func (l *LinkedList[T]) ToSlice() []T {
	result := make([]T, 0, l.length)
	current := l.head
	for current != nil {
		result = append(result, current.Value)
		current = current.Next
	}
	return result
}

// Print 打印链表内容
func (l *LinkedList[T]) Print() {
	fmt.Printf("List (len=%d): %v\n", l.Len(), l.ToSlice())
}
