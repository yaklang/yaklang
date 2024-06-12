package yakdocument_test

import (
	"testing"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakdocument"
)

type DemoStruct struct {
	Name  string
	as    string
	Func1 func(i string) *DemoStruct
	Func2 func(i string, demoStruct DemoStruct) DemoStruct
	func2 func(i string, demoStruct DemoStruct) DemoStruct
}

func (d *DemoStruct) Test1() string {
	return d.Name
}

func (d *DemoStruct) Test4() (string, error) {
	return d.Name, nil
}

func (d *DemoStruct) Test41(a, b, c string, f float64) string {
	return d.Name
}

func (d *DemoStruct) test1() (string, error) {
	return d.Name, nil
}

func TestDir(t *testing.T) {
	sh, err := yakdocument.Dir(&DemoStruct{
		Name:  "tzas",
		Func1: nil,
		Func2: nil,
	})
	if err != nil {
		log.Error(err)
		t.FailNow()
		return
	}
	sh.Show()
	sh.ShowAddDocHelper()
}
