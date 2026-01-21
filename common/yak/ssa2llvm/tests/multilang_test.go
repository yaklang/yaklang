package tests

import (
	"testing"
)

func TestLang_Yak(t *testing.T) {
	// Simple Yak function
	checkEx(t, `check = () => { return 1 + 2 }`, "yak", 3)
}

func TestLang_Go(t *testing.T) {
	// Note: ssaapi for Go might expect complete file structure
	checkEx(t, `
    package main
    func check() int {
        return 10 + 20
    }
    `, "golang", 30)
}

func TestLang_Python(t *testing.T) {
	// Python simple function
	checkEx(t, `
def check():
    return 5 * 6
`, "python", 30)
}

func TestLang_JS(t *testing.T) {
	// Javascript check
	checkEx(t, `
function check() {
    return 100 - 1;
}
`, "javascript", 99)
}
