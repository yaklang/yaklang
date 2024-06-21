package parsers

import "testing"

func TestCompiler(t *testing.T) {
	_, err := compileExp("header=\"Chart.js\" || title=\"Chart.js\"")
	if err != nil {
		t.Fatal(err)
	}
}
