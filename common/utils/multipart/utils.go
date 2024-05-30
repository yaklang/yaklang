package multipart

func BytesJoinSize(size int, s ...[]byte) []byte {
	b, i := make([]byte, size), 0
	for _, v := range s {
		i += copy(b[i:], v)
	}
	return b
}
