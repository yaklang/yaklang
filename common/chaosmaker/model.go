package chaosmaker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"yaklang/common/consts"
	"yaklang/common/jsonextractor"
	"yaklang/common/log"
	"yaklang/common/openai"
	"yaklang/common/suricata"
	"yaklang/common/utils"
	"yaklang/common/utils/bizhelper"
	"yaklang/common/yak/yaklib/codec"
	"yaklang/common/yakgrpc/yakit"
	"yaklang/common/yakgrpc/ypb"
)

type ChaosMakerRule struct {
	gorm.Model

	RawTrafficBeyondIPPacketBase64  string
	RawTrafficBeyondLinkLayerBase64 string
	RawTrafficBeyondHTTPBase64      string

	// suricata / http-request
	RuleType string

	SuricataRaw string `json:"raw"`
	Protocol    string
	Action      string
	Name        string
	NameZh      string
	ClassType   string
	ClassTypeZh string
	Group       string
	Hash        string `json:"hash" gorm:"unique_index"`

	Keywords      string
	KeywordsZh    string
	Description   string
	DescriptionZh string
	CVE           string
}

func QueryChaosMakerRule(db *gorm.DB, req *ypb.QueryChaosMakerRuleRequest) (*bizhelper.Paginator, []*ChaosMakerRule, error) {
	db = db.Model(&ChaosMakerRule{})

	params := req.GetPagination()

	db = bizhelper.ExactQueryString(db, "rule_type", req.GetRuleType())

	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
		"name", "name_zh", "class_type", "class_type_zh", "group", "keywords", "keywords_zh", "description", "description_zh",
	}, req.GetKeywords(), false)

	db = bizhelper.QueryOrder(db, params.OrderBy, params.Order)

	var ret []*ChaosMakerRule
	paging, db := bizhelper.Paging(db, int(params.Page), int(params.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func (c *ChaosMakerRule) ToGPRCModel() *ypb.ChaosMakerRule {
	return &ypb.ChaosMakerRule{
		Id:                              int64(c.ID),
		RawTrafficBeyondIpPacketBase64:  c.RawTrafficBeyondIPPacketBase64,
		RawTrafficBeyondLinkLayerBase64: c.RawTrafficBeyondLinkLayerBase64,
		RawTrafficBeyondHttpBase64:      c.RawTrafficBeyondHTTPBase64,
		RuleType:                        c.RuleType,
		SuricataRaw:                     c.SuricataRaw,
		Protocol:                        c.Protocol,
		Action:                          c.Action,
		Name:                            strings.Trim(c.Name, `"`),
		NameZh:                          strings.Trim(c.NameZh, `"`),
		ClassType:                       c.ClassType,
		ClassTypeZh:                     c.ClassTypeZh,
		Group:                           c.Group,
		Keywords:                        c.Keywords,
		KeywordsZh:                      c.KeywordsZh,
		Description:                     c.Description,
		DescriptionZh:                   c.DescriptionZh,
		CVE:                             utils.RemoveRepeatStringSlice(utils.PrettifyListFromStringSplitEx(c.CVE, ",", "|")),
	}
}

func (c *ChaosMakerRule) DecoratedByOpenAI(db *gorm.DB, opts ...openai.ConfigOption) {
	/*
		这是一个提炼攻击流量特征的任务，请提炼 %v 中的特征关键字（去除引用）？提取成 json array，以方便系统打标签和筛选，提供中文和英文的版本，放在 json 中，以 keywords 和 keywords_zh 作为字段，再描述一下这个特征（中文50字以内，去除‘检测’等字段意图），作为 description_zh 字段，同时补充他的 description（英文）
	*/
	var clearData string
	switch c.RuleType {
	case "suricata":
		clearData = strconv.Quote(c.SuricataRaw)
	case "http-request":
		raw, _ := codec.DecodeBase64(c.RawTrafficBeyondHTTPBase64)
		if raw != nil {
			clearData = strconv.Quote(string(raw))
		}
	case "tcp":
		raw, _ := codec.DecodeBase64(c.RawTrafficBeyondIPPacketBase64)
		if raw != nil {
			clearData = strconv.Quote(string(raw))
		}
	default:
		log.Errorf("unknown rule type: %v", c.RuleType)
		return
	}

	if clearData == "" {
		log.Errorf("empty clearData")
		return
	}
	client := openai.NewOpenAIClient(opts...)

	if c.CVE == "" && c.SuricataRaw != "" {
		records := regexp.MustCompile(`(?i)CVE-\d+-\d+`).FindAllString(c.SuricataRaw, -1)
		records = utils.RemoveRepeatStringSlice(strings.Split(strings.ToUpper(strings.Join(records, ",")), ","))
		if len(records) > 0 {
			c.CVE = strings.Join(records, ",")
		}
	}

	if c.NameZh == "" && c.Name != "" {
		if strings.HasPrefix(c.Name, `"ET `) {
			c.Name = `"` + strings.TrimPrefix(c.Name, `"ET `)
		}
		raw, _ := client.TranslateToChinese(c.Name)
		c.NameZh = raw
	}

	if c.Keywords == "" || c.KeywordsZh == "" {
		prompt := fmt.Sprintf(`这是一个提炼攻击流量特征的任务，
请提炼 %v 中的特征关键字（去除引用），并提取成 json array，以方便系统打标签和筛选
提供中文和英文的版本，放在 json 中，以 keywords 和 keywords_zh 作为字段
再描述一下这个特征（中文50字左右）
作为 description_zh 字段，同时补充他的 description（英文）`, clearData)
		log.Infof("start to question: %v", prompt)
		result, err := client.Chat(prompt)
		if err != nil {
			return
		}
		for _, data := range jsonextractor.ExtractStandardJSON(result) {
			var raw map[string]interface{}
			err := json.Unmarshal([]byte(data), &raw)
			if err != nil {
				continue
			}
			log.Infof("find raw answer: %v", string(data))
			c.Keywords = strings.Join(utils.InterfaceToStringSlice(utils.MapGetRawOr(raw, "keywords", []string{})), "|")
			c.KeywordsZh = strings.Join(utils.InterfaceToStringSlice(utils.MapGetRawOr(raw, "keywords_zh", []string{})), "|")
			c.Description = utils.MapGetString(raw, "description")
			c.DescriptionZh = utils.MapGetString(raw, "description_zh")
		}
	}

	err := CreateOrUpdateChaosMakerRule(db, c.Hash, c)
	if err != nil {
		log.Warn(err)
	}
}

func DecorateRules(concurrent int, proxy string) {
	var db = consts.GetGormProfileDatabase()
	swg := utils.NewSizedWaitGroup(concurrent)
	for r := range YieldChaosMakerRules(db, context.Background()) {
		swg.Add()
		r := r
		go func() {
			defer swg.Done()
			r.DecoratedByOpenAI(db, openai.WithAPIKeyFromYakitHome(), openai.WithProxy(proxy))
		}()
	}
	swg.Wait()
}

func (c *ChaosMakerRule) CalcHash() string {
	c.Hash = utils.CalcSha1(
		c.RawTrafficBeyondIPPacketBase64,
		c.RawTrafficBeyondLinkLayerBase64,
		c.RawTrafficBeyondHTTPBase64,
		c.SuricataRaw,
		c.Group,
	)
	return c.Hash
}

func (c *ChaosMakerRule) BeforeSave() error {
	if c.Hash == "" {
		c.CalcHash()
	}
	return nil
}

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
		if db := consts.GetGormProfileDatabase(); db != nil {
			db.AutoMigrate(&ChaosMakerRule{})
		}
		return nil
	})
}

func NewChaosMakerRuleFromSuricata(s *suricata.Rule) *ChaosMakerRule {
	return &ChaosMakerRule{
		SuricataRaw: s.Raw,
		Protocol:    s.Protocol,
		RuleType:    "suricata",
		Action:      s.Action,
		Name:        s.Message,
		ClassType:   s.ClassType,
	}
}

func SaveSuricata(db *gorm.DB, s *suricata.Rule) error {
	r := NewChaosMakerRuleFromSuricata(s)
	return CreateOrUpdateChaosMakerRule(db, r.CalcHash(), r)
}

func NewHTTPRequestChaosMakerRule(name string, raw []byte) *ChaosMakerRule {
	return &ChaosMakerRule{
		Model:                      gorm.Model{},
		RawTrafficBeyondHTTPBase64: codec.EncodeBase64(raw),
		RuleType:                   "http-request",
		Protocol:                   "http",
		Action:                     "alert",
		Name:                       name,
	}
}

func SaveHTTPRequest(db *gorm.DB, name string, raw []byte) error {
	r := NewHTTPRequestChaosMakerRule(name, raw)
	return CreateOrUpdateChaosMakerRule(db, r.CalcHash(), r)
}

func SaveTCPTraffic(db *gorm.DB, name string, raw []byte) error {
	r := &ChaosMakerRule{
		Model:                          gorm.Model{},
		RawTrafficBeyondIPPacketBase64: codec.EncodeBase64(raw),
		RuleType:                       "tcp",
		Protocol:                       "tcp",
		Action:                         "alert",
		Name:                           name,
	}
	return CreateOrUpdateChaosMakerRule(db, r.CalcHash(), r)
}

func SaveICMPTraffic(db *gorm.DB, name string, raw []byte) error {
	r := &ChaosMakerRule{
		Model:                          gorm.Model{},
		RawTrafficBeyondIPPacketBase64: codec.EncodeBase64(raw),
		RuleType:                       "icmp",
		Protocol:                       "icmp",
		Action:                         "alert",
		Name:                           name,
	}
	return CreateOrUpdateChaosMakerRule(db, r.CalcHash(), r)
}

var saveChaosMaker = sync.Mutex{}

func CreateOrUpdateChaosMakerRule(db *gorm.DB, hash string, i interface{}) error {
	saveChaosMaker.Lock()
	defer saveChaosMaker.Unlock()

	db = db.Model(&ChaosMakerRule{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&ChaosMakerRule{}); db.Error != nil {
		return utils.Errorf("create/update ChaosMakerRule failed: %s", db.Error)
	}

	return nil
}

func GetSuricataChaosMakerRule(db *gorm.DB, id int64) (*ChaosMakerRule, error) {
	var req ChaosMakerRule
	if db := db.Model(&ChaosMakerRule{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get ChaosMakerRule failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteSuricataChaosMakerRuleByID(db *gorm.DB, id int64) error {
	if db := db.Model(&ChaosMakerRule{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&ChaosMakerRule{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func YieldChaosMakerRules(db *gorm.DB, ctx context.Context) chan *ChaosMakerRule {
	outC := make(chan *ChaosMakerRule)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*ChaosMakerRule
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

func ExportChaosRulesToFile(db *gorm.DB, fileName string) error {
	fp, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer fp.Close()
	for result := range YieldChaosMakerRules(db.Model(&ChaosMakerRule{}), context.Background()) {
		raw, err := json.Marshal(result)
		if err != nil {
			log.Errorf("marshal rules failed: %s", err)
			continue
		}
		fp.Write(raw)
		fp.Write([]byte("\n"))
	}
	return nil
}

func ImportChaosRulesFromFile(db *gorm.DB, fileName string) error {
	fp, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer fp.Close()

	raw, _ := ioutil.ReadAll(fp)
	for result := range utils.ParseLines(string(raw)) {
		var rule ChaosMakerRule
		if err := json.Unmarshal([]byte(result), &rule); err != nil {
			log.Errorf("unmarshal rules failed: %s", err)
			continue
		}
		rule.ID = 0
		rule.DeletedAt = nil
		rule.CreatedAt = time.Now()
		rule.UpdatedAt = time.Now()
		if err := CreateOrUpdateChaosMakerRule(db, rule.Hash, rule); err != nil {
			log.Errorf("create/update rules failed: %s", err)
			continue
		}
	}
	return nil
}
