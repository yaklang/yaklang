package javaclassparser

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"io/fs"
	"testing"
)

func TestParseJar(t *testing.T) {
	jarFs, err := NewJarFSFromLocal("/Users/z3/Downloads/ysoserial-for-woodpecker-0.5.2.jar")
	if err != nil {
		t.Fatal(err)
	}
	failedFils := []string{}
	err = fs.WalkDir(jarFs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if jarFs.Ext(path) != ".class" {
			return nil
		}
		data, err := jarFs.ReadFile(path)
		if err != nil {
			failedFils = append(failedFils, path)
			log.Error(err)
			return nil
			//return err
		}
		fmt.Printf("file: %s\n", path)

		_ = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
