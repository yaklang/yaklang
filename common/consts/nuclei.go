package consts

import (
	"path/filepath"
	"yaklang/common/utils"
)

func GetNucleiTemplatesDir() string {
	return filepath.Join(utils.GetHomeDirDefault("."), "nuclei-templates/")
}
