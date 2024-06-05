package comate

import (
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"os"
	"testing"
)

func TestComateDemo(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
		return
	}
	c := &Client{}
	reader, err := c.question("Python写一额Hello World程序")
	if err != nil {
		t.Failed()
	}
	io.Copy(os.Stdout, reader)
}
