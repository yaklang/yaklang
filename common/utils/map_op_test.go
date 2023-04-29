package utils

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

type testSTruct struct {
	Key   string
	Value string
}

func TestStructToMap(t *testing.T) {
	var a = InterfaceToGeneralMap(testSTruct{
		Key:   "123",
		Value: "aa",
	})
	spew.Dump(a)
}
