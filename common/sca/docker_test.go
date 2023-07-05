package sca

import (
	"fmt"
	"testing"
)

func TestLoadDockerImageFromContext(t *testing.T) {
	pkgs, err := LoadDockerImageFromContext("5d0da3dc9764")
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range pkgs {
		fmt.Printf(`{
Name: %#v,
Version: %#v,
},
`, pkg.Name, pkg.Version)
	}
}

// func TestLoadDockerImageFromFile(t *testing.T) {
// 	pkgs, err := LoadDockerImageFromFile("/tmp/1927.tar")
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	fmt.Printf("%#v", pkgs)
// }
