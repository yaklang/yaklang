package yaklib

import "github.com/yaklang/yaklang/common/utils"

func init() {
	HttpExports["ExtractFaviconURL"] = utils.ExtractFaviconURL
}
