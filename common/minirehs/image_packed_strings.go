package minirehs

// Expressions and literals use the same first-appearance string table. In
// particular the global literal index refers to strings already used by rules.
type packedStringWriter struct{ ids map[string]uint64 }

func (w *packedStringWriter) str(b []byte, s string) []byte {
	if w == nil {
		return imagePutString(b, s)
	}
	if id, ok := w.ids[s]; ok {
		return putImageVarint(b, id+1)
	}
	w.ids[s] = uint64(len(w.ids))
	b = putImageVarint(append(b, 0), uint64(len(s)))
	return append(b, s...)
}

func (w *packedStringWriter) strings(b []byte, ss []string) []byte {
	b = putU32(b, uint32(len(ss)))
	for _, s := range ss {
		b = w.str(b, s)
	}
	return b
}
