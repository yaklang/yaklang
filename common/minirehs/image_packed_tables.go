package minirehs

import (
	"encoding/binary"
	"math/bits"
)

// These codecs describe tables, rather than Go objects: first-appearance IDs
// share identical rows across units, zero rows have no payload, and each new row
// chooses its smallest exact representation. No hash value is a row identity.
type packedTableWriter struct {
	rows map[string]uint64
	ints map[string]uint64
}

func newPackedTableWriter() *packedTableWriter {
	return &packedTableWriter{rows: make(map[string]uint64), ints: make(map[string]uint64)}
}

func putImageVarint(b []byte, v uint64) []byte { return binary.AppendUvarint(b, v) }

func imageVarint(r *imageReader) uint64 {
	if r.err != nil {
		return 0
	}
	v, n := binary.Uvarint(r.b[r.off:])
	if n <= 0 || n != imageVarintSize(v) {
		r.err = ErrCorruptImage
		return 0
	}
	r.off += n
	return v
}

func imageVarintSize(v uint64) int { return (bits.Len64(v|1) + 6) / 7 }
func imageZigzag(v int32) uint64   { return uint64(uint32(v<<1) ^ uint32(v>>31)) }
func imageUnzigzag(v uint64) int32 { return int32(uint32(v>>1) ^ -uint32(v&1)) }

func (w *packedTableWriter) row(b, row []byte) []byte {
	for len(row) > 0 && binary.LittleEndian.Uint64(row[len(row)-8:]) == 0 {
		row = row[:len(row)-8]
	}
	if len(row) == 0 {
		return append(b, 0)
	}
	if id, ok := w.rows[string(row)]; ok {
		return putImageVarint(b, id+2)
	}
	w.rows[string(row)] = uint64(len(w.rows))
	b = append(b, 1)
	var nb, nz, bitCost, wordCost int
	prevBit, prevWord := -1, -1
	for off := 0; off < len(row); off += 8 {
		v := binary.LittleEndian.Uint64(row[off:])
		if v == 0 {
			continue
		}
		nz++
		wordCost += imageVarintSize(uint64(off/8-prevWord-1)) + 8
		prevWord = off / 8
		nb += bits.OnesCount64(v)
		for v != 0 {
			p := off*8 + bits.TrailingZeros64(v)
			bitCost += imageVarintSize(uint64(p - prevBit - 1))
			prevBit = p
			v &= v - 1
		}
	}
	bitCost += imageVarintSize(uint64(nb))
	wordCost += imageVarintSize(uint64(nz))
	denseCost := imageVarintSize(uint64(len(row)/8)) + len(row)
	if bitCost <= wordCost && bitCost <= denseCost {
		b = putImageVarint(append(b, 0), uint64(nb))
		prev := -1
		for off := 0; off < len(row); off += 8 {
			v := binary.LittleEndian.Uint64(row[off:])
			for v != 0 {
				p := off*8 + bits.TrailingZeros64(v)
				b = putImageVarint(b, uint64(p-prev-1))
				prev = p
				v &= v - 1
			}
		}
		return b
	}
	if wordCost < denseCost {
		b = putImageVarint(append(b, 1), uint64(nz))
		prev := -1
		for off := 0; off < len(row); off += 8 {
			v := binary.LittleEndian.Uint64(row[off:])
			if v != 0 {
				b = putImageVarint(b, uint64(off/8-prev-1))
				b = putU64(b, v)
				prev = off / 8
			}
		}
		return b
	}
	b = putImageVarint(append(b, 2), uint64(len(row)/8))
	return append(b, row...)
}

func (w *packedTableWriter) words(b, raw []byte, width int) []byte {
	for off := 0; off < len(raw); off += width * 8 {
		b = w.row(b, raw[off:off+width*8])
	}
	return b
}

func (w *packedTableWriter) intTable(b, raw []byte) []byte {
	if len(raw) == 0 {
		return append(b, 0)
	}
	if id, ok := w.ints[string(raw)]; ok {
		return putImageVarint(b, id+2)
	}
	w.ints[string(raw)] = uint64(len(w.ints))
	b = append(b, 1)
	var runCost, deltaCost, plainCost int
	var prev int32
	for off := 0; off < len(raw); {
		v := int32(binary.LittleEndian.Uint32(raw[off:]))
		deltaCost += imageVarintSize(imageZigzag(v - prev))
		plainCost += imageVarintSize(imageZigzag(v))
		prev = v
		off += 4
	}
	for off := 0; off < len(raw); {
		v := binary.LittleEndian.Uint32(raw[off:])
		end := off + 4
		for end < len(raw) && binary.LittleEndian.Uint32(raw[end:]) == v {
			end += 4
		}
		runCost += imageVarintSize(uint64((end-off)/4)) + imageVarintSize(imageZigzag(int32(v)))
		off = end
	}
	if runCost <= deltaCost && runCost <= plainCost {
		b = append(b, 0)
		for off := 0; off < len(raw); {
			v := binary.LittleEndian.Uint32(raw[off:])
			end := off + 4
			for end < len(raw) && binary.LittleEndian.Uint32(raw[end:]) == v {
				end += 4
			}
			b = putImageVarint(b, uint64((end-off)/4))
			b = putImageVarint(b, imageZigzag(int32(v)))
			off = end
		}
		return b
	}
	mode := byte(1)
	if plainCost < deltaCost {
		mode = 2
	}
	b = append(b, mode)
	prev = 0
	for off := 0; off < len(raw); off += 4 {
		v := int32(binary.LittleEndian.Uint32(raw[off:]))
		if mode == 1 {
			b = putImageVarint(b, imageZigzag(v-prev))
		} else {
			b = putImageVarint(b, imageZigzag(v))
		}
		prev = v
	}
	return b
}

type packedTableReader struct {
	r    *imageReader
	rows []packedTableSpan
	ints []packedTableSpan
}

// Offsets into the growing native output avoid allocating a second copy of
// every dictionary row. They remain valid when append moves the output buffer,
// and these pointer-free dictionaries do not add GC scanning work.
type packedTableSpan struct{ off, size uint32 }

func newPackedTableReader(r *imageReader) (*packedTableReader, int, int) {
	nr, ni := uint64(r.u32()), uint64(r.u32())
	if nr*4+ni*3 > uint64(len(r.b)-r.off) || r.err != nil {
		r.err = ErrCorruptImage
		return &packedTableReader{r: r}, 0, 0
	}
	return &packedTableReader{r: r, rows: make([]packedTableSpan, 0, int(nr)), ints: make([]packedTableSpan, 0, int(ni))}, int(nr), int(ni)
}

func packedImageRoom(b []byte, n uint64) bool {
	return uint64(len(b)) <= maxPackedImageSize && n <= maxPackedImageSize-uint64(len(b))
}

func (d *packedTableReader) count(max uint64) int {
	n := imageVarint(d.r)
	if n > max || n > 0x7fffffff {
		d.r.err = ErrCorruptImage
		return 0
	}
	return int(n)
}

func (d *packedTableReader) row(b []byte, width int) []byte {
	id := imageVarint(d.r)
	if d.r.err != nil {
		return b
	}
	start := len(b)
	b = append(b, make([]byte, width*8)...)
	if id == 0 {
		return b
	}
	if id >= 2 {
		if id-2 >= uint64(len(d.rows)) || uint64(d.rows[id-2].size) > uint64(width)*8 {
			d.r.err = ErrCorruptImage
			return b
		}
		span := d.rows[id-2]
		copy(b[start:], b[span.off:span.off+span.size])
		return b
	}
	mode := d.r.u8()
	row := b[start:]
	switch mode {
	case 0:
		n := d.count(uint64(width) * 64)
		if n > len(d.r.b)-d.r.off {
			d.r.err = ErrCorruptImage
			break
		}
		prev := -1
		for i := 0; i < n && d.r.err == nil; i++ {
			gap := d.count(uint64(width) * 64)
			p := prev + 1 + gap
			if p >= width*64 {
				d.r.err = ErrCorruptImage
				break
			}
			off := (p / 64) * 8
			v := binary.LittleEndian.Uint64(row[off:]) | 1<<uint(p&63)
			binary.LittleEndian.PutUint64(row[off:], v)
			prev = p
		}
	case 1:
		n := d.count(uint64(width))
		if n > (len(d.r.b)-d.r.off)/9 {
			d.r.err = ErrCorruptImage
			break
		}
		prev := -1
		for i := 0; i < n && d.r.err == nil; i++ {
			p := prev + 1 + d.count(uint64(width))
			v := d.r.u64()
			if p >= width || v == 0 {
				d.r.err = ErrCorruptImage
				break
			}
			binary.LittleEndian.PutUint64(row[p*8:], v)
			prev = p
		}
	case 2:
		n := d.count(uint64(width))
		copy(row, d.r.take(n*8))
	default:
		d.r.err = ErrCorruptImage
	}
	for len(row) > 0 && binary.LittleEndian.Uint64(row[len(row)-8:]) == 0 {
		row = row[:len(row)-8]
	}
	if len(row) == 0 {
		d.r.err = ErrCorruptImage
	}
	if d.r.err == nil {
		d.rows = append(d.rows, packedTableSpan{off: uint32(start), size: uint32(len(row))})
	}
	return b
}

func (d *packedTableReader) words(b []byte, rows, width int) []byte {
	if rows < 0 || width < 1 || !packedImageRoom(b, uint64(rows)*uint64(width)*8) || rows > len(d.r.b)-d.r.off {
		d.r.err = ErrCorruptImage
		return b
	}
	for i := 0; i < rows && d.r.err == nil; i++ {
		b = d.row(b, width)
	}
	return b
}

func (d *packedTableReader) intTable(b []byte, n int) []byte {
	id := imageVarint(d.r)
	if n < 0 || !packedImageRoom(b, uint64(n)*4) || (n == 0) != (id == 0) {
		d.r.err = ErrCorruptImage
		return b
	}
	if d.r.err != nil || n == 0 {
		return b
	}
	if id >= 2 {
		if id-2 >= uint64(len(d.ints)) || uint64(d.ints[id-2].size) != uint64(n)*4 {
			d.r.err = ErrCorruptImage
			return b
		}
		span := d.ints[id-2]
		return append(b, b[span.off:span.off+span.size]...)
	}
	mode := d.r.u8()
	if mode > 2 || mode != 0 && n > len(d.r.b)-d.r.off {
		d.r.err = ErrCorruptImage
		return b
	}
	start := len(b)
	b = append(b, make([]byte, n*4)...)
	var prev int32
	for i := 0; i < n && d.r.err == nil; {
		run := 1
		if mode == 0 {
			run = d.count(uint64(n - i))
			if run == 0 {
				d.r.err = ErrCorruptImage
				break
			}
		}
		z := imageVarint(d.r)
		if z > 0xffffffff {
			d.r.err = ErrCorruptImage
			break
		}
		v := imageUnzigzag(z)
		if mode == 1 {
			v += prev
		}
		for j := 0; j < run; j++ {
			binary.LittleEndian.PutUint32(b[start+(i+j)*4:], uint32(v))
		}
		prev = v
		i += run
	}
	if d.r.err == nil {
		d.ints = append(d.ints, packedTableSpan{off: uint32(start), size: uint32(n * 4)})
	}
	return b
}
