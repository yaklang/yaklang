package yakcmds

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve"
	"github.com/yaklang/yaklang/common/cve/cvequeryops"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	_ "github.com/mattn/go-sqlite3"
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
			cli.Float64Flag{Name: "total-timeout", Usage: "total timeout (useful in CI)"},
		},
		Usage:  "Translate CVE Models to Chinese, Supported in OPENAI",
		Hidden: true,
		Action: func(c *cli.Context) error {
			totalTimeout := c.Float64("total-timeout")
			if totalTimeout <= 0 {
			}

			if c.Bool("chaosmaker-rules") {
				rule.DecorateRules(c.String("ai"), c.Int("concurrent"), c.String("proxy"))
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
			outputDB, err := consts.CreateCVEDatabase(outputFile)
			if err != nil {
				return err
			}

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
				// merge cwe
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
				err := cvequeryops.DownLoad(cvePath, c.Bool("cache"))
				if err != nil {
					log.Errorf("download failed: %s", err)
					downloadFailed = true
					return
				}
			}()
			go func() {
				defer wg.Done()

				log.Infof("using description database: %s", descDBPath)
				db, err := gorm.Open("sqlite3", descDBPath)
				if err != nil {
					log.Errorf("sqlite3 failed: %s", err)
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
	{
		Name: "cve-merge",
		Flags: []cli.Flag{
			cli.StringFlag{Name: "desc-db", Value: consts.GetCVEDescriptionDatabasePath()},
			cli.StringFlag{Name: "db", Value: consts.GetCVEDatabasePath()},
		},
		Action: func(c *cli.Context) error {
			log.Info("start to cve description and origin database")
			desc, err := gorm.Open("sqlite3", c.String("desc-db"))
			if err != nil {
				return err
			}
			cvedb, err := gorm.Open("sqlite3", c.String("db"))
			if err != nil {
				return err
			}

			cvedb = cvedb.Where("title_zh is '' or title_zh is null")
			count := 0
			updateCount := 0
			log.Infof("start to merge cve info from %s", c.String("desc-db"))
			for ins := range cveresources.YieldCVEs(cvedb, context.Background()) {
				count++
				var descIns cve.CVEDescription
				if err := desc.Where("cve = ?", ins.CVE).First(&descIns).Error; err != nil {
					continue
				}
				if descIns.CVE == "" {
					continue
				}
				if descIns.ChineseTitle != "" {
					/*
						type CVEDescription struct {
							CVE                string `json:"cve" gorm:"unique_index"`
							Title              string
							ChineseTitle       string
							Description        string
							ChineseDescription string
							OpenAISolution     string
						}*/
					ins.TitleZh = descIns.ChineseTitle
					ins.DescriptionMainZh = descIns.ChineseDescription
					ins.Solution = descIns.OpenAISolution
					cvedb.Save(ins)
					log.Infof("update cve: %v %v", ins.CVE, ins.TitleZh)
					updateCount++
				}
			}
			_ = cvedb
			log.Info("count: ", count, "updated: ", updateCount)
			desc.Close()
			cvedb.Close()

			// update cve gzip
			existedGzipCVE := c.String("db") + ".gzip"
			log.Infof("start gzip origin db: %v", existedGzipCVE)
			// if gzip existed, backup with suffix (.tmp.bak)
			if utils.GetFirstExistedPath(existedGzipCVE) != "" {
				backup := existedGzipCVE + ".tmp.bak"
				os.RemoveAll(backup)
				err := os.Rename(existedGzipCVE, backup)
				if err != nil {
					return err
				}
			}

			// gzip for cvedb
			cvefp, err := os.OpenFile(existedGzipCVE, os.O_CREATE|os.O_RDWR, 0o666)
			if err != nil {
				return err
			}
			w := gzip.NewWriter(cvefp)
			cveOrigin, err := os.Open(c.String("db"))
			if err != nil {
				return err
			}
			io.Copy(w, cveOrigin)
			w.Flush()
			w.Close()
			cvefp.Close()
			cveOrigin.Close()
			log.Infof("gzip cve finished: %v", existedGzipCVE)

			// gzip for cve-desc
			existedGzipCVE = c.String("desc-db") + ".gzip"
			log.Infof("start gzip description cve db: %v", existedGzipCVE)

			// if gzip existed, backup with suffix (.tmp.bak)
			if utils.GetFirstExistedPath(existedGzipCVE) != "" {
				backup := existedGzipCVE + ".tmp.bak"
				os.RemoveAll(backup)
				err := os.Rename(existedGzipCVE, backup)
				if err != nil {
					return err
				}
			}
			descfp, err := os.OpenFile(existedGzipCVE, os.O_CREATE|os.O_RDWR, 0o666)
			if err != nil {
				return err
			}
			descGzipW := gzip.NewWriter(descfp)
			descorigin, err := os.Open(c.String("desc-db"))
			if err != nil {
				return err
			}
			io.Copy(descGzipW, descorigin)
			descGzipW.Flush()
			descGzipW.Close()
			descfp.Close()
			descorigin.Close()
			log.Infof("gzip cve desc finished: %v", existedGzipCVE)

			return nil
		},
	},
	{
		Name:  "cve-upload",
		Usage: "upload local cve to aliyun oss (gzip)",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "ak",
				Usage: "oss aliyun access key",
			},
			cli.StringFlag{
				Name: "sk", Usage: "oss aliyun secret key",
			},
			cli.StringFlag{
				Name: "endpoint", Usage: "endpoint for aliyun oss",
				Value: `oss-accelerate.aliyuncs.com`,
			},
			cli.StringFlag{
				Name:  "bucket",
				Usage: `aliyunoss bucket name`,
				Value: "cve-db",
			},
		},
		Action: func(c *cli.Context) error {
			client, err := oss.New(c.String("endpoint"), c.String("ak"), c.String("sk"))
			if err != nil {
				log.Errorf("oss new client failed: %s", err)
				return nil
			}
			bucket, err := client.Bucket("cve-db")
			if err != nil {
				log.Errorf("fetch bucket failed: %s", err)
				return nil
			}

			// upload cve
			cvePath := consts.GetCVEDatabaseGzipPath()
			log.Infof("start to upload cve database: %v", cvePath)
			if cvePath == "" {
				return utils.Errorf("no path found for cve: %s", cvePath)
			}
			if utils.GetFirstExistedPath(cvePath) == "" {
				return utils.Errorf("no cve database found: %s", cvePath)
			}
			// check the filesize is larger than 10M
			if fi, err := os.Stat(cvePath); err != nil {
				log.Errorf("stat cve failed: %s", err)
				return err
			} else {
				if fi.Size() < 10*1024*1024 {
					log.Errorf("cve file size is too small: %v", fi.Size())
					return nil
				}
			}
			if err := bucket.PutObjectFromFile("default-cve.db.gzip", cvePath); err != nil {
				log.Errorf("upload cve failed: %s", err)
				return err
			}

			// description database
			cveDescPath := consts.GetCVEDescriptionDatabaseGzipPath()
			log.Infof("start to upload cve(translating description database: %s)", cveDescPath)
			if cveDescPath == "" {
				log.Errorf("cannot found cve database gzip path")
				return nil
			}
			if utils.GetFirstExistedPath(cveDescPath) == "" {
				return utils.Errorf("no cve database found: %s", cveDescPath)
			}
			// check filesize is larger than 10M
			if fi, err := os.Stat(cveDescPath); err != nil {
				log.Errorf("stat cve desc failed: %s", err)
				return err
			} else {
				if fi.Size() < 10*1024*1024 {
					log.Errorf("cve desc file size is too small: %v", fi.Size())
					return nil
				}
			}

			if err := bucket.PutObjectFromFile("default-cve-description.db.gzip", cveDescPath); err != nil {
				log.Errorf("upload cve desc failed: %s", err)
				return nil
			}
			return nil
		},
	},
	{
		Name: "cve-download",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "ak",
				Usage: "oss aliyun access key",
			},
			cli.StringFlag{
				Name: "sk", Usage: "oss aliyun secret key",
			},
			cli.StringFlag{
				Name: "endpoint", Usage: "endpoint for aliyun oss",
				Value: `oss-accelerate.aliyuncs.com`,
			},
			cli.StringFlag{
				Name:  "bucket",
				Usage: `aliyunoss bucket name`,
				Value: "cve-db",
			},
		},
		Action: func(c *cli.Context) error {
			client, err := oss.New(c.String("endpoint"), c.String("ak"), c.String("sk"))
			if err != nil {
				log.Errorf("oss new client failed: %s", err)
				return nil
			}
			bucket, err := client.Bucket("cve-db")
			if err != nil {
				log.Errorf("fetch bucket failed: %s", err)
				return nil
			}

			// download cve
			cvePath := consts.GetCVEDatabaseGzipPath()
			log.Infof("start to download cve database: %v", cvePath)
			if cvePath == "" {
				return utils.Errorf("no path found for cve: %s", cvePath)
			}
			if utils.GetFirstExistedPath(cvePath) != "" {
				bak := cvePath + ".bak"
				if err := os.RemoveAll(bak); err != nil {
					return err
				}
				err := os.Rename(cvePath, cvePath+".bak")
				if err != nil {
					return utils.Errorf("%v' s backup failed: %s", cvePath, err)
				}
			}
			if err := bucket.DownloadFile("default-cve.db.gzip", cvePath, 20*1024*1024); err != nil {
				log.Errorf("download cve failed: %s", err)
				return err
			}

			log.Infof("start to extract db from gzip: %v", cvePath)
			// remove old db file
			cvePathDB := consts.GetCVEDatabasePath()
			os.RemoveAll(cvePathDB)
			cveFile, err := os.OpenFile(cvePathDB, os.O_RDWR|os.O_CREATE, 0o666)
			if err != nil {
				return utils.Errorf("open file failed: %s", err)
			}
			defer cveFile.Close()
			gzipFile, err := os.Open(cvePath)
			if err != nil {
				return err
			}
			defer gzipFile.Close()
			r, err := gzip.NewReader(gzipFile)
			if err != nil {
				return utils.Errorf("gzip new reader failed: %s", err)
			}
			_, err = io.Copy(cveFile, r)
			if err != nil {
				return utils.Errorf("cve(db) copy failed: %s", err)
			}
			log.Infof("download gzip database finished: %v", cvePathDB)

			// description database
			cveDescPath := consts.GetCVEDescriptionDatabaseGzipPath()
			log.Infof("start to handle cve(translating description database: %s)", cveDescPath)
			if cveDescPath == "" {
				log.Errorf("cannot found cve database gzip path")
				return nil
			}
			var newDescDB bool
			if utils.GetFirstExistedPath(cveDescPath) == "" {
				newDescDB = true
			}
			if !newDescDB {
				err := os.Rename(cveDescPath, cveDescPath+".bak")
				if err != nil {
					return utils.Errorf("%v' s backup failed: %s", cveDescPath, err)
				}
			}

			log.Infof("start to download bucket: %s", "default-cve-description.db.gzip")
			err = bucket.DownloadFile("default-cve-description.db.gzip", cveDescPath, 20*1024*1024)
			if err != nil {
				log.Errorf("download cve desc failed: %s", err)
				return nil
			}

			log.Infof("start to un-gzip: %v", cveDescPath)
			cveDescPathDB := consts.GetCVEDescriptionDatabasePath()
			os.RemoveAll(cveDescPathDB)
			cveDescFile, err := os.OpenFile(cveDescPathDB, os.O_RDWR|os.O_CREATE, 0o666)
			if err != nil {
				return utils.Errorf("open file failed: %s", err)
			}
			defer cveDescFile.Close()
			gzipDescFile, err := os.Open(cveDescPath)
			if err != nil {
				return err
			}
			defer gzipDescFile.Close()

			r, err = gzip.NewReader(gzipDescFile)
			if err != nil {
				return utils.Errorf("gzip new reader failed: %s", err)
			}
			_, err = io.Copy(cveDescFile, r)
			if err != nil {
				return utils.Errorf("cve(desc) copy failed: %s", err)
			}
			log.Infof("download gzip database finished: %v", cveDescPathDB)
			return nil
		},
	},
}
