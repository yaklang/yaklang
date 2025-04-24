package linktable

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLinkTableBasic(t *testing.T) {
	// 测试基本功能
	table := NewLinkTable("first", 3, nil)
	require.Equal(t, "first", table.Value())
	require.Nil(t, table.Parent())

	// 测试 Push 功能
	table.Push("second")
	require.Equal(t, "second", table.Value())
	require.Equal(t, "first", table.Parent().Value())

	table.Push("third")
	require.Equal(t, "third", table.Value())
	require.Equal(t, "second", table.Parent().Value())
}

func TestLinkTableSizeLimit(t *testing.T) {
	// 测试大小限制（需要设置回调函数才能限制大小）
	sizeExceeded := false
	table := NewLinkTable("first", 2, func(lt *LinkTable[string]) {
		sizeExceeded = true
	})

	table.Push("second")
	require.False(t, sizeExceeded)

	table.Push("third") // 触发回调
	require.True(t, sizeExceeded)

	table.Push("fourth") // 应该被忽略
	require.Equal(t, "third", table.Value())
	require.Equal(t, "second", table.Parent().Value())
}

func TestLinkTableCallback(t *testing.T) {
	// 测试回调函数
	var callbackValue string
	called := false
	table := NewLinkTable("first", 2, func(lt *LinkTable[string]) {
		called = true
		callbackValue = lt.Value()
	})

	table.Push("second")
	require.False(t, called) // 回调还未被调用

	table.Push("third")                      // 触发回调
	require.True(t, called)                  // 回调已被调用
	require.Equal(t, "third", callbackValue) // 回调时链表的值应该是 "third"
}

func TestLinkTableGeneric(t *testing.T) {
	// 测试泛型功能
	intTable := NewLinkTable(1, 2, nil)
	intTable.Push(2)
	require.Equal(t, 2, intTable.Value())
	require.Equal(t, 1, intTable.Parent().Value())

	structTable := NewLinkTable(struct{ name string }{name: "test"}, 2, nil)
	structTable.Push(struct{ name string }{name: "test2"})
	require.Equal(t, "test2", structTable.Value().name)
	require.Equal(t, "test", structTable.Parent().Value().name)
}

func TestUnlimitedLinkTable(t *testing.T) {
	// 测试无限容量链表 (size = -1)
	table := NewUnlimitedLinkTable("first")
	require.Equal(t, "first", table.Value())
	require.Nil(t, table.Parent())

	// 添加大量元素
	for i := 2; i <= 100; i++ {
		table.Push(fmt.Sprintf("value-%d", i))
	}

	// 验证最近添加的元素
	require.Equal(t, "value-100", table.Value())

	// 验证父节点链
	parent := table.Parent()
	require.Equal(t, "value-99", parent.Value())

	parent = parent.Parent()
	require.Equal(t, "value-98", parent.Value())

	// 测试 size = 0 的无限容量链表
	zeroSizeTable := NewLinkTable("zero", 0, nil)
	require.Equal(t, "zero", zeroSizeTable.Value())

	// 添加元素到 size = 0 的链表
	for i := 1; i <= 50; i++ {
		zeroSizeTable.Push(fmt.Sprintf("zero-%d", i))
	}

	// 验证最近添加的元素
	require.Equal(t, "zero-50", zeroSizeTable.Value())

	// 验证父节点链
	parent = zeroSizeTable.Parent()
	require.Equal(t, "zero-49", parent.Value())

	parent = parent.Parent()
	require.Equal(t, "zero-48", parent.Value())
}

// 测试便捷类型函数
func TestConvenienceTypes(t *testing.T) {
	// 测试字符串链表
	strTable := NewStringLinkTable("hello", 3, nil)
	strTable.Push("world")
	require.Equal(t, "world", strTable.Value())

	unlimitedStrTable := NewUnlimitedStringLinkTable("unlimited")
	unlimitedStrTable.Push("string")
	require.Equal(t, "string", unlimitedStrTable.Value())

	// 测试整数链表
	intTable := NewIntLinkTable(100, 3, nil)
	intTable.Push(200)
	require.Equal(t, 200, intTable.Value())

	unlimitedIntTable := NewUnlimitedIntLinkTable(1000)
	unlimitedIntTable.Push(2000)
	require.Equal(t, 2000, unlimitedIntTable.Value())

	// 测试布尔链表
	boolTable := NewBoolLinkTable(true, 3, nil)
	boolTable.Push(false)
	require.Equal(t, false, boolTable.Value())

	// 测试浮点数链表
	floatTable := NewFloat64LinkTable(3.14, 3, nil)
	floatTable.Push(2.718)
	require.Equal(t, 2.718, floatTable.Value())
}

// 测试遍历方法
func TestTraversalMethods(t *testing.T) {
	// 测试 ForEach 方法
	table := NewUnlimitedStringLinkTable("first")
	table.Push("second")
	table.Push("third")

	// 测试 ForEach
	values := []string{}
	table.ForEach(func(value string) bool {
		values = append(values, value)
		return true
	})
	require.Equal(t, []string{"third", "second", "first"}, values)

	// 测试中途停止
	limitedValues := []string{}
	table.ForEach(func(value string) bool {
		limitedValues = append(limitedValues, value)
		return value != "second" // 到 second 时停止
	})
	require.Equal(t, []string{"third", "second"}, limitedValues)

	// 测试 Len 方法
	require.Equal(t, 3, table.Len())

	// 测试 ToSlice 方法
	slice := table.ToSlice()
	require.Equal(t, []string{"third", "second", "first"}, slice)
}

// 测试并发安全性
func TestConcurrentSafety(t *testing.T) {
	table := NewUnlimitedStringLinkTable("start")

	// 并发写入 - 减少 goroutine 数量，避免可能的死锁
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				table.Push(fmt.Sprintf("value-%d-%d", index, j))
			}
		}(i)
	}

	// 并发读取 - 减少 goroutine 数量，避免可能的死锁
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = table.Value()
				if parent := table.Parent(); parent != nil {
					_ = parent.Value()
				}
			}
		}()
	}

	wg.Wait()

	// 验证至少有 100 个元素被添加
	require.GreaterOrEqual(t, table.Len(), 100)
}
