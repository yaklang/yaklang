package cve

import (
	"github.com/yaklang/yaklang/common/consts"
	"io/ioutil"
	"path/filepath"
	"strings"
)

func getKey() string {
	raw, _ := ioutil.ReadFile(filepath.Join(consts.GetDefaultYakitBaseDir(), "openai-key.txt"))
	return strings.TrimSpace(string(raw))
}
