//go:build !irify_exclude

package sfbuildin

import (
	"io/fs"
	"path"
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

// embedPackageDir maps on-disk top-level embed dirs to package names.
// Historical directory is still named "buildin"; package name is "builtin".
var embedPackageDirs = []struct {
	Dir         string
	PackageName string
}{
	{Dir: "buildin", PackageName: schema.SyntaxFlowPackageBuiltin},
	{Dir: "agent", PackageName: schema.SyntaxFlowPackageAgent},
}

func syncAllEmbedPackagesToDB(db *gorm.DB, fsInstance filesys_interface.FileSystem, buildin bool, notify func(process float64, ruleName string)) error {
	for _, pkg := range embedPackageDirs {
		if _, err := fsInstance.Stat(pkg.Dir); err != nil {
			log.Infof("embed package dir %s not present, skip", pkg.Dir)
			continue
		}
		meta, metaErr := loadEmbedPackageMeta(fsInstance, pkg.Dir, pkg.PackageName)
		if metaErr != nil {
			log.Warnf("load package meta for %s: %v (using defaults)", pkg.Dir, metaErr)
			meta = &sfdb.PackageYAML{
				Name:    pkg.PackageName,
				Version: "1.0.0",
				Source:  schema.SyntaxFlowPackageSourceEmbed,
			}
		}
		if _, err := sfdb.GetOrCreatePackage(db, meta.Name, meta.Version, meta.Description, schema.SyntaxFlowPackageSourceEmbed, true); err != nil {
			return utils.Wrapf(err, "ensure package %s", meta.Name)
		}
		if err := SyncPackageFromFileSystemToDB(db, fsInstance, pkg.Dir, meta.Name, buildin, notify); err != nil {
			return err
		}
		if len(meta.Rules) > 0 {
			if err := prunePackageRulesNotInManifest(db, meta); err != nil {
				log.Warnf("prune package %s: %v", meta.Name, err)
			}
		}
	}
	return nil
}

func loadEmbedPackageMeta(fsInstance filesys_interface.FileSystem, dir, defaultName string) (*sfdb.PackageYAML, error) {
	raw, err := fsInstance.ReadFile(path.Join(dir, "package.yaml"))
	if err != nil {
		return nil, err
	}
	meta, err := sfdb.ParsePackageYAML(raw)
	if err != nil {
		return nil, err
	}
	if meta.Name == "" {
		meta.Name = defaultName
	}
	return meta, nil
}

func prunePackageRulesNotInManifest(db *gorm.DB, meta *sfdb.PackageYAML) error {
	keep := make(map[string]struct{}, len(meta.Rules))
	for _, r := range meta.Rules {
		if r.RuleID != "" {
			keep[r.RuleID] = struct{}{}
		}
	}
	if len(keep) == 0 {
		return nil
	}
	var existing []*schema.SyntaxFlowRule
	if err := db.Where("package_name = ?", meta.Name).Find(&existing).Error; err != nil {
		return err
	}
	for _, r := range existing {
		if _, ok := keep[r.RuleId]; ok {
			continue
		}
		if err := db.Unscoped().Delete(r).Error; err != nil {
			return err
		}
		log.Infof("pruned rule %s (%s) from package %s", r.RuleName, r.RuleId, meta.Name)
	}
	return nil
}

// SyncPackageFromFileSystemToDB imports .sf files under packageRoot into packageName.
func SyncPackageFromFileSystemToDB(db *gorm.DB, fsInstance filesys_interface.FileSystem, packageRoot, packageName string, buildin bool, notify func(process float64, ruleName string)) error {
	var (
		handledCount float64
		totalCount   float64
	)
	_ = filesys.Recursive(packageRoot, filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		if strings.HasSuffix(info.Name(), ".sf") {
			totalCount++
		}
		return nil
	}))

	return filesys.Recursive(packageRoot, filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
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
			if block == "buildin" || block == "builtin" || block == "agent" || block == packageRoot {
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
		_, err = sfdb.ImportRuleWithoutValidExWithDBAndPackage(db, name, content, s, buildin, packageName, tags...)
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
