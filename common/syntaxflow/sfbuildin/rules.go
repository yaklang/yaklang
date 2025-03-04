package sfbuildin

import (
	"embed"
	"io/fs"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

//go:embed buildin/***
var ruleFS embed.FS

func SyncEmbedRule() error {
	// log.Infof("start sync embed rule")
	sfdb.DeleteBuildInRule()

	fsInstance := filesys.NewEmbedFS(ruleFS)
	err := filesys.Recursive(".", filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		dirName, name := fsInstance.PathSplit(s)
		if !strings.HasSuffix(name, ".sf") {
			return nil
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
		content := string(raw)
		// import builtin rule
		_, err = sfdb.ImportRuleWithoutValid(name, content, true, tags...)
		if err != nil {
			log.Warnf("import rule %s error: %s", name, err)
			return err
		}
		return nil
	}))
	return utils.Wrapf(err, "init builtin rules error")
}
func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
		if yakit.Get(consts.EmbedSfBuildInRuleKey) == consts.ExistedSyntaxFlowEmbedFSHash {
			// log.Infof("already sync embed rule")
			return nil
		}
		defer yakit.Set(consts.EmbedSfBuildInRuleKey, consts.ExistedSyntaxFlowEmbedFSHash)
		return SyncEmbedRule()
	})

}

func SyntaxFlowRuleHash() (string, error) {
	return filesys.CreateEmbedFSHash(ruleFS)
}
