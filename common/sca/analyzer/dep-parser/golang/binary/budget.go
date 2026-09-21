package binary

import (
	"context"
	"encoding/binary"
	"io"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
)

// reserveBuildInfo bounds the stdlib executable readers before they allocate
// headers, name tables or module records. It does not parse executable code.
// Declared name tables can be referenced repeatedly, so their amplification
// is charged separately from file bytes. Compressed ELF table sizes are read
// from their fixed compression header before debug/elf can decompress them.
func reserveBuildInfo(ctx context.Context, r types.ReadSeekerAt) error {
	st := budget.From(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := st.Working(1024); err != nil {
		return err
	}
	pos, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	size, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if _, err = r.Seek(pos, io.SeekStart); err != nil {
		return err
	}
	if size < 0 || size > st.Limits.MaxFileBytes {
		return scanerr.New(scanerr.ResourceLimit, "binary file bytes")
	}
	// Go 1.22 saferio uses 10MiB chunks even for forged counts. This fixed
	// allowance covers simultaneously live initial chunks; proportional
	// bytes cover successful table growth, strings and ParseBuildInfo records.
	need, err := budget.SizeMul(int(size), 16)
	if err != nil {
		return err
	}
	if err = st.Working(32<<20 + need); err != nil {
		return err
	}
	read := func(dst []byte, off uint64) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if off > uint64(size) || uint64(len(dst)) > uint64(size)-off {
			return ErrUnrecognizedExe
		}
		_, err := r.ReadAt(dst, int64(off))
		return err
	}
	charge := func(count, unit uint64) error {
		if count > uint64(st.Limits.MaxExpressionNodes) || unit > uint64(^uint64(0)>>1) {
			return scanerr.New(scanerr.ResourceLimit, "binary table dimensions")
		}
		n, err := budget.SizeMul(int(count), int64(unit))
		if err != nil {
			return err
		}
		return st.Working(n)
	}
	var h [64]byte
	if size < 16 {
		return ErrUnrecognizedExe
	}
	if err = read(h[:16], 0); err != nil {
		return err
	}
	switch {
	case string(h[:4]) == "\x7fELF":
		if err = read(h[:], 0); err != nil {
			return err
		}
		var order binary.ByteOrder
		switch h[5] {
		case 1:
			order = binary.LittleEndian
		case 2:
			order = binary.BigEndian
		default:
			return ErrUnrecognizedExe
		}
		var shoff uint64
		var shnum, shent, stridx, phnum uint64
		wide := h[4] == 2
		if wide {
			shoff = order.Uint64(h[40:])
			shent = uint64(order.Uint16(h[58:]))
			shnum = uint64(order.Uint16(h[60:]))
			stridx = uint64(order.Uint16(h[62:]))
			phnum = uint64(order.Uint16(h[56:]))
		} else if h[4] == 1 {
			shoff = uint64(order.Uint32(h[32:]))
			shent = uint64(order.Uint16(h[46:]))
			shnum = uint64(order.Uint16(h[48:]))
			stridx = uint64(order.Uint16(h[50:]))
			phnum = uint64(order.Uint16(h[44:]))
		} else {
			return ErrUnrecognizedExe
		}
		width := 40
		if wide {
			width = 64
		}
		if shoff != 0 && shent < uint64(width) {
			return ErrUnrecognizedExe
		}
		var sh [64]byte
		if shoff != 0 && (shnum == 0 || stridx == 0xffff) {
			if err = read(sh[:width], shoff); err != nil {
				return err
			}
			if shnum == 0 {
				if wide {
					shnum = order.Uint64(sh[32:])
				} else {
					shnum = uint64(order.Uint32(sh[20:]))
				}
			}
			if stridx == 0xffff {
				if wide {
					stridx = uint64(order.Uint32(sh[40:]))
				} else {
					stridx = uint64(order.Uint32(sh[24:]))
				}
			}
		}
		if err = charge(shnum+phnum, 1024); err != nil {
			return err
		}
		for i := uint64(0); i < shnum; i++ {
			if shoff > uint64(size) || i > uint64(size)/max(1, shent) {
				return ErrUnrecognizedExe
			}
			if err = read(sh[:width], shoff+i*shent); err != nil {
				return err
			}
			var flags, off, n uint64
			if wide {
				flags = order.Uint64(sh[8:])
				off = order.Uint64(sh[24:])
				n = order.Uint64(sh[32:])
			} else {
				flags = uint64(order.Uint32(sh[8:]))
				off = uint64(order.Uint32(sh[16:]))
				n = uint64(order.Uint32(sh[20:]))
			}
			if flags&0x800 != 0 {
				var ch [24]byte
				width := 12
				if wide {
					width = 24
				}
				if err = read(ch[:width], off); err != nil {
					return err
				}
				if wide {
					n = order.Uint64(ch[8:])
				} else {
					n = uint64(order.Uint32(ch[4:]))
				}
				if n > uint64(st.Limits.MaxExpandedBytes) {
					return scanerr.New(scanerr.ResourceLimit, "binary compressed section")
				}
				if err = st.Archive(0, int64(n)); err != nil {
					return err
				}
				amount, e := budget.SizeMul(int(n), 4)
				if e != nil {
					return e
				}
				amount, e = budget.SizeAdd(amount, 128<<10)
				if e != nil {
					return e
				}
				if err = st.Working(amount); err != nil {
					return err
				}
			}
			if i == stridx {
				if err = charge(shnum, n); err != nil {
					return err
				}
			}
		}
	case string(h[:2]) == "MZ":
		if err = read(h[:], 0); err != nil {
			return err
		}
		off := uint64(binary.LittleEndian.Uint32(h[60:]))
		if err = read(h[:24], off); err != nil {
			return err
		}
		if string(h[:4]) != "PE\x00\x00" {
			return ErrUnrecognizedExe
		}
		sections := uint64(binary.LittleEndian.Uint16(h[6:]))
		symoff := uint64(binary.LittleEndian.Uint32(h[12:]))
		symbols := uint64(binary.LittleEndian.Uint32(h[16:]))
		if err = charge(sections+symbols, 1024); err != nil {
			return err
		}
		if symoff != 0 && symbols != 0 {
			if err = read(h[:4], symoff+18*symbols); err != nil {
				return err
			}
			table := uint64(binary.LittleEndian.Uint32(h[:4]))
			if err = charge(sections, table); err != nil {
				return err
			}
			if err = reserveSymbolNames(ctx, st, read, binary.LittleEndian, symoff+18*symbols, table, symoff, symbols, 18, true); err != nil {
				return err
			}
		}
	default:
		// Thin Mach-O is retained for direct parser callers. Scan discovery
		// historically selects ELF/PE only; fat, XCOFF and Plan9 are outside
		// the frozen input contract rather than unbudgeted stdlib fallbacks.
		var order binary.ByteOrder
		magic := binary.LittleEndian.Uint32(h[:4])
		if magic == 0xfeedface || magic == 0xfeedfacf {
			order = binary.LittleEndian
		} else {
			magic = binary.BigEndian.Uint32(h[:4])
			if magic == 0xfeedface || magic == 0xfeedfacf {
				order = binary.BigEndian
			} else {
				return ErrUnrecognizedExe
			}
		}
		if err = read(h[:32], 0); err != nil {
			return err
		}
		commands := uint64(order.Uint32(h[16:]))
		commandBytes := uint64(order.Uint32(h[20:]))
		if commandBytes > uint64(size) {
			return ErrUnrecognizedExe
		}
		if err = charge(commands, 1024); err != nil {
			return err
		}
		off := uint64(28)
		if magic == 0xfeedfacf {
			off = 32
		}
		for i := uint64(0); i < commands; i++ {
			if err = read(h[:8], off); err != nil {
				return err
			}
			cmd, n := order.Uint32(h[:4]), uint64(order.Uint32(h[4:]))
			if n < 8 || n > uint64(size)-off {
				return ErrUnrecognizedExe
			}
			if cmd == 2 {
				if n < 24 {
					return ErrUnrecognizedExe
				}
				if err = read(h[:24], off); err != nil {
					return err
				}
				symbols := uint64(order.Uint32(h[12:]))
				strings := uint64(order.Uint32(h[20:]))
				if err = charge(symbols, 1024); err != nil {
					return err
				}
				symoff := uint64(order.Uint32(h[8:]))
				stroff := uint64(order.Uint32(h[16:]))
				width := uint64(12)
				if magic == 0xfeedfacf {
					width = 16
				}
				if err = reserveSymbolNames(ctx, st, read, order, stroff, strings, symoff, symbols, width, false); err != nil {
					return err
				}
			}
			off += n
		}
	}
	return nil
}

// Precompute the next terminator once, then each repeated symbol name costs
// O(1) inspection. This avoids an adversarial symbols*string-table scan while
// reserving actual name copies rather than that pessimistic product.
func reserveSymbolNames(ctx context.Context, st *budget.State, read func([]byte, uint64) error, order binary.ByteOrder, stroff, strsize, symoff, count, width uint64, coff bool) error {
	if strsize == 0 {
		return nil
	}
	if strsize > uint64(st.Limits.MaxFileBytes) || strsize > uint64(^uint32(0)) {
		return scanerr.New(scanerr.ResourceLimit, "binary string table")
	}
	need, err := budget.SizeMul(int(strsize), 5)
	if err != nil {
		return err
	}
	if err = st.Working(need + 2*budget.SizeSlice); err != nil {
		return err
	}
	table := make([]byte, int(strsize))
	if err = read(table, stroff); err != nil {
		return err
	}
	ends := make([]uint32, len(table))
	end := uint32(len(table))
	for i := len(table) - 1; i >= 0; i-- {
		if table[i] == 0 {
			end = uint32(i)
		}
		ends[i] = end
	}
	var sym [18]byte
	var nameBytes int64
	for i := uint64(0); i < count; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err = read(sym[:width], symoff+i*width); err != nil {
			return err
		}
		offset := uint64(order.Uint32(sym[:4]))
		hasName := true
		if coff {
			hasName = offset == 0
			offset = uint64(order.Uint32(sym[4:]))
			i += uint64(sym[17])
		}
		if !hasName {
			continue
		}
		if offset >= strsize {
			return ErrUnrecognizedExe
		}
		n := int64(uint64(ends[offset]) - offset)
		nameBytes, err = budget.SizeAdd(nameBytes, 4*n+16)
		if err != nil {
			return err
		}
	}
	return st.Working(nameBytes)
}
