package cve

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io/ioutil"
	"math"
	"os"
	"strings"
	"time"
)

func TranslatingCWE(apiKeyFile string, concurrent int, cveResourceDb string) error {
	key, err := ioutil.ReadFile(apiKeyFile)
	if err != nil {
		return err
	}
	var keyStr = strings.TrimSpace(string(key))
	var db *gorm.DB // = consts.GetGormCVEDatabase()
	if cveResourceDb == "" {
		db = consts.GetGormCVEDatabase()
	} else {
		db, err = gorm.Open("sqlite3", cveResourceDb)
		if err != nil {
			log.Errorf("cannot open: %s with error: %s", cveResourceDb, err)
		}
	}
	if db == nil {
		return utils.Error("no cve database found")
	}

	descDB := consts.GetGormCVEDescriptionDatabase()
	if descDB == nil {
		return utils.Error("empty description database")
	}
	descDB.AutoMigrate(&cveresources.CWE{})

	db.AutoMigrate(&cveresources.CVE{}, &cveresources.CWE{})
	db = db.Model(&cveresources.CWE{}).Where(
		"(name_zh = '') OR " +
			"(description_zh = '') OR " +
			"(extended_description_zh = '') OR " +
			"(cwe_solution = '')")
	if concurrent <= 0 {
		concurrent = 10
	}
	var count int64
	db.Count(&count)
	if count > 0 {
		log.Infof("rest total: %v", count)
	}
	for r := range cveresources.YieldCWEs(db, context.Background()) {
		cveresources.CreateOrUpdateCWE(descDB, r.IdStr, r)
	}
	swg := utils.NewSizedWaitGroup(concurrent)
	current := 0
	for c := range cveresources.YieldCWEs(descDB, context.Background()) {
		current++
		c := c
		swg.Add()
		go func() {
			defer func() {
				swg.Done()
			}()
			start := time.Now()
			cweIns, err := MakeOpenAITranslateCWE(c, string(keyStr), "http://127.0.0.1:7890")
			log.Infof(
				"%6d/%-6d save [%v] chinese desc finished: cost: %v",
				current, count, c.CWEString(), time.Now().Sub(start).String(),
			)
			if err != nil {
				if !strings.Contains(err.Error(), `translating existed`) {
					log.Errorf("make openai working failed: %s", err)
				}

				if strings.Contains(err.Error(), `Service Unavailable`) {
					time.Sleep(time.Minute)
				}
				return
			}
			cveresources.CreateOrUpdateCWE(descDB, cweIns.IdStr, cweIns)
			end := time.Now()
			if dur := end.Sub(start); dur.Seconds() > 3 {
				return
			} else {
				time.Sleep(time.Duration(math.Floor(float64(3)-dur.Seconds())+1) * time.Second)
			}
		}()
	}
	swg.Wait()
	return nil
}

func Translating(apiKeyFile string, noCritical bool, concurrent int, cveResourceDb string) error {
	key, err := ioutil.ReadFile(apiKeyFile)
	if err != nil {
		return err
	}
	var keyStr = strings.TrimSpace(string(key))
	var db *gorm.DB // = consts.GetGormCVEDatabase()
	if cveResourceDb == "" {
		db = consts.GetGormCVEDatabase()
	} else {
		db, err = gorm.Open("sqlite3", cveResourceDb)
		if err != nil {
			log.Errorf("cannot open: %s with error: %s", cveResourceDb, err)
		}
	}
	if db == nil {
		return utils.Error("no cve database found")
	}
	db.AutoMigrate(&cveresources.CVE{}, &cveresources.CWE{})
	db = db.Model(&cveresources.CVE{}).Where("(title_zh is null) OR (title_zh = '')")
	if concurrent <= 0 {
		concurrent = 10
	}

	if os.Getenv("R") != "" {
		db = db.Order("published_date desc")
	} else {
		db = db.Order("published_date asc")
	}
	var count int64
	db.Count(&count)
	if count > 0 {
		log.Infof("rest total: %v", count)
	}
	swg := utils.NewSizedWaitGroup(concurrent)
	current := 0
	for c := range cveresources.YieldCVEs(db, context.Background()) {
		current++
		lowlevel := c.BaseCVSSv2Score <= 6.0 && c.ImpactScore <= 6.0 && c.ExploitabilityScore <= 6.0
		if !((lowlevel && noCritical) || (!lowlevel && !noCritical)) {
			continue
		}

		c := c
		swg.Add()
		go func() {
			defer func() {
				swg.Done()
			}()
			start := time.Now()
			err := MakeOpenAIWorking(c, c.DescriptionMain, string(keyStr), "http://127.0.0.1:7890")
			log.Infof(
				"%6d/%-6d save [%v] chinese desc finished: cost: %v",
				current, count, c.CVE, time.Now().Sub(start).String(),
			)

			if err != nil {
				if !strings.Contains(err.Error(), `translating existed`) {
					log.Errorf("make openai working failed: %s", err)
				}

				if strings.Contains(err.Error(), `Service Unavailable`) {
					time.Sleep(time.Minute)
				}
				return
			}
			end := time.Now()
			if dur := end.Sub(start); dur.Seconds() > 3 {
				return
			} else {
				time.Sleep(time.Duration(math.Floor(float64(3)-dur.Seconds())+1) * time.Second)
			}
		}()
	}
	swg.Wait()
	return nil
}
