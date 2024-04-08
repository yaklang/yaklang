package ssautil

import (
	"embed"
	"github.com/davecgh/go-spew/spew"
	"testing"
)

//go:embed testdata/***
var testFS embed.FS

func TestPackageLoaderEmbedFS(t *testing.T) {
	currentCheck := false
	aCheck := false
	cCheck := false
	dCheck := false
	loader, err := NewPackageLoader(
		"testdata/index.txt",
		WithEmbedFS(testFS),
		WithPackageLoaderHandler(func(operator PackageLoaderOperator, packageName string) error {
			spew.Dump(packageName)
			switch packageName {
			case ".":
				raw, err := operator.LoadFilePackage("testdata/index.txt")
				if err != nil {
					return err
				}
				if string(raw) == "index" {
					currentCheck = true
				}
			case "a":
				raw, err := operator.LoadFilePackage("testdata/a.txt")
				if err != nil {
					return err
				}
				if string(raw) == "a" {
					aCheck = true
				}
			case "c":
				raw, err := operator.LoadFilePackage("testdata/b/c/c.txt")
				if err != nil {
					return err
				}
				if string(raw) == "c" {
					cCheck = true
				}
			case "d":
				raw, err := operator.LoadFilePackage("testdata/b/c/d/d.txt")
				if err != nil {
					return err
				}
				if string(raw) == "d" {
					dCheck = true
				}
			}
			return nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	err = loader.LoadPackageByName(".")
	if err != nil {
		t.Fatal(err)
	}

	if err = loader.LoadPackageByName("a"); err != nil {
		t.Fatal(err)
	}

	if err = loader.LoadPackageByName("c"); err != nil {
		t.Fatal(err)
	}

	if err = loader.LoadPackageByName("d"); err != nil {
		t.Fatal(err)
	}

	if !currentCheck || !aCheck || !cCheck || !dCheck {
		t.Fatal("load package failed")
	}
}
