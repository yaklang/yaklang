package linktable

import (
	"sync"
)

type LinkTable[T any] struct {
	parent         *LinkTable[T]
	value          T
	size           int
	onSizeExceeded func(*LinkTable[T])
	mutex          sync.RWMutex // 用于并发安全
}

func NewLinkTable[T any](value T, size int, onSizeExceeded func(*LinkTable[T])) *LinkTable[T] {
	return &LinkTable[T]{
		parent:         nil,
		value:          value,
		size:           size,
		onSizeExceeded: onSizeExceeded,
		mutex:          sync.RWMutex{},
	}
}

// NewUnlimitedLinkTable 创建一个无限容量的链表
func NewUnlimitedLinkTable[T any](value T) *LinkTable[T] {
	return &LinkTable[T]{
		parent:         nil,
		value:          value,
		size:           -1, // 使用负数或零表示无限容量
		onSizeExceeded: nil,
		mutex:          sync.RWMutex{},
	}
}

// 便捷函数 - 字符串链表
func NewStringLinkTable(value string, size int, onSizeExceeded func(*LinkTable[string])) *LinkTable[string] {
	return NewLinkTable(value, size, onSizeExceeded)
}

func NewUnlimitedStringLinkTable(value string) *LinkTable[string] {
	return NewUnlimitedLinkTable(value)
}

// 便捷函数 - 整数链表
func NewIntLinkTable(value int, size int, onSizeExceeded func(*LinkTable[int])) *LinkTable[int] {
	return NewLinkTable(value, size, onSizeExceeded)
}

func NewUnlimitedIntLinkTable(value int) *LinkTable[int] {
	return NewUnlimitedLinkTable(value)
}

// 便捷函数 - 布尔链表
func NewBoolLinkTable(value bool, size int, onSizeExceeded func(*LinkTable[bool])) *LinkTable[bool] {
	return NewLinkTable(value, size, onSizeExceeded)
}

func NewUnlimitedBoolLinkTable(value bool) *LinkTable[bool] {
	return NewUnlimitedLinkTable(value)
}

// 便捷函数 - 浮点数链表
func NewFloat64LinkTable(value float64, size int, onSizeExceeded func(*LinkTable[float64])) *LinkTable[float64] {
	return NewLinkTable(value, size, onSizeExceeded)
}

func NewUnlimitedFloat64LinkTable(value float64) *LinkTable[float64] {
	return NewUnlimitedLinkTable(value)
}

func (l *LinkTable[T]) Push(value T) {
	// 如果链表容量已达上限并且不是无限容量，则不允许添加
	if l.size == 0 && l.onSizeExceeded != nil {
		// 只有设置了回调函数的才视为有限容量
		return
	}

	// 先获取读锁，检查是否需要触发回调
	l.mutex.RLock()
	needCallback := l.size == 1 && l.onSizeExceeded != nil
	callback := l.onSizeExceeded // 保存回调函数的引用
	l.mutex.RUnlock()

	// 再获取写锁，修改链表结构
	l.mutex.Lock()

	oldValue := l.value
	oldParent := l.parent

	// 新的父节点的大小处理：
	// 1. 对于无限容量链表 (size <= 0 或没有回调)，保持大小不变
	// 2. 对于有限容量链表，减少大小
	var newParentSize int
	if l.size <= 0 || l.onSizeExceeded == nil {
		// 无限容量链表或没有回调的链表
		newParentSize = l.size
	} else {
		// 有限容量链表
		newParentSize = l.size - 1
	}

	// 更新当前节点的值和父节点
	l.value = value
	l.parent = &LinkTable[T]{
		value:          oldValue,
		parent:         oldParent,
		size:           newParentSize,
		onSizeExceeded: l.onSizeExceeded,
		mutex:          sync.RWMutex{},
	}

	// 只有在有限容量链表时才减少 size
	if l.size > 0 && l.onSizeExceeded != nil {
		l.size--
	}

	// 在写锁内创建一个副本，用于回调
	var callbackCopy *LinkTable[T]
	if needCallback {
		callbackCopy = &LinkTable[T]{
			value:  value,
			parent: l.parent,
			size:   l.size,
		}
	}

	// 释放写锁
	l.mutex.Unlock()

	// 在锁外调用回调函数，避免死锁
	if needCallback && callback != nil {
		callback(callbackCopy)
	}
}

func (l *LinkTable[T]) Value() T {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.value
}

func (l *LinkTable[T]) Parent() *LinkTable[T] {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.parent
}

// 添加遍历全部节点的便捷方法
func (l *LinkTable[T]) ForEach(fn func(T) bool) {
	current := l
	for current != nil {
		current.mutex.RLock()
		value := current.value
		nextNode := current.parent
		current.mutex.RUnlock()

		if !fn(value) {
			break
		}

		current = nextNode
	}
}

// 获取链表长度
func (l *LinkTable[T]) Len() int {
	count := 0
	l.ForEach(func(T) bool {
		count++
		return true
	})
	return count
}

// 获取所有值的切片
func (l *LinkTable[T]) ToSlice() []T {
	var result []T
	l.ForEach(func(value T) bool {
		result = append(result, value)
		return true
	})
	return result
}
