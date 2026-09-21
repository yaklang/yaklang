package analyzer

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"

	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

// reserveZipDirectory walks the central directory without constructing entries.
// archive/zip allocates its File records during NewReader, so checking zr.File
// afterwards is too late. The count comes from actual records, not the claimed
// count in an untrusted end record. ZIP64 sizes are checked before conversion.
func reserveZipDirectory(ctx context.Context, r io.ReaderAt, size int64) error {
	st := budget.From(ctx)
	bad := func() error { return scanerr.New(scanerr.MalformedInput, "ZIP central directory") }
	if size < 22 {
		return bad()
	}
	n := int(min(size, 22+65535))
	if err := st.Working(int64(n) + 8192); err != nil {
		return err
	}
	tail := make([]byte, n)
	if _, err := r.ReadAt(tail, size-int64(n)); err != nil {
		return err
	}
	end := -1
	for i := len(tail) - 22; i >= 0; i-- {
		if bytes.Equal(tail[i:i+4], []byte{'P', 'K', 5, 6}) && i+22+int(binary.LittleEndian.Uint16(tail[i+20:])) == len(tail) {
			end = i
			break
		}
	}
	if end < 0 {
		return bad()
	}
	e := tail[end:]
	if binary.LittleEndian.Uint16(e[4:]) != 0 || binary.LittleEndian.Uint16(e[6:]) != 0 {
		return bad()
	}
	count := uint64(binary.LittleEndian.Uint16(e[10:]))
	length := uint64(binary.LittleEndian.Uint32(e[12:]))
	endOffset := size - int64(n) + int64(end)
	if count == 65535 || length == 0xffffffff || binary.LittleEndian.Uint32(e[16:]) == 0xffffffff {
		var loc [20]byte
		if endOffset < 20 {
			return bad()
		}
		if _, err := r.ReadAt(loc[:], endOffset-20); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(loc[:]) != 0x07064b50 || binary.LittleEndian.Uint32(loc[4:]) != 0 || binary.LittleEndian.Uint32(loc[16:]) != 1 {
			return bad()
		}
		offset := binary.LittleEndian.Uint64(loc[8:])
		if offset > uint64(endOffset-20) || uint64(endOffset-20)-offset < 56 {
			return bad()
		}
		var z [56]byte
		if _, err := r.ReadAt(z[:], int64(offset)); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(z[:]) != 0x06064b50 || binary.LittleEndian.Uint32(z[16:]) != 0 || binary.LittleEndian.Uint32(z[20:]) != 0 {
			return bad()
		}
		count = binary.LittleEndian.Uint64(z[32:])
		length = binary.LittleEndian.Uint64(z[40:])
		endOffset = int64(offset)
	}
	if length > uint64(endOffset) {
		return bad()
	}
	pos := endOffset - int64(length)
	actual := uint64(0)
	var header [46]byte
	for pos < endOffset {
		if err := ctx.Err(); err != nil {
			return err
		}
		if endOffset-pos < int64(len(header)) {
			return bad()
		}
		if _, err := r.ReadAt(header[:], pos); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(header[:]) != 0x02014b50 {
			return bad()
		}
		variable := int64(binary.LittleEndian.Uint16(header[28:])) + int64(binary.LittleEndian.Uint16(header[30:])) + int64(binary.LittleEndian.Uint16(header[32:]))
		if variable > endOffset-pos-46 {
			return bad()
		}
		if err := st.Archive(1, 0); err != nil {
			return err
		}
		// File/Header objects, backing pointers, name/comment copies, extra
		// fields and parsed Unicode/time metadata. Old growth stays charged.
		if err := st.Working(1024 + 4*variable); err != nil {
			return err
		}
		actual++
		pos += 46 + variable
	}
	if actual != count {
		return bad()
	}
	return nil
}
