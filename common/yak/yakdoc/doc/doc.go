package doc

import (
	"bytes"
	_ "embed"
	"encoding/gob"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
)

//go:embed doc.gob.gzip
var embedDocument []byte

var (
	defaultDocumentHelper *yakdoc.DocumentHelper
	once                  sync.Once
)

func GetDefaultDocumentHelper() *yakdoc.DocumentHelper {
	once.Do(func() {
		buf, err := utils.GzipDeCompress(embedDocument)
		if err != nil {
			log.Warnf("load embed yak document error: %v", err)
		}

		decoder := gob.NewDecoder(bytes.NewReader(buf))
		if err := decoder.Decode(&defaultDocumentHelper); err != nil {
			log.Warnf("load embed yak document error: %v", err)
		}
	})
	return defaultDocumentHelper
}
