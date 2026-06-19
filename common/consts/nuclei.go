package consts

import (
	"github.com/yaklang/yaklang/common/utils"
	"path/filepath"
)

// GetPoCDir 获取本地 nuclei 模板(PoC)的默认存放目录
// 返回值:
//   - string: 本地 nuclei 模板目录路径
//
// Example:
// ```
// // 该示例为示意性用法：获取本地模板目录
// dir = nuclei.GetPoCDir()
// println(dir)
// ```
func GetNucleiTemplatesDir() string {
	return filepath.Join(utils.GetHomeDirDefault("."), "nuclei-templates/")
}
