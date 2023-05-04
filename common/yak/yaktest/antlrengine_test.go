package yaktest

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

func TestAntlrEngine(t *testing.T) {
	os.Setenv("YAKMODE", "vm")

	err := ioutil.WriteFile("/tmp/text-111.txt", []byte("hello world"), 0644)
	if err != nil {
		panic(err)
	}
	defer os.Remove("/tmp/text-111.txt")

	cases := []YakTestCase{
		{
			Name: "测试数据库",
			Src: fmt.Sprintf(`
a = string(file.ReadFile("/tmp/text-111.txt")[0])
dump(a)
	`),
		},
	}

	Run("测试数据库链接", t, cases...)
}
