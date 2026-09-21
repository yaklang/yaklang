package binary

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

type countedBinary struct {
	*bytes.Reader
	ctx   context.Context
	reads int
}

func (r *countedBinary) Context() context.Context { return r.ctx }
func (r *countedBinary) ReadAt(b []byte, off int64) (int, error) {
	n, e := r.Reader.ReadAt(b, off)
	r.reads += n
	return n, e
}

func TestBuildInfoReservationBeforeRead(t *testing.T) {
	r := &countedBinary{Reader: bytes.NewReader(make([]byte, 4096)), ctx: budget.Bind(context.Background(), budget.Limits{MaxResultBytes: 48})}
	_, _, err := NewParser().Parse(nil, r)
	if !errors.Is(err, scanerr.ErrResourceLimit) || r.reads != 0 {
		t.Fatalf("read=%d err=%v", r.reads, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.ctx = budget.Ensure(ctx)
	_, _, err = NewParser().Parse(nil, r)
	if !errors.Is(err, context.Canceled) || r.reads != 0 {
		t.Fatalf("cancel read=%d err=%v", r.reads, err)
	}
}

func TestBuildInfoForgedTablesRejectedBeforeStdlib(t *testing.T) {
	elf := make([]byte, 128)
	copy(elf, "\x7fELF")
	elf[4] = 2
	elf[5] = 1
	binary.LittleEndian.PutUint64(elf[40:], 64)
	binary.LittleEndian.PutUint16(elf[58:], 64)
	binary.LittleEndian.PutUint16(elf[60:], 65535)
	pe := make([]byte, 128)
	copy(pe, "MZ")
	binary.LittleEndian.PutUint32(pe[60:], 64)
	copy(pe[64:], "PE\x00\x00")
	binary.LittleEndian.PutUint32(pe[80:], 0xffffffff)
	macho := make([]byte, 64)
	binary.LittleEndian.PutUint32(macho, 0xfeedfacf)
	binary.LittleEndian.PutUint32(macho[16:], 0xffffffff)
	compressed := make([]byte, 192)
	copy(compressed, elf)
	binary.LittleEndian.PutUint16(compressed[60:], 1)
	binary.LittleEndian.PutUint64(compressed[72:], 0x800)
	binary.LittleEndian.PutUint64(compressed[88:], 128)
	binary.LittleEndian.PutUint64(compressed[136:], 1<<60)
	for name, data := range map[string][]byte{"elf": elf, "pe": pe, "macho": macho, "compressed": compressed} {
		t.Run(name, func(t *testing.T) {
			r := &countedBinary{Reader: bytes.NewReader(data), ctx: budget.Bind(context.Background(), budget.Limits{MaxExpressionNodes: 100})}
			_, _, err := NewParser().Parse(nil, r)
			if !errors.Is(err, scanerr.ErrResourceLimit) || r.reads > 192 {
				t.Fatalf("read=%d err=%v", r.reads, err)
			}
		})
	}
}
