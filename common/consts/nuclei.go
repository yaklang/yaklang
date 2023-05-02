package consts

import (
	"path/filepath"
	"yaklang.io/yaklang/common/utils"
)

func GetNucleiTemplatesDir() string {
	return filepath.Join(utils.GetHomeDirDefault("."), "nuclei-templates/")
}
