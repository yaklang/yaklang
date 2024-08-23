package sfbuildin

import (
	"embed"
	"io/fs"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

//go:embed buildin/***
var ruleFS embed.FS

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
		fsInstance := filesys.NewEmbedFS(ruleFS)
		err := filesys.Recursive(".", filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
			dirName, name := fsInstance.PathSplit(s)
			if !strings.HasSuffix(name, ".sf") {
				return utils.Error("invalid sf file")
			}
			raw, err := fsInstance.ReadFile(s)
			if err != nil {
				return utils.Wrapf(err, "read file[%s] error", s)
			}

			var tags []string
			for _, block := range utils.PrettifyListFromStringSplitEx(dirName, "/", "\\", ",", "|") {
				block = strings.ToLower(block)
				if block == "buildin" {
					continue
				}
				if strings.HasPrefix(block, "cwe-") {
					result, err := regexp_utils.NewYakRegexpUtils(`(cwe-\d+)(-(.*))?`).FindStringSubmatch(block)
					if err != nil {
						continue
					}
					tags = append(tags, strings.ToUpper(result[1]))
					tags = append(tags, result[3])
					continue
				} else if strings.HasPrefix(block, "cve-") {
					result, err := regexp_utils.NewYakRegexpUtils(`(cve-\d+-\d+)([_-\.](.*))?`).FindStringSubmatch(block)
					if err != nil {
						continue
					}
					tags = append(tags, strings.ToUpper(result[1]))
					tags = append(tags, result[3])
					continue
				}
				tags = append(tags, block)
			}
			err = sfdb.ImportRuleWithoutValid(name, string(raw), true, tags...)
			if err != nil {
				log.Warnf("import rule %s error: %s", name, err)
				return err
			}
			return nil
		}))
		return utils.Wrapf(err, "init builtin rules error")
	})
}
