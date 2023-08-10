package dap

import "os"

var (
	simpleYakTestCase = `a=1
b=2`
)

type GernerateFuncTyp func() (path string, removeFunc func())

func GenerateSimpleYakTestCase() (path string, removeFunc func()) {
	f, err := os.CreateTemp("", "yak")
	if err != nil {
		return
	}
	f.WriteString(simpleYakTestCase)
	f.Close()
	return f.Name(), func() {
		f.Close()
		os.Remove(f.Name())
	}
}
