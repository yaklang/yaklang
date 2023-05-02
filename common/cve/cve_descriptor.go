package cve

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"strconv"
	"sync"
	"time"
	"yaklang/common/consts"
	"yaklang/common/cve/cveresources"
	"yaklang/common/jsonextractor"
	"yaklang/common/log"
	"yaklang/common/openai"
	"yaklang/common/utils"
	"yaklang/common/utils/bizhelper"
)

type CVEDescription struct {
	CVE                string `json:"cve" gorm:"unique_index"`
	Title              string
	ChineseTitle       string
	Description        string
	ChineseDescription string
	OpenAISolution     string
}

func InitializeCVEDescription() {
	db := consts.GetGormCVEDescriptionDatabase()
	if db == nil {
		return
	}
	db.AutoMigrate(&CVEDescription{}, &cveresources.CWE{})
}

type gormLog func(i ...interface{})

func (g gormLog) Print(i ...interface{}) {
	g(i...)
}

//
//func _mergeResource(fromDB string, toDB string) error {
//	fromDBIns, err := gorm.Open("sqlite3", fromDB)
//	if err != nil {
//		return err
//	}
//	toDBIns, err := gorm.Open("sqlite3", toDB)
//	if err != nil {
//		return err
//	}
//	var count int
//	var start = time.Now()
//	//toDBIns.SetLogger(gormLog(func(i ...interface{}) {
//	//	if len(i) <= 1 {
//	//		spew.Dump(i)
//	//	}
//	//	res := i[1:]
//	//	fmt.Printf(fmt.Sprint(i[0]), res...)
//	//}))
//	for i := range YieldCVEDescriptions(fromDBIns.Model(&CVEDescription{}).Debug(), context.Background()) {
//		count++
//		now := time.Now()
//		if count%50 == 0 {
//			fmt.Printf("count: %v now: %v duration: %v\n", count, now.String(), now.Sub(start).String())
//		}
//
//		if db := toDBIns.Exec(
//			` UPDATE "cves" SET "description_main_zh" = ?, "solution" = ?, "title_zh" = ? WHERE (cve = ?)`,
//			i.ChineseDescription, i.OpenAISolution, i.ChineseTitle, i.CVE,
//		); db.Error != nil {
//			log.Errorf("handling cve desc failed: %v", db.Error)
//		}
//		//if db := toDBIns.Debug().Model(&cveresources.CVE{}).Where("cve = ?", i.CVE).Updates(map[string]interface{}{
//		//	"title_zh":            i.ChineseTitle,
//		//	"solution":            i.OpenAISolution,
//		//	"description_main_zh": i.ChineseDescription,
//		//}); db.Error != nil {
//		//	log.Errorf("handling cve desc failed: %v", db.Error)
//		//}
//	}
//	toDBIns.Commit()
//	return nil
//}

func _migrateTable() error {
	srcDB := consts.GetGormCVEDatabase()
	if srcDB == nil {
		return utils.Error("no cve db")
	}

	if !srcDB.HasTable(&CVEDescription{}) {
		return utils.Error("no legacy cve desc")
	}

	dstDB := consts.GetGormCVEDescriptionDatabase()
	if dstDB == nil {
		return utils.Error("no dst cve desc db")
	}
	dstDB.AutoMigrate(&CVEDescription{})

	for result := range YieldCVEDescriptions(srcDB, context.Background()) {
		dstDB.Save(result)
	}
	return nil
}

var once = sync.Once{}

var transLock = sync.Mutex{}

func MakeOpenAITranslateCWE(cwe *cveresources.CWE, apiKey string, proxies ...string) (*cveresources.CWE, error) {
	once.Do(func() {
		InitializeCVEDescription()
	})

	db := consts.GetGormCVEDescriptionDatabase()
	if db == nil {
		return nil, utils.Errorf("no database found")
	}

	var proxy string
	if len(proxies) > 0 {
		proxy = proxies[0]
	}

	client := openai.NewOpenAIClient(openai.WithAPIKey(apiKey), openai.WithProxy(proxy))
	if cwe.NameZh == "" {
		data, err := client.TranslateToChinese(cwe.Name)
		if err != nil {
			log.Errorf("translate cwe name failed: %s", err)
		}
		cwe.NameZh = data
	}
	if cwe.NameZh != "" {
		log.Infof("translating cwe: %v", cwe.NameZh)
	}

	log.Infof("start to translate description: %s", cwe.CWEString())
	if cwe.DescriptionZh == "" && cwe.Description != "" {
		desc, err := client.TranslateToChinese(cwe.Description)
		if err != nil {
			log.Errorf("translate desc failed: %s", err)
		}
		cwe.DescriptionZh = desc
	}

	log.Infof("start to translate extended description: %v", cwe.CWEString())
	if cwe.ExtendedDescriptionZh == "" && cwe.ExtendedDescription != "" {
		desc, err := client.TranslateToChinese(cwe.ExtendedDescription)
		if err != nil {
			log.Errorf("translate desc ex failed: %s", err)
		}
		cwe.ExtendedDescriptionZh = desc
	}

	log.Infof("start to generate cwe solution: %v", cwe.CWEString())
	if cwe.CWESolution == "" && cwe.NameZh != "" {
		solution, err := client.Chat(fmt.Sprintf("请给出 %v 的100字以内的安全建议或修复方案", strconv.Quote(cwe.NameZh)))
		if err != nil {
			log.Errorf("generate solution failed: %s", err)
		}
		cwe.CWESolution = solution
	}

	return cwe, nil
}

func MakeOpenAIWorking(src *cveresources.CVE, cveDescription string, apiKey string, proxy ...string) error {
	once.Do(func() {
		InitializeCVEDescription()
	})

	db := consts.GetGormCVEDescriptionDatabase()
	if db == nil {
		return utils.Error("no database found")
	}

	var d CVEDescription
	if db := db.Where("cve = ?", src.CVE).First(&d); db.Error != nil {

	}
	if d.CVE != "" && d.ChineseTitle != "" {
		log.Debugf("cve: %s 's ch-Description is existed", d.CVE)
		return utils.Errorf("%v's translating existed", d.CVE)
	}

	var tmpl = fmt.Sprintf(`把 %v 整理成 JSON 数据，要求是中文，提取标题(title)，并给出中文翻译(desc)，并给出修复方案(solution)`, strconv.Quote(cveDescription))
	var proxyReal string
	if len(proxy) > 0 {
		proxyReal = proxy[0]
	}

	log.Debugf("cve: %s's being translated...", src.CVE)
	var start = time.Now()
	client := openai.NewOpenAIClient(openai.WithAPIKey(apiKey), openai.WithProxy(proxyReal))
	data, err := client.Chat(tmpl)
	if err != nil {
		println(string(data))
		log.Infof("openai.Chat met error: %s", err)
		return err
	}
	results := jsonextractor.ExtractStandardJSON(data)
	if len(results) > 0 {
		var raw = make(map[string]interface{})
		err := json.Unmarshal([]byte(jsonextractor.FixJson([]byte(results[0]))), &raw)
		if err != nil {
			log.Warnf("unmarshal origin data: ")
			fmt.Println(string(data))
			return utils.Errorf("convert json result failed: %s", err)
		}
		titleZh := utils.MapGetString(raw, "title")
		descZh := utils.MapGetString(raw, "desc")
		solutionZh := utils.MapGetString(raw, "solution")
		dec := &CVEDescription{
			CVE:                src.CVE,
			Title:              "",
			ChineseTitle:       titleZh,
			Description:        src.DescriptionMain,
			ChineseDescription: descZh,
			OpenAISolution:     solutionZh,
		}
		log.Debugf("save [%v] chinese desc finished: cost: %v", src.CVE, time.Now().Sub(start).String())
		transLock.Lock()
		for {
			if db := db.Model(&CVEDescription{}).Where(
				"cve = ?", src.CVE,
			).Assign(dec).FirstOrCreate(&CVEDescription{}); db.Error != nil {
				log.Errorf("save cve database failed: %s", db.Error)
				time.Sleep(time.Second)
				continue
			}
			break
		}
		transLock.Unlock()
	}
	return nil
}

func YieldCVEDescriptions(db *gorm.DB, ctx context.Context) chan *CVEDescription {
	outC := make(chan *CVEDescription)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*CVEDescription
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}
