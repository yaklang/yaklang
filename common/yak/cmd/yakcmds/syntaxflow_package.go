package yakcmds

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

var syntaxFlowPackageCommand = &cli.Command{
	Name:    "syntaxflow-package",
	Aliases: []string{"sf-package", "sfpkg"},
	Usage:   "manage SyntaxFlow rule packages (list/sync/export/import)",
	Subcommands: []cli.Command{
		{
			Name:  "list",
			Usage: "list installed rule packages",
			Action: func(c *cli.Context) error {
				db := consts.GetGormProfileDatabase()
				_ = db.AutoMigrate(&schema.SyntaxFlowGroup{}, &schema.SyntaxFlowRule{}).Error
				var pkgs []*schema.SyntaxFlowGroup
				if err := db.Order("group_name asc").Find(&pkgs).Error; err != nil {
					return err
				}
				if len(pkgs) == 0 {
					fmt.Println("(no packages)")
					return nil
				}
				for _, p := range pkgs {
					count := sfdb.CountRulesInPackage(db, p.GroupName)
					fmt.Printf("%-24s v%-10s source=%-8s builtin=%v rules=%d\n",
						p.GroupName, p.Version, p.Source, p.IsBuildIn, count)
				}
				return nil
			},
		},
		{
			Name:  "sync",
			Usage: "sync embed packages (builtin + agent) into local DB",
			Flags: []cli.Flag{
				cli.BoolFlag{Name: "force,f", Usage: "force sync ignoring hash gate"},
			},
			Action: func(c *cli.Context) error {
				notify := func(p float64, name string) {
					log.Infof("[%.0f%%] %s", p*100, name)
				}
				if c.Bool("force") {
					return sfbuildin.ForceSyncEmbedRule(notify)
				}
				return sfbuildin.SyncEmbedRule(notify)
			},
		},
		{
			Name:  "export",
			Usage: "export a package to zip (+ sidecar package.yaml)",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "name,n", Usage: "package name"},
				cli.StringFlag{Name: "output,o", Usage: "output zip path"},
				cli.StringFlag{Name: "password", Usage: "optional zip password"},
			},
			Action: func(c *cli.Context) error {
				name := c.String("name")
				out := c.String("output")
				if name == "" || out == "" {
					return utils.Error("--name and --output are required")
				}
				db := consts.GetGormProfileDatabase()
				meta, err := sfdb.BuildPackageYAMLFromDB(db, name)
				if err != nil {
					return err
				}
				opts := []sfdb.RuleExportOption{}
				if pw := c.String("password"); pw != "" {
					opts = append(opts, sfdb.WithExportPassword(pw))
				}
				ruleDB := db.Model(&schema.SyntaxFlowRule{}).Where("rule_group = ?", name)
				result, err := sfdb.ExportRulesToZip(utils.TimeoutContextSeconds(600), ruleDB, out, opts...)
				if err != nil {
					return err
				}
				_ = sfdb.WritePackageYAML(filepath.Dir(out), meta)
				fmt.Printf("exported package %s (%d rules) -> %s\n", name, result.Count, out)
				return nil
			},
		},
		{
			Name:  "import",
			Usage: "import a package zip or directory",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "input,i", Usage: "zip path or directory"},
				cli.StringFlag{Name: "name,n", Usage: "override / legacy package name"},
				cli.BoolFlag{Name: "force-overwrite-conflicts", Usage: "overwrite dual-key conflicts"},
				cli.StringFlag{Name: "password", Usage: "optional zip password"},
			},
			Action: func(c *cli.Context) error {
				input := c.String("input")
				if input == "" {
					return utils.Error("--input is required")
				}
				return importSyntaxFlowPackageCLI(input, c.String("name"), c.String("password"), c.Bool("force-overwrite-conflicts"))
			},
		},
	},
}

func importSyntaxFlowPackageCLI(input, name, password string, force bool) error {
	db := consts.GetGormProfileDatabase()
	_ = db.AutoMigrate(&schema.SyntaxFlowGroup{}, &schema.SyntaxFlowRule{}).Error
	pkgName := strings.TrimSpace(name)

	meta, err := sfdb.LoadPackageYAML(input)
	if err != nil {
		dir := input
		if st, e := os.Stat(input); e == nil && !st.IsDir() {
			dir = filepath.Dir(input)
		}
		meta, err = sfdb.LoadPackageYAML(dir)
	}
	if err != nil || meta == nil {
		if pkgName == "" {
			pkgName = fmt.Sprintf("imported-%d", time.Now().Unix())
		}
		meta = &sfdb.PackageYAML{
			Name:    pkgName,
			Version: "0.1.0",
			Source:  schema.SyntaxFlowPackageSourceLocal,
		}
		log.Infof("legacy import without package.yaml → %s", pkgName)
	}
	if pkgName != "" {
		meta.Name = pkgName
	}
	if _, err := sfdb.GetOrCreatePackage(db, meta.Name, meta.Version, meta.Description, schema.SyntaxFlowPackageSourceLocal, false); err != nil {
		return err
	}

	st, err := os.Stat(input)
	if err != nil {
		return err
	}
	if st.IsDir() {
		if err := sfbuildin.SyncPackageFromFileSystemToDB(db, filesys.NewLocalFs(), input, meta.Name, false, nil); err != nil {
			return err
		}
		fmt.Printf("imported package %s from directory\n", meta.Name)
		return nil
	}

	opts := []sfdb.RuleImportOption{}
	if password != "" {
		opts = append(opts, sfdb.WithImportPassword(password))
	}
	if _, err := sfdb.ImportRulesFromZip(utils.TimeoutContextSeconds(600), db, input, opts...); err != nil {
		return err
	}

	conflicts := 0
	for _, r := range meta.Rules {
		if c := sfdb.CheckRulePackageIdentityConflict(db, meta.Name, r.RuleID, r.RuleName, r.Version); c != nil {
			sfdb.LogPackageConflict(c)
			conflicts++
			if !force {
				continue
			}
		}
		_ = db.Model(&schema.SyntaxFlowRule{}).Where("rule_id = ?", r.RuleID).
			Updates(map[string]any{"rule_group": meta.Name, "version": r.Version}).Error
	}
	if conflicts > 0 && !force {
		return utils.Errorf("%d conflict(s); re-run with --force-overwrite-conflicts to overwrite", conflicts)
	}
	fmt.Printf("imported package %s\n", meta.Name)
	return nil
}
