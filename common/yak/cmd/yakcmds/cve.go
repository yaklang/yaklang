package yakcmds

import (
	"compress/gzip"
	"context"
	"github.com/jinzhu/gorm"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve"
	"github.com/yaklang/yaklang/common/cve/cvequeryops"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var CVEUtilCommands = []*cli.Command{
	{
		Name:    "translating",
		Aliases: []string{"ai-desc", "desc"},
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "apikey",
				Usage: "API Key for AI",
			},
			cli.BoolFlag{
				Name: "no-critical",
			},
			cli.IntFlag{
				Name:  "concurrent",
				Value: 10,
			},
			cli.StringFlag{
				Name: "cve-database",
			},
			cli.BoolFlag{
				Name: "cwe",
			},
			cli.BoolFlag{
				Name: "chaosmaker-rules,chaosmaker",
			},
			cli.StringFlag{Name: "proxy", Usage: "Network Proxy", EnvVar: "http_proxy"},
			cli.StringFlag{Name: "ai", Usage: "Which AI Gateway? (openai/chatglm)", Value: "openai"},
			cli.Float64Flag{Name: "timeout", Usage: "timeout for seconds", Value: 60},
		},
		Usage:  "Translate CVE Models to Chinese, Supported in OPENAI",
		Hidden: true,
		Action: func(c *cli.Context) error {
			if c.Bool("chaosmaker-rules") {
				rule.DecorateRules("chatglm", c.Int("concurrent"), c.String("proxy"))
				return nil
			}

			if c.Bool("cwe") {
				return cve.TranslatingCWE(c.String("keyfile"), c.Int("concurrent"), c.String("cve-database"))
			}
			_ = consts.GetGormCVEDatabase()
			_ = consts.GetGormCVEDescriptionDatabase()
			return cve.Translating(
				c.String("ai"),
				c.Bool("no-critical"),
				c.Int("concurrent"),
				c.String("cve-database"),
				aispec.WithAPIKey(c.String("apikey")),
				aispec.WithProxy(c.String("proxy")),
				aispec.WithTimeout(c.Float64("timeout")),
			)
		},
	},
	{
		Name:  "build-cve-database",
		Usage: "Build CVE Database in SQLite",
		Flags: []cli.Flag{
			cli.BoolFlag{Name: "cwe"},
			cli.BoolFlag{Name: "cache"},
			cli.StringFlag{Name: "output,o"},
			cli.StringFlag{Name: "description-db"},
			cli.IntFlag{Name: "year"},
			cli.BoolFlag{Name: "no-gzip"},
		},
		Action: func(c *cli.Context) error {
			cvePath := filepath.Join(consts.GetDefaultYakitBaseTempDir(), "cve")
			os.MkdirAll(cvePath, 0o755)

			/* 开始构建 */
			outputFile := c.String("output")
			if outputFile == "" {
				outputFile = consts.GetCVEDatabasePath()
			}
			outputDB, err := gorm.Open("sqlite3", outputFile)
			if err != nil {
				return err
			}
			outputDB.AutoMigrate(&cveresources.CVE{}, &cveresources.CWE{})
			gzipHandler := func() error {
				if c.Bool("no-gzip") {
					return nil
				}
				log.Infof("start to zip... %v", outputFile)
				zipFile := outputFile + ".gzip"
				fp, err := os.OpenFile(zipFile, os.O_CREATE|os.O_RDWR, 0o644)
				if err != nil {
					return err
				}
				defer fp.Close()

				w := gzip.NewWriter(fp)
				srcFp, err := os.Open(outputFile)
				if err != nil {
					return err
				}
				io.Copy(w, srcFp)
				defer srcFp.Close()
				w.Flush()
				w.Close()
				return nil
			}

			descDBPath := c.String("description-db")
			log.Infof("description-db: %v", descDBPath)
			if descDBPath == "" {
				_, _ = consts.InitializeCVEDescriptionDatabase()
				descDBPath = consts.GetCVEDescriptionDatabasePath()
			}
			descDB, err := gorm.Open("sqlite3", descDBPath)
			if err != nil {
				log.Warnf("cannot found sqlite3 cve description: %v", err)
			}

			if c.Bool("cwe") {
				cveDB := outputDB
				if descDB != nil && descDB.HasTable("cwes") && cveDB != nil {
					log.Info("cve-description database is detected, merge cve db")
					if cveDB.HasTable("cwes") {
						if db := cveDB.DropTable("cwes"); db.Error != nil {
							log.Errorf("drop cwe table failed: %s", db.Error)
						}
					}
					log.Infof("start to migrate cwe for cvedb")
					cveDB.AutoMigrate(&cveresources.CWE{})
					for cwe := range cveresources.YieldCWEs(descDB.Model(&cveresources.CVE{}), context.Background()) {
						cveresources.CreateOrUpdateCWE(cveDB, cwe.IdStr, cwe)
					}
					return gzipHandler()
				}

				log.Info("start to download cwe")
				fp, err := cvequeryops.DownloadCWE()
				if err != nil {
					return err
				}
				log.Info("start to load cwes")
				cwes, err := cvequeryops.LoadCWE(fp)
				if err != nil {
					return err
				}
				log.Infof("total cwes: %v", len(cwes))
				db := cveDB
				db.AutoMigrate(&cveresources.CWE{})
				cvequeryops.SaveCWE(db, cwes)
				return gzipHandler()
			}

			wg := new(sync.WaitGroup)
			wg.Add(2)
			var downloadFailed bool
			go func() {
				defer wg.Done()
				log.Infof("start to save cve data from database: %v", cvePath)
				if !c.Bool("cache") {
					err := cvequeryops.DownLoad(cvePath)
					if err != nil {
						log.Error("download failed: %s, err")
						downloadFailed = true
						return
					}
				}
			}()
			go func() {
				defer wg.Done()

				log.Infof("using description database: %s", descDBPath)
				db, err := gorm.Open("sqlite3", descDBPath)
				if err != nil {
					log.Error("sqlite3 failed: %s", err)
					return
				}
				log.Info("start to handling cve description db")
				v := make(map[string]cveresources.CVEDesc)
				var count int
				for i := range cve.YieldCVEDescriptions(db, context.Background()) {
					count++
					//_, ok := v[i.CVE]
					//if ok {
					//	panic("existed cache " + i.CVE)
					//}
					v[i.CVE] = cveresources.CVEDesc{
						TitleZh:           i.ChineseTitle,
						Solution:          i.OpenAISolution,
						DescriptionMainZh: i.ChineseDescription,
					}
				}
				cveresources.RegisterDesc(v)
				log.Infof("register description finished! total: %v", count)
			}()

			wg.Wait()
			if downloadFailed {
				return utils.Error("download failed")
			}

			var years []int
			if ret := c.Int("year"); ret > 0 {
				years = append(years, ret)
			}
			cvequeryops.LoadCVE(cvePath, outputFile, years...)
			return gzipHandler()
		},
	},
}
