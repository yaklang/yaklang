package surigen

import "math/rand"

func nocaseFilter(input []byte) []byte {
	var buf = make([]byte, len(input))
	copy(buf, input)
	for i := 0; i < len(buf); i++ {
		if buf[i] >= 'a' && buf[i] <= 'z' {
			if randBool() {
				buf[i] = buf[i] - 'a' + 'A'
			}
		} else if buf[i] >= 'A' && buf[i] <= 'Z' {
			if randBool() {
				buf[i] = buf[i] - 'A' + 'a'
			}
		}
	}
	return buf
}

func randBool() bool {
	return rand.Int63()%2 == 0
}
