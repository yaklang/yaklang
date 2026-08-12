package yaklib

import "github.com/yaklang/yaklang/common/utils"

func init() {
	if HttpExports == nil {
		HttpExports = make(map[string]interface{})
	}
	HttpExports["ExtractFaviconURL"] = utils.ExtractFaviconURL
}
