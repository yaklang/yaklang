package utils

import (
	"fmt"
	"io"
)

// ExampleUTF8Reader 演示UTF8Reader的基本用法
func ExampleUTF8Reader() {
	// 模拟一个逐字节读取的Reader
	text := "Hello 世界 🌍"
	reader := &mockBytewiseReader{data: []byte(text)}

	// 使用UTF8Reader包装
	utf8Reader := UTF8Reader(reader)

	// 读取数据
	buf := make([]byte, 5)
	for {
		n, err := utf8Reader.Read(buf)
		if n > 0 {
			fmt.Printf("Read: %s\n", string(buf[:n]))
		}
		if err == io.EOF {
			break
		}
	}

	// Output:
	// Read: Hello
	// Read:  世
	// Read: 界
	// Read: 🌍
}

// ExampleUTF8Reader_smallBuffer 演示小缓冲区的行为
func ExampleUTF8Reader_smallBuffer() {
	text := "你好"
	reader := &mockBytewiseReader{data: []byte(text)}
	utf8Reader := UTF8Reader(reader)

	// 使用2字节缓冲区（小于UTF-8字符长度）
	buf := make([]byte, 2)

	fmt.Println("Small buffer (2 bytes) behavior:")
	for {
		n, err := utf8Reader.Read(buf)
		if n > 0 {
			fmt.Printf("Read %d bytes: %v\n", n, buf[:n])
		}
		if err == io.EOF {
			break
		}
	}

	// Output:
	// Small buffer (2 bytes) behavior:
	// Read 1 byte: [228]
	// Read 1 byte: [189]
	// Read 1 byte: [160]
	// Read 1 byte: [229]
	// Read 1 byte: [165]
	// Read 1 byte: [189]
}

// ExampleUTF8Reader_bufferSize1 演示缓冲区长度为1时失效的行为
func ExampleUTF8Reader_bufferSize1() {
	text := "测试"
	reader := &mockBytewiseReader{data: []byte(text)}
	utf8Reader := UTF8Reader(reader)

	// 使用1字节缓冲区（UTF8Reader失效）
	buf := make([]byte, 1)

	fmt.Println("Buffer size 1 (UTF8Reader disabled):")
	var result []byte
	for {
		n, err := utf8Reader.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
	}

	fmt.Printf("Final result: %s\n", string(result))

	// Output:
	// Buffer size 1 (UTF8Reader disabled):
	// Final result: 测试
}

// ExampleUTF8Reader_realWorldUsage 展示实际使用场景
func ExampleUTF8Reader_realWorldUsage() {
	// 模拟网络数据流，可能会在UTF-8字符边界处中断
	data := "API返回: {\"message\": \"你好，世界！🌍\"}"

	// 创建一个按小块读取的reader来模拟网络传输
	chunkedReader := &mockChunkedReader{
		data:      []byte(data),
		chunkSize: 3, // 每次只读3字节，会打断UTF-8字符
	}

	// 使用UTF8Reader确保读取完整的UTF-8字符
	utf8Reader := UTF8Reader(chunkedReader)

	result, err := io.ReadAll(utf8Reader)
	if err != nil {
		panic(err)
	}

	fmt.Printf("完整读取: %s\n", string(result))

	// Output:
	// 完整读取: API返回: {"message": "你好，世界！🌍"}
}
