package yaklib

import "yaklang/common/utils"

var GzipExports = map[string]interface{}{
	"Compress":   utils.GzipCompress,
	"Decompress": utils.GzipDeCompress,
	"IsGzip":     utils.IsGzip,
}
