//go:build !irify_exclude

package sfbuildin

import (
	"io/fs"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

// embedGroupDirs maps on-disk top-level embed dirs to reserved group names.
// Historical directory is still named "buildin"; group name is "builtin".
var embedGroupDirs = []struct {
	Dir       string
	GroupName string
}{
	{Dir: "buildin", GroupName: schema.SyntaxFlowGroupBuiltin},
	{Dir: "agent", GroupName: schema.SyntaxFlowGroupAgent},
}

func syncAllEmbedGroupsToDB(db *gorm.DB, fsInstance filesys_interface.FileSystem, buildin bool, notify func(process float64, ruleName string)) error {
	for _, g := range embedGroupDirs {
		if _, err := fsInstance.Stat(g.Dir); err != nil {
			log.Infof("embed group dir %s not present, skip", g.Dir)
			continue
		}
		_ = sfdb.GetOrCreateGroups(db, []string{g.GroupName})
		if err := SyncGroupFromFileSystemToDB(db, fsInstance, g.Dir, g.GroupName, buildin, notify); err != nil {
			return err
		}
	}
	return nil
}

// SyncGroupFromFileSystemToDB imports .sf files under groupRoot into RuleGroup.
func SyncGroupFromFileSystemToDB(db *gorm.DB, fsInstance filesys_interface.FileSystem, groupRoot, groupName string, buildin bool, notify func(process float64, ruleName string)) error {
	var (
		handledCount float64
		totalCount   float64
	)
	_ = filesys.Recursive(groupRoot, filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		if strings.HasSuffix(info.Name(), ".sf") {
			totalCount++
		}
		return nil
	}))

	return filesys.Recursive(groupRoot, filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
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
			if block == "buildin" || block == "builtin" || block == "agent" || block == groupRoot {
				continue
			}
			if strings.HasPrefix(block, "cwe-") {
				result, err := regexp_utils.NewYakRegexpUtils(`(cwe-\d+)(-(.*))?`).FindStringSubmatch(block)
				if err != nil {
					continue
				}
				tags = append(tags, "cwe:"+strings.TrimPrefix(strings.ToLower(result[1]), "cwe-"))
				if result[3] != "" {
					tags = append(tags, result[3])
				}
				continue
			} else if strings.HasPrefix(block, "cve-") {
				result, err := regexp_utils.NewYakRegexpUtils(`(cve-\d+-\d+)([_-\.](.*))?`).FindStringSubmatch(block)
				if err != nil {
					continue
				}
				tags = append(tags, strings.ToUpper(result[1]))
				if result[3] != "" {
					tags = append(tags, result[3])
				}
				continue
			}
			tags = append(tags, block)
		}
		content := string(raw)
		_, err = sfdb.ImportRuleWithoutValidExWithDBAndGroup(db, name, content, s, buildin, groupName, tags...)
		if err != nil {
			log.Warnf("import rule %s error: %s", name, err)
			return err
		}
		handledCount++
		if notify != nil && totalCount > 0 {
			notify(handledCount/totalCount, "更新内置SyntaxFlow规则:"+info.Name())
		}
		return nil
	}))
}
