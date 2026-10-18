package minirehs

import "bytes"

// The native blob stores only a reduced assertion prefilter. Keep the complete
// Go preconditions too: merged existence scans consult these even without cgo.
// Omitting them would preserve matches but regress the loaded Go scan path.
func encodeImagePreconditions(d *mvsDB) []byte {
	b := []byte("NEC1")
	b = putImageNecFactor(b, d.mergedNecFactor)
	n := 0
	for _, nf := range d.assertNecFactor {
		if nf != (necFactor{}) {
			n++
		}
	}
	b = putU32(b, uint32(n))
	for i, nf := range d.assertNecFactor {
		if nf != (necFactor{}) {
			b = putI32(b, int32(i))
			b = putImageNecFactor(b, nf)
		}
	}
	return b
}

func putImageNecFactor(b []byte, nf necFactor) []byte {
	b = putI32(b, int32(nf.minRunLen))
	b = append(b, byte(nf.runClass), byte(nf.firstByte), byte(nf.lastByte))
	if nf.hasFactor {
		b = append(b, 1)
	} else {
		b = append(b, 0)
	}
	n := 0
	for _, v := range nf.requiredBytes {
		if v != 0 {
			n++
		}
	}
	b = putU32(b, uint32(n))
	for c, v := range nf.requiredBytes {
		if v != 0 {
			b = append(b, byte(c))
			b = putI32(b, int32(v))
		}
	}
	return b
}

func readImageNecFactor(r *imageReader) necFactor {
	nf := necFactor{minRunLen: int(r.i32()), runClass: byteClass(r.u8()), firstByte: byteClass(r.u8()), lastByte: byteClass(r.u8())}
	on := r.u8()
	nf.hasFactor = on == 1
	if nf.minRunLen < 0 || nf.runClass > byteClassBSlash || nf.firstByte > byteClassBSlash || nf.lastByte > byteClassBSlash || on > 1 {
		r.err = ErrCorruptImage
		return nf
	}
	n := r.count(5)
	prev := -1
	for i := 0; i < n; i++ {
		c, v := int(r.u8()), r.i32()
		if c <= prev || c >= 128 || v == 0 || v < -32768 || v > 32767 {
			r.err = ErrCorruptImage
			return nf
		}
		nf.requiredBytes[c] = int16(v)
		prev = c
	}
	return nf
}

func decodeImagePreconditions(data []byte, d *mvsDB) error {
	r := &imageReader{b: data}
	if !bytes.Equal(r.take(4), []byte("NEC1")) {
		return ErrCorruptImage
	}
	d.mergedNecFactor = readImageNecFactor(r)
	allowed := make([]bool, d.n)
	for _, i := range d.assertAlwaysOn {
		allowed[i] = true
	}
	clear(d.assertNecFactor)
	n := r.count(16)
	prev := -1
	for i := 0; i < n; i++ {
		idx := int(r.i32())
		if idx <= prev || idx >= d.n || !allowed[idx] {
			return ErrCorruptImage
		}
		d.assertNecFactor[idx] = readImageNecFactor(r)
		prev = idx
	}
	if !r.done() {
		return ErrCorruptImage
	}
	return nil
}
