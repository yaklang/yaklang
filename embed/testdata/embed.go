package testdata

import (
	"embed"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

//go:embed data
var FS embed.FS

func Asset(name string) ([]byte, error) {
	buf, err := FS.ReadFile(name)
	if strings.HasSuffix(name, ".gz") || strings.HasSuffix(name, ".gzip") {
		buf, err = utils.GzipDeCompress(buf)
	}
	return buf, err
}
