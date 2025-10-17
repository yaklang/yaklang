package cve

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

type CVEDescription struct {
	CVE                string `json:"cve" gorm:"unique_index"`
	Title              string
	ChineseTitle       string
	Description        string
	ChineseDescription string
	OpenAISolution     string
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

	for result := range YieldCVEDescriptions(srcDB, context.Background()) {
		dstDB.Save(result)
	}
	return nil
}

var transLock = sync.Mutex{}

func MakeOpenAITranslateCWE(cwe *cveresources.CWE, apiKey string, proxies ...string) (*cveresources.CWE, error) {
	return nil, utils.Error("deprecated")
	//db := consts.GetGormCVEDescriptionDatabase()
	//if db == nil {
	//	return nil, utils.Errorf("no database found")
	//}
	//
	//var proxy string
	//if len(proxies) > 0 {
	//	proxy = proxies[0]
	//}

	//client := openai.NewOpenAIClient(openai.WithAPIKey(apiKey), openai.WithProxy(proxy))
	//if cwe.NameZh == "" {
	//	data, err := client.TranslateToChinese(cwe.Name)
	//	if err != nil {
	//		log.Errorf("translate cwe name failed: %s", err)
	//	}
	//	cwe.NameZh = data
	//}
	//if cwe.NameZh != "" {
	//	log.Infof("translating cwe: %v", cwe.NameZh)
	//}
	//
	//log.Infof("start to translate description: %s", cwe.CWEString())
	//if cwe.DescriptionZh == "" && cwe.Description != "" {
	//	desc, err := client.TranslateToChinese(cwe.Description)
	//	if err != nil {
	//		log.Errorf("translate desc failed: %s", err)
	//	}
	//	cwe.DescriptionZh = desc
	//}
	//
	//log.Infof("start to translate extended description: %v", cwe.CWEString())
	//if cwe.ExtendedDescriptionZh == "" && cwe.ExtendedDescription != "" {
	//	desc, err := client.TranslateToChinese(cwe.ExtendedDescription)
	//	if err != nil {
	//		log.Errorf("translate desc ex failed: %s", err)
	//	}
	//	cwe.ExtendedDescriptionZh = desc
	//}
	//
	//log.Infof("start to generate cwe solution: %v", cwe.CWEString())
	//if cwe.CWESolution == "" && cwe.NameZh != "" {
	//	solution, err := client.Chat(fmt.Sprintf("请给出 %v 的100字以内的安全建议或修复方案", strconv.Quote(cwe.NameZh)))
	//	if err != nil {
	//		log.Errorf("generate solution failed: %s", err)
	//	}
	//	cwe.CWESolution = solution
	//}
	//
	//return cwe, nil
}

func MakeOpenAIWorking(src *cveresources.CVE, gateway aispec.AIClient) error {
	db := consts.GetGormCVEDescriptionDatabase()
	if db == nil {
		return utils.Error("no database (cve desc) found")
	}

	var d CVEDescription
	if db := db.Where("cve = ?", src.CVE).First(&d); db.Error != nil {
	}
	if d.CVE != "" && d.ChineseTitle != "" {
		log.Debugf("cve: %s 's ch-Description is existed", d.CVE)
		return utils.Errorf("%v's translating existed", d.CVE)
	}

	log.Debugf("cve: %s's being translated...", src.CVE)
	start := time.Now()

	data, err := json.Marshal(src.DescriptionMain)
	if err != nil {
		return utils.Errorf("marshal cve failed: %s", err)
	}
	raw, err := gateway.ExtractData(strconv.Quote(string(data)), "请你提炼其中有用的信息，中文标题，解决方案和描述信息", map[string]any{
		"title":          "从数据源中提取成一个精炼的英文标题",
		"title_zh":       "从数据源提炼一个精炼的中文标题",
		"solution":       "从数据源中提取出一个修复方案（中文）",
		"description_zh": "提炼或者翻译出一个适合中文的漏洞描述信息",
	})
	if err != nil {
		println(string(data))
		log.Infof("ai.Chat met error: %s", err)
		return err
	}
	zh := utils.MapGetString(raw, "title_zh")
	title := utils.MapGetString(raw, "title")
	log.Infof("handle: %v -> en:%v zh:%v", src.CVE, title, zh)
	if title == "" && zh == "" {
		log.Warnf("abnormal data data: %#v", string(data))
	}
	dec := &CVEDescription{
		CVE:                src.CVE,
		Title:              title,
		ChineseTitle:       zh,
		Description:        src.DescriptionMain,
		ChineseDescription: utils.MapGetString(raw, "description_zh"),
		OpenAISolution:     utils.MapGetString(raw, "solution"),
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
	return nil
}

func YieldCVEDescriptions(db *gorm.DB, ctx context.Context) chan *CVEDescription {
	return bizhelper.YieldModel[*CVEDescription](ctx, db)
}
