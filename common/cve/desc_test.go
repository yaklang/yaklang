package cve

import (
	"io/ioutil"
	"yaklang/common/consts"
	"path/filepath"
	"strings"
)

func getKey() string {
	raw, _ := ioutil.ReadFile(filepath.Join(consts.GetDefaultYakitBaseDir(), "openai-key.txt"))
	return strings.TrimSpace(string(raw))
}
