package regexp_utils

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestYakRegexpManager(t *testing.T) {
	res, err := NewRegexpWrapper("Server: (.*)").ReplaceAllStringFunc("Server: 123", func(s string) string {
		return "Server: $1 qwe"
	})
	spew.Dump(err)
	fmt.Println(res)
}
