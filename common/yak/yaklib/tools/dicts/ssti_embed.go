//go:generate gzip-embed -cache --source ./static --gz static.tar.gz --xor-key yaklang-dicts-v1 --no-embed
package dicts

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

const sstiXorKey = "yaklang-dicts-v1"

//go:embed static.tar.gz
var sstiEncFS embed.FS

var sstiFS = func() *gzip_embed.PreprocessingEmbed {
	ins, err := gzip_embed.NewPreprocessingEmbedWithXORKey(&sstiEncFS, "static.tar.gz", true, []byte(sstiXorKey))
	if err != nil {
		panic(err)
	}
	return ins
}()

func loadSSTIPayloads() string {
	raw, err := sstiFS.ReadFile("ssti.txt")
	if err != nil {
		panic(err)
	}
	return string(raw)
}

var SSTI = utils.PrettifyListFromStringSplited(loadSSTIPayloads(), "\n")
