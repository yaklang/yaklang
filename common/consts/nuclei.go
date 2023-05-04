package consts

import (
	"github.com/yaklang/yaklang/common/utils"
	"path/filepath"
)

func GetNucleiTemplatesDir() string {
	return filepath.Join(utils.GetHomeDirDefault("."), "nuclei-templates/")
}
