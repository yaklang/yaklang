package consts

import (
	"path/filepath"
	"github.com/yaklang/yaklang/common/utils"
)

func GetNucleiTemplatesDir() string {
	return filepath.Join(utils.GetHomeDirDefault("."), "nuclei-templates/")
}
