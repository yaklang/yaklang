//go:generate gzip-embed -cache --source ./static --gz static.tar.gz --xor-key yaklang-dicts-v1 --no-embed
package dicts

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/yaklang/yaklang/common/utils"
)

const sstiXorKey = "yaklang-dicts-v1"

//go:embed static
var sstiRawFS embed.FS

var sstiFS = filesys.NewEmbedSubFS(sstiRawFS, "static")

func loadSSTIPayloads() string {
	raw, err := sstiFS.ReadFile("ssti.txt")
	if err != nil {
		panic(err)
	}
	return string(raw)
}

var SSTI = utils.PrettifyListFromStringSplited(loadSSTIPayloads(), "\n")
