package yaktest

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

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
		cf, err := javaclassparser.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		// if path != "/Users/z3/Downloads/error-jdsc 3/decompile-err-0891f02d99bd27aa6e82c8dd.class" {
		// 	return nil
		// }
		source, err := cf.Dump()

		if err != nil {
			//return err
			println(path)
			return nil
		}
		_ = source
		_, err = ssaapi.Parse(source, ssaapi.WithLanguage(ssaapi.JAVA))
		if err != nil {
			os.WriteFile("/Users/z3/Downloads/error.java", []byte(source), 0644)
			println(path)
		}
		//fmt.Println(source)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

}
