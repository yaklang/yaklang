package java

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"io"
	"os"
	"testing"
	"time"
)

func TestUnhealthyLargeFile(t *testing.T) {
	code, err := sourceCodeSample.ReadFile("sample/fastjson/ParserConfig.java")
	if err != nil {
		t.Fatal(err)
	}

	log.SetOutput(io.Discard)

	var buf bytes.Buffer
	start := time.Now()
	ssatest.Check(t, string(code), func(prog *ssaapi.Program) error {
		buf.WriteString(fmt.Sprintf("cost: %v\n", time.Now().Sub(start)))
		start = time.Now()
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
	log.SetOutput(os.Stdout)
	fmt.Println(buf.String())
	ssa.ShowDatabaseCacheCost()
}
