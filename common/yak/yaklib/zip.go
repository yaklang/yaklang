package yaklib

import (
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

var ZipExports = map[string]interface{}{
	"Decompress": ziputil.DeCompress,
	"Compress": func(zipName string, filenames ...string) error {
		return ziputil.CompressByName(filenames, zipName)
	},
}
