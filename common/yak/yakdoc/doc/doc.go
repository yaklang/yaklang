package doc

import (
	"bytes"
	_ "embed"
	"encoding/gob"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
)

//go:embed doc.gob.gzip
var embedDocument []byte

var DefaultDocumentHelper *yakdoc.DocumentHelper

func init() {
	buf, err := utils.GzipDeCompress(embedDocument)
	if err != nil {
		log.Warnf("load embed yak document error: %v", err)
	}

	decoder := gob.NewDecoder(bytes.NewReader(buf))
	if err := decoder.Decode(&DefaultDocumentHelper); err != nil {
		log.Warnf("load embed yak document error: %v", err)
	}
}
