package yaktest

import (
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestParseClass(t *testing.T) {
	err := filepath.Walk("/Users/z3/Downloads/error-jdsc 2", func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		cf, err := javaclassparser.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		if path != "/Users/z3/Downloads/error-jdsc 2/decompile-err-12828d0c8622c36004bfe797.class" {
			return nil
		}
		source, err := cf.Dump()

		if err != nil {
			//return err
			println(path)
		}
		_ = source
		_, err = ssaapi.Parse(source, ssaapi.WithLanguage(ssaapi.JAVA))
		if err != nil {
			println(path)
		}
		//fmt.Println(source)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

}
