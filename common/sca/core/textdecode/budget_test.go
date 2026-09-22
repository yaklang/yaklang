package textdecode

import (
	"bytes"
	"context"
	"errors"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"io"
	"testing"
)

type budgetReader struct{ left, read int }

func (r *budgetReader) Read(p []byte) (int, error) {
	if r.left == 0 {
		return 0, io.EOF
	}
	n := min(r.left, len(p))
	for i := 0; i < n; i++ {
		p[i] = 'a'
	}
	r.left -= n
	r.read += n
	return n, nil
}
func TestReadReservesBeforeReading(t *testing.T) {
	for _, size := range []int{1024, 1 << 20, 8 << 20} {
		ctx := budget.Bind(context.Background(), budget.Limits{MaxResultBytes: 64})
		r := &budgetReader{left: size}
		out, err := ReadContext(ctx, r)
		if scanerr.CodeOf(err) != scanerr.ResourceLimit || out != nil || r.read != 0 {
			t.Fatalf("size=%d read=%d output=%d err=%v", size, r.read, len(out), err)
		}
	}
}
func TestReadGrowthBudgetAndBoundaries(t *testing.T) {
	for _, n := range []int{0, 1, 511, 512, 513, 1024, 8193} {
		ctx := budget.Bind(context.Background(), budget.Limits{MaxResultBytes: 1 << 20})
		r := &budgetReader{left: n}
		out, err := ReadRaw(ctx, r, int64(n))
		if err != nil || len(out) != n || !bytes.Equal(out, bytes.Repeat([]byte{'a'}, n)) {
			t.Fatalf("n=%d len=%d err=%v", n, len(out), err)
		}
	}
	r := &budgetReader{left: 8 << 20}
	ctx := budget.Bind(context.Background(), budget.Limits{MaxResultBytes: 600})
	out, err := ReadRaw(ctx, r, 16<<20)
	if scanerr.CodeOf(err) != scanerr.ResourceLimit || out != nil || r.read != 512 {
		t.Fatalf("read=%d err=%v", r.read, err)
	}
	r = &budgetReader{left: 100}
	out, err = ReadRaw(context.Background(), r, 10)
	if scanerr.CodeOf(err) != scanerr.ResourceLimit || out != nil || r.read != 11 {
		t.Fatalf("overread=%d err=%v", r.read, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r = &budgetReader{left: 100}
	if _, err = ReadRaw(ctx, r, 100); !errors.Is(err, context.Canceled) || r.read != 0 {
		t.Fatalf("cancel read=%d err=%v", r.read, err)
	}
}
func TestUTF16OutputReserve(t *testing.T) {
	input := []byte{0xff, 0xfe, 0x2d, 0x4e, 0x87, 0x65}
	ctx := budget.Bind(context.Background(), budget.Limits{MaxResultBytes: 1})
	out, err := BOMContext(ctx, input)
	if out != nil || scanerr.CodeOf(err) != scanerr.ResourceLimit {
		t.Fatalf("out=%q err=%v", out, err)
	}
	out, err = BOMContext(context.Background(), input)
	if err != nil || string(out) != "中文" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestCharsetOutputBudget(t *testing.T) {
	// The source fits the initial 512-byte buffer, but conversion would require
	// a second output buffer. Refuse it before the conversion allocation.
	ctx := budget.Bind(context.Background(), budget.Limits{MaxResultBytes: 540})
	out, err := CharsetReaderContext(ctx, "iso-8859-1", bytes.NewReader([]byte{0xe9}))
	if out != nil || scanerr.CodeOf(err) != scanerr.ResourceLimit {
		t.Fatalf("out=%v err=%v", out, err)
	}
	out, err = CharsetReaderContext(context.Background(), "iso-8859-1", bytes.NewReader([]byte{0xe9}))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(out)
	if err != nil || string(data) != "é" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}
