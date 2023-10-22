package rule

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/openai"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Storage struct {
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

func (Storage) TableName() string {
	return "chaos_maker_rules"
}

func QueryRule(db *gorm.DB, req *ypb.QueryChaosMakerRuleRequest) (*bizhelper.Paginator, []*Storage, error) {
	db = db.Model(&Storage{})

	params := req.GetPagination()

	db = bizhelper.ExactQueryString(db, "rule_type", req.GetRuleType())

	if req.GetFromId() > 0 {
		db = db.Where("id > ?", req.GetFromId())
	}

	if req.GetUntilId() > 0 {
		db = db.Where("id < ?", req.GetUntilId())
	}

	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
		"name", "name_zh", "class_type", "class_type_zh", "group", "keywords", "keywords_zh", "description", "description_zh",
	}, req.GetKeywords(), false)

	db = bizhelper.QueryOrder(db, params.OrderBy, params.Order)

	var ret []*Storage
	paging, db := bizhelper.Paging(db, int(params.Page), int(params.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func (c *Storage) ToGPRCModel() *ypb.ChaosMakerRule {
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

func (c *Storage) DecoratedByOpenAI(db *gorm.DB, opts ...openai.ConfigOption) {
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

	err := UpsertRule(db, c.Hash, c)
	if err != nil {
		log.Warn(err)
	}
}

func DecorateRules(concurrent int, proxy string) {
	var db = consts.GetGormProfileDatabase()
	swg := utils.NewSizedWaitGroup(concurrent)
	for r := range YieldRules(db, context.Background()) {
		swg.Add()
		r := r
		go func() {
			defer swg.Done()
			r.DecoratedByOpenAI(db, openai.WithAPIKeyFromYakitHome(), openai.WithProxy(proxy))
		}()
	}
	swg.Wait()
}

func (c *Storage) CalcHash() string {
	c.Hash = utils.CalcSha1(
		c.RawTrafficBeyondIPPacketBase64,
		c.RawTrafficBeyondLinkLayerBase64,
		c.RawTrafficBeyondHTTPBase64,
		c.SuricataRaw,
		c.Group,
	)
	return c.Hash
}

func (c *Storage) BeforeSave() error {
	if c.Hash == "" {
		c.CalcHash()
	}
	return nil
}

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
		if db := consts.GetGormProfileDatabase(); db != nil {
			db.AutoMigrate(&Storage{})
		}
		return nil
	})
}

func NewRuleFromSuricata(s *rule.Rule) *Storage {
	return &Storage{
		SuricataRaw: s.Raw,
		Protocol:    s.Protocol,
		RuleType:    "suricata",
		Action:      s.Action,
		Name:        s.Message,
		ClassType:   s.ClassType,
	}
}

func SaveSuricata(db *gorm.DB, s *rule.Rule) error {
	r := NewRuleFromSuricata(s)
	return UpsertRule(db, r.CalcHash(), r)
}

func NewHTTPRequestRule(name string, raw []byte) *Storage {
	return &Storage{
		Model:                      gorm.Model{},
		RawTrafficBeyondHTTPBase64: codec.EncodeBase64(raw),
		RuleType:                   "http-request",
		Protocol:                   "http",
		Action:                     "alert",
		Name:                       name,
	}
}

func SaveHTTPRequest(db *gorm.DB, name string, raw []byte) error {
	r := NewHTTPRequestRule(name, raw)
	return UpsertRule(db, r.CalcHash(), r)
}

func SaveTCPTraffic(db *gorm.DB, name string, raw []byte) error {
	r := &Storage{
		Model:                          gorm.Model{},
		RawTrafficBeyondIPPacketBase64: codec.EncodeBase64(raw),
		RuleType:                       "tcp",
		Protocol:                       "tcp",
		Action:                         "alert",
		Name:                           name,
	}
	return UpsertRule(db, r.CalcHash(), r)
}

func SaveICMPTraffic(db *gorm.DB, name string, raw []byte) error {
	r := &Storage{
		Model:                          gorm.Model{},
		RawTrafficBeyondIPPacketBase64: codec.EncodeBase64(raw),
		RuleType:                       "icmp",
		Protocol:                       "icmp",
		Action:                         "alert",
		Name:                           name,
	}
	return UpsertRule(db, r.CalcHash(), r)
}

var saveChaosMaker sync.Mutex

func UpsertRule(db *gorm.DB, hash string, i interface{}) error {
	saveChaosMaker.Lock()
	defer saveChaosMaker.Unlock()

	db = db.Model(&Storage{})

	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&Storage{}); db.Error != nil {
		return utils.Errorf("create/update Storage failed: %s", db.Error)
	}

	return nil
}

func GetSuricataChaosMakerRule(db *gorm.DB, id int64) (*Storage, error) {
	var req Storage
	if db := db.Model(&Storage{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Storage failed: %s", db.Error)
	}

	return &req, nil
}

func DeleteSuricataRuleByID(db *gorm.DB, id int64) error {
	if db := db.Model(&Storage{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&Storage{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func YieldRules(db *gorm.DB, ctx context.Context) chan *Storage {
	outC := make(chan *Storage)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*Storage
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

func ExportRulesToFile(db *gorm.DB, fileName string) error {
	fp, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer fp.Close()
	for result := range YieldRules(db.Model(&Storage{}), context.Background()) {
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

func ImportRulesFromFile(db *gorm.DB, fileName string) error {
	fp, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer fp.Close()

	raw, _ := io.ReadAll(fp)
	for result := range utils.ParseLines(string(raw)) {
		var rule Storage
		if err := json.Unmarshal([]byte(result), &rule); err != nil {
			log.Errorf("unmarshal rules failed: %s", err)
			continue
		}
		rule.ID = 0
		rule.DeletedAt = nil
		rule.CreatedAt = time.Now()
		rule.UpdatedAt = time.Now()
		if err := UpsertRule(db, rule.Hash, rule); err != nil {
			log.Errorf("create/update rules failed: %s", err)
			continue
		}
	}
	return nil
}
