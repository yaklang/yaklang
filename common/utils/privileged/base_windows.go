package privileged

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"io/ioutil"
	"os"
)

func isPrivileged() bool {
	f := fmt.Sprintf("C:/Windows/yak-tmp-%v.txt", utils.RandStringBytes(10))
	err := ioutil.WriteFile(f, []byte(utils.RandStringBytes(10)), 0644)
	if err != nil {
		fp, err := os.Open("\\\\.\\PHYSICALDRIVE0")
		if err != nil {
			return false
		}
		fp.Close()
		return true
	}
	defer func() {
		os.RemoveAll(f)
	}()
	return true
}

type Executor struct {
	AppName       string
	AppIcon       string
	DefaultPrompt string
}

func NewExecutor(appName string) *Executor {
	return &Executor{
		AppName:       appName,
		DefaultPrompt: "this operation requires administrator privileges",
	}
}

func (p *Executor) Execute(ctx context.Context, cmd string, opts ...ExecuteOption) ([]byte, error) {
	return nil, utils.Error("not implemented")
}
