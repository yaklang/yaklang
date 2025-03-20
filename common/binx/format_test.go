package binx

import (
	"bytes"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
)

func TestFormat(t *testing.T) {
	results, err := BinaryRead(
		bytes.NewBufferString("\x33\x22\x80\xff\xff\x03aaa"),
		NewInt16("ccc"),
		NewUint8("bbb"),
		NewUint16("ddd"),
		NewUint8("eee"),
		NewBuffer("value", "eee"),
	)
	if err != nil {
		t.Fatal(err)
	}
	test := assert.New(t)
	test.Equal(results[0].GetBytes(), []byte("\x33\x22"))
	test.Equal(results[1].GetBytes(), []byte("\x80"))
	test.Equal(results[1].GetBytes(), []byte("\x80"))

	ret := results[0].AsInt16()
	test.Equal(ret, int16(0x3322))
	test.Equal(results[2].Value(), uint16(0xffff))
	test.Equal(results[3].Value(), byte(3))
	test.Equal(results[4].Value(), "aaa")
}

func TestFormat3(t *testing.T) {
	results, err := BinaryRead(
		bytes.NewBufferString("\x33\x22\x80\xff\xff\x03aaa"),
		NewInt16("ccc"),
		NewUint8("bbb"),
		NewUint16("ddd"),
		NewUint8("eee"),
		NewBuffer("value", "eee"),
	)
	if err != nil {
		t.Fatal(err)
	}
	test := assert.New(t)
	test.Equal(results[0].LittleEndian().AsInt16(), int16(0x2233))
	test.Equal(results[1].GetBytes(), []byte("\x80"))
	test.Equal(results[1].GetBytes(), []byte("\x80"))

	ret := results[0].BigEndian().AsInt16()
	test.Equal(ret, int16(0x3322))
	test.Equal(results[2].Value(), uint16(0xffff))
	test.Equal(results[3].Value(), byte(3))
	test.Equal(results[4].Value(), "aaa")
}

func TestFormat2(t *testing.T) {
	va := func(i any) int64 {
		return int64(utils.InterfaceToInt(uint8(199)))
	}
	origin := int8(va(1))
	spew.Dump(origin)
}

func TestListAndStructDescriptor(t *testing.T) {
	t.Run("TestListDescriptor", func(t *testing.T) {
		// 准备测试数据: 2个uint16值 (0x1234 和 0x5678)
		testData := bytes.NewBufferString("\x12\x34\x56\x78")

		// 创建列表描述符 - 包含2个uint16元素
		uint16Desc1 := NewUint16("item1")
		uint16Desc2 := NewUint16("item2")
		listDesc := NewListDescriptor(uint16Desc1, uint16Desc2)

		// 验证列表描述符的属性
		assert.Equal(t, uint64(2), listDesc.SubPartLength)
		assert.Equal(t, 2, len(listDesc.SubPartDescriptor))

		// 使用列表描述符解析数据
		results, err := BinaryRead(testData, listDesc)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(results)) // 应返回1个列表结果

		// 验证列表结果
		listResult, ok := results[0].(*ListResult)
		assert.True(t, ok)
		assert.Equal(t, 2, listResult.Length)

		// 验证列表中的元素值
		assert.Equal(t, uint16(0x1234), listResult.Result[0].Value())
		assert.Equal(t, uint16(0x5678), listResult.Result[1].Value())

		// 测试字节序转换
		assert.Equal(t, uint16(0x3412), listResult.Result[0].LittleEndian().AsUint16())
		assert.Equal(t, uint16(0x1234), listResult.Result[0].BigEndian().AsUint16())
		assert.Equal(t, uint16(0x1234), listResult.Result[0].NetworkByteOrder().AsUint16())

		// 测试通过FindResultByIdentifier查找元素
		foundItem := FindResultByIdentifier(results, "item1")
		assert.NotNil(t, foundItem)
		assert.Equal(t, uint16(0x1234), foundItem.Value())
	})

	t.Run("TestStructDescriptor", func(t *testing.T) {
		// 准备测试数据: uint16(0xABCD) + uint8(0xEF) + uint32(0x12345678)
		testData := bytes.NewBufferString("\xAB\xCD\xEF\x12\x34\x56\x78")

		// 创建结构体描述符 - 包含不同类型的3个字段
		fieldUint16 := NewUint16("field1")
		fieldUint8 := NewUint8("field2")
		fieldUint32 := NewUint32("field3")
		structDesc := NewStructDescriptor(fieldUint16, fieldUint8, fieldUint32)

		// 验证结构体描述符的属性
		assert.Equal(t, uint64(0), structDesc.SubPartLength) // 结构体的SubPartLength应为0
		assert.Equal(t, 3, len(structDesc.SubPartDescriptor))

		// 使用结构体描述符解析数据
		results, err := BinaryRead(testData, structDesc)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(results)) // 应返回1个结构体结果

		// 验证结构体结果
		structResult, ok := results[0].(*StructResult)
		assert.True(t, ok)
		assert.Equal(t, 3, len(structResult.Result))

		// 验证结构体中的字段值
		assert.Equal(t, uint16(0xABCD), structResult.Result[0].Value())
		assert.Equal(t, uint8(0xEF), structResult.Result[1].Value())
		assert.Equal(t, uint32(0x12345678), structResult.Result[2].Value())

		// 测试不同的转换方法
		assert.Equal(t, uint16(0xABCD), structResult.Result[0].AsUint16())
		assert.Equal(t, uint16(0xABCD), structResult.Result[0].AsUint16())

		// 测试字节序
		assert.Equal(t, uint16(0xCDAB), structResult.Result[0].LittleEndian().AsUint16())

		// 测试通过FindResultByIdentifier查找字段
		foundField := FindResultByIdentifier(results, "field2")
		assert.NotNil(t, foundField)
		assert.Equal(t, uint8(0xEF), foundField.Value())
	})

	t.Run("TestNestedStructAndList", func(t *testing.T) {
		// 测试嵌套结构: 结构体中包含列表
		// 数据结构: struct{ header: uint16, items: list[2]{ uint8, uint8 } }
		// 二进制数据: 0xABCD (header) + 0x01, 0x02 (items)
		testData := bytes.NewBufferString("\xAB\xCD\x01\x02")

		// 创建嵌套结构
		itemsList := NewListDescriptor(NewUint8("item1"), NewUint8("item2"))
		nestedStruct := NewStructDescriptor(
			NewUint16("header"),
			itemsList.Name("items"), // 为列表设置名称
		)

		// 解析数据
		results, err := BinaryRead(testData, nestedStruct)
		assert.NoError(t, err)

		// 验证结构体结果
		structResult, ok := results[0].(*StructResult)
		assert.True(t, ok)
		assert.Equal(t, 2, len(structResult.Result))

		// 验证header字段
		assert.Equal(t, uint16(0xABCD), structResult.Result[0].Value())

		// 验证items列表
		itemsResult, ok := structResult.Result[1].(*ListResult)
		assert.True(t, ok)
		assert.Equal(t, 2, itemsResult.Length)
		assert.Equal(t, uint8(0x01), itemsResult.Result[0].Value())
		assert.Equal(t, uint8(0x02), itemsResult.Result[1].Value())

		// 通过FindResultByIdentifier查找嵌套项
		foundItem := FindResultByIdentifier(results, "item2")
		assert.NotNil(t, foundItem)
		assert.Equal(t, uint8(0x02), foundItem.Value())
	})

	t.Run("TestNilSafety", func(t *testing.T) {
		// 测试处理nil值的安全性
		// 这里我们创建一个结构体，但传入的数据不足
		testData := bytes.NewBufferString("\xAB") // 只有一个字节

		// 创建需要更多数据的结构体描述符
		structDesc := NewStructDescriptor(
			NewUint16("field1"), // 需要两个字节
			NewUint32("field2"), // 需要四个字节
		)

		// 尝试解析数据 - 应该返回错误
		_, err := BinaryRead(testData, structDesc)
		assert.Error(t, err) // 期望得到错误
	})
}
