package sfbuildin

import (
	"embed"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"io/fs"
	"strings"
)

//go:embed buildin/*.sf
var ruleFS embed.FS

func init() {
	fsInstance := filesys.NewEmbedFS(ruleFS)
	err := filesys.Recursive(".", filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		_, name := fsInstance.PathSplit(s)
		if !strings.HasSuffix(name, ".sf") {
			return nil
		}
		raw, err := fsInstance.ReadFile(s)
		if err != nil {
			log.Warnf("read file %s error: %s", s, err)
			return nil
		}
		err = sfdb.ImportRuleWithoutValid(name, string(raw))
		if err != nil {
			log.Warnf("import rule %s error: %s", name, err)
			return err
		}
		return nil
	}))
	if err != nil {
		log.Errorf("init buildin rule error: %s", err)
	}
}
