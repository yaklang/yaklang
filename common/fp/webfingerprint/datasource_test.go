package webfingerprint

import (
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
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
