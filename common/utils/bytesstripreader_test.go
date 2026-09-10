package utils

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"time"
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
	r2 := NewBytesStripReader(strings.NewReader(strings.Repeat("\t", 8192)+"abc"), '\t')
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

type bytesStripReadFunc func([]byte) (int, error)

func (f bytesStripReadFunc) Read(p []byte) (int, error) { return f(p) }

func TestBytesStripReaderDataAndError(t *testing.T) {
	for _, finalErr := range []error{io.EOF, io.ErrUnexpectedEOF} {
		for _, input := range []string{"a\tb\nc", "\t\n"} {
			t.Run(input+"/"+finalErr.Error(), func(t *testing.T) {
				for _, size := range []int{1, 16} {
					remaining := input
					src := bytesStripReadFunc(func(p []byte) (int, error) {
						if remaining == "" {
							t.Fatal("source read again after returning its terminal error")
						}
						n := copy(p, remaining)
						remaining = remaining[n:]
						if remaining == "" {
							return n, finalErr
						}
						return n, nil
					})
					r := NewBytesStripReader(src, '\t', '\n')
					var got bytes.Buffer
					buf := make([]byte, size)
					for {
						n, err := r.Read(buf)
						got.Write(buf[:n])
						if err != nil {
							if !errors.Is(err, finalErr) {
								t.Fatalf("size %d: got error %v, want %v", size, err, finalErr)
							}
							break
						}
						if n == 0 {
							t.Fatal("unexpected empty read")
						}
					}
					want := strings.NewReplacer("\t", "", "\n", "").Replace(input)
					if got.String() != want {
						t.Fatalf("size %d: got %q, want %q", size, got.String(), want)
					}
				}
			})
		}
	}
}

func TestBytesStripReaderEmptyRead(t *testing.T) {
	r := NewBytesStripReader(bytesStripReadFunc(func([]byte) (int, error) {
		t.Fatal("a zero-length read must not read or block on the source")
		return 0, io.EOF
	}), '\n')
	if n, err := r.Read(nil); n != 0 || err != nil {
		t.Fatalf("got (%d, %v), want (0, nil)", n, err)
	}
}

func TestBytesStripReaderNoProgress(t *testing.T) {
	calls := 0
	r := NewBytesStripReader(bytesStripReadFunc(func(p []byte) (int, error) {
		calls++
		if calls == 1 {
			return 0, nil
		}
		return copy(p, "a\nb"), io.EOF
	}), '\n')
	buf := make([]byte, 8)
	if n, err := r.Read(buf); n != 0 || err != nil || calls != 1 {
		t.Fatalf("empty source read: got (%d, %v), calls=%d", n, err, calls)
	}
	n, err := r.Read(buf)
	if string(buf[:n]) != "ab" || err != io.EOF {
		t.Fatalf("after empty read: got (%q, %v), want (ab, EOF)", buf[:n], err)
	}
}

func TestBytesStripReaderReadContract(t *testing.T) {
	input := []byte{'a', '\n', 0, 'b', '\t', 255, 'c'}
	want := []byte{'a', '\n', 'b', '\t', 'c'}
	for name, wrap := range map[string]func(io.Reader) io.Reader{
		"plain":    func(r io.Reader) io.Reader { return r },
		"one-byte": iotest.OneByteReader,
		"data-eof": iotest.DataErrReader,
	} {
		t.Run(name, func(t *testing.T) {
			r := NewBytesStripReader(wrap(bytes.NewReader(input)), 0, 255)
			if err := iotest.TestReader(r, want); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBytesStripReaderStreamsBeforeEOF(t *testing.T) {
	pr, pw := io.Pipe()
	defer pr.Close()
	defer pw.Close()
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 8)
		n, err := NewBytesStripReader(pr, '\n', '\t').Read(buf)
		if err == nil && string(buf[:n]) != "ab" {
			err = errors.New("filtered prefix differs from ab")
		}
		done <- err
	}()
	_, err := pw.Write([]byte("a\n\tb"))
	if err != nil {
		t.Fatal(err)
	}
	// Keep the writer open: the filtered prefix must be returned immediately.
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reader waited for EOF instead of returning available output")
	}
}
