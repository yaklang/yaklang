package utils

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/log"
)

// mockBytewiseReader 模拟一个字节一个字节读取的Reader
type mockBytewiseReader struct {
	data []byte
	pos  int
}

func (r *mockBytewiseReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}

	// 每次只读取一个字节，模拟网络传输的情况
	if len(p) > 0 {
		p[0] = r.data[r.pos]
		r.pos++
		return 1, nil
	}
	return 0, nil
}

// mockChunkedReader 模拟按指定块大小读取的Reader
type mockChunkedReader struct {
	data      []byte
	pos       int
	chunkSize int
}

func (r *mockChunkedReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}

	readSize := r.chunkSize
	if readSize > len(p) {
		readSize = len(p)
	}
	if readSize > len(r.data)-r.pos {
		readSize = len(r.data) - r.pos
	}

	copy(p, r.data[r.pos:r.pos+readSize])
	r.pos += readSize
	return readSize, nil
}

func TestUTF8Reader_BasicFunctionality(t *testing.T) {
	testCases := []struct {
		name string
		text string
	}{
		{"ASCII only", "Hello World"},
		{"Mixed ASCII and UTF-8", "Hello 世界"},
		{"Chinese characters", "你好世界"},
		{"Emoji", "Hello 👋 World 🌍"},
		{"Japanese", "こんにちは"},
		{"Korean", "안녕하세요"},
		{"Russian", "Привет мир"},
		{"Arabic", "مرحبا بالعالم"},
		{"Complex emoji", "🇨🇳🏠👨‍👩‍👧‍👦"},
		{"Four-byte UTF-8", "𝕳𝖊𝖑𝖑𝖔"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			log.Infof("Testing case: %s", tc.name)

			// 测试逐字节读取
			mockReader := &mockBytewiseReader{data: []byte(tc.text)}
			utf8Reader := UTF8Reader(mockReader)

			result := make([]byte, 0)
			buf := make([]byte, 8) // 使用8字节缓冲区

			for {
				n, err := utf8Reader.Read(buf)
				if n > 0 {
					result = append(result, buf[:n]...)
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			}

			if string(result) != tc.text {
				t.Errorf("Expected: %s, Got: %s", tc.text, string(result))
			}

			// 验证结果是有效的UTF-8
			if !utf8.Valid(result) {
				t.Errorf("Result is not valid UTF-8: %s", string(result))
			}
		})
	}
}

func TestUTF8Reader_BufferSize1(t *testing.T) {
	text := "Hello 世界 🌍"
	mockReader := &mockBytewiseReader{data: []byte(text)}
	utf8Reader := UTF8Reader(mockReader)

	result := make([]byte, 0)
	buf := make([]byte, 1) // 长度为1的缓冲区，应该失效

	log.Info("Testing buffer size 1 - UTF8Reader should be disabled")

	for {
		n, err := utf8Reader.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	}

	if string(result) != text {
		t.Errorf("Expected: %s, Got: %s", text, string(result))
	}
}

func TestUTF8Reader_SmallBuffer(t *testing.T) {
	text := "你好世界"
	mockReader := &mockBytewiseReader{data: []byte(text)}
	utf8Reader := UTF8Reader(mockReader)

	result := make([]byte, 0)
	buf := make([]byte, 2) // 小于UTF-8字符长度的缓冲区

	log.Info("Testing small buffer - should ensure separated reads")

	for {
		n, err := utf8Reader.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
			log.Infof("Read %d bytes: %v", n, buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	}

	if string(result) != text {
		t.Errorf("Expected: %s, Got: %s", text, string(result))
	}

	// 验证结果是有效的UTF-8
	if !utf8.Valid(result) {
		t.Errorf("Result is not valid UTF-8")
	}
}

func TestUTF8Reader_ChunkedInput(t *testing.T) {
	text := "Hello 世界 🌍 测试"

	// 测试不同的块大小
	chunkSizes := []int{2, 3, 5, 7}

	for _, chunkSize := range chunkSizes {
		t.Run(fmt.Sprintf("ChunkSize_%d", chunkSize), func(t *testing.T) {
			log.Infof("Testing chunk size: %d", chunkSize)

			mockReader := &mockChunkedReader{
				data:      []byte(text),
				chunkSize: chunkSize,
			}
			utf8Reader := UTF8Reader(mockReader)

			result := make([]byte, 0)
			buf := make([]byte, 10)

			for {
				n, err := utf8Reader.Read(buf)
				if n > 0 {
					result = append(result, buf[:n]...)
					log.Infof("Read %d bytes: %s", n, string(buf[:n]))
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			}

			if string(result) != text {
				t.Errorf("Expected: %s, Got: %s", text, string(result))
			}

			// 验证结果是有效的UTF-8
			if !utf8.Valid(result) {
				t.Errorf("Result is not valid UTF-8")
			}
		})
	}
}

func TestUTF8Reader_IncompleteCharacters(t *testing.T) {
	// 测试在UTF-8字符边界被打断的情况
	text := "测试文本"
	data := []byte(text)

	log.Info("Testing incomplete character handling")

	// 创建一个在UTF-8字符中间中断的reader
	mockReader := &mockChunkedReader{
		data:      data,
		chunkSize: 2, // 每次读2字节，会打断3字节的中文字符
	}

	utf8Reader := UTF8Reader(mockReader)
	result := make([]byte, 0)
	buf := make([]byte, 5)

	for {
		n, err := utf8Reader.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
			// 验证每次读取的内容都是有效的UTF-8
			if !utf8.Valid(buf[:n]) {
				t.Errorf("Invalid UTF-8 in chunk: %v", buf[:n])
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	}

	if string(result) != text {
		t.Errorf("Expected: %s, Got: %s", text, string(result))
	}
}

func TestUTF8Reader_EmptyInput(t *testing.T) {
	mockReader := &mockBytewiseReader{data: []byte{}}
	utf8Reader := UTF8Reader(mockReader)

	buf := make([]byte, 10)
	n, err := utf8Reader.Read(buf)

	if n != 0 || err != io.EOF {
		t.Errorf("Expected (0, EOF), got (%d, %v)", n, err)
	}
}

func TestUTF8Reader_LargeText(t *testing.T) {
	// 生成大量包含多字节字符的文本
	text := strings.Repeat("你好世界🌍", 1000)

	log.Info("Testing large text with mixed UTF-8 characters")

	mockReader := &mockBytewiseReader{data: []byte(text)}
	utf8Reader := UTF8Reader(mockReader)

	result := make([]byte, 0)
	buf := make([]byte, 100)

	for {
		n, err := utf8Reader.Read(buf)
		if n > 0 {
			result = append(result, buf[:n]...)
			// 验证每次读取的内容都是有效的UTF-8
			if !utf8.Valid(buf[:n]) {
				t.Errorf("Invalid UTF-8 in chunk")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	}

	if string(result) != text {
		t.Errorf("Large text mismatch")
	}
}

func TestUTF8Reader_CompareWithStandardReader(t *testing.T) {
	text := "Hello 世界 🌍 测试 Привет мир"

	// 使用标准reader读取
	standardReader := bytes.NewReader([]byte(text))
	standardResult, err := io.ReadAll(standardReader)
	if err != nil {
		t.Fatalf("Failed to read with standard reader: %v", err)
	}

	// 使用UTF8Reader读取（模拟字节流）
	mockReader := &mockBytewiseReader{data: []byte(text)}
	utf8Reader := UTF8Reader(mockReader)
	utf8Result, err := io.ReadAll(utf8Reader)
	if err != nil {
		t.Fatalf("Failed to read with UTF8Reader: %v", err)
	}

	// 结果应该相同
	if !bytes.Equal(standardResult, utf8Result) {
		t.Errorf("Results differ: standard=%s, utf8=%s", string(standardResult), string(utf8Result))
	}

	// 验证UTF-8有效性
	if !utf8.Valid(utf8Result) {
		t.Error("UTF8Reader result is not valid UTF-8")
	}
}

func TestUTF8Reader_InternalValidation(t *testing.T) {
	// 测试UTF-8边界检测的基本逻辑
	reader := &utf8Reader{}

	// 测试ASCII字符
	data1 := []byte("Hello")
	boundary1 := reader.findLastValidUTF8Boundary(data1, 10)
	if boundary1 != 5 {
		t.Errorf("Expected boundary 5 for ASCII, got %d", boundary1)
	}

	// 测试中文字符
	data2 := []byte("你好")
	boundary2 := reader.findLastValidUTF8Boundary(data2, 10)
	if boundary2 != 6 { // 两个中文字符，每个3字节
		t.Errorf("Expected boundary 6 for Chinese characters, got %d", boundary2)
	}

	// 测试不完整字符
	data3 := []byte("你")[:2] // 只有中文字符的前2个字节
	boundary3 := reader.findLastValidUTF8Boundary(data3, 10)
	if boundary3 != 0 { // 应该返回0，因为没有完整字符
		t.Errorf("Expected boundary 0 for incomplete character, got %d", boundary3)
	}
}

func TestUTF8PeekableReader_PeekOnePreservesChinese(t *testing.T) {
	text := "中文abc"
	reader := NewUTF8PeekableReader(&mockBytewiseReader{data: []byte(text)})

	peeked, err := reader.Peek(1)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected peek error: %v", err)
	}
	if len(peeked) == 0 {
		t.Fatal("expected at least one peeked byte")
	}

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read from utf8 peekable reader: %v", err)
	}
	if string(got) != text {
		t.Fatalf("expected %q, got %q", text, string(got))
	}
	if !utf8.Valid(got) {
		t.Fatal("utf8 peekable reader output is not valid UTF-8")
	}
}

func TestUTF8PeekableReader_PeekOneThenExpandWindow(t *testing.T) {
	text := "\"<|FACTS_CURRENT_NONCE|>中文内容<|FACTS_END_CURRENT_NONCE|>\""
	reader := NewUTF8PeekableReader(&mockBytewiseReader{data: []byte(text)})

	first, err := reader.Peek(1)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected first peek error: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("expected first peek to read data")
	}

	window, err := reader.Peek(32)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected second peek error: %v", err)
	}
	if !strings.HasPrefix(string(window), "\"<|FACTS_CURRENT_NONCE|>") {
		t.Fatalf("unexpected expanded peek content: %q", string(window))
	}

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read after peek expansion: %v", err)
	}
	if string(got) != text {
		t.Fatalf("expected %q, got %q", text, string(got))
	}
}

func BenchmarkUTF8Reader(b *testing.B) {
	text := strings.Repeat("Hello 世界 🌍 测试", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mockReader := &mockBytewiseReader{data: []byte(text)}
		utf8Reader := UTF8Reader(mockReader)

		buf := make([]byte, 64)
		for {
			_, err := utf8Reader.Read(buf)
			if err == io.EOF {
				break
			}
		}
	}
}

func TestCreateUTF8StreamMirror_RealtimeError(t *testing.T) {
	cb := NewCondBarrier()
	b1 := cb.CreateBarrier("realtime")
	pr, pw := NewPipe()
	go func() {
		defer pw.Close()
		pw.WriteString("a你好")
		cb.Wait("realtime")
		pw.WriteString("b")
		fmt.Println("finished")
	}()

	b2 := cb.CreateBarrier("done")
	mainStream := CreateUTF8StreamMirror(pr, func(reader io.Reader) {
		var buf = make([]byte, 1)
		io.ReadFull(reader, buf)
		if string(buf) != "a" {
			t.Fatal("Expected a, got ", string(buf))
			return
		}
		b1.Done()
		io.ReadAll(reader)
		b2.Done()
	})

	// Must read the main stream to avoid blocking the MultiWriter
	go io.Copy(io.Discard, mainStream)

	cb.Wait("done")
}
