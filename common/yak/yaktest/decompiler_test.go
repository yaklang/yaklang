package yaktest

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestParseClass(t *testing.T) {
	err := filepath.Walk("/Users/z3/Downloads/dec-error1", func(path string, info fs.FileInfo, err error) error {
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
		if path != "/Users/z3/Downloads/dec-error1/syntax-error--863ffa87b0fffe50a41fd1c0.class" {
			return nil
		}
		source, err := cf.Dump()

		if err != nil {
			//return err
			println(path)
			return nil
		}
		_ = source
		_, err = ssaapi.Parse(source, ssaapi.WithLanguage(ssaconfig.JAVA))
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
