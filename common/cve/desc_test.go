package cve

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"github.com/yaklang/yaklang/common/consts"
)

func getKey() string {
	raw, _ := ioutil.ReadFile(filepath.Join(consts.GetDefaultYakitBaseDir(), "openai-key.txt"))
	return strings.TrimSpace(string(raw))
}
