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
	//pattern := `session=[a-zA-Z0-9+/]{20,300}([a-zA-Z0-9+/]{1}[a-zA-Z0-9+/=]{1}|==)`
	pattern := `^0.*\x02\x01\x00\x04\x06public\xa2.*\x06\x08\+\x06\x01\x02\x01\x01\x05\x00\x04[^\x00]([^\x00]+)`
	pattern = `08\x02\x01\x00\x04\x06public\xef\xbf\xbd+\x02\x04L3\xef\xbf\xbd\x02\x01\x00\x02\x01\x000\x1d0\x1b\x06\x08+\x06\x01\x02\x01\x01\x05\x00\x04\x0fH8 Nas-4_Static`
	results, _ := GenerateOne(pattern)
	spew.Dump(results)
}

func BenchmarkGenerate(b *testing.B) {
	pattern := `\w{3}`
	for i := 0; i < b.N; i++ {
		Generate(pattern)
	}
}
