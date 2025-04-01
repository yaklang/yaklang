package ai

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
)

func TestBatchChatter(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
		return
	}
	bc := NewBatchChatter()
	apikey, err := os.ReadFile(filepath.Join(consts.GetDefaultYakitBaseDir(), "yaklang-bailian-apikey.txt"))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(apikey))
	bc.SetCallback(func(typeName string, modelName string, isReason bool, reader io.Reader) {
		fmt.Println(typeName, modelName, isReason)
		io.Copy(os.Stdout, reader)
	})
	bc.AddChatClient("openai", "sk-1234567890", "gpt-3.5-turbo")
	bc.AddChatClient("openai", "sk-1234567890", "gpt-3.5-turbo")
	bc.AddChatClient("tongyi", string(apikey), "qwen-plus")
	result, err := bc.Chat("Hello, world!")
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(result)
}
