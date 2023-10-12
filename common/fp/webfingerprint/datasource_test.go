package webfingerprint

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestLoadDefaultDataSource(t *testing.T) {

	got, err := LoadDefaultDataSource()
	if err != nil {

	}
	_ = got
}

func TestMockWebFingerPrint(t *testing.T) {

	//got, got1 := MockWebFingerPrintByName("deno_deploy")
	rule, got, got1 := MockRandomWebFingerPrints()
	spew.Dump(rule)
	fmt.Println(got, got1)
}
