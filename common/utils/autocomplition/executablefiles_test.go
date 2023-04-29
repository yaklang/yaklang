package autocomplition

import (
	"testing"
)

func TestGetPathExecutableFile(t *testing.T) {
	results := GetPathExecutableFile()
	if len(results) <= 0 {
		t.FailNow()
	}

	// pp.Println(results)
}
