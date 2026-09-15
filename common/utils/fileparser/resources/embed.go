package resources

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/filesys"
)

//go:embed static
var resourceFS embed.FS

var FS = filesys.NewEmbedSubFS(resourceFS, "static")
