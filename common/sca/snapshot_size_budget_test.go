package sca

import (
	"context"
	"errors"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"io/fs"
	"testing"
	"testing/fstest"
)

type declaredSizeInfo struct {
	fs.FileInfo
	size int64
}

func (i declaredSizeInfo) Size() int64 { return i.size }

type declaredSizeFS struct {
	fstest.MapFS
	size int64
	read *int
}

func (f declaredSizeFS) Open(name string) (fs.File, error) {
	x, e := f.MapFS.Open(name)
	if e != nil {
		return nil, e
	}
	i, e := x.Stat()
	if e != nil {
		return nil, e
	}
	return &declaredSizeFile{File: x, info: declaredSizeInfo{i, f.size}, read: f.read}, nil
}

type declaredSizeFile struct {
	fs.File
	info fs.FileInfo
	read *int
}

func (f *declaredSizeFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *declaredSizeFile) Read(b []byte) (int, error) {
	n, e := f.File.Read(b)
	*f.read += n
	return n, e
}

func TestSnapshotDeclaredSizeCannotTriggerUnchargedGrowth(t *testing.T) {
	for _, tc := range []struct {
		name     string
		declared int64
		actual   int
		ok       bool
	}{{"growing", 1, 8 << 20, false}, {"zero-growing", 0, 8 << 20, false}, {"short", 2, 1, false}, {"exact", 2, 2, true}, {"empty", 0, 0, true}} {
		t.Run(tc.name, func(t *testing.T) {
			read := 0
			src := declaredSizeFS{fstest.MapFS{"x": {Data: make([]byte, tc.actual)}}, tc.declared, &read}
			l, _ := (budget.Limits{MaxResultBytes: 256}).Normalize()
			ctx := budget.Bind(context.Background(), l)
			f, e := src.Open("x")
			if e != nil {
				t.Fatal(e)
			}
			info, _ := f.Stat()
			f.Close()
			m := materials{source: src, ctx: ctx, files: map[string]*material{"x": {info: info}}, limits: l}
			out, e := m.Open("x")
			if tc.ok {
				if e != nil {
					t.Fatal(e)
				}
				out.Close()
			} else if !errors.Is(e, scanerr.ErrInputChanged) {
				t.Fatalf("wanted input_changed: %v", e)
			}
			if int64(read) > tc.declared+1 {
				t.Fatalf("read %d beyond declared %d + probe", read, tc.declared)
			}
			if budget.From(ctx).ResultBytes() > 256 {
				t.Fatal("exceeded pre-reserved allocation")
			}
		})
	}
}
