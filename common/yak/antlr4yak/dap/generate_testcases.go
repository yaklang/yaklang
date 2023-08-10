package dap

import "os"

var (
	simpleYakTestCase = `a=1
b=2`
	funcCallYakTestCase = `func test() {
println("hello")	
}

test()`
)

type GernerateFuncTyp func() (path string, removeFunc func())

func GenerateYakTestCase(raw string) (path string, removeFunc func()) {
	f, err := os.CreateTemp("", "yak")
	if err != nil {
		return
	}
	f.WriteString(raw)
	f.Close()
	return f.Name(), func() {
		f.Close()
		os.Remove(f.Name())
	}
}

func GenerateSimpleYakTestCase() (path string, removeFunc func()) {
	return GenerateYakTestCase(simpleYakTestCase)
}

func GenerateFuncCallYakTestCase() (path string, removeFunc func()) {
	return GenerateYakTestCase(funcCallYakTestCase)
}
