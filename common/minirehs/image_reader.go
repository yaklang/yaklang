package minirehs

import "encoding/binary"

// Every allocation is bounded by bytes still in the input. Counts are checked
// before multiplication, including on 32-bit hosts; malformed input is data,
// never a reason to recover a panic or call into the native parser.
type imageReader struct {
	b          []byte
	off        int
	err        error
	packed     bool
	stringPool []string
}

func (r *imageReader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.b)-r.off {
		r.err = ErrCorruptImage
		return nil
	}
	b := r.b[r.off : r.off+n]
	r.off += n
	return b
}

func (r *imageReader) u8() byte {
	b := r.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}

func (r *imageReader) u32() uint32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

func (r *imageReader) i32() int32 { return int32(r.u32()) }

func (r *imageReader) u64() uint64 {
	b := r.take(8)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}

func (r *imageReader) count(width int) int {
	n := r.u32()
	if r.err != nil {
		return 0
	}
	if uint64(n) > uint64((len(r.b)-r.off)/width) || n > 0x7fffffff {
		r.err = ErrCorruptImage
		return 0
	}
	return int(n)
}

func (r *imageReader) bytes() []byte { return r.take(r.count(1)) }
func (r *imageReader) str() string {
	if !r.packed {
		return string(r.bytes())
	}
	id := imageVarint(r)
	if id != 0 {
		if id > uint64(len(r.stringPool)) {
			r.err = ErrCorruptImage
			return ""
		}
		return r.stringPool[id-1]
	}
	n := imageVarint(r)
	if n > uint64(len(r.b)-r.off) || r.err != nil {
		r.err = ErrCorruptImage
		return ""
	}
	s := string(r.take(int(n)))
	r.stringPool = append(r.stringPool, s)
	return s
}

func (r *imageReader) strings() []string {
	width := 4
	if r.packed {
		width = 1
	}
	n := r.count(width)
	out := make([]string, n)
	for i := range out {
		out[i] = r.str()
	}
	return out
}

func (r *imageReader) ints(n int) []int32 {
	if n < 0 || n > (len(r.b)-r.off)/4 || r.err != nil {
		r.err = ErrCorruptImage
		return nil
	}
	out := make([]int32, n)
	for i := range out {
		out[i] = r.i32()
	}
	return out
}

func (r *imageReader) intList() []int32 { return r.ints(r.count(4)) }

func (r *imageReader) words(n int) []uint64 {
	if n < 0 || n > (len(r.b)-r.off)/8 || r.err != nil {
		r.err = ErrCorruptImage
		return nil
	}
	out := make([]uint64, n)
	for i := range out {
		out[i] = r.u64()
	}
	return out
}

func (r *imageReader) done() bool { return r.err == nil && r.off == len(r.b) }

func imagePutBytes(b, v []byte) []byte {
	b = putU32(b, uint32(len(v)))
	return append(b, v...)
}

func imagePutString(b []byte, v string) []byte {
	b = putU32(b, uint32(len(v)))
	return append(b, v...)
}

func imagePutStrings(b []byte, vs []string) []byte {
	b = putU32(b, uint32(len(vs)))
	for _, v := range vs {
		b = imagePutString(b, v)
	}
	return b
}

func imagePutInts(b []byte, vs []int32) []byte {
	b = putU32(b, uint32(len(vs)))
	return putI32s(b, vs)
}

func imagePutIndexes(b []byte, vs []int) []byte {
	b = putU32(b, uint32(len(vs)))
	for _, v := range vs {
		b = putI32(b, int32(v))
	}
	return b
}

func imageIndexes(vs []int32) []int {
	out := make([]int, len(vs))
	for i, v := range vs {
		out[i] = int(v)
	}
	return out
}
