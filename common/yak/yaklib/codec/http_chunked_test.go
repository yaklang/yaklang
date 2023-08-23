package codec

import (
	"github.com/davecgh/go-spew/spew"
	uuid "github.com/satori/go.uuid"
	"testing"
)

func TestHTTPChunkedRead(t *testing.T) {
	block, data := readHTTPChunkedData([]byte(`1
a
0

aaaacccc`))
	if string(block) != "a" {
		t.Fatal("block should not be a")
	}
	if string(data) != "aaaacccc" {
		t.Fatal("data should be aaaacccc")
	}
	block, data = readHTTPChunkedData([]byte(`11
aaaabbbbddddeeeef
2
ff
1
f
0

aaaacccc`))
	if string(block) != "aaaabbbbddddeeeeffff" {
		t.Fatal("block should not be a")
	}
	if string(data) != "aaaacccc" {
		t.Fatal("data should be aaaacccc")
	}

	block, data = readHTTPChunkedData([]byte(`11;aaaaaajnkasdjkfqwpjoe
aaaabbbbddddeeeef
2;jkasdjiof
ff
1;asdfpjkaspjodf
f
0

aaaacccc`))
	if string(block) != "aaaabbbbddddeeeeffff" {
		t.Fatal("block should not be a")
	}
	if string(data) != "aaaacccc" {
		t.Fatal("data should be aaaacccc")
	}

	rand1 := "asdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffffdffffffffffffff"
	rand2 := uuid.NewV4().String()
	results := string(HTTPChunkedEncode([]byte(rand1))) + string(HTTPChunkedEncode([]byte(rand2)))
	rand1a, res := readHTTPChunkedData([]byte(results))
	spew.Dump(res)
	if string(rand1a) != rand1 {
		t.Fatal("rand1a should be rand1")
	}
	rand2a, res := readHTTPChunkedData(res)
	spew.Dump(res)
	if string(rand2a) != rand2 {
		t.Fatal("rand2a should be rand2")
	}
}
