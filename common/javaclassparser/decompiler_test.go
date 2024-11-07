package javaclassparser

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"io/fs"
	"os"
	"testing"
)

func TestParseClass(t *testing.T) {
	data, err := os.ReadFile("/Users/z3/Downloads/compiling-failed-files 2/decompiler-err-Lexer-2oO3yquqD1PlSd8KDN3CSOlr3o5.class")
	if err != nil {
		t.Fatal(err)
	}
	cf, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	source, err := cf.Dump()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(source)
}
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

func TestParseJar2(t *testing.T) {
	jarFs, err := NewJarFSFromLocal("/Users/v1ll4n/.m2/repository/com/fasterxml/jackson/datatype/jackson-datatype-jdk8/2.15.4/jackson-datatype-jdk8-2.15.4.jar")
	if err != nil {
		t.Fatal(err)
	}
	failed := []string{}
	err = filesys.Recursive(".", filesys.WithFileSystem(jarFs), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		if jarFs.Ext(s) != ".class" {
			return nil
		}
		data, err := jarFs.ReadFile(s)
		if err != nil {
			spew.Dump(err)
			failed = append(failed, s)
			return err
		}
		_ = data
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	//failedFils := []string{}
	//err = fs.WalkDir(jarFs, "/", func(path string, d fs.DirEntry, err error) error {
	//	if err != nil {
	//		return err
	//	}
	//	if d.IsDir() {
	//		return nil
	//	}
	//	if jarFs.Ext(path) != ".class" {
	//		return nil
	//	}
	//	data, err := jarFs.ReadFile(path)
	//	if err != nil {
	//		failedFils = append(failedFils, path)
	//		log.Error(err)
	//		return nil
	//		//return err
	//	}
	//	fmt.Printf("file: %s\n", path)
	//
	//	_ = data
	//	return nil
	//})
	//if err != nil {
	//	t.Fatal(err)
	//}
}
