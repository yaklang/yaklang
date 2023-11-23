package yakdoc

import (
	"fmt"
	"testing"
)

func TestGetProjectAstPackages(t *testing.T) {
	pkgs, _, err := GetProjectAstPackages()
	if err != nil {
		t.Fatal(err)
	}

	for path, pkg := range pkgs {
		fmt.Printf("%s: %s\n", path, pkg.Name)
	}
}
