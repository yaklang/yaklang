package utils

import (
	"fmt"
	"io"
	"strings"
)

func ExampleJSONStringReader_basicUsage() {
	// 基本使用示例
	input := `"123"`
	reader := JSONStringReader(strings.NewReader(input))
	result, _ := io.ReadAll(reader)
	fmt.Printf("输入: %s\n输出: %s\n", input, string(result))
	// Output:
	// 输入: "123"
	// 输出: 123
}

func ExampleJSONStringReader_escapeSequences() {
	// 转义序列示例
	input := `"abc\n123"`
	reader := JSONStringReader(strings.NewReader(input))
	result, _ := io.ReadAll(reader)
	fmt.Printf("输入: %s\n输出: %s\n", input, string(result))
	// Output:
	// 输入: "abc\n123"
	// 输出: abc
	// 123
}

func ExampleJSONStringReader_complexEscapes() {
	// 复杂转义示例
	input := `"Hello\x20World\u0021\ud83d\ude0a"`
	reader := JSONStringReader(strings.NewReader(input))
	result, _ := io.ReadAll(reader)
	fmt.Printf("输入: %s\n输出: %s\n", input, string(result))
	// Output:
	// 输入: "Hello\x20World\u0021\ud83d\ude0a"
	// 输出: Hello World!😊
}

func ExampleJSONStringReader_fallback() {
	// 回退模式示例
	input := `123`
	reader := JSONStringReader(strings.NewReader(input))
	result, _ := io.ReadAll(reader)
	fmt.Printf("输入: %s\n输出: %s\n", input, string(result))
	// Output:
	// 输入: 123
	// 输出: 123
}

func ExampleJSONStringReader_streaming() {
	// 流式读取示例
	input := `"This is a long string that demonstrates streaming capabilities"`
	reader := JSONStringReader(strings.NewReader(input))

	// 使用小缓冲区分块读取
	buf := make([]byte, 10)
	var output strings.Builder

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			output.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
	}

	fmt.Printf("流式读取输出: %s\n", output.String())
	// Output:
	// 流式读取输出: This is a long string that demonstrates streaming capabilities
}

func ExampleJSONStringReader_malformed() {
	// 畸形数据自动回退示例
	input := `"hello"world"test"`
	reader := JSONStringReader(strings.NewReader(input))
	result, _ := io.ReadAll(reader)
	fmt.Printf("输入: %s\n输出: %s\n", input, string(result))
	// Output:
	// 输入: "hello"world"test"
	// 输出: "hello"world"test"
}
