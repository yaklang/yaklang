package yakdiff

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

var Exports = map[string]any{
	"Diff":               Diff,
	"DiffFromFileSystem": FileSystemDiff,
	"DiffDir": func(i string, j string) (string, error) {
		if ok, _ := utils.PathExists(i); !ok {
			return "", errors.Errorf("path %s not existed", i)
		}
		if ok, _ := utils.PathExists(j); !ok {
			return "", errors.Errorf("path %s not existed", j)
		}
		return FileSystemDiff(filesys.NewRelLocalFs(i), filesys.NewRelLocalFs(j))
	},
}
