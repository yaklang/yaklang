package utils

import (
	"io"
	"strings"
	"testing"
)

func TestBytesStripReader(t *testing.T) {
	// 全量剔除 \n / \r / \t
	got, err := io.ReadAll(NewBytesStripReader(strings.NewReader("a\tb\nc\r\nd\t\te\n"), '\n', '\r', '\t'))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "abcde" {
		t.Fatalf("got %q, want %q", got, "abcde")
	}

	// 下游小缓冲区分块读取，验证流式语义（不依赖一次性读完）
	r := NewBytesStripReader(strings.NewReader("x\ty\nz"), '\n', '\t')
	buf := make([]byte, 2)
	var sb strings.Builder
	for {
		n, err := r.Read(buf)
		sb.Write(buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if sb.String() != "xyz" {
		t.Fatalf("chunked got %q, want %q", sb.String(), "xyz")
	}

	// 整块内容都被剔除时不应提前报 EOF 吞掉后续数据
	r2 := NewBytesStripReader(strings.NewReader("\t\t\tabc"), '\t')
	got2, err := io.ReadAll(r2)
	if err != nil {
		t.Fatal(err)
	}
	if string(got2) != "abc" {
		t.Fatalf("all-stripped chunk got %q, want %q", got2, "abc")
	}

	// 空 chars 等价透传
	got3, err := io.ReadAll(NewBytesStripReader(strings.NewReader("a\tb\nc")))
	if err != nil {
		t.Fatal(err)
	}
	if string(got3) != "a\tb\nc" {
		t.Fatalf("passthrough got %q", got3)
	}
}
