package utils

func BytesClone(raw []byte) (newBytes []byte) {
	newBytes = make([]byte, len(raw))
	copy(newBytes, raw)
	return
}
