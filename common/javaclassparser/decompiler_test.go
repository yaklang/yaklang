package javaclassparser

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestParseSingleClass(t *testing.T) {
	content, _ := os.ReadFile("/Users/z3/Downloads/JarEntry.class")
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
	err := filepath.Walk("/Users/z3/Downloads/error-jdsc 3", func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".class") {
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
		// if path != "/Users/z3/Downloads/error-jdsc 3/decompile-err-068afc96a4cd68e35eeb99e2.class" {
		// 	return nil
		// }
		source, err := cf.Dump()

		if err != nil {
			//return err
			println(path)
		}
		_ = source
		// fmt.Println(source)
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
		//if path != "net/lingala/zip4j/core/HeaderReader.class" {
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
	jarFs, err := NewJarFSFromLocal("/Users/z3/Downloads/iam.app.5.0.enc.jar")
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
