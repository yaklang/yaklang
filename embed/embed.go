package embed

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

func Asset(name string) ([]byte, error) {
	buf, err := FS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(name, ".gz") || strings.HasSuffix(name, ".gzip") {
		buf, err = utils.GzipDeCompress(buf)
	}
	return buf, err
}

func AssetDir(name string) ([]string, error) {
	dir, err := FS.ReadDir(name)
	if err != nil {
		return nil, err
	}
	entries := make([]string, 0, len(dir))
	for _, v := range dir {
		entries = append(entries, v.Name())
	}
	return entries, nil
}
