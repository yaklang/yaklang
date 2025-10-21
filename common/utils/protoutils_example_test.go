package utils_test

import (
	"bytes"
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
)

func ExampleProtoWriter() {
	var buf bytes.Buffer
	writer := utils.NewProtoWriter(&buf)

	// 写入不同类型的数据
	_ = writer.WriteString("Hello, World!")
	_ = writer.WriteVarint(12345)
	_ = writer.WriteUint32(42)
	_ = writer.WriteFloat32(3.14)
	_ = writer.WriteBool(true)

	fmt.Printf("Written %d bytes\n", buf.Len())
	// Output: Written 32 bytes
}

func ExampleProtoReader() {
	// 准备测试数据
	var buf bytes.Buffer
	writer := utils.NewProtoWriter(&buf)
	_ = writer.WriteString("Hello")
	_ = writer.WriteVarint(100)
	_ = writer.WriteBool(true)

	// 读取数据
	reader := utils.NewProtoReader(&buf)
	str, _ := reader.ReadString()
	num, _ := reader.ReadVarint()
	flag, _ := reader.ReadBool()

	fmt.Printf("String: %s, Number: %d, Bool: %t\n", str, num, flag)
	// Output: String: Hello, Number: 100, Bool: true
}

func ExampleProtoWriter_WriteMagicHeader() {
	var buf bytes.Buffer
	writer := utils.NewProtoWriter(&buf)

	// 写入魔数头（必须是16字节）
	_ = writer.WriteMagicHeader("MYAPP_FORMAT_V1_")

	fmt.Printf("Magic header written: %d bytes\n", buf.Len())
	// Output: Magic header written: 16 bytes
}

func ExampleProtoReader_ReadMagicHeader() {
	// 准备测试数据
	var buf bytes.Buffer
	writer := utils.NewProtoWriter(&buf)
	_ = writer.WriteMagicHeader("MYAPP_FORMAT_V1_")

	// 验证魔数头
	reader := utils.NewProtoReader(&buf)
	err := reader.ReadMagicHeader("MYAPP_FORMAT_V1_")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println("Magic header validated successfully")
	}
	// Output: Magic header validated successfully
}

func ExampleProtoWriter_complete() {
	// 完整的序列化示例
	var buf bytes.Buffer
	writer := utils.NewProtoWriter(&buf)

	// 写入一个简单的数据结构
	type Person struct {
		Name string
		Age  int32
		City string
	}

	person := Person{
		Name: "张三",
		Age:  30,
		City: "北京",
	}

	// 序列化
	_ = writer.WriteString(person.Name)
	_ = writer.WriteInt32(person.Age)
	_ = writer.WriteString(person.City)

	// 反序列化
	reader := utils.NewProtoReader(&buf)
	name, _ := reader.ReadString()
	age, _ := reader.ReadInt32()
	city, _ := reader.ReadString()

	fmt.Printf("Name: %s, Age: %d, City: %s\n", name, age, city)
	// Output: Name: 张三, Age: 30, City: 北京
}
