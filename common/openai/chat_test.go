package openai

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
)

func getKey() string {
	raw, _ := ioutil.ReadFile(filepath.Join(consts.GetDefaultYakitBaseDir(), "openai-key.txt"))
	return strings.TrimSpace(string(raw))
}

func TestClient_Chat(t *testing.T) {
	rsp, err := NewOpenAIClient(
		WithProxy("http://127.0.0.1:7890"),
		WithAPIKey(getKey()),
	).Chat("Hello")
	if err != nil {
		panic(err)
	}
	spew.Dump(rsp)
}
