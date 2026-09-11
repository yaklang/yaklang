//go:generate xorencode -input embed_data/ssti.txt -output embed_data/ssti.txt.enc -key yaklang-dicts-v1
package dicts

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/xorencoded"
)

const sstiXorKey = "yaklang-dicts-v1"

//go:embed embed_data/ssti.txt.enc
var sstiEncFS embed.FS

var _ = func() {
	// keep embed_data/ssti.txt.enc as a compile-time dependency
	_ = sstiEncFS
}

// loadSSTIPayloads reads and decodes the XOR-embedded SSTI dictionary file.
func loadSSTIPayloads() string {
	content, err := xorencoded.LoadEmbedFile(sstiEncFS, "embed_data/ssti.txt.enc", []byte(sstiXorKey))
	if err != nil {
		panic(err)
	}
	return content
}

var SSTI = utils.PrettifyListFromStringSplited(loadSSTIPayloads(), "\n")
