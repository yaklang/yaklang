package regen

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func Test_ExampleGenerate(t *testing.T) {
	pattern := `(abc|bcd){2}`
	results, _ := Generate(pattern)

	fmt.Printf("%#v\n", results)
}

func Test_GenerateOne(t *testing.T) {
	pattern := `session=[a-zA-Z0-9+/]{20,300}([a-zA-Z0-9+/]{1}[a-zA-Z0-9+/=]{1}|==)`
	results, _ := GenerateOne(pattern)
	spew.Dump(results)
}

func BenchmarkGenerate(b *testing.B) {
	pattern := `\w{3}`
	for i := 0; i < b.N; i++ {
		Generate(pattern)
	}
}
