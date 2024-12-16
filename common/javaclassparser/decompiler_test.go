package javaclassparser

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSingleClass(t *testing.T) {
	content, _ := os.ReadFile("/Users/z3/Code/go/yaklang/build/error/decompiler-err-Policy-2pkCnMhN5xbG4Q7zkVDvQsszN0o.class")
	cf, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	//if path != "/Users/z3/Downloads/compiling-failed-files 3/decompiler-err-LinkedBlockingDeque-2oO3vnOHDunZXdMyn8b5VRlh1bt.class" {
	//	return nil
	//}
	source, err := cf.Dump()

	if err != nil {
		t.Fatal(err)
	}
	println(source)
}
func TestParseClass(t *testing.T) {
	err := filepath.Walk("/Users/z3/Downloads/error-jdsc 2", func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		cf, err := Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		//if path != "/Users/z3/Downloads/compiling-failed-files 3/decompiler-err-LinkedBlockingDeque-2oO3vnOHDunZXdMyn8b5VRlh1bt.class" {
		//	return nil
		//}
		source, err := cf.Dump()

		if err != nil {
			//return err
			println(path)
		}
		_ = source
		//fmt.Println(source)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

}
func TestParseJar(t *testing.T) {
	jarFs, err := NewJarFSFromLocal("/Users/z3/Downloads/iam.app.5.0.enc.jar")
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
		fmt.Printf("file: %s\n", path)
		//if path != "com/simp/action/audit/access/MouseLogListAction.class" {
		//	return nil
		//}
		data, err := jarFs.ReadFile(path)
		if err != nil {
			failedFils = append(failedFils, path)
			log.Error(err)
			return nil
			//return err
		}

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
