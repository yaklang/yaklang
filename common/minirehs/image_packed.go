package minirehs

import (
	"bytes"
	"encoding/binary"
	"math/bits"
	"unicode/utf8"
)

// Compact storage is bounded independently of compressed size. Larger compiled
// databases can still use the legacy dense image representation.
const maxPackedImageSize uint64 = 256 << 20

func packImageBlob(raw []byte, npat int) ([]byte, error) {
	if uint64(len(raw)) > maxPackedImageSize {
		return nil, ErrUnsupportedImage
	}
	r := &imageReader{b: raw}
	magic, version := r.take(4), r.u32()
	if !bytes.Equal(magic, mvsBlobMagic[:]) || !validMVSBlobVersion(version) || r.u32() != uint32(npat) {
		return nil, ErrCorruptImage
	}
	merged, nu := int(r.i32()), int(r.u32())
	if nu < 0 || nu > npat+1 || merged != -1 && merged != nu-1 {
		return nil, ErrCorruptImage
	}
	slots, offs, lens := r.ints(npat), r.ints(nu), r.ints(nu)
	bitmap := make([]byte, (npat+7)/8)
	next := 0
	for i, s := range slots {
		if s == -1 {
			continue
		}
		if int(s) != next || int(s) == merged {
			return nil, ErrCorruptImage
		}
		bitmap[i/8] |= 1 << uint(i&7)
		next++
	}
	if next+boolInt(merged >= 0) != nu || r.err != nil {
		return nil, ErrCorruptImage
	}
	b := append([]byte("MVP1"), make([]byte, 4)...)
	if version == 4 {
		b[3] = '2' // Preserve canonical DFA semantics through packing.
	}
	binary.LittleEndian.PutUint32(b[4:], uint32(len(raw)))
	b = append(b, byte(boolInt(merged >= 0)))
	b = append(b, make([]byte, 8)...)
	b = append(b, bitmap...)
	w := newPackedTableWriter()
	for i := 0; i < nu; i++ {
		if int(offs[i]) != r.off || lens[i] < 16 {
			return nil, ErrCorruptImage
		}
		unit := r.take(int(lens[i]))
		var err error
		b, err = packImageUnit(b, unit, w)
		if err != nil {
			return nil, err
		}
	}
	if !r.done() {
		return nil, ErrCorruptImage
	}
	binary.LittleEndian.PutUint32(b[9:], uint32(len(w.rows)))
	binary.LittleEndian.PutUint32(b[13:], uint32(len(w.ints)))
	return b, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func packReverseImageUnit(raw []byte) ([]byte, error) {
	b := putU32([]byte("MPU1"), uint32(len(raw)))
	b = append(b, make([]byte, 8)...)
	w := newPackedTableWriter()
	b, err := packImageUnit(b, raw, w)
	if err != nil {
		return nil, err
	}
	binary.LittleEndian.PutUint32(b[8:], uint32(len(w.rows)))
	binary.LittleEndian.PutUint32(b[12:], uint32(len(w.ints)))
	return b, nil
}

func packImageUnit(b, raw []byte, w *packedTableWriter) ([]byte, error) {
	r := &imageReader{b: raw}
	np, nw, ns, flags := int(r.u32()), int(r.u32()), int(r.u32()), r.u32()
	if !packedUnitDimensions(np, nw, ns, flags) {
		return nil, ErrCorruptImage
	}
	for _, n := range []uint64{uint64(np), uint64(ns), uint64(flags)} {
		b = putImageVarint(b, n)
	}
	for _, rows := range []int{1, 1, 1, 1, np, ns} {
		x := r.take(rows * nw * 8)
		if r.err != nil {
			return nil, r.err
		}
		b = w.words(b, x, nw)
	}
	for field, n := range []int{ns + 1, 128, np} {
		x := r.take(n * 4)
		if r.err != nil {
			return nil, r.err
		}
		if field == 1 {
			continue // asciiSym is derived exactly from cuts at load time.
		}
		b = w.intTable(b, x)
	}
	if flags&8 != 0 {
		for _, rows := range []int{1, 1, np} {
			x := r.take(rows * nw * 8)
			if r.err != nil {
				return nil, r.err
			}
			b = w.words(b, x, nw)
		}
	}
	if flags&2 != 0 {
		for _, rows := range []int{1, 1, np, 1} {
			x := r.take(rows * 8)
			if r.err != nil {
				return nil, r.err
			}
			b = w.words(b, x, 1)
		}
		for group := 0; group < 3; group++ {
			n := r.count(1 + 8*nw)
			b = putImageVarint(b, uint64(n))
			if group == 1 {
				positions := r.take(n * 4)
				if r.err != nil {
					return nil, r.err
				}
				b = w.intTable(b, positions)
			}
			guards, rows := r.take(n), r.take(n*nw*8)
			if r.err != nil {
				return nil, r.err
			}
			b = append(b, guards...)
			b = w.words(b, rows, nw)
		}
		for i := 0; i < 4; i++ {
			b = putImageVarint(b, imageZigzag(r.i32()))
		}
	}
	if flags&4 != 0 {
		n := int(r.u32())
		if n < 1 || n > 512 {
			return nil, ErrCorruptImage
		}
		b = putImageVarint(b, uint64(n))
		next, accept := r.take(n*256*4), r.take(n)
		if r.err != nil {
			return nil, r.err
		}
		b = w.intTable(b, next)
		b = append(b, accept...)
	}
	if !r.done() {
		return nil, ErrCorruptImage
	}
	return b, nil
}

func packedUnitDimensions(np, nw, ns int, flags uint32) bool {
	if np < 1 || np > 0x7fffffbf || nw != (np+63)/64 || ns < 1 || ns > utf8.MaxRune+1 || flags&^uint32(15) != 0 {
		return false
	}
	minimum := (4+uint64(np)+uint64(ns))*uint64(nw)*8 + (uint64(ns)+129+uint64(np))*4 + 16
	return minimum <= maxPackedImageSize
}

func unpackImageBlob(packed []byte, npat int) ([]byte, error) {
	r := &imageReader{b: packed}
	magic := r.take(4)
	version := uint32(3)
	if bytes.Equal(magic, []byte("MVP2")) {
		version = 4
	} else if !bytes.Equal(magic, []byte("MVP1")) {
		return nil, ErrCorruptImage
	}
	size := uint64(r.u32())
	merged := r.u8()
	d, nr, ni := newPackedTableReader(r)
	bitmap := r.take((npat + 7) / 8)
	if r.err != nil || merged > 1 || size > maxPackedImageSize || npat&7 != 0 && bitmap[len(bitmap)-1]>>uint(npat&7) != 0 {
		return nil, ErrCorruptImage
	}
	nu := int(merged)
	for _, v := range bitmap {
		nu += bits.OnesCount8(v)
	}
	head := 20 + npat*4 + nu*8
	if uint64(head) > size || nu > len(packed)-r.off {
		return nil, ErrCorruptImage
	}
	// Reserve only a bounded portion of a possibly forged declared size.
	capacity := size
	if capacity > uint64(len(packed))*32 {
		capacity = uint64(len(packed)) * 32
	}
	if capacity < uint64(head) {
		capacity = uint64(head)
	}
	b := make([]byte, head, int(capacity))
	copy(b, mvsBlobMagic[:])
	binary.LittleEndian.PutUint32(b[4:], version)
	binary.LittleEndian.PutUint32(b[8:], uint32(npat))
	mu := int32(-1)
	if merged != 0 {
		mu = int32(nu - 1)
	}
	binary.LittleEndian.PutUint32(b[12:], uint32(mu))
	binary.LittleEndian.PutUint32(b[16:], uint32(nu))
	next := int32(0)
	for i := 0; i < npat; i++ {
		slot := int32(-1)
		if bitmap[i/8]&(1<<uint(i&7)) != 0 {
			slot = next
			next++
		}
		binary.LittleEndian.PutUint32(b[20+i*4:], uint32(slot))
	}
	for i := 0; i < nu && r.err == nil; i++ {
		start := len(b)
		b = unpackImageUnit(b, d)
		if uint64(len(b)) > size {
			return nil, ErrCorruptImage
		}
		binary.LittleEndian.PutUint32(b[20+npat*4+i*4:], uint32(start))
		binary.LittleEndian.PutUint32(b[20+npat*4+nu*4+i*4:], uint32(len(b)-start))
	}
	if !r.done() || uint64(len(b)) != size || len(d.rows) != nr || len(d.ints) != ni {
		return nil, ErrCorruptImage
	}
	return b, nil
}

func unpackReverseImageUnit(packed []byte) ([]byte, error) {
	r := &imageReader{b: packed}
	if !bytes.Equal(r.take(4), []byte("MPU1")) {
		return nil, ErrCorruptImage
	}
	size := uint64(r.u32())
	if size > maxPackedImageSize {
		return nil, ErrCorruptImage
	}
	d, nr, ni := newPackedTableReader(r)
	b := unpackImageUnit(nil, d)
	if !r.done() || uint64(len(b)) != size || len(d.rows) != nr || len(d.ints) != ni {
		return nil, ErrCorruptImage
	}
	return b, nil
}

func unpackImageUnit(b []byte, d *packedTableReader) []byte {
	r := d.r
	np, ns := d.count(0x7fffffbf), d.count(utf8.MaxRune+1)
	flags := uint32(d.count(15))
	nw := (np + 63) / 64
	if r.err != nil || !packedUnitDimensions(np, nw, ns, flags) {
		r.err = ErrCorruptImage
		return b
	}
	b = putU32(b, uint32(np))
	b = putU32(b, uint32(nw))
	b = putU32(b, uint32(ns))
	b = putU32(b, flags)
	for _, rows := range []int{1, 1, 1, 1, np, ns} {
		b = d.words(b, rows, nw)
	}
	cutStart := len(b)
	b = d.intTable(b, ns+1)
	if r.err != nil {
		return b
	}
	cuts := b[cutStart:]
	if binary.LittleEndian.Uint32(cuts) != 0 || binary.LittleEndian.Uint32(cuts[ns*4:]) != utf8.MaxRune+1 {
		r.err = ErrCorruptImage
		return b
	}
	for i := 1; i <= ns; i++ {
		if binary.LittleEndian.Uint32(cuts[i*4:]) <= binary.LittleEndian.Uint32(cuts[(i-1)*4:]) {
			r.err = ErrCorruptImage
			return b
		}
	}
	sym := 0
	for c := 0; c < 128; c++ {
		for uint32(c) >= binary.LittleEndian.Uint32(cuts[(sym+1)*4:]) {
			sym++
		}
		b = putI32(b, int32(sym))
	}
	b = d.intTable(b, np)
	if flags&8 != 0 {
		for _, rows := range []int{1, 1, np} {
			b = d.words(b, rows, nw)
		}
	}
	if flags&2 != 0 {
		for _, rows := range []int{1, 1, np, 1} {
			b = d.words(b, rows, 1)
		}
		for group := 0; group < 3; group++ {
			n := d.count(uint64(len(r.b) - r.off))
			b = putU32(b, uint32(n))
			if group == 1 {
				b = d.intTable(b, n)
			}
			guards := r.take(n)
			b = append(b, guards...)
			b = d.words(b, n, nw)
		}
		for i := 0; i < 4; i++ {
			z := imageVarint(r)
			if z > 0xffffffff {
				r.err = ErrCorruptImage
			}
			b = putI32(b, imageUnzigzag(z))
		}
	}
	if flags&4 != 0 {
		n := d.count(512)
		if n == 0 {
			r.err = ErrCorruptImage
			return b
		}
		b = putU32(b, uint32(n))
		b = d.intTable(b, n*256)
		b = append(b, r.take(n)...)
	}
	return b
}
