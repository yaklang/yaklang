package utils

import "testing"

func BenchmarkError1000(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Error("an_error")
	}
}

func BenchmarkError10000(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Error("an_error")
	}
}

func BenchmarkWrappedError1000(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Wrap(Error("an_error"), "wrapped")
	}
}

func BenchmarkWrappedError10000(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Wrap(Error("an_error"), "wrapped")
	}
}
