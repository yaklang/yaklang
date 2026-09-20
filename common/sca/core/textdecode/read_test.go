package textdecode

import "testing"

func FuzzBOM(f *testing.F) {
	for _, b := range [][]byte{[]byte("plain"), {255, 254, 61, 216, 0, 222}, {254, 255, 0, 65}, {255, 254, 0}, {255, 254, 0, 216}, {239, 187, 191}} {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) <= 1<<18 {
			BOM(b)
		}
	})
}
